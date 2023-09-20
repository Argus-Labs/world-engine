package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
)

// Query allowed for the querying of entities within a World.
type Query struct {
	impl *ecs.Query
}

// NewQuery creates a new Query.
func NewQuery(filter ComponentFilter) *Query {
	return &Query{ecs.NewQuery(filter)}
}

// QueryCallBackFn represents a function that can operate on a single EntityID, and returns whether the next EntityID
// should be processed.
type QueryCallBackFn func(EntityID) bool

// Each executes the given callback function on every EntityID that matches this query. If any call to callback returns
// falls, no more entities will be processed.
func (q *Query) Each(w *World, callback QueryCallBackFn) {
	q.impl.Each(w.impl, func(eid entity.ID) bool {
		return callback(eid)
	})
}

// Count returns the number of entities that match this query.
func (q *Query) Count(w *World) int {
	return q.impl.Count(w.impl)
}

// First returns the first entity that matches this query.
func (q *Query) First(w *World) (id EntityID, err error) {
	return q.impl.First(w.impl)
}

// ComponentFilter represents a filter that will be passed to NewQuery to help decide which entities should be
// returned in the query.
type ComponentFilter = filter.LayoutFilter

// And returns entities that match ALL the given filters.
func And(filters ...ComponentFilter) ComponentFilter {
	return filter.And(filters...)
}

// Contains returns entities that have been associated with all the given components. Entities that have been associated
// with other components not listed will still be returned.
func Contains(components ...AnyComponentType) ComponentFilter {
	return filter.Contains(toIComponentType(components)...)
}

// Exact returns entities that have the exact set of given components (order is not important). Entities that have been
// associated with other component not listed will NOT be returned.
func Exact(components ...AnyComponentType) ComponentFilter {
	return filter.Exact(toIComponentType(components)...)
}

// Not returns entities that do NOT match the given filter.
func Not(layoutFilter ComponentFilter) ComponentFilter {
	return filter.Not(layoutFilter)
}

// Or returns entities that match 1 or more of the given filters.
func Or(filters ...ComponentFilter) ComponentFilter {
	return filter.Or(filters...)
}
