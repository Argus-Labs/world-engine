package router

import (
	"context"
	"github.com/rotisserie/eris"
	zerolog "github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"net"
	"pkg.world.dev/world-engine/cardinal/txpool"
	shardtypes "pkg.world.dev/world-engine/evm/x/shard/types"
	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
	shard "pkg.world.dev/world-engine/rift/shard/v2"
	"pkg.world.dev/world-engine/sign"
)

const (
	defaultPort = "9020"
)

//go:generate mockgen -source=router.go -package mocks -destination=mocks/router.go

// Router provides functionality for Cardinal to interact with the EVM Base Shard.
// This involves a few responsibilities:
//   - Receiving API requests from EVM smart contracts on the base shard.
//   - Sending transactions to the base shard's game sequencer.
//   - Querying transactions from the base shard to rebuild game state.
type Router interface {
	// SubmitTxBlob submits transactions processed in a tick to the base shard.
	SubmitTxBlob(
		ctx context.Context,
		processedTxs txpool.TxMap,
		namespace string,
		epoch,
		unixTimestamp uint64,
	) error

	// QueryTransactions queries transactions from the base shard.
	QueryTransactions(ctx context.Context, req *shardtypes.QueryTransactionsRequest) (
		*shardtypes.QueryTransactionsResponse,
		error,
	)

	// Shutdown gracefully stops the EVM gRPC handler.
	Shutdown()
	// Start serves the EVM gRPC server.
	Start() error
}

var _ routerv1.MsgServer = (*router)(nil)
var _ Router = (*router)(nil)

type router struct {
	routerv1.MsgServer

	provider       Provider
	ShardSequencer shard.TransactionHandlerClient
	ShardQuerier   shardtypes.QueryClient

	port     string
	shutdown func()
}

func New(sequencerAddr, baseShardQueryAddr string, provider Provider) (Router, error) {
	rtr := &router{port: defaultPort, provider: provider}

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

func (r *router) SubmitTxBlob(
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

func (r *router) QueryTransactions(ctx context.Context, req *shardtypes.QueryTransactionsRequest) (
	*shardtypes.QueryTransactionsResponse,
	error,
) {
	res, err := r.ShardQuerier.Transactions(ctx, req)
	return res, eris.Wrap(err, "")
}

func (r *router) Shutdown() {
	if r.shutdown != nil {
		r.shutdown()
	}
}

func (r *router) Start() error {
	server := grpc.NewServer()
	routerv1.RegisterMsgServer(server, r)
	listener, err := net.Listen("tcp", ":"+r.port)
	if err != nil {
		return eris.Wrapf(err, "error listening to port %s", r.port)
	}
	go func() {
		err = eris.Wrap(server.Serve(listener), "error serving server")
		if err != nil {
			zerolog.Fatal().Err(err).Msg(eris.ToString(err, true))
		}
	}()
	r.shutdown = server.GracefulStop
	return nil
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
