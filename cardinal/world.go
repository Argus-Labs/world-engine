package cardinal

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/component_metadata"
	"pkg.world.dev/world-engine/cardinal/ecs/ecb"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/cardinal/events"
	"pkg.world.dev/world-engine/cardinal/evm"
	"pkg.world.dev/world-engine/cardinal/server"
)

type World struct {
	implWorld       *ecs.World
	server          *server.Handler
	evmServer       evm.Server
	gameManager     *server.GameManager
	isGameRunning   atomic.Bool
	tickChannel     <-chan time.Time
	tickDoneChannel chan<- uint64
	serverOptions   []server.Option
	cleanup         func()
}

type (
	// EntityID represents a single entity in the World. An EntityID is tied to
	// one or more components.
	EntityID = entity.ID
	TxHash   = transaction.TxHash
	Receipt  = receipt.Receipt

	// System is a function that process the transaction in the given transaction queue.
	// Systems are automatically called during a world tick, and they must be registered
	// with a world using AddSystem or AddSystems.
	System func(WorldContext) error
)

// NewWorld creates a new World object using Redis as the storage layer.
func NewWorld(addr, password string, opts ...WorldOption) (*World, error) {
	ecsOptions, serverOptions, cardinalOptions := separateOptions(opts)
	log.Log().Msg("Running in normal mode, using external Redis")
	if addr == "" {
		return nil, errors.New("redis address is required")
	}
	if password == "" {
		log.Log().Msg("Redis password is not set, make sure to set up redis with password in prod")
	}

	redisStore := storage.NewRedisStorage(storage.Options{
		Addr:     addr,
		Password: password, // make sure to set this in prod
		DB:       0,        // use default DB
	}, "world")
	storeManager, err := ecb.NewManager(redisStore.Client)
	if err != nil {
		return nil, err
	}

	ecsWorld, err := ecs.NewWorld(&redisStore, storeManager, ecsOptions...)
	if err != nil {
		return nil, err
	}

	world := &World{
		implWorld:     ecsWorld,
		serverOptions: serverOptions,
	}
	world.isGameRunning.Store(false)
	for _, opt := range cardinalOptions {
		opt(world)
	}

	return world, nil
}

// NewMockWorld creates a World that uses an in-memory redis DB as the storage layer.
// This is only suitable for local development.
func NewMockWorld(opts ...WorldOption) (*World, error) {
	ecsOptions, serverOptions, cardinalOptions := separateOptions(opts)
	eventHub := events.CreateWebSocketEventHub()
	ecsOptions = append(ecsOptions, ecs.WithEventHub(eventHub))
	implWorld, mockWorldCleanup := ecs.NewMockWorld(ecsOptions...)
	world := &World{
		implWorld:     implWorld,
		serverOptions: serverOptions,
		cleanup:       mockWorldCleanup,
	}
	world.isGameRunning.Store(false)
	for _, opt := range cardinalOptions {
		opt(world)
	}
	return world, nil
}

// CreateMany creates multiple entities in the world, and returns the slice of ids for the newly created
// entities. At least 1 component must be provided.
func CreateMany(wCtx WorldContext, num int, components ...component_metadata.Component) ([]EntityID, error) {
	return component.CreateMany(wCtx.getECSWorldContext(), num, components...)
}

// Create creates a single entity in the world, and returns the id of the newly created entity.
// At least 1 component must be provided.
func Create(wCtx WorldContext, components ...component_metadata.Component) (EntityID, error) {
	return component.Create(wCtx.getECSWorldContext(), components...)
}

// SetComponent Set sets component data to the entity.
func SetComponent[T component_metadata.Component](wCtx WorldContext, id entity.ID, comp *T) error {
	return component.SetComponent[T](wCtx.getECSWorldContext(), id, comp)
}

// GetComponent Get returns component data from the entity.
func GetComponent[T component_metadata.Component](wCtx WorldContext, id entity.ID) (comp *T, err error) {
	return component.GetComponent[T](wCtx.getECSWorldContext(), id)
}

// UpdateComponent Updates a component on an entity
func UpdateComponent[T component_metadata.Component](wCtx WorldContext, id entity.ID, fn func(*T) *T) error {
	return component.UpdateComponent[T](wCtx.getECSWorldContext(), id, fn)
}

// AddComponentTo Adds a component on an entity
func AddComponentTo[T component_metadata.Component](wCtx WorldContext, id entity.ID) error {
	return component.AddComponentTo[T](wCtx.getECSWorldContext(), id)
}

