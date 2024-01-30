package cardinal

import (
	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/events"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type WorldContext interface {
	// NewSearch creates a new Search object that can iterate over entities that match
	// a given Component filter.
	//
	// For example:
	// err := worldCtx.NewSearch(cardinal.Exact(Health{})).Each(worldCtx, func(id cardinal.EntityID) bool {
	// 		...process each entity id...
	// })
	// if err != nil {
	// 		return err
	// }

	// CurrentTick returns the current game tick of the world.
	CurrentTick() uint64

	// Timestamp represents the timestamp of the current tick.
	Timestamp() uint64

	// EmitEvent broadcasts an event message to all subscribed clients.
	EmitEvent(event string)

	// Logger returns a zerolog.Logger. Additional metadata information is often attached to
	// this logger (e.g. the name of the active System).
	Logger() *zerolog.Logger

	Engine() engine.Context
}

type worldContext struct {
	engine engine.Context
}

func (wCtx *worldContext) EmitEvent(event string) {
	wCtx.Engine().EmitEvent(&events.Event{Message: event})
}

func (wCtx *worldContext) CurrentTick() uint64 {
	return wCtx.engine.CurrentTick()
}

func (wCtx *worldContext) Timestamp() uint64 { return wCtx.engine.Timestamp() }

func (wCtx *worldContext) Logger() *zerolog.Logger {
	return wCtx.engine.Logger()
}

func (wCtx *worldContext) Engine() engine.Context {
	return wCtx.engine
}
