package router

import (
	"context"
	"errors"
	"fmt"
	zerolog "github.com/rs/zerolog/log"
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

// SendMessage is the server impl that receives SendMessage requests from the base shard client.
func (r *router) SendMessage(_ context.Context, req *routerv1.SendMessageRequest,
) (*routerv1.SendMessageResponse, error) {
	// first we check if we can extract the transaction associated with the id
	msgType, exists := r.provider.GetMessageByName(req.MessageId)
	if !exists || !msgType.IsEVMCompatible() {
		return &routerv1.SendMessageResponse{
			Errs: fmt.Errorf(
				"message with name %s either does not exist, or did not have EVM support "+
					"enabled", req.MessageId,
			).
				Error(),
			EvmTxHash: req.EvmTxHash,
			Code:      CodeUnsupportedMessage,
		}, nil
	}

	// decode the evm bytes into the transaction
	msgValue, err := msgType.DecodeEVMBytes(req.Message)
	if err != nil {
		return &routerv1.SendMessageResponse{
			Errs: fmt.Errorf("failed to decode bytes into ABI type: %w", err).
				Error(),
			EvmTxHash: req.EvmTxHash,
			Code:      CodeInvalidFormat,
		}, nil
	}

	// check if the sender has a linked persona address. if not don't process the transaction.
	personaTag, err := r.provider.GetPersonaForEVMAddress(req.Sender)
	if err != nil {
		return &routerv1.SendMessageResponse{
			Errs: fmt.Errorf("unable to find persona tag associated with the EVM address %q: %w", req.Sender, err).
				Error(),
			EvmTxHash: req.EvmTxHash,
			Code:      CodeUnauthorized,
		}, nil
	}

	// since we are injecting the msgValue directly, all we need is the persona tag in the signed payload.
	// the sig checking happens in the server's Handler, not in ecs.Engine.
	sig := &sign.Transaction{PersonaTag: personaTag}
	r.provider.AddEVMTransaction(msgType.ID(), msgValue, sig, req.EvmTxHash)

	// wait for the next tick so the msgValue gets processed
	success := r.provider.WaitForNextTick()
	if !success {
		return &routerv1.SendMessageResponse{
			EvmTxHash: req.EvmTxHash,
			Code:      CodeServerUnresponsive,
		}, nil
	}

	// check for the msgValue receipt.
	result, errs, evmTxHash, exists := r.provider.ConsumeEVMMsgResult(req.EvmTxHash)
	if !exists {
		return &routerv1.SendMessageResponse{
			EvmTxHash: req.EvmTxHash,
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

// QueryShard is the server impl that answers query requests from the base shard client.
func (r *router) QueryShard(_ context.Context, req *routerv1.QueryShardRequest) (
	*routerv1.QueryShardResponse, error,
) {
	zerolog.Logger.Debug().Msgf("get request for %q", req.Resource)
	reply, err := r.provider.HandleEVMQuery(req.Resource, req.Request)
	if err != nil {
		zerolog.Logger.Error().Err(err).Msg("failed to handle query")
		return nil, err
	}
	zerolog.Logger.Debug().Msgf("sending back reply: %v", reply)
	return &routerv1.QueryShardResponse{Response: reply}, nil
}
