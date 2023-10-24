package cardinal

import (
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

type WorldContext interface {
	NewSearch(filter Filter) (*Search, error)
	CurrentTick() uint64
	Logger() *zerolog.Logger
	getECSWorldContext() ecs.WorldContext
}

type worldContext struct {
	implContext ecs.WorldContext
}

func (wCtx *worldContext) CurrentTick() uint64 {
	return wCtx.implContext.CurrentTick()
}

func (wCtx *worldContext) Logger() *zerolog.Logger {
	return wCtx.implContext.Logger()
}

func (wCtx *worldContext) NewSearch(filter Filter) (*Search, error) {
	ecsSearch, err := wCtx.implContext.NewSearch(filter.convertToFilterable())
	if err != nil {
		return nil, err
	}
	return &Search{impl: ecsSearch}, nil
}

func (wCtx *worldContext) getECSWorldContext() ecs.WorldContext {
	return wCtx.implContext
}

func (w *worldContext) TestOnlyGetECSWorld() *ecs.World {
	return w.implContext.GetWorld()
}

func TestOnlyGetECSWorld(worldCtx WorldContext) *ecs.World {
	w := worldCtx.(*worldContext)
	return w.implContext.GetWorld()
}
