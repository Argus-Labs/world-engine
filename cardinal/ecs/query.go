package ecs

import (
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/interfaces"
)

type cache struct {
	archetypes []interfaces.ArchetypeID
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
	filter      interfaces.IComponentFilter
}

// NewQuery creates a new query.
// It receives arbitrary filters that are used to filter entities.
func NewQuery(filter interfaces.IComponentFilter) *Query {
	return &Query{
		archMatches: make(map[Namespace]*cache),
		filter:      filter,
	}
}

type QueryCallBackFn func(interfaces.EntityID) bool

// Each iterates over all entities that match the query.
// If you would like to stop the iteration, return false to the callback. To continue iterating, return true.
func (q *Query) Each(w interfaces.IWorld, callback QueryCallBackFn) {
	result := q.evaluateQuery(w)
	iter := storage.NewEntityIterator(0, w.StoreManager().GetArchAccessor(), result)
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
func (q *Query) Count(w interfaces.IWorld) int {
	result := q.evaluateQuery(w)
	iter := storage.NewEntityIterator(0, w.StoreManager().GetArchAccessor(), result)
	ret := 0
	for iter.HasNext() {
		entities := iter.Next()
		ret += len(entities)
	}
	return ret
}

// First returns the first entity that matches the query.
func (q *Query) First(w interfaces.IWorld) (id interfaces.EntityID, err error) {
	result := q.evaluateQuery(w)
	iter := storage.NewEntityIterator(0, w.StoreManager().GetArchAccessor(), result)
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

func (q *Query) evaluateQuery(world interfaces.IWorld) []interfaces.ArchetypeID {
	w := Namespace(world.Namespace())
	if _, ok := q.archMatches[w]; !ok {
		q.archMatches[w] = &cache{
			archetypes: make([]interfaces.ArchetypeID, 0),
			seen:       0,
		}
	}
	cache := q.archMatches[w]
	for it := world.StoreManager().GetArchCompIdxStore().SearchFrom(q.filter, cache.seen); it.HasNext(); {
		cache.archetypes = append(cache.archetypes, it.Next())
	}
	cache.seen = world.StoreManager().GetArchAccessor().Count()
	return cache.archetypes
}
