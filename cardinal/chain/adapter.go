package chain

import (
	shardgrpc "buf.build/gen/go/argus-labs/world-engine/grpc/go/shard/v1/shardv1grpc"
	shardv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"context"
	"google.golang.org/grpc"
)

//go:generate mockgen -source=adapter.go -package mocks -destination mocks/adapter.go
type Adapter interface {
	ReadAll() []byte
	Submit(ctx context.Context, bz []byte) error
}

type Config struct {
	ShardReceiverAddr string `json:"shard_receiver_addr,omitempty"`
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

type adapterImpl struct {
	cfg           Config
	ShardReceiver shardgrpc.ShardHandlerClient
}

func (a adapterImpl) ReadAll() []byte {
	//TODO implement me
	panic("implement me")
}

func (a adapterImpl) Submit(ctx context.Context, bz []byte) error {
	req := &shardv1.SubmitShardBatchRequest{Batch: bz}
	_, err := a.ShardReceiver.SubmitShardBatch(ctx, req)
	return err
}
