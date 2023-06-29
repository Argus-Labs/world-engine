package shard

import (
	shardgrpc "buf.build/gen/go/argus-labs/world-engine/grpc/go/shard/v1/shardv1grpc"
	shard "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"context"
	"fmt"
	"github.com/argus-labs/world-engine/chain/x/shard/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"sync"
)

var (
	_ shardgrpc.ShardHandlerServer = &Server{}
)

type Server struct {
	moduleAddr sdk.AccAddress
	lock       sync.Mutex
	msgQueue   []types.SubmitBatchRequest
}

func NewShardServer(accAddr sdk.AccAddress) *Server {
	return &Server{
		moduleAddr: accAddr,
		lock:       sync.Mutex{},
		msgQueue:   nil,
	}
}

func (s *Server) FlushMessages() []types.SubmitBatchRequest {
	// no-op if we have nothing
	if len(s.msgQueue) == 0 {
		return nil
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	// copy the transactions
	msgs := make([]types.SubmitBatchRequest, len(s.msgQueue))
	copy(msgs, s.msgQueue)

	// clear the queue
	s.msgQueue = s.msgQueue[:0]
	return msgs
}

func (s *Server) SubmitShardBatch(ctx context.Context, request *shard.SubmitShardBatchRequest) (*shard.SubmitShardBatchResponse, error) {
	var sbr types.SubmitBatchRequest
	sbr.Batch = request.Batch
	sbr.Sender = s.moduleAddr.String()
	if err := sbr.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("cannot submit batch to blockchain. invalid submission: %w", err)
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	s.msgQueue = append(s.msgQueue, sbr)
	return nil, nil
}
