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
	"google.golang.org/grpc/credentials/insecure"
	"os"

	"pkg.berachain.dev/polaris/eth/core"
	"pkg.berachain.dev/polaris/eth/core/types"

	"cosmossdk.io/log"
)

type Result struct {
	Code    uint64
	Message []byte
}

//go:generate mockgen -source=router.go -package mocks -destination mocks/router.go
type Router interface {
	// SendMessage sends the msg payload to the game shard indicated by the namespace, if such namespace exists on chain.
	SendMessage(ctx context.Context, namespace, sender string, msgID uint64, msg []byte) (*Result, error)
	Query(ctx context.Context, request []byte, resource, namespace string) ([]byte, error)
	DispatchOrDequeue(_ *types.Transaction, result *core.ExecutionResult)
}

var (
	_ Router = &router{}
)

type router struct {
	cardinalAddr  string
	logger        log.Logger
	queuedMessage *v1.SendMessageRequest
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

// NewRouter returns a new router instance with a connection to a single cardinal shard instance.
// TODO(technicallyty): its a bit unclear how im going to query the state machine here, so router is just going to
// take the cardinal address directly for now...
func NewRouter(cardinalAddr string, logger log.Logger, opts ...Option) Router {
	r := &router{cardinalAddr: cardinalAddr, logger: logger, creds: insecure.NewCredentials()}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *router) DispatchOrDequeue(tx *types.Transaction, result *core.ExecutionResult) {
	if result.Failed() {
		r.clearQueue()
	} else {
		r.dispatchMessage()
	}
}

func (r *router) dispatchMessage() {
	defer r.clearQueue()
	// we do not need to pass in a namespace, since we just default to a given cardinal addr anyways.
	// this will eventually need to update to have a proper mapping of namespace -> game shard EVM grpc address.
	// https://linear.app/arguslabs/issue/WORLD-13/update-router-to-look-up-the-correct-namespace-mapping
	client, err := r.getConnectionForNamespace("")
	if err != nil {
		// TODO: once we implement https://linear.app/arguslabs/issue/WORLD-8/implement-evm-callbacks, we need to store
		// this error in the callback storage module.
		r.logger.Error("error getting game shard gRPC connection", "error", err)
		return
	}
	res, err := client.SendMessage(context.Background(), r.queuedMessage)
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

func (r *router) clearQueue() {
	r.queuedMessage = nil
}

func (r *router) SendMessage(ctx context.Context, namespace, sender string, msgID uint64, msg []byte) (*Result, error) {
	req := &v1.SendMessageRequest{
		Sender:    sender,
		MessageId: msgID,
		Message:   msg,
	}
	if r.queuedMessage != nil {
		return nil, fmt.Errorf("INTERNAL: message was already queued in the router")
	}
	r.queuedMessage = req
	return &Result{
		Code:    0,
		Message: []byte("message queued"),
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

// TODO: we eventually want this to work via namespace mappings by registered game shards.
// https://linear.app/arguslabs/issue/WORLD-13/update-router-to-look-up-the-correct-namespace-mapping
// https://linear.app/arguslabs/issue/WORLD-370/register-game-shard-on-base-shard
func (r *router) getConnectionForNamespace(ns string) (routerv1grpc.MsgClient, error) {
	conn, err := grpc.Dial(
		r.cardinalAddr,
		grpc.WithTransportCredentials(r.creds),
	)
	if err != nil {
		return nil, fmt.Errorf("error connecting to %s address for namespace %s", r.cardinalAddr, ns)
	}
	return routerv1grpc.NewMsgClient(conn), nil
}
