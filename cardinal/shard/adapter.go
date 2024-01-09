package shard

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"os"
	"pkg.world.dev/world-engine/cardinal/txpool"

	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/sign"

	"google.golang.org/grpc"

	shardv2 "pkg.world.dev/world-engine/rift/shard/v2"

	shardtypes "pkg.world.dev/world-engine/evm/x/shard/types"
)

// Adapter is a type that helps facilitate communication with the EVM base shard.
type Adapter interface {
	WriteAdapter
	QueryAdapter
}

// WriteAdapter provides the functionality to send transactions to the EVM base shard.
type WriteAdapter interface {
	// Submit submits a transaction to the EVM base shard's game tx sequencer, where the tx data will be sequenced and
	// stored on chain.
	Submit(ctx context.Context, txs txpool.TxMap, namespace string, epoch, unixTimestamp uint64) error
}

// QueryAdapter provides the functionality to query transactions from the EVM base shard.
type QueryAdapter interface {
	// QueryTransactions queries transactions stored on-chain. This is primarily used to rebuild state during world
	// recovery.
	QueryTransactions(
		context.Context,
		*shardtypes.QueryTransactionsRequest) (*shardtypes.QueryTransactionsResponse, error)
}

type AdapterConfig struct {
	// ShardSequencerAddr is the address to submit transactions to the EVM base shard's game shard sequencer server.
	ShardSequencerAddr string

	// EVMBaseShardAddr is the address to query the EVM base shard's shard storage cosmos module.
	EVMBaseShardAddr string
}

var (
	_ Adapter = &adapterImpl{}
)

type adapterImpl struct {
	cfg            AdapterConfig
	ShardSequencer shardv2.TransactionHandlerClient
	ShardQuerier   shardtypes.QueryClient

	// opts
	creds credentials.TransportCredentials
}

func loadClientCredentials(path string) (credentials.TransportCredentials, error) {
	// Load certificate of the CA who signed server's certificate
	pemServerCA, err := os.ReadFile(path)
	if err != nil {
		return nil, eris.Wrap(err, "")
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(pemServerCA) {
		return nil, eris.Errorf("failed to add server CA's certificate")
	}

	// Create the credentials and return it
	config := &tls.Config{
		RootCAs: certPool,
	}

	return credentials.NewTLS(config), nil
}

func NewAdapter(cfg AdapterConfig, opts ...Option) (Adapter, error) {
	a := &adapterImpl{cfg: cfg, creds: insecure.NewCredentials()}
	for _, opt := range opts {
		opt(a)
	}

	// we need secure comms here because only this connection should be able to send stuff to the shard receiver.
	conn, err := grpc.Dial(cfg.ShardSequencerAddr, grpc.WithTransportCredentials(a.creds))
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	a.ShardSequencer = shardv2.NewTransactionHandlerClient(conn)

	// we don't need secure comms for this connection, cause we're just querying cosmos public RPC endpoints.
	conn2, err := grpc.Dial(cfg.EVMBaseShardAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	a.ShardQuerier = shardtypes.NewQueryClient(conn2)
	return a, nil
}

func (a adapterImpl) Submit(ctx context.Context, processedTxs txpool.TxMap, namespace string, epoch, unixTimestamp uint64) error {
	messageIDtoTxs := make(map[uint64]*shardv2.Transactions, len(processedTxs))
	for msgID, txs := range processedTxs {
		protoTxs := make([]*shardv2.Transaction, len(txs))
		for _, txData := range txs {
			protoTxs = append(protoTxs, transactionToProto(txData.Tx))
		}
		messageIDtoTxs[uint64(msgID)] = &shardv2.Transactions{Txs: protoTxs}
	}
	req := shardv2.SubmitTransactionsRequest{
		Epoch:         epoch,
		UnixTimestamp: unixTimestamp,
		Namespace:     namespace,
		Transactions:  messageIDtoTxs,
	}
	_, err := a.ShardSequencer.Submit(ctx, &req)
	return err
}

func (a adapterImpl) QueryTransactions(
	ctx context.Context,
	req *shardtypes.QueryTransactionsRequest,
) (
	*shardtypes.QueryTransactionsResponse,
	error,
) {
	res, err := a.ShardQuerier.Transactions(ctx, req)
	return res, eris.Wrap(err, "")
}

//nolint:unused // will be used soon.. just refactoring things..
func transactionToProto(sp *sign.Transaction) *shardv2.Transaction {
	return &shardv2.Transaction{
		PersonaTag: sp.PersonaTag,
		Namespace:  sp.Namespace,
		Nonce:      sp.Nonce,
		Signature:  sp.Signature,
		Body:       sp.Body,
	}
}
