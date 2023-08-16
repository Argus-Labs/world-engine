package cardinal

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/inmem"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/cardinal/shard"
)

type World struct {
	impl *ecs.World
}

// System is a function that process the transaction in the given transaction queue.
// Systems are automatically called during a world tick, and they must be registered
// with a world using AddSystem or AddSystems.
type System func(*World, *TransactionQueue) error

type (
	// EntityID represents a single entity in the World. An EntityID is tied to
	// one or more components.
	EntityID = storage.EntityID
	TxHash   = transaction.TxHash
)

// NewWorldUsingRedis creates a new World object using Redis as the storage layer.
func NewWorldUsingRedis(addr, password string, opts ...WorldOption) (*World, error) {
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
	world, err := ecs.NewWorld(worldStorage, opts...)
	if err != nil {
		return nil, err
	}

	return &World{
		impl: world,
	}, nil
}

// NewWorldInMemory creates a World that uses an in-memory redis DB as the storage
// layer. This is only suitable for local development.
func NewWorldInMemory(opts ...WorldOption) (*World, error) {
	return &World{
		impl: inmem.NewECSWorld(opts...),
	}, nil
}

// AddSystem adds the given system to the world object so that the system will be executed
// at every game tick.
func (w *World) AddSystem(system System) {
	w.impl.AddSystem(func(world *ecs.World, queue *ecs.TransactionQueue) error {
		return system(&World{world}, &TransactionQueue{queue})
	})
}

// AddSystems allows for the adding of multiple systems in a single call. See AddSystem for more details.
func (w *World) AddSystems(systems ...System) {
	for _, s := range systems {
		w.AddSystem(s)
	}
}

func toITransactionType(ins []AnyTransaction) []transaction.ITransaction {
	out := make([]transaction.ITransaction, 0, len(ins))
	for _, t := range ins {
		out = append(out, t.Convert())
	}
	return out
}

func toIReadType(ins []AnyReadType) []ecs.IRead {
	out := make([]ecs.IRead, 0, len(ins))
	for _, r := range ins {
		out = append(out, r.Convert())
	}
	return out
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
// to this world. After StartGame is called, RegisterComponents, RegisterTransactions, RegisterReads, and AddSystem(s) may
// not be called. If StartGame doesn't encounter any errors, it will block on the given done channel.
func (w *World) StartGame(loopInterval time.Duration, disableSigVerification bool, port string, done chan struct{}) error {
	if err := w.impl.LoadGameState(); err != nil {
		return err
	}
	var opts []server.Option
	if disableSigVerification {
		opts = append(opts, server.DisableSignatureVerification())
	}
	opts = append(opts, server.WithPort(port))
	txh, err := server.NewHandler(w.impl, opts...)
	if err != nil {
		return err
	}
	w.impl.StartGameLoop(context.Background(), loopInterval)
	go txh.Serve()
	<-done
	return nil
}

// RegisterComponents adds the given components to the game world. After components are added, entities
// with these components may be created. This function must only be called once.
func (w *World) RegisterComponents(components ...AnyComponentType) error {
	return w.impl.RegisterComponents(toIComponentType(components)...)
}

// RegisterTransactions adds the given transactions to the game world. HTTP endpoints to queue up/execute these
// transaction will automatically be created when StartGame is called.
func (w *World) RegisterTransactions(txs ...AnyTransaction) error {
	return w.impl.RegisterTransactions(toITransactionType(txs)...)
}

// RegisterReads adds the given read capabilities to the game world. HTTP endpoints to use these reads
// will automatically be created when StartGame is called.
func (w *World) RegisterReads(reads ...AnyReadType) error {
	return w.impl.RegisterReads(toIReadType(reads)...)
}

type WorldOption = ecs.Option

func WithAdapter(adapter shard.Adapter) WorldOption {
	return ecs.WithAdapter(adapter)
}

func WithReceiptHistorySize(size int) WorldOption {
	return ecs.WithReceiptHistorySize(size)
}

func WithNamespace(namespace string) WorldOption {
	return ecs.WithNamespace(namespace)
}
