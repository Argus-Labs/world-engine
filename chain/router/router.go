package router

import "context"

type Result struct {
	Code    uint64
	Message string
}

//go:generate mockgen -source=router.go -package mocks -destination mocks/router.go
type Router interface {
	Send(ctx context.Context, namespace string, sender string, msg []byte) (Result, error)
}
