package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

// Query allowed for the querying of entities within a World.
type Query struct {
	impl *ecs.Query
}

// NewQuery creates a new Query.
func NewQuery(filter LayoutFilter) *Query {
	return &Query{ecs.NewQuery(filter)}
}

// QueryCallBackFn represents a function that can operate on a single EntityID.
type QueryCallBackFn func(EntityID) bool

// Each executes the given callback function on every EntityID that matches this query.
func (q *Query) Each(w *World, callback QueryCallBackFn) {
	q.impl.Each(w.impl, func(eid storage.EntityID) bool {
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

type LayoutFilter = filter.LayoutFilter

func And(filters ...LayoutFilter) LayoutFilter {
	return filter.And(filters...)
}

func Contains(components ...AnyComponentType) LayoutFilter {
	return filter.Contains(toIComponentType(components)...)
}

func Exact(components ...AnyComponentType) LayoutFilter {
	return filter.Exact(toIComponentType(components)...)
}

func Not(layoutFilter LayoutFilter) LayoutFilter {
	return filter.Not(layoutFilter)
}

func Or(filters ...LayoutFilter) LayoutFilter {
	return filter.Or(filters...)
}
