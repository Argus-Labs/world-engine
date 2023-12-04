package router

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	namespacetypes "pkg.world.dev/world-engine/evm/x/namespace/types"
	routerv1 "pkg.world.dev/world-engine/rift/router/v1"

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
	// PostBlockHook implements the polaris EVM PostBlock hook. It runs after an EVM tx execution has finished
	// processing.
	PostBlockHook(types.Transactions, types.Receipts, types.Signer)
}

type GetQueryCtxFn func(height int64, prove bool) (sdk.Context, error)

type GetAddressFn func(
	ctx context.Context,
	request *namespacetypes.AddressRequest,
) (*namespacetypes.AddressResponse, error)

var (
	defaultStorageTimeout        = 1 * time.Hour
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

// NewRouter returns a Router.
func NewRouter(logger log.Logger, ctxGetter GetQueryCtxFn, addrGetter GetAddressFn, opts ...Option) Router {
	r := &routerImpl{
		logger:      logger,
		creds:       insecure.NewCredentials(),
		queue:       newMsgQueue(),
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

func (r *routerImpl) PostBlockHook(transactions types.Transactions, receipts types.Receipts, signer types.Signer) {
	r.logger.Info("running PostBlockHook",
		"num_transactions", len(transactions),
	)
	// loop over all txs
	for i, tx := range transactions {
		r.logger.Info("working on transaction", "tx_hash", tx.Hash().String())
		// we check the `To` address because the precompile msg.Sender will be the contract address,
		// NOT the tx.Origin address, or, more formally, the EOA address that triggered the tx.
		if txTo := tx.To(); txTo != nil {
			toAddr := *txTo
			// check if theres a cross-shard tx queued from this address
			if r.queue.IsSet(toAddr) {
				receipt := receipts[i]
				// ensure this tx was executed successfully. we don't want to send a tx to Cardinal if the
				// EVM tx failed.
				if receipt.Status == ethtypes.ReceiptStatusSuccessful {
					r.logger.Debug("attempting to dispatch tx",
						"sender", toAddr.String(),
						"txHash", receipt.TxHash.String(),
					)
					r.dispatchMessage(toAddr, receipt.TxHash)
				}
			}
		}
	}
	r.queue.Clear()
}

const (
	CodeConnectionError = iota + 100
	CodeServerError
)

func (r *routerImpl) dispatchMessage(sender common.Address, txHash common.Hash) {
	// get the message from the queue.
	nsMsg, exists := r.queue.Message(sender)
	if !exists {
		r.logger.Error("no message found in queue for sender", "sender", sender.String())
		return
	}
	r.logger.Info("found cross-shard message in queue", "tx_hash", txHash.String())
	msg := nsMsg.msg
	msg.Sender = strings.ToLower(msg.Sender) // normalize the request
	namespace := nsMsg.namespace
	msg.EvmTxHash = txHash.String()
	r.logger.Info("attempting to get client connection")
	client, err := r.getConnectionForNamespace(namespace)
	if err != nil {
		r.logger.Error("failed to get client connection")
		r.resultStore.SetResult(
			&routerv1.SendMessageResponse{
				EvmTxHash: msg.EvmTxHash,
				Code:      CodeConnectionError,
				Errs:      "error getting game shard gRPC connection"},
		)
		r.logger.Error("error getting game shard gRPC connection", "error", err, "namespace", namespace)
		return
	}
	r.logger.Info("Sending tx to game shard",
		"evm_tx_hash", txHash.String(),
		"namespace", namespace,
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
		r.logger.Info("successfully sent message to game shard", "result", res.String())
		r.resultStore.SetResult(res)
	}()
}

func (r *routerImpl) SendMessage(_ context.Context, namespace, sender, msgID string, msg []byte) error {
	r.logger.Info("received SendMessage request",
		"namespace", namespace,
		"sender", sender,
		"msgID", msgID,
	)
	req := &routerv1.SendMessageRequest{
		Sender:    sender,
		MessageId: msgID,
		Message:   msg,
	}
	r.logger.Info("attempting to set queue...")
	err := r.queue.Set(common.HexToAddress(sender), namespace, req)
	if err != nil {
		r.logger.Error("failed to queue message", "error", err.Error())
		return err
	}
	r.logger.Info("successfully set queue")
	return nil
}

func (r *routerImpl) MessageResult(_ context.Context, evmTxHash string) ([]byte, string, uint32, error) {
	res, ok := r.resultStore.Result(evmTxHash)
	if !ok {
		return nil, "", 0, fmt.Errorf("no result found for tx %s", evmTxHash)
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

func (r *routerImpl) getConnectionForNamespace(ns string) (routerv1.MsgClient, error) {
	// CLI is currently broken. Rollkit is looking into the issue.
	// So for now, we just use an address loaded from env.
	// ctx := r.getSDKCtx()
	// res, err := r.getAddr(ctx, &namespacetypes.AddressRequest{Namespace: ns})
	// if err != nil {
	//	return nil, err
	// }
	addr := os.Getenv("GAME_SHARD_ADDR")
	if addr == "" {
		return nil, fmt.Errorf("no address set for router to dial")
	}
	conn, err := grpc.Dial(
		addr,
		grpc.WithTransportCredentials(r.creds),
	)
	if err != nil {
		return nil, fmt.Errorf("error connecting to '%s' for namespace '%s'", addr, ns)
	}
	return routerv1.NewMsgClient(conn), nil
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
