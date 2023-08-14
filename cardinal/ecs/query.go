package ecs

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
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
func (q *Query) Each(w *World, callback QueryCallBackFn) {
	accessor := w.StorageAccessor()
	result := q.evaluateQuery(w, &accessor)
	iter := storage.NewEntityIterator(0, accessor.Archetypes, result)
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
	accessor := w.StorageAccessor()
	result := q.evaluateQuery(w, &accessor)
	iter := storage.NewEntityIterator(0, accessor.Archetypes, result)
	ret := 0
	for iter.HasNext() {
		entities := iter.Next()
		ret += len(entities)
	}
	return ret
}

// First returns the first entity that matches the query.
func (q *Query) First(w *World) (id storage.EntityID, ok bool, err error) {
	accessor := w.StorageAccessor()
	result := q.evaluateQuery(w, &accessor)
	iter := storage.NewEntityIterator(0, accessor.Archetypes, result)
	if !iter.HasNext() {
		return storage.BadID, false, err
	}
	for iter.HasNext() {
		entities := iter.Next()
		if len(entities) > 0 {
			return entities[0], true, nil
		}
	}
	return storage.BadID, false, err
}

func (q *Query) evaluateQuery(world *World, accessor *StorageAccessor) []storage.ArchetypeID {
	w := Namespace(world.Namespace())
	if _, ok := q.layoutMatches[w]; !ok {
		q.layoutMatches[w] = &cache{
			archetypes: make([]storage.ArchetypeID, 0),
			seen:       0,
		}
	}
	cache := q.layoutMatches[w]
	for it := accessor.Index.SearchFrom(q.filter, cache.seen); it.HasNext(); {
		cache.archetypes = append(cache.archetypes, it.Next())
	}
	cache.seen = accessor.Archetypes.Count()
	return cache.archetypes
}
