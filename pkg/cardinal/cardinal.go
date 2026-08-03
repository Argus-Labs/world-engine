package cardinal

import (
	"context"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/introspect"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/telemetry/posthog"
	"github.com/argus-labs/world-engine/pkg/telemetry/sentry"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	addressService = ":8080"
	addressPProf   = ":6060"
)

// World represents your game world and serves as the main entry point for Cardinal.
type World struct {
	world           *ecs.World            // The ECS world storing the game's state and systems
	commands        command.Manager       // Receives commands for systems
	events          event.Manager         // Collects and dispatches events
	address         *micro.ServiceAddress // This world's NATS address
	service         *service              // ConnectRPC direct client-facing service
	snapshotStorage snapshot.Storage      // Snapshot storage; the read side of the snapshot path
	// The write side. Inline by default, so a hand-ticked world owns no background goroutine.
	// StartGame — the only caller with a guaranteed shutdown to stop it — swaps in the async writer,
	// which takes the upload off the tick goroutine; see asyncSnapshotWriter for the
	// single-flight/latest-wins rule and the durability trade that buys.
	snapshotWriter snapshotWriter
	// Latest world state; swap only, never mutate. Its only reader is DebugService.GetState,
	// whose handler is mounted only when debug is on (see service.init), so every publish is
	// gated on debug != nil: with debug off the publish would feed a consumer that cannot exist
	// while pinning a full deep-copied world-state graph (~1 MB serialized at 5000 entities) in
	// the heap until the next publish replaces it. NewWorld seeds it either way, so GetState is
	// always servable.
	state atomic.Pointer[cardinalv1.Snapshot]
	// True once restore() has established the in-memory world as the authoritative copy of the
	// persisted state — which includes the legitimate "no snapshot exists yet" case, where the
	// fresh world IS the state. Until then an empty world means nothing: the snapshot may be
	// unread (corrupt, unreadable version, storage error), so persisting the empty world would
	// destroy the good one. Written and read only on the goroutine running StartGame.
	stateAuthoritative bool
	debug              *debugModule        // For debug only utils and services
	pprof              *pprofModule        // Optional pprof HTTP server
	currentTick        Tick                // The current tick
	options            WorldOptions        // Options
	tel                telemetry.Telemetry // Telemetry for logging and tracing
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

	// Seed a valid empty state so GetState is always servable, even before the first tick.
	// This one is unconditional: it is a single empty envelope, and the debug module does not
	// exist yet at this point.
	world.state.Store(&cardinalv1.Snapshot{WorldState: &cardinalv1.WorldState{}})

	// Set ECS on component register callback (used for introspection).
	world.world.OnComponentRegister(func(zero ecs.Component) error {
		return world.debug.register(introspect.Component, zero)
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

	// Snapshots are written inline until something takes responsibility for stopping a background
	// writer. StartGame does, and swaps in the async one; a world ticked by hand never has a
	// goroutine to stop. See World.useAsyncSnapshotWriter.
	world.snapshotWriter = newInlineSnapshotWriter(world.snapshotStorage, tel.GetLogger("snapshot"))

	// Create the debug module only if debug is on.
	if *options.Debug {
		world.debug = newDebugModule(world)
	}

	// Create the pprof module only if pprof is on.
	if *options.Pprof {
		world.pprof = newPprofModule(tel)
	}

	return world, nil
}

// StartGame launches your game and runs it until stopped.
func (w *World) StartGame() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	defer w.shutdown()
	defer w.tel.RecoverAndFlush(true)

	// Take snapshot uploads off the tick goroutine. This is the one place it may happen: the
	// deferred shutdown above is what stops the writer's goroutine again, and no other way of
	// running a World has one. See World.useAsyncSnapshotWriter.
	w.useAsyncSnapshotWriter()

	// pprof comes up before any producer (NATS, tick loop) so a profile stays
	// reachable during boot hangs. DebugService is no longer started here — it
	// mounts on the service port in w.service.init (dev-only), below.
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

// Tick advances the world by one step.
//
// ctx is unused since snapshot uploads stopped running here: the writer they go to outlives any one
// tick and carries its own context (see asyncSnapshotWriter), so nothing on this path takes a
// deadline from the caller any more. The parameter stays because Tick is the public step API, called
// by the shard loop, by the debug step control and by every game's tests — and because a tick is
// exactly the scope a future trace span would want.
func (w *World) Tick(ctx context.Context, timestamp time.Time) {
	_ = ctx

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

	// Publish state to snapshot and debug module.
	w.persistState(timestamp)

	// Increment tick height.
	w.currentTick.height++
}

// persistState serializes world state once, hands it to storage when a snapshot is due, and
// publishes it to w.state for the debug module when there is one.
// Best effort: we just log errors instead of returning them, which would cause the
// world to stop and restart, effectively losing unsaved state. If a state serialization
// fails, the main loop still continues and we retry in the next persistState call.
func (w *World) persistState(timestamp time.Time) {
	snapshotDue := w.currentTick.height%uint64(w.options.SnapshotRate) == 0
	if !snapshotDue && w.debug == nil {
		return
	}

	worldState, err := w.world.ToProto()
	if err != nil {
		w.tel.Logger.Warn().Err(err).Msg("failed to serialize the world's state")
		return
	}
	// Debug-only consumer, and the envelope is not even built when debug is off (see World.state).
	if w.debug != nil {
		w.state.Store(&cardinalv1.Snapshot{
			TickHeight: w.currentTick.height,
			Timestamp:  timestamppb.New(timestamp),
			WorldState: worldState,
		})
	}

	if snapshotDue {
		w.snapshot(timestamp, worldState)
	}
}

// snapshot wraps an already-built world state in an envelope and hands it to the snapshot writer.
//
// It does not touch storage: marshaling and the network write happen on the writer's goroutine, so
// the only cost a snapshot tick pays here is building the envelope. What stays on the tick goroutine
// is ToProto, in persistState above — it reads live ECS and is only correct while the world is
// quiescent. Everything downstream of it operates on a graph made of fresh slices that nothing
// mutates afterwards, which is what makes handing it to another goroutine safe.
//
// Best-effort, unchanged: nothing is returned because there is nothing the tick loop could do with a
// storage failure but log it and lose unsaved state by stopping. The writer logs; the next snapshot
// retries.
func (w *World) snapshot(timestamp time.Time, worldState *cardinalv1.WorldState) {
	w.snapshotWriter.write(&cardinalv1.Snapshot{
		TickHeight: w.currentTick.height,
		Timestamp:  timestamppb.New(timestamp),
		WorldState: worldState,
		Version:    snapshot.CurrentVersion,
	})
}

// finalSnapshot writes the last snapshot of a run, during shutdown. It only hands the snapshot to
// the writer; shutdown waits for it to land by calling drainSnapshotWrites straight after.
//
// It is skipped unless restore() established the in-memory world as the authoritative copy of the
// persisted state (see World.stateAuthoritative). Shutdown runs on every exit path, including the
// one taken when the restore itself failed — a corrupt or unreadable snapshot, a version this build
// refuses, a storage error. On that path the world is empty only because it was never loaded, and
// storing it would overwrite the good snapshot with nothing, turning a recoverable read failure
// into total state loss. Losing the ticks since the last snapshot is the lesser harm, so we log
// loudly instead and leave what is stored alone.
func (w *World) finalSnapshot() {
	if !w.stateAuthoritative {
		w.tel.Logger.Warn().Msg("skipping final snapshot: world state was never restored")
		return
	}

	worldState, err := w.world.ToProto()
	if err != nil {
		w.tel.Logger.Warn().Err(err).Msg("failed to serialize world for final snapshot")
		return
	}
	w.snapshot(time.Now(), worldState)
}

func (w *World) restore(ctx context.Context) error {
	logger := w.tel.GetLogger("snapshot")

	logger.Debug().Msg("restoring from snapshot")
	snap, err := w.snapshotStorage.Load(ctx)
	if err != nil {
		if eris.Is(err, snapshot.ErrSnapshotNotFound) {
			// Nothing persisted yet: the fresh world is the state, so it is authoritative and the
			// final snapshot is the first one this shard ever writes.
			logger.Debug().Msg("no snapshot found")
			w.stateAuthoritative = true
			return nil
		}
		return eris.Wrap(err, "failed to load snapshot")
	}

	// Refuse a snapshot this build cannot interpret. Checked again here, after the backends check
	// their own bytes, because Storage is an interface: an implementation that never decodes bytes
	// (the in-memory test storage, for instance) has nothing to check them against. Failing the
	// restore stops the world, which is the point — starting from a mis-read world would overwrite
	// the good snapshot with a wrong one on the next snapshot tick.
	if err := snapshot.ValidateVersion(snap.GetVersion()); err != nil {
		return eris.Wrap(err, "refusing to restore snapshot")
	}

	// Restore the ECS world from the loaded proto; storage already unmarshaled it.
	// A stored envelope without a world state is degenerate, but the published state must never
	// be nil, so substitute an empty one.
	worldState := snap.GetWorldState()
	if worldState == nil {
		worldState = &cardinalv1.WorldState{}
	}
	if err := w.world.FromProto(worldState); err != nil {
		return eris.Wrap(err, "failed to restore world from snapshot")
	}

	// Only update shard state after successful restoration and validation.
	w.currentTick.height = snap.GetTickHeight() + 1

	// The world now holds the persisted state, so it may be written back — including from a
	// panicking run, which is exactly the crash-recovery case a final snapshot exists for.
	w.stateAuthoritative = true

	// Publish the loaded proto as-is; it already is the restored state. Debug-only consumer, and
	// with debug off this would pin the restored graph for the whole life of the process, since
	// persistState never replaces it (see World.state).
	if w.debug != nil {
		w.state.Store(&cardinalv1.Snapshot{
			TickHeight: snap.GetTickHeight(),
			Timestamp:  snap.GetTimestamp(),
			WorldState: worldState,
		})
	}
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

	// 1. Final snapshot. Producer-side; serialize world state for snapshot, then wait for it and
	// anything the writer still owes to reach storage. The wait belongs here, ahead of NATS
	// teardown, for the same reason the snapshot itself does: past this point the shard is coming
	// apart, and an upload nobody waits for is an upload that does not happen.
	w.finalSnapshot()
	w.drainSnapshotWrites(ctx)

	// 2. Shard service (NATS) — drain queued commands/events. Typically quick,
	// but the producer side should stop before observers do.
	if err := w.service.shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("service shutdown error")
		w.tel.CaptureException(ctx, err)
	}

	// 3. Pprof server — observer; in-flight /profile and /trace captures (up to
	// seconds=N) can take tens of seconds, so it drains last on the leftover
	// budget. (DebugService has no server to drain — it rode the service server,
	// step 2.)
	if err := w.pprof.Shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("pprof server shutdown error")
		w.tel.CaptureException(ctx, err)
	}

	// 4. Telemetry last so log lines from steps 1-3 are flushed.
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

	// Republish state so it doesn't describe the pre-reset world, and clear perf data. Debug-only
	// consumer (see World.state), so with debug off the serialization is skipped outright.
	if w.debug != nil {
		if worldState, err := w.world.ToProto(); err != nil {
			w.tel.Logger.Warn().Err(err).Msg("failed to serialize the world's state")
		} else {
			w.state.Store(&cardinalv1.Snapshot{
				TickHeight: w.currentTick.height,
				Timestamp:  timestamppb.New(w.currentTick.timestamp),
				WorldState: worldState,
			})
		}
	}
	w.debug.resetPerf()
}

type Tick struct {
	height    uint64
	timestamp time.Time
}
