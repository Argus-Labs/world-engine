package chain

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"embed"
	"fmt"
	"google.golang.org/grpc/credentials"

	shardgrpc "buf.build/gen/go/argus-labs/world-engine/grpc/go/shard/v1/shardv1grpc"
	shardv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"google.golang.org/grpc"

	shardtypes "github.com/argus-labs/world-engine/chain/x/shard/types"
)

// Adapter is a type that helps facilitate communication with the EVM base shard.
type Adapter interface {
	Submit(ctx context.Context, namespace string, tick uint64, txs []byte) error
	QueryBatches(ctx context.Context, req *shardtypes.QueryBatchesRequest) (*shardtypes.QueryBatchesResponse, error)
}

type Config struct {
	// ShardReceiverAddr is the address to communicate with the secure shard submission channel.
	ShardReceiverAddr string `json:"shard_receiver_addr,omitempty"`

	// EVMBaseShardAddr is the address to submit transactions and query directly with the EVM base shard.
	EVMBaseShardAddr string `json:"evm_base_shard_addr"`
}

var (
	//go:embed cert
	f embed.FS
	_ Adapter = &adapterImpl{}
)

type adapterImpl struct {
	cfg           Config
	ShardReceiver shardgrpc.ShardHandlerClient
	ShardQuerier  shardtypes.QueryClient
}

func loadClientCredentials() (credentials.TransportCredentials, error) {
	// Load certificate of the CA who signed server's certificate
	pemServerCA, err := f.ReadFile("cert/ca-cert.pem")
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

func NewAdapter(cfg Config) (Adapter, error) {
	creds, err := loadClientCredentials()
	if err != nil {
		return nil, err
	}
	a := &adapterImpl{cfg: cfg}
	conn, err := grpc.Dial(cfg.ShardReceiverAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, err
	}
	a.ShardReceiver = shardgrpc.NewShardHandlerClient(conn)

	conn2, err := grpc.Dial(cfg.EVMBaseShardAddr)
	if err != nil {
		return nil, err
	}
	a.ShardQuerier = shardtypes.NewQueryClient(conn2)
	return a, nil
}

// Submit submits the transaction bytes to the EVM base shard.
func (a adapterImpl) Submit(ctx context.Context, namespace string, tick uint64, txs []byte) error {
	req := &shardv1.SubmitShardBatchRequest{Namespace: namespace, TickId: tick, Batch: txs}
	_, err := a.ShardReceiver.SubmitShardBatch(ctx, req)
	return err
}

func (a adapterImpl) QueryBatches(ctx context.Context, req *shardtypes.QueryBatchesRequest) (*shardtypes.QueryBatchesResponse, error) {
	return a.ShardQuerier.Batches(ctx, req)
}
