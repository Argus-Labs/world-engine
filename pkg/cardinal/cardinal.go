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

const (
	addressService = ":8080"
	addressDebug   = ":8081"
	addressPProf   = ":6060"
)

// World represents your game world and serves as the main entry point for Cardinal.
type World struct {
	world           *ecs.World            // The ECS world storing the game's state and systems
	commands        command.Manager       // Receives commands for systems
	events          event.Manager         // Collects and dispatches events
	address         *micro.ServiceAddress // This world's NATS address
	service         *service              // ConnectRPC direct client-facing service
	snapshotStorage snapshot.Storage      // Snapshot storage
	debug           *debugModule          // For debug only utils and services
	pprof           *pprofModule          // Optional pprof HTTP server
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

	// Create the ConnectRPC client-facing service.
	world.service = newService(world, options.AuthMode, options.ArgusAuthURL)

	// Register event handlers with the ConnectRPC service publishers.
	world.events.RegisterHandler(event.KindDefault, world.service.publishDefaultEvent)
	world.events.RegisterHandler(event.KindInterShardCommand, world.service.publishInterShardCommand)

	// Setup snapshot storage.
	switch options.SnapshotStorageType {
	case snapshot.StorageTypeJetStream:
		snapshotJS, err := snapshot.NewJetStreamStorage(snapshot.JetStreamStorageOptions{
			Logger:     tel.GetLogger("snapshot"),
			Address:    world.address,
			NATSConfig: options.NATSConfig,
		})
		if err != nil {
			return nil, eris.Wrap(err, "failed to create jetstream snapshot storage")
		}
		world.snapshotStorage = snapshotJS
	case snapshot.StorageTypeS3:
		snapshotS3, err := snapshot.NewS3Storage(snapshot.S3StorageOptions{
			Logger:  tel.GetLogger("snapshot"),
			Address: world.address,
		})
		if err != nil {
			return nil, eris.Wrap(err, "failed to create S3 snapshot storage")
		}
		world.snapshotStorage = snapshotS3
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
		world.debug = &debug
	}

	// Create the pprof module only if pprof is on.
	if *options.Pprof {
		world.pprof = newPprofModule(tel)
	}

	return world, nil
}

// StartGame launches your game and runs it until stopped.
func (w *World) StartGame() {
	// Freeze the command codec registry: all codecs register from generated init() (before now), so
	// any later RegisterCommandCodec is a bug. Sealing turns that into a clear panic instead of an
	// unrecoverable concurrent-map write against the tick loop's hot-path reads.
	command.Seal()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	defer w.shutdown()
	defer w.tel.RecoverAndFlush(true)

	// Observers bracket the lifecycle: telemetry is up from NewWorld; debug
	// and pprof come up here, BEFORE any producer (NATS, tick loop), so they
	// remain available during boot failures (e.g. NATS connect hangs/retries).
	// The mirror is in shutdown(): observers torn down LAST so they outlive
	// the producers' teardown — see the comment block there.
	w.debug.Init(addressDebug)
	w.pprof.Init(addressPProf)

	// Start the NATS connection and handler. Failures here panic; observers
	// above are already running, so a goroutine/stack profile is reachable
	// during the panic window via the deferred shutdown chain.
	// Start the ConnectRPC client-facing service.
	if err := w.service.init(addressService); err != nil {
		panic(eris.Wrap(err, "failed to initialize service"))
	}

	w.tel.CaptureEvent(ctx, "Start Game", nil)

	if err := w.run(ctx); err != nil {
		w.tel.CaptureException(ctx, err)
		w.tel.Logger.Error().Err(err).Msg("failed running world")
	}
}

