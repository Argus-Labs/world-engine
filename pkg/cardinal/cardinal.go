package cardinal

import (
	"context"
	"os/signal"
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
	"github.com/rotisserie/eris"
)

const (
	addressService = ":8080"
	addressPProf   = ":6060"
)

// World contains the game state and is the main Cardinal API.
type World struct {
	world           *ecs.World            // ECS state and systems
	commands        command.Manager       // Commands for systems
	events          event.Manager         // Events and event handlers
	address         *micro.ServiceAddress // NATS address
	service         *service              // ConnectRPC client service
	snapshotStorage snapshot.Storage      // Snapshot reader
	snapshotWriter  snapshot.Writer       // Snapshot writer
	debug           *debugModule          // Debug tools and services
	pprof           *pprofModule          // Optional pprof HTTP server
	currentTick     Tick                  // Current tick
	options         WorldOptions          // World options
	tel             telemetry.Telemetry   // Logs and traces
}

// NewWorld creates a game world with the specified options.
func NewWorld(opts WorldOptions) (*World, error) {
	// Read and validate options.
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

	// Initialize telemetry.
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
		events:   event.NewManager(1024),
		address: micro.GetAddress(
			options.Region, micro.RealmWorld, options.Organization, options.Project, options.ShardID),
		currentTick: Tick{height: 0},
		options:     options,
		tel:         tel,
	}

	// Register components for introspection.
	world.world.OnComponentRegister(func(zero ecs.Component) error {
		return world.debug.register(introspect.Component, zero)
	})

	// Create the ConnectRPC client service.
	world.service = newService(world, options.AuthMode, options.ArgusAuthURL)

	// Connect event handlers to service publishers.
	world.events.RegisterHandler(event.KindDefault, world.service.publishDefaultEvent)
	world.events.RegisterHandler(event.KindInterShardCommand, world.service.publishInterShardCommand)

	// Initialize snapshot storage.
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

	// Write snapshots synchronously until StartGame starts the asynchronous writer.
	world.snapshotWriter = snapshot.NewSyncWriter(world.snapshotStorage, tel.GetLogger("snapshot"))

	// Create the optional debug module.
	if *options.Debug {
		world.debug = newDebugModule(world)
	}

	// Create the optional pprof module.
	if *options.Pprof {
		world.pprof = newPprofModule(tel)
	}

	return world, nil
}

// StartGame runs the game until the process receives a stop signal.
func (w *World) StartGame() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Move snapshot writes off the tick goroutine.
	w.snapshotWriter.Stop(context.Background())
	w.snapshotWriter = snapshot.NewAsyncWriter(w.snapshotStorage, &w.tel)

	defer w.shutdown()
	defer w.tel.RecoverAndFlush(true)

	// Start pprof before the service to support profiles during startup failures.
	w.pprof.Init(addressPProf)

	// Start the NATS connection and ConnectRPC service.
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
	// Initialize the world and run initialization systems.
	w.world.Init()

	if err := w.restore(ctx); err != nil {
		return eris.Wrap(err, "failed to restore state from snapshot")
	}
	// Final snapshot.
	defer func() {
		w.snapshotWriter.Write(w.currentTick.height, w.encodeSnapshot(time.Now()))
	}()

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
				w.Tick(time.Now())
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
			w.Tick(time.Now())
		case replyCh := <-w.debug.pauseChan():
			w.debug.setPaused(true)
			replyCh <- w.currentTick.height
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Tick advances the world by one step.
func (w *World) Tick(timestamp time.Time) {
	_ = w.commands.Drain()

	w.currentTick.timestamp = timestamp
	w.debug.startPerfTick()

	// Advance the ECS world.
	w.world.Tick()

	w.debug.recordTick(w.currentTick.height, timestamp)

	// Send events.
	if err := w.events.Dispatch(); err != nil {
		w.tel.Logger.Warn().Err(err).Msg("errors encountered dispatching events")
	}

	// Publish state for snapshots and debugging.
	w.persistState(timestamp)

	// Increase the tick height.
	w.currentTick.height++
}

