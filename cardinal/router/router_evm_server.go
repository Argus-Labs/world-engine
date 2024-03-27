package router

import (
	"context"
	"errors"
	"fmt"
	"slices"

	zerolog "github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

	provider   Provider
	grpcServer *grpc.Server
	routerKey  string
}

func newEvmServer(p Provider, routerKey string) *evmServer {
	e := &evmServer{
		provider:  p,
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
func (e *evmServer) SendMessage(
	_ context.Context, req *routerv1.SendMessageRequest,
) (*routerv1.SendMessageResponse, error) {
	// first we check if we can extract the transaction associated with the id
	msgType, exists := e.provider.GetMessageByFullName(req.GetMessageId())
	if !exists || !msgType.IsEVMCompatible() {
		return &routerv1.SendMessageResponse{
			Errs: fmt.Errorf(
				"message with name %s either does not exist, or did not have EVM support "+
					"enabled", req.GetMessageId(),
			).
				Error(),
			EvmTxHash: req.GetEvmTxHash(),
			Code:      CodeUnsupportedMessage,
		}, nil
	}

	// decode the evm bytes into the transaction
	msgValue, err := msgType.DecodeEVMBytes(req.GetMessage())
	if err != nil {
		return &routerv1.SendMessageResponse{
			Errs: fmt.Errorf("failed to decode bytes into ABI type: %w", err).
				Error(),
			EvmTxHash: req.GetEvmTxHash(),
			Code:      CodeInvalidFormat,
		}, nil
	}

	// get the signer component for the persona tag the request wants to use, and check if the evm address in the
	// sender is present in the signer component's authorized address list.
	signer, err := e.provider.GetSignerComponentForPersona(req.GetPersonaTag())
	if err != nil {
		return &routerv1.SendMessageResponse{
			Errs: fmt.Errorf("unable to find persona tag %q: %w", req.GetPersonaTag(), err).
				Error(),
			EvmTxHash: req.GetEvmTxHash(),
			Code:      CodeUnauthorized,
		}, nil
	}
	if !slices.Contains(signer.AuthorizedAddresses, req.GetSender()) {
		return &routerv1.SendMessageResponse{
			Errs: fmt.Errorf("persona tag %q has not authorized address %q", req.GetPersonaTag(), req.GetSender()).
				Error(),
			EvmTxHash: req.GetEvmTxHash(),
			Code:      CodeUnauthorized,
		}, nil
	}

	// since we are injecting the msgValue directly, all we need is the persona tag in the signed payload.
	// the sig checking happens in the grpcServer's Handler, not in ecs.Engine.
	sig := &sign.Transaction{PersonaTag: req.GetPersonaTag()}
	e.provider.AddEVMTransaction(msgType.ID(), msgValue, sig, req.GetEvmTxHash())

	// wait for the next tick so the msgValue gets processed
	success := e.provider.WaitForNextTick()
	if !success {
		return &routerv1.SendMessageResponse{
			EvmTxHash: req.GetEvmTxHash(),
			Code:      CodeServerUnresponsive,
		}, nil
	}

	// check for the msgValue receipt.
	result, errs, evmTxHash, exists := e.provider.ConsumeEVMMsgResult(req.GetEvmTxHash())
	if !exists {
		return &routerv1.SendMessageResponse{
			EvmTxHash: req.GetEvmTxHash(),
			Code:      CodeNoResult,
		}, nil
	}

	// we got a receipt, so lets clean it up and return it.
	var errStr string
	code := CodeSuccess
	if retErr := errors.Join(errs...); retErr != nil {
		code = CodeTxFailed
		errStr = retErr.Error()
	}
	return &routerv1.SendMessageResponse{
		Errs:      errStr,
		Result:    result,
		EvmTxHash: evmTxHash,
		Code:      code,
	}, nil
}

// QueryShard is the grpcServer impl that answers query requests from the base shard client.
func (e *evmServer) QueryShard(_ context.Context, req *routerv1.QueryShardRequest) (
	*routerv1.QueryShardResponse, error,
) {
	zerolog.Logger.Debug().Msgf("get request for %q", req.GetResource())
	reply, err := e.provider.HandleEVMQuery(req.GetResource(), req.GetRequest())
	if err != nil {
		zerolog.Logger.Error().Err(err).Msg("failed to handle query")
		return nil, err
	}
	zerolog.Logger.Debug().Msgf("sending back reply: %v", reply)
	return &routerv1.QueryShardResponse{Response: reply}, nil
}
