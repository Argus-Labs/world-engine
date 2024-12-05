package router

import (
	"context"
	"net"

	"github.com/argus-labs/go-jobqueue"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"pkg.world.dev/world-engine/cardinal/router/iterator"
	"pkg.world.dev/world-engine/cardinal/txpool"
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
	provider          Provider
	ShardSequencer    shard.TransactionHandlerClient
	namespace         string
	server            *evmServer
	sequencerJobQueue *jobqueue.JobQueue[*shard.SubmitTransactionsRequest]

	// serverAddr is the address the evmServer listens on. This is set once `Start` is called.
	serverAddr string
	port       string
	routerKey  string

	tracer trace.Tracer
}

func New(namespace, sequencerAddr, routerKey string, world Provider, opts ...Option) (Router, error) {
	tracer := otel.Tracer("router")
	rtr := &router{
		provider:  world,
		namespace: namespace,
		port:      defaultPort,
		routerKey: routerKey,
		tracer:    tracer,
	}
	for _, opt := range opts {
		opt(rtr)
	}

	conn, err := grpc.NewClient(
		sequencerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(credentials.NewTokenCredential(routerKey)),
	)
	if err != nil {
		return nil, eris.Wrapf(err, "error dialing shard seqeuncer address at %q", sequencerAddr)
	}
	rtr.ShardSequencer = shard.NewTransactionHandlerClient(conn)

	// The job queue will have been initialized if the router option for in-memory job queues is used.
	// If it's not, we need to initialize it here.
	if rtr.sequencerJobQueue == nil {
		// TODO: add a world options to configure the jobqueue
		rtr.sequencerJobQueue, err = jobqueue.New[*shard.SubmitTransactionsRequest](
			"./.cardinal/badger",
			"submit-tx",
			20, //nolint:mnd // Will do this later
			handleSubmitTx(rtr.ShardSequencer, tracer),
		)
		if err != nil {
			return nil, eris.Wrap(err, "failed to create job queue")
		}
	}

	rtr.server = newEvmServer(world, routerKey)
	routerv1.RegisterMsgServer(rtr.server.grpcServer, rtr.server)
	return rtr, nil
}

func (r *router) RegisterGameShard(ctx context.Context) error {
	log.Info().Msg("Registering game shard with EVM base shard")

	_, err := r.ShardSequencer.RegisterGameShard(ctx, &shard.RegisterGameShardRequest{
		Namespace:     r.namespace,
		RouterAddress: r.serverAddr,
	})
	if err != nil {
		return eris.Wrap(err, "failed to register game shard to base shard")
	}

	log.Info().Msg("Game shard registered with EVM base shard")
	return nil
}

func (r *router) SubmitTxBlob(
	ctx context.Context,
	processedTxs txpool.TxMap,
	epoch,
	unixTimestamp uint64,
) error {
	_, span := r.tracer.Start(ctx, "router.submit-tx-blob")
	defer span.End()

	messageIDtoTxs := make(map[uint64]*shard.Transactions)
	for msgID, txs := range processedTxs {
		protoTxs := make([]*shard.Transaction, 0, len(txs))
		for _, txData := range txs {
			tx := txData.Tx
			protoTxs = append(protoTxs, &shard.Transaction{
				PersonaTag: tx.PersonaTag,
				Namespace:  tx.Namespace,
				Timestamp:  tx.Timestamp,
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

	_, err := r.sequencerJobQueue.Enqueue(&req)
	if err != nil {
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return eris.Wrap(err, "failed to submit tx sequencing payload to job queue")
	}

	return nil
}

func (r *router) TransactionIterator() iterator.Iterator {
	return iterator.New(r.provider.GetMessageByID, r.namespace, r.ShardSequencer)
}

func (r *router) Shutdown() {
	if r.server != nil {
		r.server.grpcServer.GracefulStop()
	}
	_ = r.sequencerJobQueue.Stop()
}

func (r *router) Start() error {
	log.Info().Msg("Rollup mode enabled - starting router")

	listener, err := net.Listen("tcp", ":"+r.port)
	if err != nil {
		return eris.Wrapf(err, "error listening to port %s", r.port)
	}
	go func() {
		err = eris.Wrap(r.server.grpcServer.Serve(listener), "error serving gRPC server")
		if err != nil {
			log.Fatal().Err(err).Msg(eris.ToString(err, true))
		}
	}()
	r.serverAddr = listener.Addr().String()

	log.Info().Msg("Router started")
	return nil
}

func handleSubmitTx(sequencer shard.TransactionHandlerClient, tracer trace.Tracer) func(
	jobqueue.JobContext, *shard.SubmitTransactionsRequest,
) error {
	return func(_ jobqueue.JobContext, req *shard.SubmitTransactionsRequest) error {
		_, span := tracer.Start(context.Background(), "router.job-queue.submit-tx")
		defer span.End()

		_, err := sequencer.Submit(context.Background(), req)
		if err != nil {
			span.SetStatus(codes.Error, eris.ToString(err, true))
			span.RecordError(err)
			return eris.Wrap(err, "failed to submit transactions to sequencer")
		}
		return nil
	}
}
