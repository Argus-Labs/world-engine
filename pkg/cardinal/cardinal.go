package cardinal

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/epoch"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
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

	tickHeight      uint64       // Tick height
	epochHeight     uint64       // Epoch height
	ticks           []epoch.Tick // List of ticks in the current epoch
	epochLog        epoch.Log
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
		tickHeight:  0,
		epochHeight: 0,
		ticks:       make([]epoch.Tick, 0, options.EpochFrequency),
		options:     options,
		tel:         tel,
	}

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

	// Setup epoch log.
	epochLog, err := epoch.NewJetStreamLog(epoch.JetStreamLogOptions{
		Client:    client,
		Address:   address,
		Telemetry: &tel,
	})
	if err != nil {
		return nil, eris.Wrap(err, "failed to create jetstream epoch log")
	}
	world.epochLog = epochLog

	// Setup snapshot storage.
	snapshotStorage, err := snapshot.NewJetStreamStorage(snapshot.JetStreamStorageOptions{
		Client:  client,
		Address: address,
	})
	if err != nil {
		return nil, eris.Wrap(err, "failed to create jetstream snapshot storage")
	}
	world.snapshotStorage = snapshotStorage

	return world, nil
}

// TODO: initialize client and service here instead of in constructor
// StartGame launches your game and runs it until stopped.
func (w *World) StartGame() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	defer w.shutdown()
	defer w.tel.RecoverAndFlush(true)

	w.tel.CaptureEvent(ctx, "Start Game", nil)

	if err := w.run(ctx); err != nil {
		w.tel.CaptureException(ctx, err)
		w.tel.Logger.Error().Err(err).Msg("failed running world")
	}
}

func (w *World) run(ctx context.Context) error {
	// Initialize world schedulers.
	w.world.Init()

	if err := w.sync(); err != nil {
		return eris.Wrap(err, "failed to sync shard state")
	}

	// Core shard loop based on the mode.
	logger := w.tel.GetLogger("shard")
	logger.Info().Str("mode", w.options.Mode.String()).Msg("starting core shard loop")
	switch w.options.Mode {
	case ModeLeader:
		return w.runLeader(ctx)
	case ModeFollower:
		return w.runFollower(ctx)
	default:
		assert.That(true, "unreachable")
	}

	return nil
}

func (w *World) currentTick() (epoch.Tick, error) {
	if len(w.ticks) == 0 {
		return epoch.Tick{}, eris.New("cannot get current tick during inter-tick period")
	}
	return w.ticks[len(w.ticks)-1], nil
}

// shutdown performs graceful cleanup of world resources, such as closing services
// and releasing any held resources. It is called automatically on shutdown.
func (w *World) shutdown() {
	// Create a timeout context for shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	w.tel.Logger.Info().Msg("Shutting down world")

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

func (w *World) registerCommand(name string) error {
	if w.options.Mode != ModeLeader {
		return nil
	}
	return w.service.AddGroup("command").AddEndpoint(name, func(ctx context.Context, req *micro.Request) *micro.Response {
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
