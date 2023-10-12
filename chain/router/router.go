package router

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"os"
	routerv1 "pkg.world.dev/world-engine/rift/router/v1"

	"pkg.berachain.dev/polaris/eth/core"
	"pkg.berachain.dev/polaris/eth/core/types"

	"cosmossdk.io/log"
)

type Result struct {
	Code    uint64
	Message []byte
}

//go:generate mockgen -source=routerImpl.go -package mocks -destination mocks/routerImpl.go
type Router interface {
	// SendMessage sends the msg payload to the game shard indicated by the namespace, if such namespace exists on chain.
	SendMessage(ctx context.Context, namespace, sender string, msgID uint64, msg []byte) (*Result, error)
	// Query queries a game shard.
	Query(ctx context.Context, request []byte, resource, namespace string) ([]byte, error)
	// HandleDispatch implements the polaris EVM PostTxProcessing hook. It runs after an EVM tx execution has finished
	// processing.
	HandleDispatch(_ *types.Transaction, result *core.ExecutionResult)
}

var (
	_ Router = &routerImpl{}
)

type routerImpl struct {
	cardinalAddr string
	logger       log.Logger
	queue        *msgQueue
	// opts
	creds credentials.TransportCredentials
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

// NewRouter returns a new routerImpl instance with a connection to a single cardinal shard instance.
// TODO(technicallyty): its a bit unclear how im going to query the state machine here, so routerImpl is just going to
// take the cardinal address directly for now...
func NewRouter(cardinalAddr string, logger log.Logger, opts ...Option) Router {
	r := &routerImpl{cardinalAddr: cardinalAddr, logger: logger, creds: insecure.NewCredentials()}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *routerImpl) HandleDispatch(tx *types.Transaction, result *core.ExecutionResult) {
	// no-op if nothing was queued after an evm tx.
	if !r.queue.IsSet() {
		return
	}
	// if tx failed, just clear the queue, we're not going to send the message.
	if result.Failed() {
		r.queue.Clear()
	} else {
		r.dispatchMessage()
	}
}

func (r *routerImpl) dispatchMessage() {
	defer r.queue.Clear()
	// we do not need to pass in a namespace, since we just default to a given cardinal addr anyways.
	// this will eventually need to update to have a proper mapping of namespace -> game shard EVM grpc address.
	// https://linear.app/arguslabs/issue/WORLD-13/update-router-to-look-up-the-correct-namespace-mapping
	client, err := r.getConnectionForNamespace(r.queue.namespace)
	if err != nil {
		// TODO: once we implement https://linear.app/arguslabs/issue/WORLD-8/implement-evm-callbacks, we need to store
		// this error in the callback storage module.
		r.logger.Error("error getting game shard gRPC connection", "error", err)
		return
	}
	res, err := client.SendMessage(context.Background(), r.queue.msg)
	if err != nil {
		// TODO: once we implement https://linear.app/arguslabs/issue/WORLD-8/implement-evm-callbacks, we need to store
		// this error in the callback storage module.
		r.logger.Error("failed to send message to game shard", "error", err)
		return
	}
	// TODO: once we implement https://linear.app/arguslabs/issue/WORLD-8/implement-evm-callbacks, we need to store
	// the result in the callback storage module.
	_ = res
}

func (r *routerImpl) SendMessage(ctx context.Context, namespace, sender string, msgID uint64, msg []byte) (*Result, error) {
	req := &routerv1.SendMessageRequest{
		Sender:    sender,
		MessageId: msgID,
		Message:   msg,
	}
	if r.queue.IsSet() {
		// this should never happen, but let's catch it anyway.
		return nil, fmt.Errorf("INTERNAL ERROR: message queue was not cleared")
	}
	r.queue.Set(namespace, req)
	return &Result{
		Code:    0,
		Message: []byte("message queued"),
	}, nil
}

func (r *routerImpl) Query(ctx context.Context, request []byte, resource, namespace string) ([]byte, error) {
	client, err := r.getConnectionForNamespace(namespace)
	if err != nil {
		return nil, err
	}
	res, err := client.QueryShard(ctx, &routerv1.QueryShardRequest{
		Resource: resource,
		Request:  request,
	})
	if err != nil {
		return nil, err
	}
	return res.Response, nil
}

// TODO: we eventually want this to work via namespace mappings by registered game shards.
// https://linear.app/arguslabs/issue/WORLD-13/update-router-to-look-up-the-correct-namespace-mapping
// https://linear.app/arguslabs/issue/WORLD-370/register-game-shard-on-base-shard
func (r *routerImpl) getConnectionForNamespace(ns string) (routerv1.MsgClient, error) {
	conn, err := grpc.Dial(
		r.cardinalAddr,
		grpc.WithTransportCredentials(r.creds),
	)
	if err != nil {
		return nil, fmt.Errorf("error connecting to %s address for namespace %s", r.cardinalAddr, ns)
	}
	return routerv1.NewMsgClient(conn), nil
}
