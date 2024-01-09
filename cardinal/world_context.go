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
	NewSearch(filter Filter) *Search

	// CurrentTick returns the current game tick of the world.
	CurrentTick() uint64

	// Timestamp represents the timestamp of the current tick.
	Timestamp() uint64

	// EmitEvent broadcasts an event message to all subscribed clients.
	EmitEvent(event string)

	// Logger returns a zerolog.Logger. Additional metadata information is often attached to
	// this logger (e.g. the name of the active System).
	Logger() *zerolog.Logger

	Instance() ecs.WorldContext
}

type worldContext struct {
	instance ecs.WorldContext
}

func (wCtx *worldContext) EmitEvent(event string) {
	wCtx.instance.GetWorld().EmitEvent(&events.Event{Message: event})
}

func (wCtx *worldContext) CurrentTick() uint64 {
	return wCtx.instance.CurrentTick()
}

func (wCtx *worldContext) Timestamp() uint64 { return wCtx.instance.Timestamp() }

func (wCtx *worldContext) Logger() *zerolog.Logger {
	return wCtx.instance.Logger()
}

func (wCtx *worldContext) NewSearch(filter Filter) *Search {
	return &Search{impl: NewLazyContainer[*ecs.Search](func() (*ecs.Search, error) {
		ecsSearch, err := wCtx.instance.NewSearch(filter.convertToFilterable())
		if err != nil {
			return nil, err
		}
		return ecsSearch, nil
	})}
}

func (wCtx *worldContext) Instance() ecs.WorldContext {
	return wCtx.instance
}
