package ecs

import (
	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	storage2 "pkg.world.dev/world-engine/cardinal/engine/storage"
)

type cache struct {
	archetypes []archetype.ID
	seen       int
}

// Query represents a query for entities.
// It is used to filter entities based on their components.
// It receives arbitrary filters that are used to filter entities.
// It contains a cache that is used to avoid re-evaluating the query.
// So it is not recommended to create a new query every time you want
// to filter entities with the same query.
type Query struct {
	archMatches map[Namespace]*cache
	filter      filter.ComponentFilter
}

// NewQuery creates a new query.
// It receives arbitrary filters that are used to filter entities.
func NewQuery(filter filter.ComponentFilter) *Query {
	return &Query{
		archMatches: make(map[Namespace]*cache),
		filter:      filter,
	}
}

type QueryCallBackFn func(entity.ID) bool

// Each iterates over all entities that match the query.
// If you would like to stop the iteration, return false to the callback. To continue iterating, return true.
func (q *Query) Each(w *World, callback QueryCallBackFn) {
	result := q.evaluateQuery(w)
	iter := storage2.NewEntityIterator(0, w.store.ArchAccessor, result)
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
func (q *Query) Count(w *World) int {
	result := q.evaluateQuery(w)
	iter := storage2.NewEntityIterator(0, w.store.ArchAccessor, result)
	ret := 0
	for iter.HasNext() {
		entities := iter.Next()
		ret += len(entities)
	}
	return ret
}

// First returns the first entity that matches the query.
func (q *Query) First(w *World) (id entity.ID, err error) {
	result := q.evaluateQuery(w)
	iter := storage2.NewEntityIterator(0, w.store.ArchAccessor, result)
	if !iter.HasNext() {
		return storage2.BadID, err
	}
	for iter.HasNext() {
		entities := iter.Next()
		if len(entities) > 0 {
			return entities[0], nil
		}
	}
	return storage2.BadID, err
}

func (q *Query) evaluateQuery(world *World) []archetype.ID {
	w := Namespace(world.Namespace())
	if _, ok := q.archMatches[w]; !ok {
		q.archMatches[w] = &cache{
			archetypes: make([]archetype.ID, 0),
			seen:       0,
		}
	}
	cache := q.archMatches[w]
	for it := world.store.ArchCompIdxStore.SearchFrom(q.filter, cache.seen); it.HasNext(); {
		cache.archetypes = append(cache.archetypes, it.Next())
	}
	cache.seen = world.store.ArchAccessor.Count()
	return cache.archetypes
}
