package ecs

import (
	"errors"
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/gamestate"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
	"pkg.world.dev/world-engine/cardinal/ecs/search"
	"pkg.world.dev/world-engine/cardinal/events"
	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"pkg.world.dev/world-engine/sign"
)

var (
	ErrCannotModifyStateWithReadOnlyContext = errors.New("cannot modify state with read only context")
)

type engineContext struct {
	engine   *Engine
	txQueue  *txpool.TxQueue
	logger   *zerolog.Logger
	readOnly bool
}

func NewEngineContextForTick(engine *Engine, queue *txpool.TxQueue, logger *zerolog.Logger) engine.Context {
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

func NewEngineContext(engine *Engine) engine.Context {
	return &engineContext{
		engine:   engine,
		logger:   engine.Logger,
		readOnly: false,
	}
}

func NewReadOnlyEngineContext(engine *Engine) engine.Context {
	return &engineContext{
		engine:   engine,
		txQueue:  nil,
		logger:   engine.Logger,
		readOnly: true,
	}
}

// interface guard
var _ engine.Context = (*engineContext)(nil)

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

func (e *engineContext) SetLogger(logger zerolog.Logger) {
	e.logger = &logger
}

func (e *engineContext) NewSearch(filter filter.ComponentFilter) *search.Search {
	return e.engine.NewSearch(filter)
}

func (e *engineContext) GetComponentByName(name string) (component.ComponentMetadata, error) {
	return e.engine.GetComponentByName(name)
}

func (e *engineContext) AddMessageError(id message.TxHash, err error) {
	e.engine.AddMessageError(id, err)
}

func (e *engineContext) SetMessageResult(id message.TxHash, a any) {
	e.engine.SetMessageResult(id, a)
}

func (e *engineContext) GetTransactionReceipt(id message.TxHash) (any, []error, bool) {
	return e.engine.GetTransactionReceipt(id)
}

func (e *engineContext) EmitEvent(event string) {
	e.engine.EmitEvent(&events.Event{Message: event})
}

func (e *engineContext) GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error) {
	return e.engine.GetSignerForPersonaTag(personaTag, tick)
}

func (e *engineContext) GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error) {
	return e.engine.GetTransactionReceiptsForTick(tick)
}

func (e *engineContext) ReceiptHistorySize() uint64 {
	return e.engine.ReceiptHistorySize()
}

func (e *engineContext) Namespace() string {
	return string(e.engine.Namespace())
}

func (e *engineContext) ListMessages() []message.Message {
	return e.engine.ListMessages()
}

func (e *engineContext) AddTransaction(id message.TypeID, v any, sig *sign.Transaction) (uint64, message.TxHash) {
	return e.engine.AddTransaction(id, v, sig)
}

func (e *engineContext) GetTxQueue() *txpool.TxQueue {
	return e.txQueue
}

func (e *engineContext) IsReadOnly() bool {
	return e.readOnly
}

func (e *engineContext) StoreManager() gamestate.Manager {
	return e.engine.GameStateManager()
}

func (e *engineContext) StoreReader() gamestate.Reader {
	sm := e.StoreManager()
	if e.IsReadOnly() {
		return sm.ToReadOnly()
	}
	return sm
}
