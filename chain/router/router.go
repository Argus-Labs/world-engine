package router

import (
	"context"
)

type Result struct {
	Code    uint64
	Message []byte
}

//go:generate mockgen -source=router.go -package mocks -destination mocks/router.go
type Router interface {
	Send(ctx context.Context, namespace, sender string, msg []byte) (Result, error)
}

var _ Router = &router{}

type router struct{}

func NewRouter() Router {
	return &router{}
}

func (r *router) Send(_ context.Context, _, _ string, _ []byte) (Result, error) {
	// TODO: impl
	return Result{}, nil
}
