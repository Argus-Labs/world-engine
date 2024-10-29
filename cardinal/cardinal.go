package cardinal

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
	ddotel "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentelemetry"
	ddtracer "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"pkg.world.dev/world-engine/cardinal/config"
	"pkg.world.dev/world-engine/cardinal/plugin/task"
	"pkg.world.dev/world-engine/cardinal/router"
	"pkg.world.dev/world-engine/cardinal/router/iterator"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/cardinal/storage/redis"
	"pkg.world.dev/world-engine/cardinal/telemetry"
	"pkg.world.dev/world-engine/cardinal/tick"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/world"
)

const (
	RedisDialTimeOut = 150
)

type Cardinal struct {
	cancel      context.CancelFunc
	tickChannel <-chan time.Time
	isReplica   bool
	config      config.Config

	world  *world.World
	server *server.Server
	router router.Router

	telemetry *telemetry.Manager
	tracer    trace.Tracer // Tracer for Cardinal

	subscribers []chan *tick.Tick
	mu          *sync.RWMutex
	closed      bool

	startHook func() error
}

func New(opts ...CardinalOption) (*Cardinal, *world.World, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, eris.Wrap(err, "Failed to load config to start world")
	}
	cardinalOpts, routerOpts, worldOpts := separateOptions(opts)

	// Initialize telemetry
	var tm *telemetry.Manager
	if cfg.TelemetryTraceEnabled || cfg.TelemetryProfilerEnabled {
		tm, err = telemetry.New(cfg.TelemetryTraceEnabled, cfg.TelemetryProfilerEnabled)
		if err != nil {
			return nil, nil, eris.Wrap(err, "failed to create telemetry manager")
		}
	}

	rs := redis.NewRedisStorage(redis.Options{
		Addr:        cfg.RedisAddress,
		Password:    cfg.RedisPassword,
		DB:          0,                              // use default DB
		DialTimeout: RedisDialTimeOut * time.Second, // Increase startup dial timeout
	}, cfg.CardinalNamespace)

	w, err := world.New(&rs, worldOpts...)
	if err != nil {
		return nil, nil, eris.Wrap(err, "failed to create world")
	}

	s, err := server.New(w)
	if err != nil {
		return nil, nil, eris.Wrap(err, "failed to create server")
	}

	// Initialize shard router if running in rollup mode
	var rtr router.Router
	if cfg.CardinalRollupEnabled {
		rtr, err = router.New(w, routerOpts...)
		if err != nil {
			return nil, nil, eris.Wrap(err, "failed to initialize shard router")
		}
	}

	c := &Cardinal{
		world:     w,
		server:    s,
		router:    rtr,
		telemetry: tm,
		tracer:    otel.Tracer("cardinal"),
		mu:        &sync.RWMutex{},
		isReplica: false,
		config:    *cfg,
	}

	// Apply options
	for _, opt := range cardinalOpts {
		opt(c)
	}

	// Register plugins
	world.RegisterPlugin(w, task.NewPlugin())

	return c, w, nil
}

func (c *Cardinal) Start() error {
	var ctx context.Context
	ctx, c.cancel = context.WithCancel(context.Background())

	err := c.world.Init()
	if err != nil {
		return eris.Wrap(err, "failed to init world")
	}

	// Handles SIGINT and SIGTERM signals and starts the shutdown process.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		c.Stop()
	}()

	// It is possible to inject a custom tick channel that provides manual control over when ticks are executed
	// by passing in the WithTickChannel option on the cardinal.New function.
	// If the tick channel is not set, an auto-ticker will be used that ticks every second.
	// TODO: this should be configurable via an environnment variable or config file.
	if c.tickChannel == nil {
		c.tickChannel = time.Tick(time.Second)
	}

	if c.config.CardinalRollupEnabled {
		err = c.syncLoop(ctx)
		if err != nil {
			return eris.Wrap(err, "failed to sync loop")
		}
	}

	go c.server.Serve(ctx)

	if c.startHook != nil {
		err := c.startHook()
		if err != nil {
			return eris.Wrap(err, "failed to run start hook")
		}
	}

	if !c.isReplica {
		err := c.tickLoop(ctx)
		if err != nil {
			return eris.Wrap(err, "failed to tick loop")
		}
	}

	return nil
}

