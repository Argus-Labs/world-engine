package cardinal

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/inmem"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
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
	EntityID = entity.ID
	TxHash   = transaction.TxHash

	// System is a function that process the transaction in the given transaction queue.
	// Systems are automatically called during a world tick, and they must be registered
	// with a world using AddSystem or AddSystems.
	System func(*World, *TransactionQueue, *Logger) error
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
	ecsWorld, err := ecs.NewWorld(worldStorage, ecsOptions...)
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
	txh, err := server.NewHandler(world.implWorld, world.serverOptions...)
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
		implWorld:     inmem.NewECSWorld(ecsOptions...),
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
func (w *World) CreateMany(num int, components ...AnyComponentType) ([]EntityID, error) {
	return w.implWorld.CreateMany(num, toIComponentType(components)...)
}

// Create creates a single entity in the world, and returns the id of the newly created entity.
// At least 1 component must be provided.
func (w *World) Create(components ...AnyComponentType) (EntityID, error) {
	return w.implWorld.Create(toIComponentType(components)...)
}

// Remove removes the given entity id from the world.
func (w *World) Remove(id EntityID) error {
	return w.implWorld.Remove(id)
}

// StartGame starts running the world game loop. Each time a message arrives on the tickChannel, a world tick is attempted.
// In addition, an HTTP server (listening on the given port) is created so that game transactions can be sent
// to this world. After StartGame is called, RegisterComponents, RegisterTransactions, RegisterReads, and AddSystem may
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
	txh, err := server.NewHandler(w.implWorld, w.serverOptions...)
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

// RegisterSystems allows for the adding of multiple systems in a single call. See AddSystem for more details.
// RegisterSystems adds the given system(s) to the world object so that the system will be executed at every
// game tick. This Register method can be called multiple times.
func (w *World) RegisterSystems(systems ...System) {
	for _, system := range systems {
		w.implWorld.AddSystem(func(world *ecs.World, queue *transaction.TxQueue, logger *ecslog.Logger) error {
			return system(&World{implWorld: world}, &TransactionQueue{queue}, &Logger{logger})
		})
	}
}

// RegisterComponents adds the given components to the game world. After components are added, entities
// with these components may be created. This Register method must only be called once.
func (w *World) RegisterComponents(components ...AnyComponentType) error {
	return w.implWorld.RegisterComponents(toIComponentType(components)...)
}

// RegisterTransactions adds the given transactions to the game world. HTTP endpoints to queue up/execute these
// transaction will automatically be created when StartGame is called. This Register method must only be called once.
func (w *World) RegisterTransactions(txs ...AnyTransaction) error {
	return w.implWorld.RegisterTransactions(toITransactionType(txs)...)
}

// RegisterReads adds the given read capabilities to the game world. HTTP endpoints to use these reads
// will automatically be created when StartGame is called. This Register method must only be called once.
func (w *World) RegisterReads(reads ...AnyReadType) error {
	return w.implWorld.RegisterReads(toIReadType(reads)...)
}

func (w *World) CurrentTick() uint64 {
	return w.implWorld.CurrentTick()
}

func (w *World) Tick(ctx context.Context) error {
	return w.implWorld.Tick(ctx)
}
