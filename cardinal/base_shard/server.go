package base_shard

import (
	"context"
	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"

	"buf.build/gen/go/argus-labs/world-engine/grpc/go/router/v1/routerv1grpc"
	"buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/router/v1"
)

var _ routerv1grpc.MsgServer = &srv{}

type ITransactionTypes map[string]transaction.ITransaction

// srv needs two main things to operate.
// 1. it needs a
type srv struct {
	it ITransactionTypes
}

func (s srv) SendMsg(ctx context.Context, msg *routerv1.MsgSend) (*routerv1.MsgSendResponse, error) {
	itx := s.it[msg.MessageId]
	tx, err := itx.DecodeEVMBytes(msg.Message)
	if err != nil {
		return nil, err
	}

}
