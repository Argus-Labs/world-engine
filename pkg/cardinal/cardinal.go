package cardinal

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/telemetry/posthog"
	"github.com/argus-labs/world-engine/pkg/telemetry/sentry"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
)

// World represents your game world and serves as the main entry point for Cardinal.
type World struct {
	world           *ecs.World            // The ECS world storing the game's state and systems
	commands        command.Manager       // Receives commands for systems
	events          event.Manager         // Collects and dispatches events
	address         *micro.ServiceAddress // This world's NATS address
	service         *service              // micro.Service wrapper
	snapshotStorage snapshot.Storage      // Snapshot storage
	debug           *debugModule          // For debug only utils and services
	currentTick     Tick                  // The current tick
	options         WorldOptions          // Options
	tel             telemetry.Telemetry   // Telemetry for logging and tracing
}

// NewWorld creates a new game world with the specified configuration.
func NewWorld(opts WorldOptions) (*World, error) {
	// Load and validate options.
	envs, err := loadWorldOptionsEnv()
	if err != nil {
		return nil, eris.Wrap(err, "failed to load world options env vars")
	}
	options := newDefaultWorldOptions()
	options.apply(envs.toOptions())
	options.apply(opts)
	if err := options.validate(); err != nil {
		return nil, eris.Wrap(err, "invalid world options")
	}

	// Setup telemetry.
	tel, err := telemetry.New(telemetry.Options{
		ServiceName: "cardinal",
		SentryOptions: sentry.Options{
			Tags: options.getSentryTags(),
		},
		PosthogOptions: posthog.Options{
			DistinctID:     options.Organization,
			BaseProperties: options.getPosthogBaseProperties(),
		},
	})
	if err != nil {
		return nil, eris.Wrap(err, "failed to initialize telemetry")
	}
	defer tel.RecoverAndFlush(true)

	world := &World{
		world:    ecs.NewWorld(),
		commands: command.NewManager(),
		events:   event.NewManager(1024), // Default event channel capacity
		address: micro.GetAddress(
			options.Region, micro.RealmWorld, options.Organization, options.Project, options.ShardID),
		currentTick: Tick{height: 0}, // timestamp will be set by cardinal.Tick
		options:     options,
		tel:         tel,
	}

	// Set ECS on componet register callback (used for introspect).
	world.world.OnComponentRegister(func(zero ecs.Component) error {
		return world.debug.register("component", zero)
	})

	// Create the service.
	service := newService(world)
	world.service = service

	// Register event handlers with the service's (NATS) publishers.
	world.events.RegisterHandler(event.KindDefault, service.publishDefaultEvent)
	world.events.RegisterHandler(event.KindInterShardCommand, service.publishInterShardCommand)

	// Setup snapshot storage.
	switch options.SnapshotStorageType {
	case snapshot.StorageTypeJetStream:
		snapshotJS, err := snapshot.NewJetStreamStorage(snapshot.JetStreamStorageOptions{
			Logger:  tel.GetLogger("snapshot"),
			Address: world.address,
		})
		if err != nil {
			return nil, eris.Wrap(err, "failed to create jetstream snapshot storage")
		}
		world.snapshotStorage = snapshotJS
	case snapshot.StorageTypeNop:
		world.snapshotStorage = snapshot.NewNopStorage()
	case snapshot.StorageTypeUndefined:
		fallthrough
	default:
		panic("unreachable")
	}

	// Create the debug module only if debug is on.
	if *options.Debug {
		debug := newDebugModule(world)
		debug.control.isPaused.Store(true)
		world.debug = &debug
	}

	return world, nil
}

// StartGame launches your game and runs it until stopped.
func (w *World) StartGame() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	defer w.shutdown()
	defer w.tel.RecoverAndFlush(true)

	// Start the NATS connection and handler.
	if err := w.service.init(); err != nil {
		panic(eris.Wrap(err, "failed to initialize service"))
	}

	// Start the debug server.
	w.debug.Init(":8080")

	w.tel.CaptureEvent(ctx, "Start Game", nil)

	if err := w.run(ctx); err != nil {
		w.tel.CaptureException(ctx, err)
		w.tel.Logger.Error().Err(err).Msg("failed running world")
	}
}

