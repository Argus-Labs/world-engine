package cardinal

import (
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/store"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
)

type CardinalSpecificWorldContextMethods interface {
	NewSearch(filter CardinalFilter) (*Search, error)
	GetECSWorldContext() ECSWorldContext
}

type WorldContext interface {
	CardinalSpecificWorldContextMethods
	ecs.GeneralWorldContextMethods
}

type CardinalWorldContext struct {
	implWorld   *World
	implContext ecs.WorldContext
}

func (wCtx *CardinalWorldContext) IsReadOnly() bool {
	return wCtx.IsReadOnly()
}

func (wCtx *CardinalWorldContext) GetTxQueue() *transaction.TxQueue {
	return wCtx.GetTxQueue()
}

func (wCtx *CardinalWorldContext) StoreReader() store.Reader {
	return wCtx.StoreReader()
}

func (wCtx *CardinalWorldContext) StoreManager() store.IManager {
	return wCtx.implContext.StoreManager()
}

func (wCtx *CardinalWorldContext) CurrentTick() uint64 {
	return wCtx.implContext.CurrentTick()
}

func (wCtx *CardinalWorldContext) Logger() *zerolog.Logger {
	return wCtx.implContext.Logger()
}

func (wCtx *CardinalWorldContext) NewSearch(filter CardinalFilter) (*Search, error) {
	ecsSearch, err := wCtx.implContext.NewSearch(filter.ConvertToFilterable())
	if err != nil {
		return nil, err
	}
	return &Search{impl: ecsSearch}, nil
}

func (wCtx *CardinalWorldContext) GetECSWorldContext() ECSWorldContext {
	return wCtx.implContext
}
