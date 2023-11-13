package cardinal

import (
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/events"
)

type WorldContext interface {
	// NewSearch creates a new Search object that can iterate over entities that match
	// a given Component filter.
	//
	// For example:
	// search, err := worldCtx.NewSearch(cardinal.Exact(Health{}))
	// if err != nil {
	// 		return err
	// }
	// err = search.Each(worldCtx, func(id cardinal.EntityID) bool {
	// 		...process each entity id...
	// }
	// if err != nil {
	// 		return err
	// }
	NewSearch(filter Filter) (*Search, error)

	// CurrentTick returns the current game tick of the world.
	CurrentTick() uint64

	// EmitEvent broadcasts an event message to all subscribed clients.
	EmitEvent(event string)

	// Logger returns a zerolog.Logger. Additional metadata information is often attached to
	// this logger (e.g. the name of the active System).
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