func (w *World) run(ctx context.Context) error {
	// Initialize world schedulers.
	w.world.Init()

	if err := w.restore(ctx); err != nil {
		return eris.Wrap(err, "failed to restore state from snapshot")
	}

	logger := w.tel.GetLogger("shard")
	logger.Info().Msg("starting core shard loop")

	ticker := time.NewTicker(time.Duration(float64(time.Second) / w.options.TickRate))
	defer ticker.Stop()

	for {
		if w.debug != nil && w.debug.control.isPaused.Load() {
			select {
			case <-w.debug.control.resumeCh:
				w.debug.control.isPaused.Store(false)
			case replyCh := <-w.debug.control.stepCh:
				if err := w.Tick(ctx, time.Now()); err != nil {
					replyCh <- 0
					return eris.Wrap(err, "failed to run tick during step")
				}
				replyCh <- w.currentTick.height
			case replyCh := <-w.debug.control.resetCh:
				w.reset()
				replyCh <- struct{}{}
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}

		select {
		case <-ticker.C:
			if err := w.Tick(ctx, time.Now()); err != nil {
				return eris.Wrap(err, "failed to run tick")
			}
		case replyCh := <-w.debug.control.pauseCh:
			w.debug.control.isPaused.Store(true)
			replyCh <- w.currentTick.height
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (w *World) Tick(ctx context.Context, timestamp time.Time) error {
	// TODO: commands returned to be used for debug epoch log.
	_ = w.commands.Drain()

	w.currentTick.timestamp = timestamp

	// Tick ECS world.
	err := w.world.Tick()
	if err != nil {
		return eris.Wrap(err, "one or more systems failed")
	}

	// Emit events.
	if err := w.events.Dispatch(); err != nil {
		w.tel.Logger.Warn().Err(err).Msg("errors encountered dispatching events")
	}

	// Publish snapshot.
	if w.currentTick.height%uint64(w.options.SnapshotRate) == 0 {
		snapshotCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		w.snapshot(snapshotCtx, timestamp)
		cancel()
	}

	// Increment tick height.
	w.currentTick.height++

	return nil
}

// snapshot persists the world state as a best-effort operation. Snapshots are best effort only, and
// we just log errors instead of returning it, which would cause the world to stop and restart,
// effectively losing unsaved state. If a snapshot fails, the main loop still continues and we retry
// in the next snapshot call.
func (w *World) snapshot(ctx context.Context, timestamp time.Time) {
	worldState, err := w.world.ToProto()
	if err != nil {
		w.tel.Logger.Warn().Err(err).Msg("failed to serialize world for snapshot")
		return
	}
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(worldState)
	if err != nil {
		w.tel.Logger.Warn().Err(err).Msg("failed to marshal world state to bytes")
		return
	}
	snap := &snapshot.Snapshot{
		TickHeight: w.currentTick.height,
		Timestamp:  timestamp,
		Data:       data,
		Version:    snapshot.CurrentVersion,
	}
	if err := w.snapshotStorage.Store(ctx, snap); err != nil {
		w.tel.Logger.Warn().Err(err).Msg("failed to store snapshot")
		return
	}
	w.tel.Logger.Info().Msg("published snapshot")
}

func (w *World) restore(ctx context.Context) error {
	logger := w.tel.GetLogger("snapshot")

	logger.Debug().Msg("restoring from snapshot")
	snap, err := w.snapshotStorage.Load(ctx)
	if err != nil {
		if eris.Is(err, snapshot.ErrSnapshotNotFound) {
			logger.Debug().Msg("no snapshot found")
			return nil
		}
		return eris.Wrap(err, "failed to load snapshot")
	}

	// Unmarshal snapshot bytes into proto and restore ECS world.
	var worldState cardinalv1.WorldState
	if err := proto.Unmarshal(snap.Data, &worldState); err != nil {
		return eris.Wrap(err, "failed to unmarshal snapshot data")
	}
	if err := w.world.FromProto(&worldState); err != nil {
		return eris.Wrap(err, "failed to restore world from snapshot")
	}

	// Only update shard state after successful restoration and validation.
	w.currentTick.height = snap.TickHeight + 1

	return nil
}

// shutdown performs graceful cleanup of world resources, such as closing services
// and releasing any held resources. It is called automatically on shutdown.
func (w *World) shutdown() {
	// Create a timeout context for shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	w.tel.Logger.Info().Msg("Shutting down world")

	snapshotCtx, snapshotCancel := context.WithTimeout(ctx, 2*time.Second)
	w.snapshot(snapshotCtx, time.Now())
	snapshotCancel()

	// Shutdown debug server.
	if err := w.debug.Shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("debug server shutdown error")
		w.tel.CaptureException(ctx, err)
	}

	// Shutdown shard service.
	if err := w.service.shutdown(); err != nil {
		w.tel.Logger.Error().Err(err).Msg("service shutdown error")
		w.tel.CaptureException(ctx, err)
	}

	// Shutdown telemetry.
	if err := w.tel.Shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("telemetry shutdown error")
	}

	w.tel.Logger.Info().Msg("World shutdown complete")
}

func (w *World) reset() {
	w.world.Reset()
	w.commands.Clear()
	w.events.Clear()
	w.currentTick.height = 0
	w.currentTick.timestamp = time.Time{}
}

type Tick struct {
	height    uint64
	timestamp time.Time
}
