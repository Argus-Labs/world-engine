package cardinal

import (
	"reflect"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/receipt"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/txpool"
	"pkg.world.dev/world-engine/cardinal/worldstage"
	"pkg.world.dev/world-engine/sign"
)

// interface guard
var _ Context = (*worldContext)(nil)

//go:generate mockgen -source=context.go -package mocks -destination=mocks/context.go
type Context interface {
	// Timestamp returns the UNIX timestamp of the tick.
	Timestamp() uint64
	// CurrentTick returns the current tick.
	CurrentTick() uint64
	// Logger returns the logger that can be used to log messages from within system or query.
	Logger() *zerolog.Logger
	// EmitEvent emits an event that will be broadcast to all websocket subscribers.
	EmitEvent(map[string]any) error
	// EmitStringEvent emits a string event that will be broadcast to all websocket subscribers.
	// This method is provided for backwards compatability. EmitEvent should be used for most cases.
	EmitStringEvent(string) error
	// Namespace returns the namespace of the world.
	Namespace() string

	// For internal use.

	// SetLogger is used to inject a new logger configuration to an engine context that is already created.
	setLogger(logger zerolog.Logger)
	addMessageError(id types.TxHash, err error)
	setMessageResult(id types.TxHash, a any)
	GetComponentByName(name string) (types.ComponentMetadata, error)
	GetMessageByType(mType reflect.Type) (types.Message, bool)
	GetTransactionReceipt(id types.TxHash) (any, []error, bool)
	GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error)
	GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error)
	ReceiptHistorySize() uint64
	AddTransaction(id types.MessageID, v any, sig *sign.Transaction) (uint64, types.TxHash)
	IsWorldReady() bool
	StoreReader() gamestate.Reader
	StoreManager() gamestate.Manager
	GetTxPool() *txpool.TxPool
	IsReadOnly() bool
}

type worldContext struct {
	world    *World
	txPool   *txpool.TxPool
	logger   *zerolog.Logger
	readOnly bool
}

func newWorldContextForTick(world *World, txPool *txpool.TxPool) Context {
	return &worldContext{
		world:    world,
		txPool:   txPool,
		logger:   &log.Logger,
		readOnly: false,
	}
}

func NewWorldContext(world *World) Context {
	return &worldContext{
		world:    world,
		txPool:   nil,
		logger:   &log.Logger,
		readOnly: false,
	}
}

func NewReadOnlyWorldContext(world *World) Context {
	return &worldContext{
		world:    world,
		txPool:   nil,
		logger:   &log.Logger,
		readOnly: true,
	}
}

// Timestamp returns the UNIX timestamp of the tick.
func (ctx *worldContext) Timestamp() uint64 {
	return ctx.world.timestamp.Load()
}

func (ctx *worldContext) CurrentTick() uint64 {
	return ctx.world.CurrentTick()
}

func (ctx *worldContext) Logger() *zerolog.Logger {
	return ctx.logger
}

func (ctx *worldContext) GetMessageByType(mType reflect.Type) (types.Message, bool) {
	return ctx.world.GetMessageByType(mType)
}

func (ctx *worldContext) setLogger(logger zerolog.Logger) {
	ctx.logger = &logger
}

func (ctx *worldContext) GetComponentByName(name string) (types.ComponentMetadata, error) {
	return ctx.world.GetComponentByName(name)
}

func (ctx *worldContext) addMessageError(id types.TxHash, err error) {
	// TODO(scott): i dont trust exposing this to the users. this should be fully abstracted away.
	ctx.world.receiptHistory.AddError(id, err)
}

func (ctx *worldContext) setMessageResult(id types.TxHash, a any) {
	// TODO(scott): i dont trust exposing this to the users. this should be fully abstracted away.
	ctx.world.receiptHistory.SetResult(id, a)
}

func (ctx *worldContext) GetTransactionReceipt(id types.TxHash) (any, []error, bool) {
	rec, ok := ctx.world.receiptHistory.GetReceipt(id)
	if !ok {
		return nil, nil, false
	}
	return rec.Result, rec.Errs, true
}

func (ctx *worldContext) EmitEvent(event map[string]any) error {
	return ctx.world.tickResults.AddEvent(event)
}

func (ctx *worldContext) EmitStringEvent(e string) error {
	return ctx.world.tickResults.AddStringEvent(e)
}

func (ctx *worldContext) GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error) {
	return ctx.world.GetSignerForPersonaTag(personaTag, tick)
}

func (ctx *worldContext) GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error) {
	return ctx.world.GetTransactionReceiptsForTick(tick)
}

func (ctx *worldContext) ReceiptHistorySize() uint64 {
	return ctx.world.receiptHistory.Size()
}

func (ctx *worldContext) Namespace() string {
	return ctx.world.Namespace()
}

func (ctx *worldContext) AddTransaction(id types.MessageID, v any, sig *sign.Transaction) (uint64, types.TxHash) {
	return ctx.world.AddTransaction(id, v, sig)
}

func (ctx *worldContext) GetTxPool() *txpool.TxPool {
	return ctx.txPool
}

func (ctx *worldContext) IsReadOnly() bool {
	return ctx.readOnly
}

func (ctx *worldContext) StoreManager() gamestate.Manager {
	return ctx.world.entityStore
}

func (ctx *worldContext) StoreReader() gamestate.Reader {
	sm := ctx.StoreManager()
	if ctx.IsReadOnly() {
		return sm.ToReadOnly()
	}
	return sm
}

func (ctx *worldContext) IsWorldReady() bool {
	stage := ctx.world.worldStage.Current()
	return stage == worldstage.Ready ||
		stage == worldstage.Running ||
		stage == worldstage.Recovering
}