func (w *World) run(ctx context.Context) error {
	// Initialize world and run init systems.
	w.world.Init()

	if err := w.restore(ctx); err != nil {
		return eris.Wrap(err, "failed to restore state from snapshot")
	}

	logger := w.tel.GetLogger("shard")
	logger.Info().Msg("starting core shard loop")

	ticker := time.NewTicker(time.Duration(float64(time.Second) / w.options.TickRate))
	defer ticker.Stop()

	for {
		if w.debug.isPaused() {
			select {
			case <-w.debug.resumeChan():
				w.debug.setPaused(false)
			case replyCh := <-w.debug.stepChan():
				w.Tick(ctx, time.Now())
				replyCh <- w.currentTick.height
			case replyCh := <-w.debug.resetChan():
				w.reset()
				replyCh <- struct{}{}
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}

		select {
		case <-ticker.C:
			w.Tick(ctx, time.Now())
		case replyCh := <-w.debug.pauseChan():
			w.debug.setPaused(true)
			replyCh <- w.currentTick.height
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (w *World) Tick(ctx context.Context, timestamp time.Time) {
	// TODO: commands returned to be used for debug epoch log.
	_ = w.commands.Drain()

	w.currentTick.timestamp = timestamp
	w.debug.startPerfTick()

	// Tick ECS world.
	w.world.Tick()

	w.debug.recordTick(w.currentTick.height, timestamp)

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
	w.tel.Logger.Debug().Msg("published snapshot")
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
	w.debug.resetPerf()

	return nil
}

// shutdown performs graceful cleanup of world resources, such as closing services
// and releasing any held resources. It is called automatically on shutdown.
func (w *World) shutdown() {
	// Create a timeout context for shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	w.tel.Logger.Info().Msg("Shutting down world")

	// Shutdown order matters: every step below shares the same 10s ctx budget.
	// We tear down producers (snapshot, NATS) BEFORE observers (debug, pprof)
	// so that an in-flight introspection call — e.g. a 30s CPU profile or a
	// pod-log tail — has a chance to drain during the NATS shutdown phase
	// instead of being severed on the first cleanup step. Telemetry goes last
	// so it can flush log lines emitted by every preceding step.

	// 1. Final snapshot. Producer-side, fixed 2s sub-budget.
	snapshotCtx, snapshotCancel := context.WithTimeout(ctx, 2*time.Second)
	w.snapshot(snapshotCtx, time.Now())
	snapshotCancel()

	// 2. Shard service (NATS) — drain queued commands/events. Typically quick,
	// but the producer side should stop before observers do.
	if err := w.service.shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("service shutdown error")
		w.tel.CaptureException(ctx, err)
	}

	// 3. Debug server — observer; cheap to drain because in-flight ConnectRPC
	// debug calls (Pause/Step/etc.) are short-lived. Doing this BEFORE pprof
	// gives the more-likely-long-running pprof captures the rest of the budget.
	if err := w.debug.Shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("debug server shutdown error")
		w.tel.CaptureException(ctx, err)
	}

	// 4. Pprof server — observer; in-flight captures (especially /profile and
	// /trace, both up to seconds=N) may legitimately take tens of seconds.
	// http.Server.Shutdown blocks until they complete or until the shared ctx
	// expires; whatever budget is left after steps 1-3 is what they get.
	if err := w.pprof.Shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("pprof server shutdown error")
		w.tel.CaptureException(ctx, err)
	}

	// 5. Telemetry last so log lines from steps 1-4 are flushed.
	if err := w.tel.Shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("telemetry shutdown error")
	}

	w.tel.Logger.Info().Msg("World shutdown complete")
}

func (w *World) reset() {
	// Reset ECS world and rerun the init systems.
	w.world.Reset()
	w.world.Init()

	// Clear command and event buffers from previous tick.
	w.commands.Clear()
	w.events.Clear()

	// Reset tick bookkeeping fields.
	w.currentTick.height = 0
	w.currentTick.timestamp = time.Time{}

	// Reset perf collector.
	w.debug.resetPerf()
}

type Tick struct {
	height    uint64
	timestamp time.Time
}
