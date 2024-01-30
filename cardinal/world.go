package cardinal

import (
	"context"
	"errors"
	"github.com/rs/zerolog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"pkg.world.dev/world-engine/cardinal/router"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/gamestate"
	"pkg.world.dev/world-engine/cardinal/ecs/iterators"
	"pkg.world.dev/world-engine/cardinal/ecs/storage/redis"
	"pkg.world.dev/world-engine/cardinal/gamestage"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/cardinal/statsd"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/message"
)

var (
	ErrEntitiesCreatedBeforeStartGame = errors.New("entities should not be created before start game")

	ErrEntityDoesNotExist                = iterators.ErrEntityDoesNotExist
	ErrEntityMustHaveAtLeastOneComponent = iterators.ErrEntityMustHaveAtLeastOneComponent
	ErrComponentNotOnEntity              = iterators.ErrComponentNotOnEntity
	ErrComponentAlreadyOnEntity          = iterators.ErrComponentAlreadyOnEntity
	ErrComponentNotRegistered            = iterators.ErrMustRegisterComponent
)

type (
	// EntityID represents a single entity in the World. An EntityID is tied to
	// one or more components.
	EntityID = entity.ID
	TxHash   = message.TxHash
	Receipt  = receipt.Receipt
)

type World struct {
	engine          *ecs.Engine
	server          *server.Server
	tickChannel     <-chan time.Time
	tickDoneChannel chan<- uint64
	serverOptions   []server.Option
	cleanup         func()

	// gameSequenceStage describes what stage the game is in (e.g. starting, running, shut down, etc)
	gameSequenceStage gamestage.Atomic
	endStartGame      chan bool
}

// NewWorld creates a new World object using Redis as the storage layer.
func NewWorld(opts ...WorldOption) (*World, error) {
	ecsOptions, serverOptions, cardinalOptions := separateOptions(opts)

	// Load config. Fallback value is used if it's not set.
	cfg := getWorldConfig()
	if err := cfg.Validate(); err != nil {
		return nil, eris.Wrapf(err, "invalid configuration")
	}

	if err := setLogLevel(cfg.CardinalLogLevel); err != nil {
		return nil, eris.Wrap(err, "")
	}

	log.Logger.Info().Msgf("Starting a new Cardinal world in %s mode", cfg.CardinalMode)
	if cfg.CardinalMode == RunModeDev {
		ecsOptions = append(ecsOptions, ecs.WithPrettyLog())
		serverOptions = append(serverOptions, server.WithPrettyPrint())
	}
	redisStore := redis.NewRedisStorage(redis.Options{
		Addr:     cfg.RedisAddress,
		Password: cfg.RedisPassword,
		DB:       0, // use default DB
	}, cfg.CardinalNamespace)
	entityCommandBuffer, err := gamestate.NewEntityCommandBuffer(redisStore.Client)
	if err != nil {
		return nil, err
	}

	eng, err := ecs.NewEngine(
		&redisStore,
		entityCommandBuffer,
		ecs.Namespace(cfg.CardinalNamespace),
		ecsOptions...,
	)
	if err != nil {
		return nil, err
	}

	if cfg.CardinalMode == RunModeProd {
		rtr, err := router.New(cfg.BaseShardSequencerAddress, cfg.BaseShardQueryAddress, eng)
		if err != nil {
			return nil, err
		}
		eng.SetRouter(rtr)
	}

	var metricTags []string
	if cfg.CardinalMode != "" {
		metricTags = append(metricTags, string("cardinal_mode:"+cfg.CardinalMode))
	}
	if cfg.CardinalNamespace != "" {
		metricTags = append(metricTags, "cardinal_namespace:"+cfg.CardinalNamespace)
	}

	if cfg.StatsdAddress != "" || cfg.TraceAddress != "" {
		if err = statsd.Init(cfg.StatsdAddress, cfg.TraceAddress, metricTags); err != nil {
			return nil, eris.Wrap(err, "unable to init statsd")
		}
	} else {
		log.Logger.Warn().Msg("statsd is disabled")
	}

	world := &World{
		engine:            eng,
		serverOptions:     serverOptions,
		endStartGame:      make(chan bool),
		gameSequenceStage: gamestage.NewAtomic(),
	}

	// Apply options
	for _, opt := range cardinalOptions {
		opt(world)
	}

	// Register Persona plugin
	personaPlugin := persona.NewInternalPlugin()
	err = personaPlugin.Register(eng)
	if err != nil {
		return nil, err
	}

	return world, nil
}

