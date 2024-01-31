package router

import (
	"context"
	"errors"
	"fmt"
	"github.com/rotisserie/eris"
	zerolog "github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/cardinal/types/message"
	shardtypes "pkg.world.dev/world-engine/evm/x/shard/types"
	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
	shard "pkg.world.dev/world-engine/rift/shard/v2"
	"pkg.world.dev/world-engine/sign"
)

const (
	defaultPort = "9020"
)

type Provider interface {
	GetMessageByName(string) (message.Message, bool)
	GetQueryByName(string) (ecs.Query, bool)
	HandleQuery(query ecs.Query, request any) (any, error)
	GetPersonaForEVMAddress(string) (string, error)
	WaitForNextTick() bool
	AddEVMTransaction(id message.TypeID, msgValue any, tx *sign.Transaction, evmTxHash string) (tick uint64, txHash message.TxHash)
	ConsumeEVMMsgResult(evmTxHash string) (ecs.EVMTxReceipt, bool)
}

var _ routerv1.MsgServer = &Router{}

type Router struct {
	routerv1.MsgServer

	provider       Provider
	ShardSequencer shard.TransactionHandlerClient
	ShardQuerier   shardtypes.QueryClient

	port string
}

func New(sequencerAddr, baseShardQueryAddr string, opts ...Option) (*Router, error) {
	rtr := &Router{port: defaultPort}
	for _, opt := range opts {
		opt(rtr)
	}

	conn, err := grpc.Dial(sequencerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, eris.Wrapf(err, "error dialing shard seqeuncer address at %q", sequencerAddr)
	}
	rtr.ShardSequencer = shard.NewTransactionHandlerClient(conn)

	// we don't need secure comms for this connection, cause we're just querying cosmos public RPC endpoints.
	conn2, err := grpc.Dial(baseShardQueryAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, eris.Wrapf(err, "error dialing evm base shard address at %q", baseShardQueryAddr)
	}
	rtr.ShardQuerier = shardtypes.NewQueryClient(conn2)
	return rtr, nil
}

const (
	CodeSuccess = iota
	CodeTxFailed
	CodeNoResult
	CodeServerUnresponsive
	CodeUnauthorized
	CodeUnsupportedMessage
	CodeInvalidFormat
)

func (r *Router) SendMessage(_ context.Context, req *routerv1.SendMessageRequest) (*routerv1.SendMessageResponse, error) {
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
	receipt, exists := r.provider.ConsumeEVMMsgResult(req.EvmTxHash)
	if !exists {
		return &routerv1.SendMessageResponse{
			EvmTxHash: req.EvmTxHash,
			Code:      CodeNoResult,
		}, nil
	}

	// we got a receipt, so lets clean it up and return it.
	var errStr string
	code := CodeSuccess
	if retErr := errors.Join(receipt.Errs...); retErr != nil {
		code = CodeTxFailed
		errStr = retErr.Error()
	}
	return &routerv1.SendMessageResponse{
		Errs:      errStr,
		Result:    receipt.ABIResult,
		EvmTxHash: receipt.EVMTxHash,
		Code:      uint32(code),
	}, nil
}

func (r *Router) QueryShard(_ context.Context, req *routerv1.QueryShardRequest) (
	*routerv1.QueryShardResponse, error,
) {
	zerolog.Logger.Debug().Msgf("get request for %q", req.Resource)
	queryType, ok := r.provider.GetQueryByName(req.Resource)
	if !ok || !queryType.IsEVMCompatible() {
		return nil, eris.Errorf("query %q was either not found or not EVM compatible", req.Resource)
	}
	ecsRequest, err := queryType.DecodeEVMRequest(req.Request)
	if err != nil {
		zerolog.Logger.Error().Err(err).Msg("failed to decode query request")
		return nil, err
	}
	reply, err := r.provider.HandleQuery(queryType, ecsRequest)
	if err != nil {
		zerolog.Logger.Error().Err(err).Msg("failed to handle query")
		return nil, err
	}
	zerolog.Logger.Debug().Msg("successfully handled query")
	bz, err := queryType.EncodeEVMReply(reply)
	if err != nil {
		zerolog.Logger.Error().Err(err).Msg("failed to encode query reply for EVM")
		return nil, err
	}
	zerolog.Logger.Debug().Msgf("sending back reply: %v", reply)
	return &routerv1.QueryShardResponse{Response: bz}, nil
}
func (r *Router) QueryTransactions(ctx context.Context, req *shardtypes.QueryTransactionsRequest) (
	*shardtypes.QueryTransactionsResponse,
	error,
) {
	res, err := r.ShardQuerier.Transactions(ctx, req)
	return res, eris.Wrap(err, "")
}

func (r *Router) Submit(
	ctx context.Context,
	processedTxs txpool.TxMap,
	namespace string,
	epoch,
	unixTimestamp uint64,
) error {
	messageIDtoTxs := make(map[uint64]*shard.Transactions)
	for msgID, txs := range processedTxs {
		protoTxs := make([]*shard.Transaction, 0, len(txs))
		for _, txData := range txs {
			protoTxs = append(protoTxs, transactionToProto(txData.Tx))
		}
		messageIDtoTxs[uint64(msgID)] = &shard.Transactions{Txs: protoTxs}
	}
	req := shard.SubmitTransactionsRequest{
		Epoch:         epoch,
		UnixTimestamp: unixTimestamp,
		Namespace:     namespace,
		Transactions:  messageIDtoTxs,
	}
	_, err := r.ShardSequencer.Submit(ctx, &req)
	return eris.Wrap(err, "")
}

func transactionToProto(sp *sign.Transaction) *shard.Transaction {
	return &shard.Transaction{
		PersonaTag: sp.PersonaTag,
		Namespace:  sp.Namespace,
		Nonce:      sp.Nonce,
		Signature:  sp.Signature,
		Body:       sp.Body,
	}
}
