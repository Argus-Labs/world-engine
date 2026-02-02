package cardinal

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/service"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/telemetry/posthog"
	"github.com/argus-labs/world-engine/pkg/telemetry/sentry"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"google.golang.org/grpc/codes"
)

// World represents your game world and serves as the main entry point for Cardinal.
type World struct {
	world   *ecs.World // The ECS world storing the game's state and systems
	service *service.ShardService

	commands command.Manager
	events   event.Manager

	debug *debugModule

	currentTick     Tick
	snapshotStorage snapshot.Storage

	options WorldOptions
	tel     telemetry.Telemetry // Telemetry for logging and tracing
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

	// Setup ECS ecsWorld.
	ecsWorld := ecs.NewWorld()

	world := &World{
		world:       ecsWorld,
		commands:    command.NewManager(),
		events:      event.NewManager(),
		currentTick: Tick{height: 0}, // timestamp will be set by cardinal.Tick
		options:     options,
		tel:         tel,
	}

	ecsWorld.OnComponentRegister(world.registerComponent)

	// Setup message bus.
	client, err := micro.NewClient(micro.WithLogger(tel.GetLogger("client")))
	if err != nil {
		return nil, eris.Wrap(err, "failed to initialize micro client")
	}
	address := micro.GetAddress(
		options.Region, micro.RealmWorld, options.Organization, options.Project, options.ShardID)
	service, err := service.NewShardService(service.ShardServiceOptions{
		Client:    client,
		Address:   address,
		World:     ecsWorld,
		Telemetry: &tel,
	})
	if err != nil {
		return nil, eris.Wrap(err, "failed to create micro service")
	}
	world.service = service

	// Register event handlers with the service's (NATS) publishers.
	world.events.RegisterHandler(event.KindDefault, service.PublishDefaultEvent)
	world.events.RegisterHandler(event.KindDefault, service.PublishInterShardCommand)

	// Setup snapshot storage.
	snapshotStorage, err := snapshot.NewJetStreamStorage(snapshot.JetStreamStorageOptions{
		Client:  client,
		Address: address,
	})
	if err != nil {
		return nil, eris.Wrap(err, "failed to create jetstream snapshot storage")
	}
	world.snapshotStorage = snapshotStorage

	if *options.Debug {
		debug := newDebugModule(world)
		world.debug = &debug
	}

	return world, nil
}

// TODO: initialize client and service here instead of in constructor
// StartGame launches your game and runs it until stopped.
func (w *World) StartGame() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	defer w.shutdown()
	defer w.tel.RecoverAndFlush(true)

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

	if err := w.restore(); err != nil {
		return eris.Wrap(err, "failed to restore state from snapshot")
	}

	logger := w.tel.GetLogger("shard")
	logger.Info().Msg("starting core shard loop")

	ticker := time.NewTicker(time.Duration(float64(time.Second) / w.options.TickRate))
	defer ticker.Stop()

	// TODO: select from debug channel to pause/play ticks.
	for {
		select {
		case <-ticker.C:
			if err := w.Tick(time.Now()); err != nil {
				return eris.Wrap(err, "failed to run tick")
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (w *World) Tick(timestamp time.Time) error {
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

	data, err := w.world.Serialize()
	if err != nil {
		return eris.Wrap(err, "failed to serialize world")
	}

	// Publish snapshot.
	if w.currentTick.height%uint64(w.options.SnapshotRate) == 0 {
		snapshot := &snapshot.Snapshot{
			TickHeight: w.currentTick.height,
			Timestamp:  timestamp,
			Data:       data,
		}
		if err := w.snapshotStorage.Store(snapshot); err != nil {
			return eris.Wrap(err, "failed to published snapshot")
		}
		w.tel.Logger.Info().Msg("published snapshot")
	}

	// Increment tick height.
	w.currentTick.height++

	return nil
}

func (w *World) restore() error {
	logger := w.tel.GetLogger("snapshot")

	if !w.snapshotStorage.Exists() {
		logger.Debug().Msg("no snapshot found")
		return nil
	}

	logger.Debug().Msg("restoring from snapshot")
	snapshot, err := w.snapshotStorage.Load()
	if err != nil {
		return eris.Wrap(err, "failed to load snapshot")
	}

	// Attempt to restore ECS world from snapshot.
	if err := w.world.Deserialize(snapshot.Data); err != nil {
		return eris.Wrap(err, "failed to restore world from snapshot")
	}

	// Only update shard state after successful restoration and validation.
	w.currentTick.height = snapshot.TickHeight + 1

	return nil
}

// shutdown performs graceful cleanup of world resources, such as closing services
// and releasing any held resources. It is called automatically on shutdown.
func (w *World) shutdown() {
	// Create a timeout context for shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	w.tel.Logger.Info().Msg("Shutting down world")

	// Shutdown debug server.
	if err := w.debug.Shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("debug server shutdown error")
		w.tel.CaptureException(ctx, err)
	}

	// Shutdown shard service.
	if w.service != nil {
		if err := w.service.Close(); err != nil {
			w.tel.Logger.Error().Err(err).Msg("message bus shutdown error")
			w.tel.CaptureException(ctx, err)
		}
	}

	// Shutdown telemetry.
	if err := w.tel.Shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("telemetry shutdown error")
	}

	w.tel.Logger.Info().Msg("World shutdown complete")
}

func (w *World) registerCommand(zero command.CommandPayload) error {
	// Register a handler with the service.
	return w.service.AddGroup("command").AddEndpoint(zero.Name(), func(ctx context.Context, req *micro.Request) *micro.Response {
		// Check if shard is shutting down.
		select {
		case <-ctx.Done():
			return micro.NewErrorResponse(req, eris.Wrap(ctx.Err(), "context cancelled"), codes.Canceled)
		default:
			// Continue processing.
		}

		command := &iscv1.Command{}
		if err := req.Payload.UnmarshalTo(command); err != nil {
			return micro.NewErrorResponse(req, eris.Wrap(err, "failed to parse request payload"), codes.InvalidArgument)
		}

		if err := protovalidate.Validate(command); err != nil {
			return micro.NewErrorResponse(req, eris.Wrap(err, "failed to validate command"), codes.InvalidArgument)
		}

		if micro.String(w.service.Address) != micro.String(command.GetAddress()) {
			return micro.NewErrorResponse(req, eris.New("command address doesn't match shard address"), codes.InvalidArgument)
		}

		if err := w.commands.Enqueue(command); err != nil {
			return micro.NewErrorResponse(req, eris.Wrap(err, "failed to enqueue command"), codes.InvalidArgument)
		}

		return micro.NewSuccessResponse(req, nil)
	})
}

func (w *World) registerComponent(zero ecs.Component) error {
	return w.debug.register("component", zero)
}

type Tick struct {
	height    uint64
	timestamp time.Time
}
