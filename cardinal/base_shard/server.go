package base_shard

import (
	"context"

	"buf.build/gen/go/argus-labs/world-engine/grpc/go/router/v1/routerv1grpc"
	"buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/router/v1"
	"github.com/ethereum/go-ethereum/accounts/abi"
)

var _ routerv1grpc.MsgServer = &srv{}

type AbiTypeMap map[string]EVMTransactionType

type EVMTransactionType struct {
	evmType    abi.Type
	underlying any
}

// srv needs two main things to operate.
// 1. it needs a
type srv struct {
	at AbiTypeMap
}

func (s srv) SendMsg(ctx context.Context, msg *routerv1.MsgSend) (*routerv1.MsgSendResponse, error) {
	etx := s.at[msg.MessageId]
	args := abi.Arguments{{Type: etx.evmType}}
	unpacked, err := args.Unpack(msg.Message)
	if err != nil {
		return nil, err
	}
}
