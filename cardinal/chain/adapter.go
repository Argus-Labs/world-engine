package chain

import (
	"context"

	shardgrpc "buf.build/gen/go/argus-labs/world-engine/grpc/go/shard/v1/shardv1grpc"
	shardv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"google.golang.org/grpc"

	shardtypes "github.com/argus-labs/world-engine/chain/x/shard/types"
)

// Adapter is a type that helps facilitate communication with a blockchain.
type Adapter interface {
	Submit(ctx context.Context, namespace string, tick uint64, txs []byte) error
	QueryBatch(ctx context.Context, req *shardtypes.QueryBatchesRequest) (*shardtypes.QueryBatchesResponse, error)
}

type Config struct {
	// ShardReceiverAddr is the address to communicate with the secure shard submission channel.
	ShardReceiverAddr string `json:"shard_receiver_addr,omitempty"`

	// BaseShardAddr is the rollup' base address to submit transactions and query directly with the rollup.
	BaseShardAddr string `json:"base_shard_addr"`
}

var _ Adapter = &adapterImpl{}

type adapterImpl struct {
	cfg           Config
	ShardReceiver shardgrpc.ShardHandlerClient
	ShardQuerier  shardtypes.QueryClient
}

func NewAdapter(cfg Config) (Adapter, error) {
	a := &adapterImpl{cfg: cfg}
	conn, err := grpc.Dial(cfg.ShardReceiverAddr)
	if err != nil {
		return nil, err
	}
	a.ShardReceiver = shardgrpc.NewShardHandlerClient(conn)

	conn2, err := grpc.Dial(cfg.BaseShardAddr)
	if err != nil {
		return nil, err
	}
	a.ShardQuerier = shardtypes.NewQueryClient(conn2)
	return a, nil
}

// Submit submits the transaction bytes to the connected blockchain.
func (a adapterImpl) Submit(ctx context.Context, namespace string, tick uint64, txs []byte) error {
	req := &shardv1.SubmitShardBatchRequest{Namespace: namespace, TickId: tick, Batch: txs}
	_, err := a.ShardReceiver.SubmitShardBatch(ctx, req)
	return err
}

func (a adapterImpl) QueryBatch(ctx context.Context, req *shardtypes.QueryBatchesRequest) (*shardtypes.QueryBatchesResponse, error) {
	return a.ShardQuerier.Batches(ctx, req)
}
