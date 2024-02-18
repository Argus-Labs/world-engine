package cardinal

import (
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/events"
	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/receipt"
	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/worldstage"
	"pkg.world.dev/world-engine/sign"
)

type worldContext struct {
	world    *World
	txQueue  *txpool.TxQueue
	logger   *zerolog.Logger
	readOnly bool
}

func newWorldContextForTick(world *World, txQueue *txpool.TxQueue) engine.Context {
	return &worldContext{
		world:    world,
		txQueue:  txQueue,
		logger:   world.Logger,
		readOnly: false,
	}
}

func NewWorldContext(world *World) engine.Context {
	return &worldContext{
		world:    world,
		txQueue:  nil,
		logger:   world.Logger,
		readOnly: false,
	}
}

func NewReadOnlyWorldContext(world *World) engine.Context {
	return &worldContext{
		world:    world,
		txQueue:  nil,
		logger:   world.Logger,
		readOnly: true,
	}
}

// interface guard
var _ engine.Context = (*worldContext)(nil)

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

func (ctx *worldContext) SetLogger(logger zerolog.Logger) {
	ctx.logger = &logger
}

func (ctx *worldContext) GetComponentByName(name string) (types.ComponentMetadata, error) {
	return ctx.world.GetComponentByName(name)
}

func (ctx *worldContext) AddMessageError(id types.TxHash, err error) {
	// TODO(scott): i dont trust exposing this to the users. this should be fully abstracted away.
	ctx.world.receiptHistory.AddError(id, err)
}

func (ctx *worldContext) SetMessageResult(id types.TxHash, a any) {
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

func (ctx *worldContext) EmitEvent(event string) {
	ctx.world.eventHub.EmitEvent(&events.Event{Message: event})
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
	return string(ctx.world.namespace)
}

func (ctx *worldContext) AddTransaction(id types.MessageID, v any, sig *sign.Transaction) (uint64, types.TxHash) {
	return ctx.world.AddTransaction(id, v, sig)
}

func (ctx *worldContext) GetTxQueue() *txpool.TxQueue {
	return ctx.txQueue
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

func (ctx *worldContext) UseNonce(signerAddress string, nonce uint64) error {
	return ctx.world.UseNonce(signerAddress, nonce)
}

func (ctx *worldContext) IsWorldReady() bool {
	return ctx.world.worldStage.Current() == worldstage.Ready || ctx.world.worldStage.Current() == worldstage.Running
}
