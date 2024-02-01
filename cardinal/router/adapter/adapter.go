package adapter

/*
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

	shard "pkg.world.dev/world-engine/rift/shard/v2"

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

type Config struct {
	// ShardSequencerAddr is the address to submit transactions to the EVM base shard's game shard sequencer server.
	ShardSequencerAddr string

	// EVMBaseShardAddr is the address to query the EVM base shard's shard storage cosmos module.
	EVMBaseShardAddr string
}

var (
	_ Adapter = &adapterImpl{}
)

type adapterImpl struct {
	cfg            Config
	ShardSequencer shard.TransactionHandlerClient
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

func New(cfg Config, opts ...Option) (Adapter, error) {
	a := &adapterImpl{cfg: cfg, creds: insecure.NewCredentials()}
	for _, opt := range opts {
		opt(a)
	}

	// we need secure comms here because only this connection should be able to send stuff to the shard receiver.
	conn, err := grpc.Dial(cfg.ShardSequencerAddr, grpc.WithTransportCredentials(a.creds))
	if err != nil {
		return nil, eris.Wrapf(err, "error dialing shard seqeuncer address at %q", cfg.ShardSequencerAddr)
	}
	a.ShardSequencer = shard.NewTransactionHandlerClient(conn)

	// we don't need secure comms for this connection, cause we're just querying cosmos public RPC endpoints.
	conn2, err := grpc.Dial(cfg.EVMBaseShardAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, eris.Wrapf(err, "error dialing evm base shard address at %q", cfg.EVMBaseShardAddr)
	}
	a.ShardQuerier = shardtypes.NewQueryClient(conn2)
	return a, nil
}

func (a adapterImpl) Submit(
	ctx context.Context,
	processedTxs txpool.TxMap,
	namespace string,
	epoch,
	unixTimestamp uint64,
) error {
	messageIDtoTxs := make(map[uint64]*shard.Transactions)
	for msgID, txs := range processedTxs {
		protoTxs := make([]*shard.Transaction, 0, len(txs))
		for _, txData := range txs {
			protoTxs = append(protoTxs, transactionToProto(txData.Tx))
		}
		messageIDtoTxs[uint64(msgID)] = &shard.Transactions{Txs: protoTxs}
	}
	req := shard.SubmitTransactionsRequest{
		Epoch:         epoch,
		UnixTimestamp: unixTimestamp,
		Namespace:     namespace,
		Transactions:  messageIDtoTxs,
	}
	_, err := a.ShardSequencer.Submit(ctx, &req)
	return eris.Wrap(err, "")
}

func (a adapterImpl) QueryTransactions(ctx context.Context, req *shardtypes.QueryTransactionsRequest) (
	*shardtypes.QueryTransactionsResponse,
	error,
) {
	res, err := a.ShardQuerier.Transactions(ctx, req)
	return res, eris.Wrap(err, "")
}

func transactionToProto(sp *sign.Transaction) *shard.Transaction {
	return &shard.Transaction{
		PersonaTag: sp.PersonaTag,
		Namespace:  sp.Namespace,
		Nonce:      sp.Nonce,
		Signature:  sp.Signature,
		Body:       sp.Body,
	}
}
*/
