package cardinal

import (
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/world_context"
)

type worldContextCardinalSpecificMethods interface {
	NewSearch(filter CardinalFilter) (*Search, error)
	getECSWorldContext() ECSWorldContext
}

type WorldContext interface {
	worldContextCardinalSpecificMethods
	world_context.WorldContext
}

type ConcreteWorldContext struct {
	implContext ecs.WorldContext
}

func (wCtx *ConcreteWorldContext) IsReadOnly() bool {
	return wCtx.IsReadOnly()
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

func (wCtx *ConcreteWorldContext) getECSWorldContext() ECSWorldContext {
	return wCtx.implContext
}