// NewMockWorld creates a World object that uses miniredis as the storage layer suitable for local development.
// If you are creating a World for unit tests, use NewTestWorld.
func NewMockWorld(opts ...WorldOption) (*World, error) {
	world, err := NewWorld(append(opts, withMockRedis())...)
	if err != nil {
		return world, err
	}
	return world, nil
}

func (w *World) Engine() *ecs.Engine {
	return w.engine
}

func (w *World) CurrentTick() uint64 {
	return w.engine.CurrentTick()
}

func (w *World) Tick(ctx context.Context) error {
	return w.engine.Tick(ctx)
}

// Init Registers a system that only runs once on a new game before tick 0.
func (w *World) Init(system systems.System) {
	w.engine.AddInitSystem(system)
}

// StartGame starts running the world game loop. Each time a message arrives on the tickChannel, a world tick is
// attempted. In addition, an HTTP server (listening on the given port) is created so that game messages can be sent
// to this world. After StartGame is called, RegisterComponent, RegisterMessages, RegisterQueries, and RegisterSystems
// may not be called. If StartGame doesn't encounter any errors, it will block forever, running the server and ticking
// the game in the background.
func (w *World) StartGame() error {
	ok := w.gameSequenceStage.CompareAndSwap(gamestage.StagePreStart, gamestage.StageStarting)
	if !ok {
		return errors.New("game has already been started")
	}

	if err := w.engine.LoadGameState(); err != nil {
		if errors.Is(err, ecs.ErrEntitiesCreatedBeforeLoadingGameState) {
			return eris.Wrap(ErrEntitiesCreatedBeforeStartGame, "")
		}
		return err
	}

	var err error
	w.server, err = server.New(w.instance, w.instance.GetEventHub().NewWebSocketEventHandler(), w.serverOptions...)
	if err != nil {
		return err
	}

	if err := w.instance.RunRouter(); err != nil {
		return eris.Wrap(err, "failed to start router service")
	}

	if w.tickChannel == nil {
		w.tickChannel = time.Tick(time.Second) //nolint:staticcheck // its ok.
	}
	w.engine.StartGameLoop(context.Background(), w.tickChannel, w.tickDoneChannel)
	go func() {
		ok := w.gameSequenceStage.CompareAndSwap(gamestage.StageStarting, gamestage.StageRunning)
		if !ok {
			log.Fatal().Msg("game was started prematurely")
		}
		if err := w.server.Serve(); errors.Is(err, http.ErrServerClosed) {
			log.Info().Err(err).Msgf("the server has been closed: %s", eris.ToString(err, true))
		} else if err != nil {
			log.Fatal().Err(err).Msgf("the server has failed: %s", eris.ToString(err, true))
		}
	}()

	// handle shutdown via a signal
	w.handleShutdown()
	<-w.endStartGame
	return err
}

func (w *World) IsGameRunning() bool {
	return w.gameSequenceStage.Load() == gamestage.StageRunning
}

func (w *World) Shutdown() error {
	if w.cleanup != nil {
		w.cleanup()
	}
	ok := w.gameSequenceStage.CompareAndSwap(gamestage.StageRunning, gamestage.StageShuttingDown)
	if !ok {
		// Either the world hasn't been started, or we've already shut down.
		return nil
	}
	// The CompareAndSwap returned true, so this call is responsible for actually
	// shutting down the game.
	defer func() {
		w.gameSequenceStage.Store(gamestage.StageShutDown)
	}()
	if w.server != nil {
		if err := w.server.Shutdown(); err != nil {
			return err
		}
	}
	close(w.endStartGame)
	return w.Engine().Shutdown()
}

