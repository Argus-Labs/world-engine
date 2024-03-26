package router

import (
	"context"
	"net"

	"github.com/rotisserie/eris"
	zerolog "github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"pkg.world.dev/world-engine/cardinal/router/iterator"
	"pkg.world.dev/world-engine/cardinal/types/txpool"
	shardtypes "pkg.world.dev/world-engine/evm/x/shard/types"
	"pkg.world.dev/world-engine/rift/credentials"
	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
	shard "pkg.world.dev/world-engine/rift/shard/v2"
)

const (
	defaultPort = "9020"
)

var _ Router = (*router)(nil)

//go:generate mockgen -source=router.go -package mocks -destination=mocks/router.go

// Router provides functionality for Cardinal to interact with the EVM Base Shard.
// This involves a few responsibilities:
//   - Registering itself to the base shard.
//   - Receiving API requests from EVM smart contracts on the base shard.
//   - Sending transactions to the base shard's game sequencer.
//   - Querying transactions from the base shard to rebuild game state.
type Router interface {
	// RegisterGameShard registers this game shard to the base shard. This is ONLY needed so that the base shard can
	// route requests from the EVM to this game shard by using its namespace.
	RegisterGameShard(context.Context) error

	// SubmitTxBlob submits transactions processed in a tick to the base shard.
	SubmitTxBlob(
		ctx context.Context,
		processedTxs txpool.TxMap,
		epoch,
		unixTimestamp uint64,
	) error

	TransactionIterator() iterator.Iterator

	// Shutdown gracefully stops the EVM gRPC handler.
	Shutdown()
	// Start serves the EVM gRPC server.
	Start() error
}

type router struct {
	provider       Provider
	ShardSequencer shard.TransactionHandlerClient
	ShardQuerier   shardtypes.QueryClient
	namespace      string
	server         *evmServer
	// serverAddr is the address the evmServer listens on. This is set once `Start` is called.
	serverAddr string
	port       string
	routerKey  string
}

func New(namespace, sequencerAddr, baseShardQueryAddr, routerKey string, provider Provider) (Router, error) {
	rtr := &router{namespace: namespace, port: defaultPort, provider: provider, routerKey: routerKey}

	conn, err := grpc.Dial(
		sequencerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(credentials.NewSimpleTokenCredential(routerKey)),
	)
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

	rtr.server = newEvmServer(provider, routerKey)
	routerv1.RegisterMsgServer(rtr.server.grpcServer, rtr.server)
	return rtr, nil
}

func (r *router) RegisterGameShard(ctx context.Context) error {
	_, err := r.ShardSequencer.RegisterGameShard(ctx, &shard.RegisterGameShardRequest{
		Namespace:     r.namespace,
		RouterAddress: r.serverAddr,
	})
	return err
}

func (r *router) SubmitTxBlob(
	ctx context.Context,
	processedTxs txpool.TxMap,
	epoch,
	unixTimestamp uint64,
) error {
	messageIDtoTxs := make(map[uint64]*shard.Transactions)
	for msgID, txs := range processedTxs {
		protoTxs := make([]*shard.Transaction, 0, len(txs))
		for _, txData := range txs {
			tx := txData.Tx
			protoTxs = append(protoTxs, &shard.Transaction{
				PersonaTag: tx.PersonaTag,
				Namespace:  tx.Namespace,
				Nonce:      tx.Nonce,
				Signature:  tx.Signature,
				Body:       tx.Body,
			})
		}
		messageIDtoTxs[uint64(msgID)] = &shard.Transactions{Txs: protoTxs}
	}
	req := shard.SubmitTransactionsRequest{
		Epoch:         epoch,
		UnixTimestamp: unixTimestamp,
		Namespace:     r.namespace,
		Transactions:  messageIDtoTxs,
	}
	_, err := r.ShardSequencer.Submit(ctx, &req)
	return eris.Wrap(err, "")
}

func (r *router) TransactionIterator() iterator.Iterator {
	return iterator.New(r.provider.GetMessageByID, r.namespace, r.ShardQuerier)
}

func (r *router) Shutdown() {
	if r.server != nil {
		r.server.grpcServer.GracefulStop()
	}
}

func (r *router) Start() error {
	listener, err := net.Listen("tcp", ":"+r.port)
	if err != nil {
		return eris.Wrapf(err, "error listening to port %s", r.port)
	}
	go func() {
		err = eris.Wrap(r.server.grpcServer.Serve(listener), "error serving gRPC server")
		if err != nil {
			zerolog.Fatal().Err(err).Msg(eris.ToString(err, true))
		}
	}()
	r.serverAddr = listener.Addr().String()
	return nil
}
