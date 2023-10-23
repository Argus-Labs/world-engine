package cardinal

import (
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

type WorldContext interface {
	NewSearch(filter CardinalFilter) (*Search, error)
	getECSWorldContext() ECSWorldContext
	CurrentTick() uint64
	Logger() *zerolog.Logger
	IsReadOnly() bool
}

type worldContext struct {
	implContext ecs.WorldContext
}

func (wCtx *worldContext) IsReadOnly() bool {
	return wCtx.IsReadOnly()
}

func (wCtx *worldContext) CurrentTick() uint64 {
	return wCtx.implContext.CurrentTick()
}

func (wCtx *worldContext) Logger() *zerolog.Logger {
	return wCtx.implContext.Logger()
}

func (wCtx *worldContext) NewSearch(filter CardinalFilter) (*Search, error) {
	ecsSearch, err := wCtx.implContext.NewSearch(filter.ConvertToFilterable())
	if err != nil {
		return nil, err
	}
	return &Search{impl: ecsSearch}, nil
}

func (wCtx *worldContext) getECSWorldContext() ECSWorldContext {
	return wCtx.implContext
}
