package router

import (
	"buf.build/gen/go/argus-labs/world-engine/grpc/go/router/v1/routerv1grpc"
	v1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/router/v1"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"os"
)

type Result struct {
	Code    uint64
	Message []byte
}

//go:generate mockgen -source=router.go -package mocks -destination mocks/router.go
type Router interface {
	// Send sends the msg payload to the game shard indicated by the namespace, if such namespace exists on chain.
	Send(ctx context.Context, namespace, sender string, msgID uint64, msg []byte) (*Result, error)
	Query(ctx context.Context, request []byte, resource, namespace string) ([]byte, error)
}

var (
	_ Router = &router{}
)

type router struct {
	cardinalAddr string
	clientOpts   []grpc.DialOption
}

func loadClientCredentials(path string) (credentials.TransportCredentials, error) {
	// Load certificate of the CA who signed server's certificate
	pemServerCA, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(pemServerCA) {
		return nil, fmt.Errorf("failed to add server CA's certificate")
	}

	// Create the credentials and return it
	config := &tls.Config{
		RootCAs: certPool,
	}

	return credentials.NewTLS(config), nil
}

// NewRouter returns a new router instance with a connection to a single cardinal shard instance.
// TODO(technicallyty): its a bit unclear how im going to query the state machine here, so router is just going to
// take the cardinal address directly for now...
func NewRouter(cardinalAddr string, opts ...Option) Router {
	r := &router{cardinalAddr: cardinalAddr}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *router) Send(ctx context.Context, namespace, sender string, msgID uint64, msg []byte) (*Result, error) {
	client, err := r.getConnectionForNamespace(namespace)
	if err != nil {
		return nil, err
	}
	req := &v1.MsgSend{
		Sender:    sender,
		MessageId: msgID,
		Message:   msg,
	}
	res, err := client.SendMsg(ctx, req)
	if err != nil {
		return nil, err
	}
	return &Result{
		Code:    res.Code,
		Message: res.Message,
	}, nil
}

func (r *router) Query(ctx context.Context, request []byte, resource, namespace string) ([]byte, error) {
	client, err := r.getConnectionForNamespace(namespace)
	if err != nil {
		return nil, err
	}
	res, err := client.QueryShard(ctx, &v1.QueryShardRequest{
		Resource: resource,
		Request:  request,
	})
	if err != nil {
		return nil, err
	}
	return res.Response, nil
}

func (r *router) getConnectionForNamespace(ns string) (routerv1grpc.MsgClient, error) {
	conn, err := grpc.Dial(
		r.cardinalAddr,
		r.clientOpts...,
	)
	if err != nil {
		return nil, fmt.Errorf("error connecting to %s address for namespace %s", r.cardinalAddr, ns)
	}
	return routerv1grpc.NewMsgClient(conn), nil
}
