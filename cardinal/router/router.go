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
	ddotel "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentelemetry"
	ddtracer "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"pkg.world.dev/world-engine/cardinal/config"
	"pkg.world.dev/world-engine/cardinal/router/iterator"
	"pkg.world.dev/world-engine/cardinal/tick"
	"pkg.world.dev/world-engine/cardinal/world"
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
	SubmitTxBlob(ctx context.Context, tick *tick.Tick) error

	TransactionIterator() iterator.Iterator

	// Shutdown gracefully stops the EVM gRPC handler.
	Shutdown()
	// Start serves the EVM gRPC server.
	Start() error
}

type router struct {
	world             *world.World
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

func New(world *world.World, opts ...Option) (Router, error) {
	tracer := otel.Tracer("router")

	cfg, err := config.Load()
	if err != nil {
		return nil, eris.Wrap(err, "Failed to load config to start world")
	}

	rtr := &router{
		world:     world,
		namespace: cfg.CardinalNamespace,
		port:      defaultPort,
		routerKey: cfg.BaseShardRouterKey,
		tracer:    tracer,
	}
	for _, opt := range opts {
		opt(rtr)
	}

	conn, err := grpc.NewClient(
		cfg.BaseShardSequencerAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(credentials.NewTokenCredential(cfg.BaseShardRouterKey)),
	)
	if err != nil {
		return nil, eris.Wrapf(err, "error dialing shard seqeuncer address at %q", cfg.BaseShardSequencerAddress)
	}
	rtr.ShardSequencer = shard.NewTransactionHandlerClient(conn)

	// The job queue will have been initialized if the router option for in-memory job queues is used.
	// If it's not, we need to initialize it here.
	if rtr.sequencerJobQueue == nil {
		// TODO: add a world options to configure the jobqueue
		rtr.sequencerJobQueue, err = jobqueue.New[*shard.SubmitTransactionsRequest](
			"./.cardinal/badger",
			"submit-tx",
			20, //nolint:gomnd // Will do this later
			handleSubmitTx(rtr.ShardSequencer, tracer),
		)
		if err != nil {
			return nil, eris.Wrap(err, "failed to create job queue")
		}
	}

	rtr.server = newEvmServer(world, cfg.BaseShardRouterKey)
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

func (r *router) SubmitTxBlob(ctx context.Context, tick *tick.Tick) error {
	_, span := r.tracer.Start(ddotel.ContextWithStartOptions(ctx, ddtracer.Measured()), "router.submit-tx-blob")
	defer span.End()

	transactions := make(map[string]*shard.Transactions)
	for msgID, txs := range tick.Txs {
		protoTxs := make([]*shard.Transaction, 0, len(txs))
		for _, tx := range txs {
			protoTxs = append(protoTxs, &shard.Transaction{
				PersonaTag: tx.Tx.PersonaTag,
				Namespace:  tx.Tx.Namespace,
				Nonce:      tx.Tx.Nonce,
				Signature:  tx.Tx.Signature,
				Body:       tx.Tx.Body,
			})
		}
		transactions[msgID] = &shard.Transactions{Txs: protoTxs}
	}

	req := shard.SubmitTransactionsRequest{
		Epoch:         uint64(tick.ID),
		UnixTimestamp: uint64(tick.Timestamp),
		Namespace:     r.namespace,
		Transactions:  transactions,
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
	return iterator.New(r.namespace, r.ShardSequencer)
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
		_, span := tracer.Start(ddotel.ContextWithStartOptions(context.Background(), ddtracer.Measured()),
			"router.job-queue.submit-tx")
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
