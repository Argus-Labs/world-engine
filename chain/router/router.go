package router

import (
	"context"

	"buf.build/gen/go/argus-labs/argus/grpc/go/v1/routerv1grpc"
	routerv1 "buf.build/gen/go/argus-labs/argus/protocolbuffers/go/v1"
	"google.golang.org/grpc"

	"github.com/argus-labs/world-engine/chain/router/errors"
)

type Result struct {
	Code    uint64
	Message []byte
}

//go:generate mockgen -source=router.go -package mocks -destination mocks/router.go
type Router interface {
	Send(ctx context.Context, namespace, sender string, msg []byte) (Result, error)
	RegisterNamespace(namespace, serverAddr string) error
}

var _ Router = &router{}

type router struct {
	namespaces map[string]routerv1grpc.MsgClient
}

func (r *router) Send(ctx context.Context, namespace, sender string, msg []byte) (Result, error) {
	srv, ok := r.namespaces[namespace]
	if !ok {
		return Result{}, errors.ErrNamespaceNotFound(namespace)
	}
	msgSend := &routerv1.MsgSend{
		Sender:  sender,
		Message: msg,
	}
	res, err := srv.SendMsg(ctx, msgSend)
	if err != nil {
		return Result{
			Code:    errors.Failed,
			Message: []byte(err.Error()), // TODO(technicallyty): need more thinking on this..
		}, err
	}
	// put bytes into proto message and send to server
	return Result{
		Code:    res.Code,
		Message: res.Message,
	}, nil
}

func (r *router) RegisterNamespace(namespace, serverAddr string) error {
	cc, err := grpc.Dial(serverAddr)
	if err != nil {
		return err
	}
	client := routerv1grpc.NewMsgClient(cc)
	r.namespaces[namespace] = client
	return nil
}
