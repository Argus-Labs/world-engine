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

type ConcreteWorldContext struct {
	implContext ecs.WorldContext
}

func (wCtx *ConcreteWorldContext) IsReadOnly() bool {
	return wCtx.IsReadOnly()
}

func (wCtx *ConcreteWorldContext) GetTxQueue() *transaction.TxQueue {
	return wCtx.GetTxQueue()
}

func (wCtx *ConcreteWorldContext) StoreReader() store.Reader {
	return wCtx.StoreReader()
}

func (wCtx *ConcreteWorldContext) StoreManager() store.IManager {
	return wCtx.implContext.StoreManager()
}

func (wCtx *ConcreteWorldContext) CurrentTick() uint64 {
	return wCtx.implContext.CurrentTick()
}

func (wCtx *ConcreteWorldContext) Logger() *zerolog.Logger {
	return wCtx.implContext.Logger()
}

func (wCtx *ConcreteWorldContext) NewSearch(filter CardinalFilter) (*Search, error) {
	ecsSearch, err := wCtx.implContext.NewSearch(filter.ConvertToFilterable())
	if err != nil {
		return nil, err
	}
	return &Search{impl: ecsSearch}, nil
}

func (wCtx *ConcreteWorldContext) GetECSWorldContext() ECSWorldContext {
	return wCtx.implContext
}
