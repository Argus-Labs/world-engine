package router

import (
	"context"

	"buf.build/gen/go/argus-labs/world-engine/grpc/go/router/v1/routerv1grpc"

	routertypes "github.com/argus-labs/world-engine/chain/x/router/types"
)

type Result struct {
	Code    uint64
	Message []byte
}

type NamespaceClients map[string]routerv1grpc.MsgClient

//go:generate mockgen -source=router.go -package mocks -destination mocks/router.go
type Router interface {
	Send(ctx context.Context, namespace, sender string, msg []byte) (Result, error)
}

var _ Router = &router{}

type router struct {
	routerModule routertypes.QueryServiceClient
}

func NewRouter(rm routertypes.QueryServiceClient) Router {
	return &router{rm}
}

func (r *router) Send(ctx context.Context, namespace, sender string, msg []byte) (Result, error) {
	// TODO: impl
	return Result{}, nil
}
