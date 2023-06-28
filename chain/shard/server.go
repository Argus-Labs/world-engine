package shard

import (
	shardgrpc "buf.build/gen/go/argus-labs/world-engine/grpc/go/shard/v1/shardv1grpc"
	shard "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"context"
)

var (
	_ shardgrpc.ShardHandlerServer = &shardServer{}
)

type shardServer struct {
	batches [][]byte
}

func (s shardServer) SubmitShardBatch(ctx context.Context, request *shard.SubmitShardBatchRequest) (*shard.SubmitShardBatchResponse, error) {
	return nil, nil
}
