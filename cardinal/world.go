package cardinal

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"time"

	"pkg.world.dev/world-engine/cardinal/shard/adapter"
	"pkg.world.dev/world-engine/cardinal/shard/evm"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/gamestate"
	"pkg.world.dev/world-engine/cardinal/ecs/iterators"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
	"pkg.world.dev/world-engine/cardinal/ecs/storage/redis"
	"pkg.world.dev/world-engine/cardinal/gamestage"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/cardinal/statsd"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
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

type World struct {
	instance        *ecs.Engine
	server          *server.Server
	evmServer       evm.Server
	tickChannel     <-chan time.Time
	tickDoneChannel chan<- uint64
	serverOptions   []server.Option
	cleanup         func()

	// gameSequenceStage describes what stage the game is in (e.g. starting, running, shut down, etc)
	gameSequenceStage gamestage.Atomic
	endStartGame      chan bool
}

type (
	// EntityID represents a single entity in the World. An EntityID is tied to
	// one or more components.
	EntityID = entity.ID
	TxHash   = message.TxHash
	Receipt  = receipt.Receipt

	// System is a function that process the transaction in the given transaction queue.
	// Systems are automatically called during a world tick, and they must be registered
	// with a world using RegisterSystems.
	System func(WorldContext) error
)

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
	if cfg.CardinalMode == RunModeProd {
		a, err := adapter.New(adapter.Config{
			ShardSequencerAddr: cfg.BaseShardSequencerAddress,
			EVMBaseShardAddr:   cfg.BaseShardQueryAddress,
		})
		if err != nil {
			return nil, eris.Wrapf(err, "failed to instantiate adapter")
		}
		ecsOptions = append(ecsOptions, ecs.WithAdapter(a))
	} else {
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

	ecsWorld, err := ecs.NewEngine(
		&redisStore,
		entityCommandBuffer,
		ecs.Namespace(cfg.CardinalNamespace),
		ecsOptions...,
	)
	if err != nil {
		return nil, err
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
		instance:          ecsWorld,
		serverOptions:     serverOptions,
		endStartGame:      make(chan bool),
		gameSequenceStage: gamestage.NewAtomic(),
	}

	// Apply options
	for _, opt := range cardinalOptions {
		opt(world)
	}

	return world, nil
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

// NewMockWorld creates a World object that uses miniredis as the storage layer suitable for local development.
// If you are creating a World for unit tests, use NewTestWorld.
func NewMockWorld(opts ...WorldOption) (*World, error) {
	world, err := NewWorld(append(opts, withMockRedis())...)
	if err != nil {
		return world, err
	}
	return world, nil
}

// CreateMany creates multiple entities in the world, and returns the slice of ids for the newly created
// entities. At least 1 component must be provided.
func CreateMany(wCtx WorldContext, num int, components ...component.Component) ([]EntityID, error) {
	ids, err := ecs.CreateMany(wCtx.Engine(), num, components...)
	if wCtx.Engine().IsReadOnly() || err == nil {
		return ids, err
	}
	return nil, logAndPanic(wCtx, err)
}

// Create creates a single entity in the world, and returns the id of the newly created entity.
// At least 1 component must be provided.
func Create(wCtx WorldContext, components ...component.Component) (EntityID, error) {
	id, err := ecs.Create(wCtx.Engine(), components...)
	if wCtx.Engine().IsReadOnly() || err == nil {
		return id, err
	}
	return 0, logAndPanic(wCtx, err)
}

// SetComponent Set sets component data to the entity.
func SetComponent[T component.Component](wCtx WorldContext, id entity.ID, comp *T) error {
	err := ecs.SetComponent[T](wCtx.Engine(), id, comp)
	if wCtx.Engine().IsReadOnly() || err == nil {
		return err
	}
	if eris.Is(err, ErrEntityDoesNotExist) ||
		eris.Is(err, ErrComponentNotOnEntity) {
		return err
	}
	return logAndPanic(wCtx, err)
}

// GetComponent Get returns component data from the entity.
func GetComponent[T component.Component](wCtx WorldContext, id entity.ID) (*T, error) {
	result, err := ecs.GetComponent[T](wCtx.Engine(), id)
	_ = result
	if wCtx.Engine().IsReadOnly() || err == nil {
		return result, err
	}
	if eris.Is(err, ErrEntityDoesNotExist) ||
		eris.Is(err, ErrComponentNotOnEntity) {
		return nil, err
	}

	return nil, logAndPanic(wCtx, err)
}

// UpdateComponent Updates a component on an entity.
func UpdateComponent[T component.Component](wCtx WorldContext, id entity.ID, fn func(*T) *T) error {
	err := ecs.UpdateComponent[T](wCtx.Engine(), id, fn)
	if wCtx.Engine().IsReadOnly() || err == nil {
		return err
	}
	if eris.Is(err, ErrEntityDoesNotExist) ||
		eris.Is(err, ErrComponentNotOnEntity) {
		return err
	}

	return logAndPanic(wCtx, err)
}

// AddComponentTo Adds a component on an entity.
func AddComponentTo[T component.Component](wCtx WorldContext, id entity.ID) error {
	err := ecs.AddComponentTo[T](wCtx.Engine(), id)
	if wCtx.Engine().IsReadOnly() || err == nil {
		return err
	}
	if eris.Is(err, ErrEntityDoesNotExist) ||
		eris.Is(err, ErrComponentAlreadyOnEntity) {
		return err
	}

	return logAndPanic(wCtx, err)
}

// RemoveComponentFrom Removes a component from an entity.
func RemoveComponentFrom[T component.Component](wCtx WorldContext, id entity.ID) error {
	err := ecs.RemoveComponentFrom[T](wCtx.Engine(), id)
	if wCtx.Engine().IsReadOnly() || err == nil {
		return err
	}
	if eris.Is(err, ErrEntityDoesNotExist) ||
		eris.Is(err, ErrComponentNotOnEntity) ||
		eris.Is(err, ErrEntityMustHaveAtLeastOneComponent) {
		return err
	}
	return logAndPanic(wCtx, err)
}

// Remove removes the given entity id from the world.
func Remove(wCtx WorldContext, id EntityID) error {
	return wCtx.Engine().GetEngine().Remove(id)
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

	if err := w.instance.LoadGameState(); err != nil {
		if errors.Is(err, ecs.ErrEntitiesCreatedBeforeLoadingGameState) {
			return eris.Wrap(ErrEntitiesCreatedBeforeStartGame, "")
		}
		return err
	}
	srvr, err := server.New(w.instance, w.instance.GetEventHub().NewWebSocketEventHandler(), w.serverOptions...)
	if err != nil {
		return err
	}
	w.server = srvr

	w.evmServer, err = evm.NewServer(w.instance)
	if err != nil {
		if !errors.Is(eris.Cause(err), evm.ErrNoEVMTypes) {
			return err
		}
		w.instance.Logger.Debug().
			Msgf("no EVM messages or queries specified. EVM server will not run: %s", eris.ToString(err, true))
	} else {
		w.instance.Logger.Debug().Msg("running world with EVM server")
		err = w.evmServer.Serve()
		if err != nil {
			return err
		}
	}

	if w.tickChannel == nil {
		w.tickChannel = time.Tick(time.Second) //nolint:staticcheck // its ok.
	}
	w.instance.StartGameLoop(context.Background(), w.tickChannel, w.tickDoneChannel)
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
	if w.evmServer != nil {
		w.evmServer.Shutdown()
	}
	if w.server != nil {
		if err := w.server.Shutdown(); err != nil {
			return err
		}
	}
	close(w.endStartGame)
	return w.Engine().Shutdown()
}

func RegisterSystems(w *World, systems ...System) error {
	for _, system := range systems {
		functionName := filepath.Base(runtime.FuncForPC(reflect.ValueOf(system).Pointer()).Name())
		sys := system
		w.instance.RegisterSystemWithName(
			func(eCtx ecs.EngineContext) error {
				return sys(
					&worldContext{
						engine: eCtx,
					},
				)
			}, functionName,
		)
	}
	return nil
}

func RegisterComponent[T component.Component](world *World) error {
	return ecs.RegisterComponent[T](world.instance)
}

// RegisterMessages adds the given messages to the game world. HTTP endpoints to queue up/execute these
// messages will automatically be created when StartGame is called. This Register method must only be called once.
func RegisterMessages(w *World, msgs ...AnyMessage) error {
	return w.instance.RegisterMessages(toMessageType(msgs)...)
}

// RegisterQuery adds the given query to the game world. HTTP endpoints to use these queries
// will automatically be created when StartGame is called. This function does not add EVM support to the query.
func RegisterQuery[Request any, Reply any](
	world *World,
	name string,
	handler func(wCtx WorldContext, req *Request) (*Reply, error),
) error {
	err := ecs.RegisterQuery[Request, Reply](
		world.instance,
		name,
		func(wCtx ecs.EngineContext, req *Request) (*Reply, error) {
			return handler(&worldContext{engine: wCtx}, req)
		},
	)
	if err != nil {
		return err
	}
	return nil
}

// RegisterQueryWithEVMSupport adds the given query to the game world. HTTP endpoints to use these queries
// will automatically be created when StartGame is called. This Register method must only be called once.
// This function also adds EVM support to the query.
func RegisterQueryWithEVMSupport[Request any, Reply any](
	world *World,
	name string,
	handler func(wCtx WorldContext, req *Request) (*Reply, error),
) error {
	err := ecs.RegisterQuery[Request, Reply](
		world.instance,
		name,
		func(eCtx ecs.EngineContext, req *Request) (*Reply, error) {
			return handler(&worldContext{engine: eCtx}, req)
		},
		ecs.WithQueryEVMSupport[Request, Reply](),
	)
	if err != nil {
		return err
	}
	return nil
}

func (w *World) Engine() *ecs.Engine {
	return w.instance
}

func (w *World) CurrentTick() uint64 {
	return w.instance.CurrentTick()
}

func (w *World) Tick(ctx context.Context) error {
	return w.instance.Tick(ctx)
}

// Init Registers a system that only runs once on a new game before tick 0.
func (w *World) Init(system System) {
	w.instance.AddInitSystem(
		func(eCtx ecs.EngineContext) error {
			return system(&worldContext{engine: eCtx})
		},
	)
}

// logAndPanic logs the given error and panics. An error is returned so the syntax:
// return logAndPanic(wCtx, err)
// can be used at the end of state-mutating methods. This method will never actually return.
func logAndPanic(wCtx WorldContext, err error) error {
	wCtx.Logger().Panic().Err(err).Msgf("fatal error: %v", eris.ToString(err, true))
	return err
}
