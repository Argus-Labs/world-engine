package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
)

// Query allowed for the querying of entities within a World.
type Query struct {
	impl *ecs.Query
}

// NewQuery creates a new Query.
func (w *World) NewQuery(filter ecs.Filterable) (*Query, error) {
	q, err := w.implWorld.NewQuery(filter)
	if err != nil {
		return nil, err
	}
	return &Query{q}, nil
}

// QueryCallBackFn represents a function that can operate on a single EntityID, and returns whether the next EntityID
// should be processed.
type QueryCallBackFn func(EntityID) bool

// Each executes the given callback function on every EntityID that matches this query. If any call to callback returns
// falls, no more entities will be processed.
func (q *Query) Each(w *World, callback QueryCallBackFn) {
	q.impl.Each(w.implWorld, func(eid entity.ID) bool {
		return callback(eid)
	})
}

// Count returns the number of entities that match this query.
func (q *Query) Count(w *World) int {
	return q.impl.Count(w.implWorld)
}

// First returns the first entity that matches this query.
func (q *Query) First(w *World) (id EntityID, err error) {
	return q.impl.First(w.implWorld)
}
