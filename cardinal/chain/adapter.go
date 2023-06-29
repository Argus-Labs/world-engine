package chain

import (
	"context"

	shardgrpc "buf.build/gen/go/argus-labs/world-engine/grpc/go/shard/v1/shardv1grpc"
	shardv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"google.golang.org/grpc"
)

//go:generate mockgen -source=adapter.go -package mocks -destination mocks/adapter.go

// Adapter is a type that helps facilitate communication with a blockchain.
type Adapter interface {
	Submit(ctx context.Context, bz []byte) error
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

func (a adapterImpl) Submit(ctx context.Context, bz []byte) error {
	req := &shardv1.SubmitShardBatchRequest{Batch: bz}
	_, err := a.ShardReceiver.SubmitShardBatch(ctx, req)
	return err
}
