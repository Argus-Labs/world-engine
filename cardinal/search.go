package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/types/entity"
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
	err := q.impl.Each(
		wCtx.Engine(), func(eid entity.ID) bool {
			return callback(eid)
		},
	)
	if wCtx.Engine().IsReadOnly() || err == nil {
		return err
	}
	return logAndPanic(wCtx, err)
}

// Count returns the number of entities that match this search.
func (q *Search) Count(wCtx WorldContext) (int, error) {
	num, err := q.impl.Count(wCtx.Engine())
	if wCtx.Engine().IsReadOnly() || err == nil {
		return num, err
	}
	return 0, logAndPanic(wCtx, err)
}

// First returns the first entity that matches this search.
func (q *Search) First(wCtx WorldContext) (EntityID, error) {
	id, err := q.impl.First(wCtx.Engine())
	if wCtx.Engine().IsReadOnly() || err == nil {
		return id, err
	}
	return 0, logAndPanic(wCtx, err)
}
