package cardinal

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/inmem"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/cardinal/shard"
	"pkg.world.dev/world-engine/sign"
)

type World struct {
	impl *ecs.World
}

type AnyTransaction interface {
	Convert() transaction.ITransaction
}

type AnyReadType interface {
	Convert() ecs.IRead
}

type (
	System        = ecs.System
	EntityID      = storage.EntityID
	Entity        = storage.Entity
	TxID          = transaction.TxID
	SignedPayload = sign.SignedPayload
)

type TxData[T any] struct {
	impl ecs.TxData[T]
}

type TransactionType[In, Out any] struct {
	impl *ecs.TransactionType[In, Out]
}

type ReadType[Request, Reply any] struct {
	impl *ecs.ReadType[Request, Reply]
}

func NewWorldUsingRedis(addr, password string, opts ...WorldOption) (*World, error) {
	log.Log().Msg("Running in normal mode, using external Redis")
	if addr == "" {
		log.Log().Msg("Redis address is not set, using fallback - localhost:6379")
		addr = "localhost:6379"
	}
	if password == "" {
		log.Log().Msg("Redis password is not set, make sure to set up redis with password in prod")
	}

	rs := storage.NewRedisStorage(storage.Options{
		Addr:     addr,
		Password: password, // make sure to set this in prod
		DB:       0,        // use default DB
	})
	worldStorage := storage.NewWorldStorage(&rs)
	world, err := ecs.NewWorld(worldStorage, opts...)
	if err != nil {
		return nil, err
	}

	return &World{
		impl: world,
	}, nil
}

func NewWorldInMemory(opts ...WorldOption) (*World, error) {
	return &World{
		impl: inmem.NewECSWorld(opts...),
	}, nil
}

func (w *World) AddSystems(s ...System) {
	w.impl.AddSystems(s...)
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

func (w *World) CreateMany(num int, components ...AnyComponentType) ([]EntityID, error) {
	return w.impl.CreateMany(num, toIComponentType(components)...)
}

func (w *World) Create(components ...AnyComponentType) (EntityID, error) {
	return w.impl.Create(toIComponentType(components)...)
}

func (w *World) Remove(id EntityID) error {
	return w.impl.Remove(id)
}

func (w *World) StartGame(loopInterval time.Duration, disableSigVerification bool, port string) error {
	if err := w.impl.LoadGameState(); err != nil {
		return err
	}
	var opts []server.Option
	if disableSigVerification {
		opts = append(opts, server.DisableSignatureVerification())
	}
	txh, err := server.NewHandler(w.impl, opts...)
	if err != nil {
		return err
	}
	w.impl.StartGameLoop(context.Background(), loopInterval)
	go txh.Serve("", port)
	return nil
}

func (w *World) RegisterComponents(components ...AnyComponentType) error {
	return w.impl.RegisterComponents(toIComponentType(components)...)
}

func (w *World) RegisterTransactions(txs ...AnyTransaction) error {
	return w.impl.RegisterTransactions(toITransactionType(txs)...)
}

func (w *World) RegisterReads(reads ...AnyReadType) error {
	return w.impl.RegisterReads(toIReadType(reads)...)
}

func NewReadType[Request any, Reply any](
	name string,
	handler func(*World, Request) (Reply, error),
) *ReadType[Request, Reply] {
	return &ReadType[Request, Reply]{
		impl: ecs.NewReadType[Request, Reply](name, func(world *ecs.World, req Request) (Reply, error) {
			outerWorld := &World{impl: world}
			return handler(outerWorld, req)
		}),
	}
}

type TransactionQueue struct {
	impl *ecs.TransactionQueue
}

func NewTransactionType[In, Out any](name string) *TransactionType[In, Out] {
	return &TransactionType[In, Out]{
		impl: ecs.NewTransactionType[In, Out](name),
	}
}

func (t *TransactionType[In, Out]) AddError(world *World, id TxID, err error) {
	world.impl.AddTransactionError(id, err)
}

func (t *TransactionType[In, Out]) SetResult(world *World, id TxID, result Out) {
	world.impl.SetTransactionResult(id, result)
}

func (t *TransactionType[In, Out]) GetReceipt(world *World, id TxID) (v Out, errs []error, ok bool) {
	return t.impl.GetReceipt(world.impl, id)
}

func (t *TransactionType[In, Out]) In(tq *TransactionQueue) []TxData[In] {
	ecsTxData := t.impl.In(tq.impl)
	out := make([]TxData[In], 0, len(ecsTxData))
	for _, tx := range ecsTxData {
		out = append(out, TxData[In]{
			impl: tx,
		})
	}
	return out
}

func (t *TxData[T]) ID() TxID {
	return t.impl.ID
}

func (t *TxData[T]) Value() T {
	return t.impl.Value
}

func (t *TxData[T]) Sig() *SignedPayload {
	return t.impl.Sig
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
