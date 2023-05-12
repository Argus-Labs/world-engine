package router

import (
	"context"
	"fmt"
)

type Result struct {
	Code    uint64
	Message string
}

//go:generate mockgen -source=router.go -package mocks -destination mocks/router.go
type Router interface {
	Send(ctx context.Context, namespace string, sender string, msg []byte) (Result, error)
	RegisterNamespace(namespace string, serverAddr string) error
}

var _ Router = &router{}

type router struct {
	namespaces map[string]string
}

func (r *router) Send(ctx context.Context, namespace string, sender string, msg []byte) (Result, error) {
	addr, ok := r.namespaces[namespace]
	if !ok {
		return Result{}, fmt.Errorf("namespace %s not found", namespace)
	}
	// connect to client given addr
	_ = addr
	// put bytes into proto message and send to server
	return Result{}, nil
}

func (r *router) RegisterNamespace(namespace string, serverAddr string) error {
	r.namespaces[namespace] = serverAddr
	return nil
}
