package ecs

import (
	"errors"

	"github.com/rs/zerolog"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/store"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
)

type WorldContext interface {
	CurrentTick() uint64
	Logger() *zerolog.Logger
	NewSearch(filter Filterable) (*Search, error)

	// For internal use.
	GetWorld() *World
	StoreReader() store.Reader
	StoreManager() store.IManager
	GetTxQueue() *transaction.TxQueue
	IsReadOnly() bool
}

var (
	ErrCannotModifyStateWithReadOnlyContext = errors.New("cannot modify state with read only context")
)

type worldContext struct {
	world    *World
	txQueue  *transaction.TxQueue
	logger   *ecslog.Logger
	readOnly bool
}

func NewWorldContextForTick(world *World, queue *transaction.TxQueue, logger *ecslog.Logger) WorldContext {
	return &worldContext{
		world:    world,
		txQueue:  queue,
		logger:   logger,
		readOnly: false,
	}
}

func NewWorldContext(world *World) WorldContext {
	return &worldContext{
		world:    world,
		readOnly: false,
	}
}

func NewReadOnlyWorldContext(world *World) WorldContext {
	return &worldContext{
		world:    world,
		txQueue:  nil,
		readOnly: true,
	}
}

func (w *worldContext) CurrentTick() uint64 {
	return w.world.CurrentTick()
}

func (w *worldContext) Logger() *zerolog.Logger {
	if w.logger != nil {
		return w.logger.Logger
	}
	return w.world.Logger.Logger
}

func (w *worldContext) GetWorld() *World {
	return w.world
}

func (w *worldContext) GetTxQueue() *transaction.TxQueue {
	return w.txQueue
}

func (w *worldContext) IsReadOnly() bool {
	return w.readOnly
}

func (w *worldContext) StoreManager() store.IManager {
	return w.world.StoreManager()
}

func (w *worldContext) StoreReader() store.Reader {
	sm := w.StoreManager()
	if w.IsReadOnly() {
		return sm.ToReadOnly()
	}
	return sm
}

func (w *worldContext) NewSearch(filter Filterable) (*Search, error) {
	return w.world.NewSearch(filter)
}