// RemoveComponentFrom Removes a component from an entity
func RemoveComponentFrom[T component_metadata.Component](wCtx WorldContext, id entity.ID) error {
	return component.RemoveComponentFrom[T](wCtx.getECSWorldContext(), id)
}

// Remove removes the given entity id from the world.
func Remove(wCtx WorldContext, id EntityID) error {
	return wCtx.getECSWorldContext().GetWorld().Remove(id)
}

// StartGame starts running the world game loop. Each time a message arrives on the tickChannel, a world tick is attempted.
// In addition, an HTTP server (listening on the given port) is created so that game transactions can be sent
// to this world. After StartGame is called, RegisterComponent, RegisterTransactions, RegisterQueries, and AddSystem may
// not be called. If StartGame doesn't encounter any errors, it will block forever, running the server and ticking
// the game in the background.
func (w *World) StartGame() error {
	if w.IsGameRunning() {
		return errors.New("game already running")
	}

	if err := w.implWorld.LoadGameState(); err != nil {
		return err
	}
	eventHub := events.CreateWebSocketEventHub()
	w.implWorld.SetEventHub(eventHub)
	eventBuilder := events.CreateNewWebSocketBuilder("/events", events.CreateWebSocketEventHandler(eventHub))
	handler, err := server.NewHandler(w.implWorld, eventBuilder, w.serverOptions...)
	if err != nil {
		return err
	}
	w.server = handler

	w.evmServer, err = evm.NewServer(w.implWorld)
	if err != nil {
		if !errors.Is(err, evm.ErrNoEVMTypes) {
			return err
		}
		w.implWorld.Logger.Debug().Msg("no EVM transactions or queries specified. EVM server will not run")
	} else {
		w.implWorld.Logger.Debug().Msg("running world with EVM server")
		err = w.evmServer.Serve()
		if err != nil {
			return err
		}
	}

	if w.tickChannel == nil {
		w.tickChannel = time.Tick(time.Second)
	}
	w.implWorld.StartGameLoop(context.Background(), w.tickChannel, w.tickDoneChannel)
	gameManager := server.NewGameManager(w.implWorld, w.server)
	w.gameManager = &gameManager
	go func() {
		w.isGameRunning.Store(true)
		if err := w.server.Serve(); err != nil {
			log.Fatal().Err(err)
		}
	}()

	//handle shutdown via a signal
	signalChannel := make(chan os.Signal, 1)
	go func() {
		signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)
		for sig := range signalChannel {
			if sig == syscall.SIGINT || sig == syscall.SIGTERM {
				err := w.ShutDown()
				if err != nil {
					log.Err(err).Msgf("There was an error during shutdown.")
				}
				return
			}
		}
	}()
	select {}
}

func (w *World) IsGameRunning() bool {
	return w.isGameRunning.Load()
}

func (w *World) ShutDown() error {
	if w.cleanup != nil {
		w.cleanup()
	}
	if w.evmServer != nil {
		w.evmServer.Shutdown()
	}
	if w.IsGameRunning() {
		err := w.gameManager.Shutdown()
		if err != nil {
			return err
		}
		w.isGameRunning.Store(false)
	}
	return nil
}

func RegisterSystems(w *World, systems ...System) {
	for _, system := range systems {
		functionName := filepath.Base(runtime.FuncForPC(reflect.ValueOf(system).Pointer()).Name())
		sys := system
		w.implWorld.AddSystemWithName(func(wCtx ecs.WorldContext) error {
			return sys(&worldContext{
				implContext: wCtx,
			})
		}, functionName)
	}
}

func RegisterComponent[T component_metadata.Component](world *World) error {
	return ecs.RegisterComponent[T](world.implWorld)
}

// RegisterTransactions adds the given transactions to the game world. HTTP endpoints to queue up/execute these
// transaction will automatically be created when StartGame is called. This Register method must only be called once.
func RegisterTransactions(w *World, txs ...AnyTransaction) error {
	return w.implWorld.RegisterTransactions(toITransactionType(txs)...)
}

// RegisterQueries adds the given query capabilities to the game world. HTTP endpoints to use these queries
// will automatically be created when StartGame is called. This Register method must only be called once.
func RegisterQueries(w *World, queries ...AnyQueryType) error {
	return w.implWorld.RegisterQueries(toIQueryType(queries)...)
}

func (w *World) CurrentTick() uint64 {
	return w.implWorld.CurrentTick()
}

func (w *World) Tick(ctx context.Context) error {
	return w.implWorld.Tick(ctx)
}

func (w *World) Init(fn func(WorldContext)) {
	ecsWorldCtx := ecs.NewWorldContext(w.implWorld)
	fn(&worldContext{implContext: ecsWorldCtx})
}
