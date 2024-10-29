package router

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"pkg.world.dev/world-engine/cardinal/tick"
	"pkg.world.dev/world-engine/cardinal/world"
	"pkg.world.dev/world-engine/rift/credentials"
	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
	"pkg.world.dev/world-engine/sign"
)

const (
	CodeSuccess = uint32(iota)
	CodeTxFailed
	CodeNoResult
	CodeServerUnresponsive
	CodeUnauthorized
	CodeUnsupportedMessage
	CodeInvalidFormat
)

var _ routerv1.MsgServer = (*evmServer)(nil)

type evmServer struct {
	routerv1.MsgServer

	world      *world.World
	grpcServer *grpc.Server
	routerKey  string
}

func newEvmServer(w *world.World, routerKey string) *evmServer {
	e := &evmServer{
		world:     w,
		routerKey: routerKey,
	}
	e.grpcServer = grpc.NewServer(grpc.UnaryInterceptor(e.serverCallInterceptor))
	return e
}

// serverCallInterceptor catches calls to handlers and ensures they have the right secret key.
func (e *evmServer) serverCallInterceptor(
	ctx context.Context,
	req any,
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp any, err error) {
	// we only want to guard the SendMessage method. not the query shard method.
	if _, ok := req.(*routerv1.SendMessageRequest); !ok {
		return handler(ctx, req)
	}

	rtrKey, err := credentials.TokenFromIncomingContext(ctx)
	if err != nil {
		return nil, err
	}

	if rtrKey != e.routerKey {
		return nil, status.Errorf(codes.Unauthenticated, "invalid %s", credentials.TokenKey)
	}

	return handler(ctx, req)
}

// SendMessage is the grpcServer impl that receives SendMessage requests from the base shard client.
func (e *evmServer) SendMessage(_ context.Context, req *routerv1.SendMessageRequest) (
	*routerv1.SendMessageResponse, error,
) {
	// since we are injecting the msgValue directly, all we need is the persona tag in the signed payload.
	// the sig checking happens in the grpcServer's Handler, not in ecs.Engine.
	tx := &sign.Transaction{
		PersonaTag: req.GetPersonaTag(),
		Body:       req.GetMessage(),
	}

	// txHash is not be confused with EvmTxHash. txHash is Cardinal's internal representation of TxHash, while EvmTxHash
	// is the hash of the EVM transaction that triggered the request.
	txHash, err := e.world.AddEVMTransaction(req.GetMessageId(), tx, req.GetSender(),
		common.HexToHash(req.GetEvmTxHash()))
	if err != nil {
		return nil, eris.Wrap(err, "failed to add evm transaction to tx pool")
	}

	// Attempt to get receipt until timeout
	var recJSON []byte
	timeout := time.After(5 * time.Second)

loop:
	for {
		select {
		case <-timeout:
			return nil, eris.Wrap(err, "failed to get receipt")
		default:
			recJSON, err = e.world.GetReceiptBytes(*txHash)
			if err == nil {
				break loop // We need to use a labeled break to escape the select + for loop.
			}
		}
	}

	// check for the msgValue receipt.
	recJSON, err = e.world.GetReceiptBytes(*txHash)
	if err != nil {
		return &routerv1.SendMessageResponse{
			EvmTxHash: req.GetEvmTxHash(),
			Code:      CodeNoResult,
		}, nil
	}

	var rec tick.Receipt
	if err := json.Unmarshal(recJSON, &rec); err != nil {
		return &routerv1.SendMessageResponse{
			EvmTxHash: req.GetEvmTxHash(),
			Code:      CodeTxFailed,
		}, nil
	}

	code := CodeSuccess
	var txErr string
	if rec.Error != "" {
		code = CodeTxFailed
		txErr = rec.Error
	}

	msg, ok := e.world.GetMessage(req.GetMessageId())
	if !ok {
		return &routerv1.SendMessageResponse{
			EvmTxHash: req.GetEvmTxHash(),
			Code:      CodeUnsupportedMessage,
		}, nil
	}

	result, err := msg.ABIEncode(rec.Result)
	if err != nil {
		return &routerv1.SendMessageResponse{
			EvmTxHash: req.GetEvmTxHash(),
			Code:      CodeInvalidFormat,
		}, nil
	}

	return &routerv1.SendMessageResponse{
		Errs:      txErr,
		Result:    result,
		EvmTxHash: req.GetEvmTxHash(),
		Code:      code,
	}, nil
}

// QueryShard is the grpcServer impl that answers query requests from the base shard client.
func (e *evmServer) QueryShard(_ context.Context, req *routerv1.QueryShardRequest) (
	*routerv1.QueryShardResponse, error,
) {
	log.Debug().Msgf("get request for %q", req.GetResource())

	// TODO(scott): the group name should not be hardcoded
	reply, err := e.world.HandleQueryEVM("game", req.GetResource(), req.GetRequest())
	if err != nil {
		log.Error().Err(err).Msg("failed to handle query")
		return nil, err
	}

	log.Debug().Msgf("sending back reply: %v", reply)
	return &routerv1.QueryShardResponse{Response: reply}, nil
}
