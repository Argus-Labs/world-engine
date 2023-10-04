package query

import (
	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/world_namespace"
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
	archMatches map[world_namespace.Namespace]*cache
	filter      filter.ComponentFilter
}

// NewQuery creates a new query.
// It receives arbitrary filters that are used to filter entities.
func NewQuery(filter filter.ComponentFilter) *Query {
	return &Query{
		archMatches: make(map[world_namespace.Namespace]*cache),
		filter:      filter,
	}
}

type QueryCallBackFn func(entity.ID) bool

// Each iterates over all entities that match the query.
// If you would like to stop the iteration, return false to the callback. To continue iterating, return true.
func (q *Query) Each(namespace world_namespace.Namespace, worldStorage *storage.WorldStorage, callback QueryCallBackFn) {
	result := q.evaluateQuery(namespace, worldStorage)
	iter := storage.NewEntityIterator(0, worldStorage.ArchAccessor, result)
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
func (q *Query) Count(namespace string, worldStorage *storage.WorldStorage) int {
	result := q.evaluateQuery(world_namespace.Namespace(namespace), worldStorage)
	iter := storage.NewEntityIterator(0, worldStorage.ArchAccessor, result)
	ret := 0
	for iter.HasNext() {
		entities := iter.Next()
		ret += len(entities)
	}
	return ret
}

// First returns the first entity that matches the query.
func (q *Query) First(namespace string, worldStorage *storage.WorldStorage) (id entity.ID, err error) {
	result := q.evaluateQuery(world_namespace.Namespace(namespace), worldStorage)
	iter := storage.NewEntityIterator(0, worldStorage.ArchAccessor, result)
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

func (q *Query) evaluateQuery(namespace world_namespace.Namespace, s *storage.WorldStorage) []archetype.ID {
	w := namespace
	if _, ok := q.archMatches[w]; !ok {
		q.archMatches[w] = &cache{
			archetypes: make([]archetype.ID, 0),
			seen:       0,
		}
	}
	cache := q.archMatches[w]
	for it := s.ArchCompIdxStore.SearchFrom(q.filter, cache.seen); it.HasNext(); {
		cache.archetypes = append(cache.archetypes, it.Next())
	}
	cache.seen = s.ArchAccessor.Count()
	return cache.archetypes
}
