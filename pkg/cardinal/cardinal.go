package cardinal

import (
	"context"
	"crypto/sha256"
	"os/signal"
	"syscall"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/service"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/telemetry/posthog"
	"github.com/argus-labs/world-engine/pkg/telemetry/sentry"
	"github.com/rotisserie/eris"
)

// World represents your game world and serves as the main entry point for Cardinal.
type World struct {
	*micro.Shard                     // Embedded base shard functionalities
	tel          telemetry.Telemetry // Telemetry for logging and tracing
}

// NewWorld creates a new game world with the specified configuration.
func NewWorld(opts WorldOptions) (*World, error) {
	config, err := loadWorldConfig()
	if err != nil {
		return nil, eris.Wrap(err, "failed to load world config")
	}

	options := newDefaultWorldOptions()
	config.applyToOptions(&options)
	options.apply(opts)
	if err := options.validate(); err != nil {
		return nil, eris.Wrap(err, "invalid world options")
	}

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

	client, err := micro.NewClient(micro.WithLogger(tel.GetLogger("client")))
	if err != nil {
		return nil, eris.Wrap(err, "failed to initialize micro client")
	}

	address := micro.GetAddress(options.Region, micro.RealmWorld, options.Organization, options.Project, options.ShardID)

	cardinal := newCardinal()

	cardinalShard, err := micro.NewShard(cardinal, micro.ShardOptions{
		Client:                 client,
		Address:                address,
		EpochFrequency:         options.EpochFrequency,
		TickRate:               options.TickRate,
		Telemetry:              &tel,
		SnapshotStorageType:    options.SnapshotStorageType,
		SnapshotStorageOptions: options.SnapshotStorageOptions,
	})
	if err != nil {
		return nil, eris.Wrap(err, "failed to initialize shard")
	}

	// Initialize service only if we're in leader mode.
	if cardinalShard.Mode() == micro.ModeLeader {
		err := cardinal.initService(client, address, options.PrivateKey, &tel, cardinalShard.IsDisablePersona())
		if err != nil {
			return nil, eris.Wrap(err, "failed to initialize cardinal service")
		}
	}

	return &World{
		Shard: cardinalShard,
		tel:   tel,
	}, nil
}

// StartGame launches your game and runs it until stopped.
func (w *World) StartGame() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	defer w.shutdown()
	defer w.tel.RecoverAndFlush(true)

	w.tel.CaptureEvent(ctx, "Start Game", nil)

	if err := w.Run(ctx); err != nil {
		w.tel.CaptureException(ctx, err)
		w.tel.Logger.Error().Err(err).Msg("failed running world")
	}
}

func (w *World) getWorld() *ecs.World {
	base, ok := w.Base().(*cardinal)
	assert.That(ok, "cardinal shard didn't embed cardinal")

	return base.world
}

// shutdown performs graceful cleanup of world resources, such as closing services
// and releasing any held resources. It is called automatically on shutdown.
func (w *World) shutdown() {
	// Create a timeout context for shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	w.tel.Logger.Info().Msg("Shutting down world")

	base, ok := w.Base().(*cardinal)
	assert.That(ok, "cardinal shard didn't embed cardinal")

	if err := base.shutdown(); err != nil {
		w.tel.Logger.Error().Err(err).Msg("cardinal shutdown error")
		w.tel.CaptureException(ctx, err)
	}

	// Shutdown telemetry.
	if err := w.tel.Shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("telemetry shutdown error")
	}

	w.tel.Logger.Info().Msg("World shutdown complete")
}

// -------------------------------------------------------------------------------------------------
// Cardinal shard implementation
// -------------------------------------------------------------------------------------------------

// cardinal implements the methods for the micro.ShardEngine interface.
type cardinal struct {
	world   *ecs.World            // The ECS world storing the game's state and systems
	service *service.ShardService // Microservice for handling network communication

	// Snapshot caching for performance optimization. This is here so we don't have to reserialize
	// the world state when it hasn't changed.
	cachedSnapshot []byte // Cached serialized state
	isDirty        bool   // True if state has changed since last snapshot
}

var _ micro.ShardEngine = &cardinal{}

// newCardinal creates a new cardinal instance.
func newCardinal() *cardinal {
	return &cardinal{
		world:          ecs.NewWorld(),
		service:        nil, // Will be initialized only in leader mode
		cachedSnapshot: nil,
		isDirty:        true, // Start as dirty to force initial snapshot generation
	}
}

func (c *cardinal) Init() error {
	c.world.InitSchedulers()
	if err := c.world.InitSystems(); err != nil {
		return eris.Wrap(err, "failed to run init systems")
	}
	return nil
}

func (c *cardinal) StateHash() ([]byte, error) {
	snapshot, err := c.Snapshot()
	if err != nil {
		return nil, eris.Wrap(err, "failed to create snapshot for state hash")
	}

	hash := sha256.Sum256(snapshot)
	return hash[:], nil
}

func (c *cardinal) Tick(tick micro.Tick) error {
	events, err := c.world.Tick(tick.Data.Commands)
	if err != nil {
		return eris.Wrap(err, "one or more systems failed")
	}

	c.invalidateCache() // Mark state as dirty since it has changed

	// Publish events only if systems completed successfully and service is initialized.
	if c.service != nil {
		c.service.Publish(events)
	}

	return nil
}

func (c *cardinal) Replay(tick micro.Tick) error {
	_, err := c.world.Tick(tick.Data.Commands)
	if err != nil {
		return eris.Wrap(err, "one or more systems failed")
	}

	c.invalidateCache() // Mark state as dirty since it has changed
	return nil
}

func (c *cardinal) Snapshot() ([]byte, error) {
	if !c.isDirty && c.cachedSnapshot != nil {
		return c.cachedSnapshot, nil
	}

	data, err := c.world.Serialize()
	if err != nil {
		return nil, eris.Wrap(err, "failed to serialize world state")
	}

	// Cache the snapshot and mark as clean.
	c.cachedSnapshot = data
	c.isDirty = false
	return data, nil
}

func (c *cardinal) Restore(data []byte) error {
	if err := c.world.Deserialize(data); err != nil {
		return eris.Wrap(err, "failed to restore world state")
	}

	// Re-initialize schedulers since we don't call Init to do it for us.
	c.world.InitSchedulers()

	c.invalidateCache() // Mark state as dirty since it has changed
	return nil
}

func (c *cardinal) Reset() {
	c.world = ecs.NewWorld()
	c.invalidateCache() // Mark state as dirty since it has changed
}

func (c *cardinal) shutdown() error {
	if c.service != nil {
		if err := c.service.Close(); err != nil {
			return eris.Wrap(err, "failed to close service")
		}
	}
	return nil
}

// initService initializes the cardinal service for leader mode.
func (c *cardinal) initService(
	client *micro.Client,
	address *micro.ServiceAddress,
	privateKey string,
	tel *telemetry.Telemetry,
	disablePersona bool,
) error {
	microService, err := service.NewShardService(service.ShardServiceOptions{
		Client:         client,
		Address:        address,
		World:          c.world,
		Telemetry:      tel,
		PrivateKey:     privateKey,
		DisablePersona: disablePersona,
	})
	if err != nil {
		return eris.Wrap(err, "failed to create micro service")
	}
	c.service = microService
	return nil
}

// invalidateCache invalidates the snapshot cache and marks the state as dirty.
func (c *cardinal) invalidateCache() {
	c.cachedSnapshot = nil
	c.isDirty = true
}