func (c *Cardinal) syncLoop(ctx context.Context) error {
	syncChannel := make(chan tick.Proposal)

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		err := c.router.TransactionIterator().Each(func(batches []*iterator.TxBatch, tick, timestamp uint64) error {
			select {
			case <-ctx.Done():
				return nil
			default:
				log.Info().Msgf("Found transactions for tick %d", tick)

				var txMap types.TxMap
				for _, tx := range batches {
					msgType, ok := c.world.GetMessage(tx.MsgName)
					if !ok {
						return eris.New("failed to get message")
					}

					msg, err := msgType.Decode(tx.Tx.Body)
					if err != nil {
						return eris.Wrap(err, "failed to decode message")
					}

					txData := types.TxData{
						Tx:              tx.Tx,
						Msg:             msg,
						EVMSourceTxHash: nil, // TODO: this seems suspect.
					}
					txMap[tx.MsgName] = append(txMap[tx.MsgName], txData)
				}
				proposal := c.world.PrepareSyncTick(int64(tick), int64(timestamp), txMap)
				syncChannel <- proposal
			}
			return nil
		})
		if err != nil {
			return eris.Wrap(err, "encountered an error while syncing from chain")
		}

		// Once we finish syncing, syncChannel should be closed.
		// This signals to the syncLoop that it should exit.
		// TODO: Handle continuous syncing where there are new transactions batches coming in.
		close(syncChannel)

		return nil
	})

	eg.Go(func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case proposal, ok := <-syncChannel:
				// When we finish syncing, syncChannel will be closed.
				// If that's the case, we should exit the loop.
				if !ok {
					return nil
				}

				// Currently, since we do not post batches for ticks without transactions, we would need to fast forward
				// the tick if we encounter any gaps.
				// We want to tick forward until the last finalized tick is exactly one tick behind the tick we are
				// sychronizing to.
				if c.world.LastFinalizedTick() < proposal.ID-1 {
					// TODO: Non-deterministic behavior here. We need to know the historical timestamp to be able to
					//  do deterministic fast forwarding of the tick.
					ffProposal := c.world.PrepareSyncTick(c.world.LastFinalizedTick()+1,
						proposal.Timestamp, make(types.TxMap))

					err := c.nextTick(ctx, &ffProposal)
					if err != nil {
						return eris.Wrap(err, "failed to fast forward tick")
					}

					return nil
				}

				err := c.nextTick(ctx, &proposal)
				if err != nil {
					return eris.Wrap(err, "failed to apply tick")
				}
			}
		}
	})

	return eg.Wait()
}

func (c *Cardinal) tickLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-c.tickChannel:
			proposal := c.world.PrepareTick(c.world.CopyTransactions(ctx))
			err := c.nextTick(ctx, &proposal)
			if err != nil {
				return eris.Wrap(err, "failed to apply tick")
			}
		}
	}
}

// isSyncMode will return true if the world is not fully synchronized with the EVM shard.
// In a replica shard, this should always return true because we want to continuously listen for new transactions from
// the leader shard.
func (c *Cardinal) isSyncMode() bool {
	if c.isReplica {
		return true
	}
	// TODO: check whether we are the tip tick of the leader shard.

	return false
}

func (c *Cardinal) nextTick(ctx context.Context, proposal *tick.Proposal) error {
	ctx, span := c.tracer.Start(ddotel.ContextWithStartOptions(ctx, ddtracer.Measured()), "world.tick")
	defer span.End()

	startTime := time.Now()

	t, err := c.world.ApplyTick(ctx, proposal)
	if err != nil {
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return eris.Wrap(err, "failed to apply tick")
	}

	err = c.world.CommitTick(t)
	if err != nil {
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return eris.Wrap(err, "failed to commit tick")
	}

	if !c.isSyncMode() && !c.isReplica && c.config.CardinalRollupEnabled {
		// Broadcast tick
		err = c.server.BroadcastEvent(t)
		if err != nil {
			span.SetStatus(codes.Error, eris.ToString(err, true))
			span.RecordError(err)
			return eris.Wrap(err, "failed to broadcast tick")
		}

		// Submit tick to router
		err = c.router.SubmitTxBlob(ctx, t)
		if err != nil {
			span.SetStatus(codes.Error, eris.ToString(err, true))
			span.RecordError(err)
			return eris.Wrap(err, "failed to submit transactions to base shard")
		}
	}

	c.publishTick(t)

	log.Info().
		Int64("tick", t.ID).
		Dur("duration", time.Since(startTime)).
		Int("tx_count", len(t.Receipts)).
		Msg("Tick completed")

	return nil
}

func (c *Cardinal) Subscribe() <-chan *tick.Tick {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	r := make(chan *tick.Tick)

	c.subscribers = append(c.subscribers, r)

	return r
}

func (c *Cardinal) publishTick(t *tick.Tick) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return
	}

	for _, ch := range c.subscribers {
		ch <- t
	}
}

func (c *Cardinal) Stop() {
	c.cancel()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	for _, ch := range c.subscribers {
		close(ch)
	}
}

func (c *Cardinal) World() *world.World {
	return c.world
}
