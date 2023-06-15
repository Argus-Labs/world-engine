package evm

import (
	"context"
	"fmt"
	"net"

	"buf.build/gen/go/argus-labs/world-engine/grpc/go/router/v1/routerv1grpc"
	routerv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/router/v1"
	"google.golang.org/grpc"

	"github.com/argus-labs/world-engine/cardinal/ecs"
)

type Config struct {
	networkType string
	grpcAddress string
}

var _ routerv1grpc.MsgServer = &evmReceiverServer{}

type evmReceiverServer struct {
	w          *ecs.World
	txHandlers map[string]TxHandler
}

func StartEVMReceiver(w *ecs.World, handlers map[string]TxHandler, cfg Config) error {
	ers := &evmReceiverServer{
		w:          w,
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

func newReceiver(w *ecs.World, handlers map[string]TxHandler) *evmReceiverServer {
	return &evmReceiverServer{
		w:          w,
		txHandlers: handlers,
	}
}

func (e *evmReceiverServer) SendMsg(ctx context.Context, send *routerv1.MsgSend) (*routerv1.MsgSendResponse, error) {
	tp, ok := e.txHandlers[send.MessageName]
	if !ok {
		return nil, fmt.Errorf("transaction with name %s not found", send.MessageName)
	}

	err := tp.UnmarshalAndSubmit(send.Message, e.w.AddTransaction)
	if err != nil {
		return nil, err
	}
	return &routerv1.MsgSendResponse{}, nil
}