func RegisterSystems(w *World, sys ...systems.System) error {
	return w.engine.RegisterSystems(sys...)
}

func RegisterComponent[T component.Component](world *World) error {
	return ecs.RegisterComponent[T](world.engine)
}

// RegisterMessages adds the given messages to the game world. HTTP endpoints to queue up/execute these
// messages will automatically be created when StartGame is called. This Register method must only be called once.
func RegisterMessages(w *World, msgs ...message.Message) error {
	return w.engine.RegisterMessages(msgs...)
}

// RegisterQuery adds the given query to the game world. HTTP endpoints to use these queries
// will automatically be created when StartGame is called. This function does not add EVM support to the query.
func RegisterQuery[Request any, Reply any](
	world *World,
	name string,
	handler func(eCtx engine.Context, req *Request) (*Reply, error),
	opts ...ecs.QueryOption[Request, Reply],
) error {
	return ecs.RegisterQuery[Request, Reply](world.Engine(), name, handler, opts...)
}

// RegisterQueryWithEVMSupport adds the given query to the game world. HTTP endpoints to use these queries
// will automatically be created when StartGame is called. This Register method must only be called once.
// This function also adds EVM support to the query.
func RegisterQueryWithEVMSupport[Request any, Reply any](
	world *World,
	name string,
	handler func(eCtx engine.Context, req *Request) (*Reply, error),
) error {
	return ecs.RegisterQuery[Request, Reply](world.Engine(), name, handler, ecs.WithQueryEVMSupport[Request, Reply]())
}

// logAndPanic logs the given error and panics. An error is returned so the syntax:
// return logAndPanic(eCtx, err)
// can be used at the end of state-mutating methods. This method will never actually return.
func logAndPanic(eCtx engine.Context, err error) error {
	eCtx.Logger().Panic().Err(err).Msgf("fatal error: %v", eris.ToString(err, true))
	return err
}

func setLogLevel(levelStr string) error {
	if levelStr == "" {
		return eris.New("log level must not be empty")
	}
	level, err := zerolog.ParseLevel(levelStr)
	if err != nil {
		var exampleLogLevels = strings.Join([]string{
			zerolog.DebugLevel.String(),
			zerolog.InfoLevel.String(),
			zerolog.WarnLevel.String(),
			zerolog.ErrorLevel.String(),
			zerolog.Disabled.String(),
		}, ", ")
		return eris.Errorf("log level %q is invalid, try one of: %v.", levelStr, exampleLogLevels)
	}
	zerolog.SetGlobalLevel(level)
	return nil
}

func applyProductionOptions(
	cfg WorldConfig,
	ecsOptions *[]ecs.Option,
) error {
	log.Logger.Info().Msg("Starting a new Cardinal world in production mode")
	if cfg.RedisPassword == "" {
		return eris.New("REDIS_PASSWORD is required in production")
	}
	if cfg.CardinalNamespace == DefaultNamespace {
		return eris.New(
			"CARDINAL_NAMESPACE cannot be the default value in production to avoid replay attack",
		)
	}
	if cfg.BaseShardSequencerAddress == "" || cfg.BaseShardQueryAddress == "" {
		return eris.New("must supply BASE_SHARD_SEQUENCER_ADDRESS and BASE_SHARD_QUERY_ADDRESS for production " +
			"mode Cardinal worlds")
	}
	adpt, err := adapter.New(adapter.Config{
		ShardSequencerAddr: cfg.BaseShardSequencerAddress,
		EVMBaseShardAddr:   cfg.BaseShardQueryAddress,
	})
	if err != nil {
		return eris.Wrapf(err, "failed to instantiate adapter")
	}
	*ecsOptions = append(*ecsOptions, ecs.WithAdapter(adpt))
	return nil
}

func (w *World) handleShutdown() {
	signalChannel := make(chan os.Signal, 1)
	go func() {
		signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)
		for sig := range signalChannel {
			if sig == syscall.SIGINT || sig == syscall.SIGTERM {
				err := w.Shutdown()
				if err != nil {
					log.Err(err).Msgf("There was an error during shutdown.")
				}
				return
			}
		}
	}()
}
