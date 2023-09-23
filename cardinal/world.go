package cardinal

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/entityid"
	"pkg.world.dev/world-engine/cardinal/ecs/inmem"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/cardinal/server"
)

type World struct {
	impl          *ecs.World
	loopInterval  time.Duration
	serverOptions []server.Option
}

type (
	// EntityID represents a single entity in the World. An EntityID is tied to
	// one or more components.
	EntityID = entityid.ID
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
		impl:          ecsWorld,
		serverOptions: serverOptions,
	}
	for _, opt := range cardinalOptions {
		opt(world)
	}

	return world, nil
}

// NewMockWorld creates a World that uses an in-memory redis DB as the storage layer.
// This is only suitable for local development.
func NewMockWorld(opts ...WorldOption) (*World, error) {
	ecsOptions, serverOptions, cardinalOptions := separateOptions(opts)
	world := &World{
		impl:          inmem.NewECSWorld(ecsOptions...),
		serverOptions: serverOptions,
	}
	for _, opt := range cardinalOptions {
		opt(world)
	}
	return world, nil
}

// CreateMany creates multiple entities in the world, and returns the slice of ids for the newly created
// entities. At least 1 component must be provided.
func (w *World) CreateMany(num int, components ...AnyComponentType) ([]EntityID, error) {
	return w.impl.CreateMany(num, toIComponentType(components)...)
}

// Create creates a single entity in the world, and returns the id of the newly created entity.
// At least 1 component must be provided.
func (w *World) Create(components ...AnyComponentType) (EntityID, error) {
	return w.impl.Create(toIComponentType(components)...)
}

// Remove removes the given entity id from the world.
func (w *World) Remove(id EntityID) error {
	return w.impl.Remove(id)
}

// StartGame starts running the world game loop. After loopInterval time passes, a world tick is attempted.
// In addition, an HTTP server (listening on the given port) is created so that game transactions can be sent
// to this world. After StartGame is called, RegisterComponents, RegisterTransactions, RegisterReads, and AddSystem may
// not be called. If StartGame doesn't encounter any errors, it will block forever, running the server and ticking
// the game in the background.
func (w *World) StartGame() error {
	if err := w.impl.LoadGameState(); err != nil {
		return err
	}
	txh, err := server.NewHandler(w.impl, w.serverOptions...)
	if err != nil {
		return err
	}
	if w.loopInterval == 0 {
		w.loopInterval = time.Second
	}
	w.impl.StartGameLoop(context.Background(), w.loopInterval)
	go func() {
		if err := txh.Serve(); err != nil {
			log.Fatal().Err(err)
		}
	}()
	select {}
}

// RegisterSystems allows for the adding of multiple systems in a single call. See AddSystem for more details.
// RegisterSystems adds the given system(s) to the world object so that the system will be executed at every
// game tick. This Register method can be called multiple times.
func (w *World) RegisterSystems(systems ...System) {
	for _, system := range systems {
		w.impl.AddSystem(func(world *ecs.World, queue *transaction.TxQueue, logger *ecslog.Logger) error {
			return system(&World{impl: world}, &TransactionQueue{queue}, &Logger{logger})
		})
	}
}

// RegisterComponents adds the given components to the game world. After components are added, entities
// with these components may be created. This Register method must only be called once.
func (w *World) RegisterComponents(components ...AnyComponentType) error {
	return w.impl.RegisterComponents(toIComponentType(components)...)
}

// RegisterTransactions adds the given transactions to the game world. HTTP endpoints to queue up/execute these
// transaction will automatically be created when StartGame is called. This Register method must only be called once.
func (w *World) RegisterTransactions(txs ...AnyTransaction) error {
	return w.impl.RegisterTransactions(toITransactionType(txs)...)
}

// RegisterReads adds the given read capabilities to the game world. HTTP endpoints to use these reads
// will automatically be created when StartGame is called. This Register method must only be called once.
func (w *World) RegisterReads(reads ...AnyReadType) error {
	return w.impl.RegisterReads(toIReadType(reads)...)
}

func (w *World) CurrentTick() uint64 {
	return w.impl.CurrentTick()
}

func (w *World) Tick(ctx context.Context) error {
	return w.impl.Tick(ctx)
}
