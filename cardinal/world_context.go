package cardinal

import (
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/events"
)

type WorldContext interface {
	NewSearch(filter Filter) (*Search, error)
	CurrentTick() uint64
	EmitEvent(event string)
	Logger() *zerolog.Logger
	getECSWorldContext() ecs.WorldContext
}

type worldContext struct {
	implContext ecs.WorldContext
}

func (wCtx *worldContext) EmitEvent(event string) {
	wCtx.getECSWorldContext().GetWorld().EmitEvent(&events.Event{Message: event})
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
