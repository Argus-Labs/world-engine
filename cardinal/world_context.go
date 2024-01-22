package cardinal

import (
	"errors"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/events"

	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs/store"
	"pkg.world.dev/world-engine/cardinal/txpool"
)

type WorldContext interface {
	Timestamp() uint64
	CurrentTick() uint64
	Logger() *zerolog.Logger
	NewSearch(filter filter.ComponentFilter) *ecs.Search

	// For internal use.
	GetEngine() *ecs.Engine
	StoreReader() store.Reader
	StoreManager() store.IManager
	GetTxQueue() *txpool.TxQueue
	IsReadOnly() bool
	EmitEvent(event string)
}

var (
	ErrCannotModifyStateWithReadOnlyContext = errors.New("cannot modify state with read only context")
)

type worldContext struct {
	engine   *ecs.Engine
	txQueue  *txpool.TxQueue
	logger   *zerolog.Logger
	readOnly bool
}

func NewEngineContextForTick(engine *ecs.Engine, queue *txpool.TxQueue, logger *zerolog.Logger) WorldContext {
	if logger == nil {
		logger = engine.Logger
	}
	return &worldContext{
		engine:   engine,
		txQueue:  queue,
		logger:   logger,
		readOnly: false,
	}
}

func NewWorldContext(engine *ecs.Engine) WorldContext {
	return &worldContext{
		engine:   engine,
		logger:   engine.Logger,
		readOnly: false,
	}
}

func NewReadOnlyWorldContext(engine *ecs.Engine) WorldContext {
	return &worldContext{
		engine:   engine,
		txQueue:  nil,
		logger:   engine.Logger,
		readOnly: true,
	}
}

// Timestamp returns the UNIX timestamp of the tick.
func (e *worldContext) Timestamp() uint64 {
	return e.engine.Timestamp()
}

func (e *worldContext) CurrentTick() uint64 {
	return e.engine.CurrentTick()
}

func (e *worldContext) Logger() *zerolog.Logger {
	return e.logger
}

func (e *worldContext) GetEngine() *ecs.Engine {
	return e.engine
}

func (e *worldContext) GetTxQueue() *txpool.TxQueue {
	return e.txQueue
}

func (e *worldContext) IsReadOnly() bool {
	return e.readOnly
}

func (e *worldContext) StoreManager() store.IManager {
	return e.engine.StoreManager()
}

func (e *worldContext) StoreReader() store.Reader {
	sm := e.StoreManager()
	if e.IsReadOnly() {
		return sm.ToReadOnly()
	}
	return sm
}

func (e *worldContext) NewSearch(filter filter.ComponentFilter) *ecs.Search {
	return e.engine.NewSearch(filter)
}

func (e *worldContext) EmitEvent(event string) {
	e.engine.EmitEvent(&events.Event{Message: event})
}
