package shard

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"os"
	"pkg.world.dev/world-engine/sign"

	shardgrpc "buf.build/gen/go/argus-labs/world-engine/grpc/go/shard/v1/shardv1grpc"
	shardv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"google.golang.org/grpc"

	shardtypes "pkg.world.dev/world-engine/chain/x/shard/types"
)

// Adapter is a type that helps facilitate communication with the EVM base shard.
type Adapter interface {
	WriteAdapter
	ReadAdapter
}

// WriteAdapter provides the functionality to send transactions to the EVM base shard.
type WriteAdapter interface {
	// Submit submits a transaction to the EVM base shard's game tx sequencer, where the tx data will be sequenced and
	// stored on chain.
	Submit(ctx context.Context, p *sign.SignedPayload, txID, epoch uint64) error
}

// ReadAdapter provides the functionality to read transactions from the EVM base shard.
type ReadAdapter interface {
	// QueryTransactions queries transactions stored on-chain. This is primarily used to rebuild state during world
	// recovery.
	QueryTransactions(
		context.Context,
		*shardtypes.QueryTransactionsRequest) (*shardtypes.QueryTransactionsResponse, error)
}

type AdapterConfig struct {
	// ShardSequencerAddr is the address to submit transactions to the EVM base shard's game shard sequencer server.
	ShardSequencerAddr string `json:"shard_receiver_addr,omitempty"`

	// EVMBaseShardAddr is the address to query the EVM base shard's shard storage cosmos module.
	EVMBaseShardAddr string `json:"evm_base_shard_addr"`
}

var (
	_ Adapter = &adapterImpl{}
)

type adapterImpl struct {
	cfg            AdapterConfig
	ShardSequencer shardgrpc.ShardHandlerClient
	ShardQuerier   shardtypes.QueryClient

	// opts
	creds credentials.TransportCredentials
}

func loadClientCredentials(path string) (credentials.TransportCredentials, error) {
	// Load certificate of the CA who signed server's certificate
	pemServerCA, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(pemServerCA) {
		return nil, fmt.Errorf("failed to add server CA's certificate")
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
		return nil, err
	}
	a.ShardSequencer = shardgrpc.NewShardHandlerClient(conn)

	// we don't need secure comms for this connection, cause we're just querying cosmos public RPC endpoints.
	conn2, err := grpc.Dial(cfg.EVMBaseShardAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	a.ShardQuerier = shardtypes.NewQueryClient(conn2)
	return a, nil
}

func (a adapterImpl) Submit(ctx context.Context, sp *sign.SignedPayload, txID uint64, epoch uint64) error {
	req := &shardv1.SubmitShardTxRequest{Tx: signedPayloadToProto(sp), Epoch: epoch, TxId: txID}
	_, err := a.ShardSequencer.SubmitShardTx(ctx, req)
	return err
}

func (a adapterImpl) QueryTransactions(
	ctx context.Context,
	req *shardtypes.QueryTransactionsRequest,
) (
	*shardtypes.QueryTransactionsResponse,
	error,
) {
	return a.ShardQuerier.Transactions(ctx, req)
}

func signedPayloadToProto(sp *sign.SignedPayload) *shardv1.SignedPayload {
	return &shardv1.SignedPayload{
		PersonaTag: sp.PersonaTag,
		Namespace:  sp.Namespace,
		Nonce:      sp.Nonce,
		Signature:  sp.Signature,
		Body:       sp.Body,
	}
}