// persistState serializes the world for snapshots and the debug service. Encoding cannot fail
// (a component that cannot marshal asserts inside the ECS), so there is no retry path.
func (w *World) persistState(timestamp time.Time) {
	snapshotDue := w.currentTick.height%uint64(w.options.SnapshotRate) == 0
	if !snapshotDue && w.debug == nil {
		return
	}

	data := w.encodeSnapshot(timestamp)

	// Publish state only when the debug service is enabled. The decode allocates, so it happens
	// only on this dev path — and before Write, which takes ownership of the bytes.
	w.debug.publishState(data)

	if snapshotDue {
		w.snapshotWriter.Write(w.currentTick.height, data)
	}
}

// encodeSnapshot produces the complete snapshot bytes for the current tick: the ECS sizes and
// streams its world state directly into one exactly-sized buffer, and the envelope is hand-encoded
// around it. No intermediate proto graph exists; the buffer is the freeze-frame.
func (w *World) encodeSnapshot(timestamp time.Time) []byte {
	bodySize := w.world.StateWireSize()
	return snapshot.Encode(w.currentTick.height, timestamp, bodySize, w.world.AppendStateWire)
}

func (w *World) restore(ctx context.Context) error {
	logger := w.tel.GetLogger("snapshot")

	logger.Debug().Msg("restoring from snapshot")
	data, err := w.snapshotStorage.Load(ctx)
	if err != nil {
		if eris.Is(err, snapshot.ErrSnapshotNotFound) {
			logger.Debug().Msg("no snapshot found")
			return nil
		}
		return eris.Wrap(err, "failed to load snapshot")
	}

	// Decode validates the bytes and refuses versions this build cannot read.
	snap, err := snapshot.Decode(data)
	if err != nil {
		return eris.Wrap(err, "refusing to restore snapshot")
	}

	if err := w.world.FromProto(snap.GetWorldState()); err != nil {
		return eris.Wrap(err, "failed to restore world from snapshot")
	}

	// Update the tick only after a successful restore.
	w.currentTick.height = snap.GetTickHeight() + 1

	// Publish the restored state for GetState; a no-op when the debug service is disabled.
	w.debug.publishSnapshot(snap)
	return nil
}

// shutdown writes the final state and stops world services.
func (w *World) shutdown() {
	// Give all shutdown steps one shared timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	w.tel.Logger.Info().Msg("Shutting down world")

	// Finish all snapshot writes before the service stops.
	if err := w.snapshotWriter.Drain(ctx); err != nil {
		w.tel.Logger.Error().Err(err).
			Msg("snapshot writes did not finish before shutdown; the last snapshot of this run may be lost")
	}
	w.snapshotWriter.Stop(ctx)

	// Drain queued commands and events.
	if err := w.service.shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("service shutdown error")
		w.tel.CaptureException(ctx, err)
	}

	// Stop pprof after the service so active profiles have more time to finish.
	if err := w.pprof.Shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("pprof server shutdown error")
		w.tel.CaptureException(ctx, err)
	}

	// Stop telemetry last so it can send all shutdown logs.
	if err := w.tel.Shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("telemetry shutdown error")
	}

	w.tel.Logger.Info().Msg("World shutdown complete")
}

func (w *World) reset() {
	// Reset the ECS world and run initialization systems again.
	w.world.Reset()
	w.world.Init()

	// Clear pending commands and events.
	w.commands.Clear()
	w.events.Clear()

	// Reset the tick.
	w.currentTick.height = 0
	w.currentTick.timestamp = time.Time{}

	// Publish the reset state when the debug service is enabled.
	if w.debug != nil {
		w.debug.publishState(w.encodeSnapshot(w.currentTick.timestamp))
	}
	w.debug.resetPerf()
}

type Tick struct {
	height    uint64
	timestamp time.Time
}
