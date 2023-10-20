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

type CardinalWorldContextStruct struct {
	implWorld   *World
	implContext ecs.WorldContext
}

func (wCtx *CardinalWorldContextStruct) IsReadOnly() bool {
	return wCtx.IsReadOnly()
}

func (wCtx *CardinalWorldContextStruct) GetTxQueue() *transaction.TxQueue {
	return wCtx.GetTxQueue()
}

func (wCtx *CardinalWorldContextStruct) StoreReader() store.Reader {
	return wCtx.StoreReader()
}

func (wCtx *CardinalWorldContextStruct) StoreManager() store.IManager {
	return wCtx.implContext.StoreManager()
}

func (wCtx *CardinalWorldContextStruct) CurrentTick() uint64 {
	return wCtx.implContext.CurrentTick()
}

func (wCtx *CardinalWorldContextStruct) Logger() *zerolog.Logger {
	return wCtx.implContext.Logger()
}

func (wCtx *CardinalWorldContextStruct) NewSearch(filter CardinalFilter) (*Search, error) {
	ecsSearch, err := wCtx.implContext.NewSearch(filter.ConvertToFilterable())
	if err != nil {
		return nil, err
	}
	return &Search{impl: ecsSearch}, nil
}

func (wCtx *CardinalWorldContextStruct) GetECSWorldContext() ECSWorldContext {
	return wCtx.implContext
}

func convertEcsWorldContextToCardinalWorldContext(ecsWorldContext ECSWorldContext) WorldContext {
	return &CardinalWorldContextStruct{
		implContext: ecsWorldContext,
	}
}
