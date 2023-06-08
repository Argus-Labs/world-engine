package ecs

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

type cache struct {
	archetypes []storage.ArchetypeID
	seen       int
}

// Query represents a query for entityLocationStore.
// It is used to filter entityLocationStore based on their componentStore.
// It receives arbitrary filters that are used to filter entityLocationStore.
// It contains a cache that is used to avoid re-evaluating the query.
// So it is not recommended to create a new query every time you want
// to filter entityLocationStore with the same query.
type Query struct {
	layoutMatches map[WorldId]*cache
	filter        filter.LayoutFilter
}

// NewQuery creates a new query.
// It receives arbitrary filters that are used to filter entityLocationStore.
func NewQuery(filter filter.LayoutFilter) *Query {
	return &Query{
		layoutMatches: make(map[WorldId]*cache),
		filter:        filter,
	}
}

// Each iterates over all entityLocationStore that match the query.
func (q *Query) Each(w *World, callback func(storage.EntityID)) {
	accessor := w.StorageAccessor()
	result := q.evaluateQuery(w, &accessor)
	iter := storage.NewEntityIterator(0, accessor.Archetypes, result)
	for iter.HasNext() {
		entities := iter.Next()
		for _, id := range entities {
			callback(id)
		}
	}
}

// Count returns the number of entityLocationStore that match the query.
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
	w := world.ID()
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
