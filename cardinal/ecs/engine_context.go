package ecs

import (
	"errors"

	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs/store"
	"pkg.world.dev/world-engine/cardinal/txpool"
)

type EngineContext interface {
	Timestamp() uint64
	CurrentTick() uint64
	Logger() *zerolog.Logger
	NewSearch(filter Filterable) (*Search, error)
	NewLazySearch(filter Filterable) *LazySearch
	DoAtomic(atomicFn func() (any, error)) (any, error)

	// For internal use.
	GetEngine() *Engine
	StoreReader() store.Reader
	StoreManager() store.IManager
	GetTxQueue() *txpool.TxQueue
	IsReadOnly() bool
}

var (
	ErrCannotModifyStateWithReadOnlyContext = errors.New("cannot modify state with read only context")
)

type engineContext struct {
	engine   *Engine
	txQueue  *txpool.TxQueue
	logger   *zerolog.Logger
	readOnly bool
}

func NewEngineContextForTick(engine *Engine, queue *txpool.TxQueue, logger *zerolog.Logger) EngineContext {
	if logger == nil {
		logger = engine.Logger
	}
	return &engineContext{
		engine:   engine,
		txQueue:  queue,
		logger:   logger,
		readOnly: false,
	}
}

func NewEngineContext(engine *Engine) EngineContext {
	return &engineContext{
		engine:   engine,
		logger:   engine.Logger,
		readOnly: false,
	}
}

func NewReadOnlyEngineContext(engine *Engine) EngineContext {
	return &engineContext{
		engine:   engine,
		txQueue:  nil,
		logger:   engine.Logger,
		readOnly: true,
	}
}

// Timestamp returns the UNIX timestamp of the tick.
func (e *engineContext) Timestamp() uint64 {
	return e.engine.timestamp.Load()
}

func (e *engineContext) CurrentTick() uint64 {
	return e.engine.CurrentTick()
}

func (e *engineContext) Logger() *zerolog.Logger {
	return e.logger
}

func (e *engineContext) GetEngine() *Engine {
	return e.engine
}

func (e *engineContext) GetTxQueue() *txpool.TxQueue {
	return e.txQueue
}

func (e *engineContext) IsReadOnly() bool {
	return e.readOnly
}

func (e *engineContext) StoreManager() store.IManager {
	return e.engine.StoreManager()
}

func (e *engineContext) StoreReader() store.Reader {
	sm := e.StoreManager()
	if e.IsReadOnly() {
		return sm.ToReadOnly()
	}
	return sm
}

func (e *engineContext) NewSearch(filter Filterable) (*Search, error) {
	return e.engine.NewSearch(filter)
}

func (e *engineContext) NewLazySearch(filter Filterable) *LazySearch {
	return e.engine.NewLazySearch(filter)
}

func (e *engineContext) DoAtomic(atomicFn func() (any, error)) (any, error) {
	// Create a new ECB to store pending operations
	err := e.StoreManager().NewECB()
	if err != nil {
		return nil, err
	}
	// Execute atomicFn, if there is an error, discard the pending state accrued
	result, err := atomicFn()
	if err != nil {
		e.StoreManager().DiscardPending()
		return nil, err
	}
	// Close the newly created ECB and push it's pending state to it's predecessor
	err = e.StoreManager().CloseECB()
	if err != nil {
		return nil, err
	}
	return result, nil
}
