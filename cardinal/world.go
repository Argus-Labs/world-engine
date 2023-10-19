package cardinal

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/component_metadata"
	"pkg.world.dev/world-engine/cardinal/ecs/ecb"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/cardinal/server"
)

type World struct {
	implWorld       *ecs.World
	server          *server.Handler
	gameManager     *server.GameManager
	isGameRunning   atomic.Bool
	tickChannel     <-chan time.Time
	tickDoneChannel chan<- uint64
	serverOptions   []server.Option
}

type (
	// EntityID represents a single entity in the World. An EntityID is tied to
	// one or more components.
	EntityID        = entity.ID
	TxHash          = transaction.TxHash
	ECSWorldContext = ecs.WorldContext

	// System is a function that process the transaction in the given transaction queue.
	// Systems are automatically called during a world tick, and they must be registered
	// with a world using AddSystem or AddSystems.
	System func(ECSWorldContext) error
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

	rs := storage.NewRedisStorage(storage.Options{
		Addr:     addr,
		Password: password, // make sure to set this in prod
		DB:       0,        // use default DB
	}, "world")
	worldStorage := storage.NewWorldStorage(&rs)
	storeManager, err := ecb.NewManager(rs.Client)
	if err != nil {
		return nil, err
	}
	ecsWorld, err := ecs.NewWorld(worldStorage, storeManager, ecsOptions...)
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
	txh, err := server.NewHandler(world.implWorld, nil, world.serverOptions...)
	if err != nil {
		return nil, err
	}
	world.server = txh
	return world, nil
}

// NewMockWorld creates a World that uses an in-memory redis DB as the storage layer.
// This is only suitable for local development.
func NewMockWorld(opts ...WorldOption) (*World, error) {
	ecsOptions, serverOptions, cardinalOptions := separateOptions(opts)
	world := &World{
		implWorld:     ecs.NewMockWorld(ecsOptions...),
		serverOptions: serverOptions,
	}
	world.isGameRunning.Store(false)
	for _, opt := range cardinalOptions {
		opt(world)
	}
	return world, nil
}

// CreateMany creates multiple entities in the world, and returns the slice of ids for the newly created
// entities. At least 1 component must be provided.
func CreateMany(wCtx ECSWorldContext, num int, components ...component_metadata.Component) ([]EntityID, error) {
	return component.CreateMany(wCtx, num, components...)
}

// Create creates a single entity in the world, and returns the id of the newly created entity.
// At least 1 component must be provided.
func Create(wCtx ECSWorldContext, components ...component_metadata.Component) (EntityID, error) {
	return component.Create(wCtx, components...)
}

// SetComponent Set sets component data to the entity.
func SetComponent[T component_metadata.Component](wCtx ECSWorldContext, id entity.ID, comp *T) error {
	return component.SetComponent[T](wCtx, id, comp)
}

// GetComponent Get returns component data from the entity.
func GetComponent[T component_metadata.Component](wCtx ECSWorldContext, id entity.ID) (comp *T, err error) {
	return component.GetComponent[T](wCtx, id)
}

// UpdateComponent Updates a component on an entity
func UpdateComponent[T component_metadata.Component](wCtx ECSWorldContext, id entity.ID, fn func(*T) *T) error {
	return component.UpdateComponent[T](wCtx, id, fn)
}

// AddComponentTo Adds a component on an entity
func AddComponentTo[T component_metadata.Component](wCtx ECSWorldContext, id entity.ID) error {
	return component.AddComponentTo[T](wCtx, id)
}

// RemoveComponentFrom Removes a component from an entity
func RemoveComponentFrom[T component_metadata.Component](wCtx ECSWorldContext, id entity.ID) error {
	return component.RemoveComponentFrom[T](wCtx, id)
}

// Remove removes the given entity id from the world.
func (w *World) Remove(id EntityID) error {
	return w.implWorld.Remove(id)
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
	if w.tickChannel == nil {
		w.tickChannel = time.Tick(time.Second)
	}
	w.implWorld.StartGameLoop(context.Background(), w.tickChannel, w.tickDoneChannel)
	txh, err := server.NewHandler(w.implWorld, nil, w.serverOptions...)
	if err != nil {
		return err
	}
	w.server = txh
	gameManager := server.NewGameManager(w.implWorld, w.server)
	w.gameManager = &gameManager
	go func() {
		w.isGameRunning.Store(true)
		if err := w.server.Serve(); err != nil {
			log.Fatal().Err(err)
		}
	}()
	select {}
}

func (w *World) IsGameRunning() bool {
	return w.isGameRunning.Load()
}

func (w *World) ShutDown() error {
	if !w.IsGameRunning() {
		return errors.New("game is not running")
	}
	err := w.gameManager.Shutdown()
	if err != nil {
		return err
	}
	w.isGameRunning.Store(false)
	return nil
}

func RegisterSystems(w *World, systems ...System) {
	for _, system := range systems {
		w.implWorld.AddSystem(func(wCtx ECSWorldContext) error {
			return system(wCtx)
		})
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

func (w *World) Init(fn func(ECSWorldContext)) {
	ecsWorldCtx := ecs.NewWorldContext(w.implWorld)
	fn(ecsWorldCtx)
}
