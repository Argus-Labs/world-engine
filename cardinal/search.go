package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
)

// Search allowed for the querying of entities within a World.
type Search struct {
	impl *ecs.Search
}

// SearchCallBackFn represents a function that can operate on a single EntityID, and returns whether the next EntityID
// should be processed.
type SearchCallBackFn func(EntityID) bool

// Each executes the given callback function on every EntityID that matches this search. If any call to callback returns
// falls, no more entities will be processed.
func (q *Search) Each(wCtx WorldContext, callback SearchCallBackFn) error {
	return q.impl.Each(wCtx.getECSWorldContext(), func(eid entity.ID) bool {
		return callback(eid)
	})
}

// Count returns the number of entities that match this search.
func (q *Search) Count(wCtx WorldContext) (int, error) {
	return q.impl.Count(wCtx.getECSWorldContext())
}

// First returns the first entity that matches this search.
func (q *Search) First(wCtx WorldContext) (EntityID, error) {
	return q.impl.First(wCtx.getECSWorldContext())
}
