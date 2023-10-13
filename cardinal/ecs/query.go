package ecs

import (
	"fmt"

	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/store"
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
	result := q.evaluateQuery(w.GetNameSpace(), w.StoreManager())
	iter := storage.NewEntityIterator(0, w.StoreManager(), result)
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
	result := q.evaluateQuery(w.GetNameSpace(), w.StoreManager())
	iter := storage.NewEntityIterator(0, w.StoreManager(), result)
	ret := 0
	for iter.HasNext() {
		entities := iter.Next()
		ret += len(entities)
	}
	return ret
}

// First returns the first entity that matches the query.
func (q *Query) First(w *World) (id entity.ID, err error) {
	result := q.evaluateQuery(w.GetNameSpace(), w.StoreManager())
	iter := storage.NewEntityIterator(0, w.StoreManager(), result)
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

func (q *Query) MustFirst(w *World) entity.ID {
	id, err := q.First(w)
	if err != nil {
		panic(fmt.Sprintf("no entity matches the query."))
	}
	return id
}

func (q *Query) evaluateQuery(namespace Namespace, sm store.IManager) []archetype.ID {
	if _, ok := q.archMatches[namespace]; !ok {
		q.archMatches[namespace] = &cache{
			archetypes: make([]archetype.ID, 0),
			seen:       0,
		}
	}
	cache := q.archMatches[namespace]
	for it := sm.SearchFrom(q.filter, cache.seen); it.HasNext(); {
		cache.archetypes = append(cache.archetypes, it.Next())
	}
	cache.seen = sm.ArchetypeCount()
	return cache.archetypes
}
