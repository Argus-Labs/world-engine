package ecs

import (
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

type cache struct {
	archetypes []storage.ArchetypeID
	seen       int
}

// Query represents a query for entities.
// It is used to filter entities based on their components.
// It receives arbitrary filters that are used to filter entities.
// It contains a cache that is used to avoid re-evaluating the query.
// So it is not recommended to create a new query every time you want
// to filter entities with the same query.
type Query struct {
	layoutMatches map[Namespace]*cache
	filter        filter.LayoutFilter
}

// NewQuery creates a new query.
// It receives arbitrary filters that are used to filter entities.
func NewQuery(filter filter.LayoutFilter) *Query {
	return &Query{
		layoutMatches: make(map[Namespace]*cache),
		filter:        filter,
	}
}

type QueryCallBackFn func(storage.EntityID) bool

// Each iterates over all entities that match the query.
// If you would like to stop the iteration, return false to the callback. To continue iterating, return true.
func (q *Query) Each(ctx WorldContext, callback QueryCallBackFn) {
	result := q.evaluateQuery(ctx)
	iter := storage.NewEntityIterator(0, ctx.World.store.ArchAccessor, result)
	for iter.HasNext() {
		entities := iter.Next()
		for _, id := range entities {
			cont := callback(id)
			if !cont {
				return
			}
		}
	}
}

// Count returns the number of entities that match the query.
func (q *Query) Count(ctx WorldContext) int {
	result := q.evaluateQuery(ctx)
	iter := storage.NewEntityIterator(0, ctx.World.store.ArchAccessor, result)
	ret := 0
	for iter.HasNext() {
		entities := iter.Next()
		ret += len(entities)
	}
	return ret
}

// First returns the first entity that matches the query.
func (q *Query) First(ctx WorldContext) (id storage.EntityID, err error) {
	result := q.evaluateQuery(ctx)
	iter := storage.NewEntityIterator(0, ctx.World.store.ArchAccessor, result)
	if !iter.HasNext() {
		return storage.BadID, err
	}
	for iter.HasNext() {
		entities := iter.Next()
		if len(entities) > 0 {
			return entities[0], nil
		}
	}
	return storage.BadID, err
}

func (q *Query) evaluateQuery(ctx WorldContext) []storage.ArchetypeID {
	w := Namespace(ctx.World.Namespace())
	if _, ok := q.layoutMatches[w]; !ok {
		q.layoutMatches[w] = &cache{
			archetypes: make([]storage.ArchetypeID, 0),
			seen:       0,
		}
	}
	cache := q.layoutMatches[w]
	for it := ctx.World.store.ArchCompIdxStore.SearchFrom(q.filter, cache.seen); it.HasNext(); {
		cache.archetypes = append(cache.archetypes, it.Next())
	}
	cache.seen = ctx.World.store.ArchAccessor.Count()
	return cache.archetypes
}
