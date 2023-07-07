package router

import (
	"context"

	"buf.build/gen/go/argus-labs/world-engine/grpc/go/router/v1/routerv1grpc"
	v1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/router/v1"
	"google.golang.org/grpc"
)

type Result struct {
	Code    uint64
	Message []byte
}

//go:generate mockgen -source=router.go -package mocks -destination mocks/router.go
type Router interface {
	// Send sends the msg payload to the game shard indicated by the namespace, if such namespace exists on chain.
	Send(ctx context.Context, namespace, sender string, msgID uint64, msg []byte) (*Result, error)
}

var _ Router = &router{}

type router struct {
	rtr routerv1grpc.MsgClient
}

// NewRouter returns a new router instance with a connection to a single cardinal shard instance.
// TODO(technicallyty): its a bit unclear how im going to query the state machine here, so router is just going to
// take the cardinal address directly for now...
func NewRouter(cardinalAddr string) (Router, error) {
	conn, err := grpc.Dial(cardinalAddr)
	if err != nil {
		return nil, err
	}

	return &router{rtr: routerv1grpc.NewMsgClient(conn)}, nil
}

func (r *router) Send(ctx context.Context, _, sender string, msgID uint64, msg []byte) (*Result, error) {
	req := &v1.MsgSend{
		Sender:    sender,
		MessageId: msgID,
		Message:   msg,
	}
	res, err := r.rtr.SendMsg(ctx, req)
	if err != nil {
		return nil, err
	}
	return &Result{
		Code:    res.Code,
		Message: res.Message,
	}, nil
}
