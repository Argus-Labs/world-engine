package router

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"os"
	namespacetypes "pkg.world.dev/world-engine/chain/x/namespace/types"
	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
	"time"

	"pkg.berachain.dev/polaris/eth/core"
	"pkg.berachain.dev/polaris/eth/core/types"

	"cosmossdk.io/log"
)

// Router defines the methods required to interact with a game shard. The methods are invoked from EVM smart contracts.
type Router interface {
	// SendMessage queues a message to be sent to a game shard.
	SendMessage(_ context.Context, namespace string, sender string, msgID string, msg []byte) error
	// Query queries a game shard.
	Query(ctx context.Context, request []byte, resource, namespace string) ([]byte, error)
	// MessageResult gets the game shard transaction Result that originated from an EVM tx.
	MessageResult(_ context.Context, evmTxHash string) ([]byte, string, uint32, error)
	// HandleDispatch implements the polaris EVM PostTxProcessing hook. It runs after an EVM tx execution has finished
	// processing.
	HandleDispatch(_ *types.Transaction, result *core.ExecutionResult)
}

type GetQueryCtxFn func(height int64, prove bool) (sdk.Context, error)

type GetAddressFn func(
	ctx context.Context,
	request *namespacetypes.AddressRequest,
) (*namespacetypes.AddressResponse, error)

var (
	defaultStorageTimeout        = 10 * time.Minute
	_                     Router = &routerImpl{}
)

type routerImpl struct {
	logger log.Logger
	queue  *msgQueue

	resultStore ResultStorage

	getQueryCtx GetQueryCtxFn
	getAddr     GetAddressFn

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

// NewRouter returns a Router.
func NewRouter(logger log.Logger, ctxGetter GetQueryCtxFn, addrGetter GetAddressFn, opts ...Option) Router {
	r := &routerImpl{
		logger:      logger,
		creds:       insecure.NewCredentials(),
		queue:       new(msgQueue),
		resultStore: NewMemoryResultStorage(defaultStorageTimeout),
		getQueryCtx: ctxGetter,
		getAddr:     addrGetter,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *routerImpl) getSDKCtx() sdk.Context {
	ctx, _ := r.getQueryCtx(0, false)
	return ctx
}

func (r *routerImpl) HandleDispatch(tx *types.Transaction, result *core.ExecutionResult) {
	// no-op if nothing was queued after an evm tx.
	if !r.queue.IsSet() {
		return
	}
	// if tx failed, just clear the queue, we're not going to send the message.
	if result.Failed() {
		r.logger.Debug("EVM Execution Failed, clearing the message queue")
		r.queue.Clear()
	} else {
		r.logger.Debug("Dispatching EVM transaction to game shard...")
		r.dispatchMessage(tx.Hash())
	}
}

const (
	CodeConnectionError = iota + 100
	CodeServerError
)

func (r *routerImpl) dispatchMessage(txHash common.Hash) {
	defer r.queue.Clear()
	msg := r.queue.msg
	msg.EvmTxHash = txHash.String()
	namespace := r.queue.namespace

	client, err := r.getConnectionForNamespace(namespace)
	if err != nil {
		r.resultStore.SetResult(
			&routerv1.SendMessageResponse{
				EvmTxHash: msg.EvmTxHash,
				Code:      CodeConnectionError,
				Errs:      "error getting game shard gRPC connection"},
		)
		r.logger.Error("error getting game shard gRPC connection", "error", err)
		return
	}

	r.logger.Debug("Sending tx to game shard",
		"evm_tx_hash", txHash.String(),
		"game_shard_namespace", namespace,
		"sender", msg.Sender,
		"msg_id", msg.MessageId,
	)

	// send the message in a new goroutine. we do this so that we don't make tx inclusion slower.
	go func() {
		res, err := client.SendMessage(context.Background(), msg)
		if err != nil {
			r.resultStore.SetResult(
				&routerv1.SendMessageResponse{
					EvmTxHash: msg.EvmTxHash,
					Code:      CodeServerError,
					Errs:      err.Error()},
			)
			r.logger.Error("failed to send message to game shard", "error", err)
			return
		}
		r.logger.Debug("successfully sent message to game shard")
		r.resultStore.SetResult(res)
	}()
}

func (r *routerImpl) SendMessage(_ context.Context, namespace, sender string, msgID string, msg []byte) error {
	req := &routerv1.SendMessageRequest{
		Sender:    sender,
		MessageId: msgID,
		Message:   msg,
	}
	if r.queue.IsSet() {
		// this should never happen, but let's catch it anyway.
		return fmt.Errorf("INTERNAL ERROR: message queue was not cleared")
	}
	r.queue.Set(namespace, req)
	return nil
}

func (r *routerImpl) MessageResult(_ context.Context, evmTxHash string) ([]byte, string, uint32, error) {
	res, ok := r.resultStore.Result(evmTxHash)
	if !ok {
		return nil, "", 0, fmt.Errorf("no resultStore found")
	}
	return res.Result, res.Errs, res.Code, nil
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
	ctx := r.getSDKCtx()
	res, err := r.getAddr(ctx, &namespacetypes.AddressRequest{Namespace: ns})
	if err != nil {
		return nil, err
	}
	conn, err := grpc.Dial(
		res.Address,
		grpc.WithTransportCredentials(r.creds),
	)
	if err != nil {
		return nil, fmt.Errorf("error connecting to '%s' for namespace '%s'", res.Address, ns)
	}
	return routerv1.NewMsgClient(conn), nil
}
