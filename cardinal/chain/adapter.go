package chain

import (
	"context"

	shardgrpc "buf.build/gen/go/argus-labs/world-engine/grpc/go/shard/v1/shardv1grpc"
	shardv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"google.golang.org/grpc"
)

// Adapter is a type that helps facilitate communication with a blockchain.
type Adapter interface {
	Submit(ctx context.Context, namespace string, tick uint64, txs []byte) error
}

type Config struct {
	ShardReceiverAddr string `json:"shard_receiver_addr,omitempty"`
}

var _ Adapter = &adapterImpl{}

type adapterImpl struct {
	cfg           Config
	ShardReceiver shardgrpc.ShardHandlerClient
}

func NewAdapter(cfg Config) (Adapter, error) {
	a := &adapterImpl{cfg: cfg}
	conn, err := grpc.Dial(cfg.ShardReceiverAddr)
	if err != nil {
		return nil, err
	}
	client := shardgrpc.NewShardHandlerClient(conn)
	a.ShardReceiver = client
	return a, nil
}

// Submit submits the transaction bytes to the connected blockchain.
func (a adapterImpl) Submit(ctx context.Context, namespace string, tick uint64, txs []byte) error {
	req := &shardv1.SubmitShardBatchRequest{Namespace: namespace, TickId: tick, Batch: txs}
	_, err := a.ShardReceiver.SubmitShardBatch(ctx, req)
	return err
}
