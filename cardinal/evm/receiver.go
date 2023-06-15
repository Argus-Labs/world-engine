package evm

import (
	"context"
	"fmt"
	"net"

	"buf.build/gen/go/argus-labs/world-engine/grpc/go/router/v1/routerv1grpc"
	routerv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/router/v1"
	"google.golang.org/grpc"
)

type Config struct {
	networkType string
	grpcAddress string
}

var _ routerv1grpc.MsgServer = &EvmReceiverServer{}

type EvmReceiverServer struct {
	txq        TransactionQueuer
	txHandlers map[string]TxHandler
}

func StartEVMReceiver(txq TransactionQueuer, handlers map[string]TxHandler, cfg Config) error {
	ers := &EvmReceiverServer{
		txq:        txq,
		txHandlers: handlers,
	}
	lis, err := net.Listen(cfg.networkType, cfg.grpcAddress)
	if err != nil {
		return err
	}
	var opts []grpc.ServerOption
	srv := grpc.NewServer(opts...)
	routerv1grpc.RegisterMsgServer(srv, ers)
	go func() {
		err := srv.Serve(lis)
		if err != nil {
			panic(err)
		}
	}()
	return nil
}

func NewReceiver(txq TransactionQueuer, handlers map[string]TxHandler) *EvmReceiverServer {
	return &EvmReceiverServer{
		txq:        txq,
		txHandlers: handlers,
	}
}

func (e *EvmReceiverServer) SendMsg(ctx context.Context, send *routerv1.MsgSend) (*routerv1.MsgSendResponse, error) {
	tp, ok := e.txHandlers[send.MessageName]
	if !ok {
		return nil, fmt.Errorf("transaction with name %s not found", send.MessageName)
	}

	err := tp.UnmarshalAndSubmit(send.Message, e.txq.AddTransaction)
	if err != nil {
		return nil, err
	}
	return &routerv1.MsgSendResponse{}, nil
}
