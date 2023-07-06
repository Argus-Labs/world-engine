package base_shard

import (
	"context"
	"fmt"
	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"
	"google.golang.org/grpc"
	"net"

	"buf.build/gen/go/argus-labs/world-engine/grpc/go/router/v1/routerv1grpc"
	"buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/router/v1"
)

var _ routerv1grpc.MsgServer = &srv{}

// ITransactionTypes is a map that maps message names to transaction types.
// Its important to note that these names will originate from EVM contracts.
type ITransactionTypes map[transaction.TypeID]transaction.ITransaction

type TxQueuer interface {
	AddTransaction(transaction.TypeID, any)
}

// srv needs two main things to operate.
// 1. it needs a
type srv struct {
	it  ITransactionTypes
	txq TxQueuer
}

func NewServer(it ITransactionTypes, txq TxQueuer) routerv1grpc.MsgServer {
	return &srv{it, txq}
}

func (s *srv) Serve(addr string) error {
	server := grpc.NewServer()
	routerv1grpc.RegisterMsgServer(server, s)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() {
		err = server.Serve(listener)
		if err != nil {
			panic(err)
		}
	}()
	return nil
}

func (s *srv) SendMsg(ctx context.Context, msg *routerv1.MsgSend) (*routerv1.MsgSendResponse, error) {
	itx, ok := s.it[transaction.TypeID(msg.MessageId)]
	if !ok {
		return nil, fmt.Errorf("no transaction with ID %d is registerd in this world", msg.MessageId)
	}
	tx, err := itx.DecodeEVMBytes(msg.Message)
	if err != nil {
		return nil, err
	}
	// add transaction to the world queue
	s.txq.AddTransaction(itx.ID(), tx)
	return &routerv1.MsgSendResponse{}, nil
}
