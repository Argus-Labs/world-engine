package ecs

import (
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/iterators"
	"pkg.world.dev/world-engine/cardinal/ecs/store"
	"pkg.world.dev/world-engine/cardinal/types/archetype"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

type cache struct {
	archetypes []archetype.ID
	seen       int
}

// Search represents a search for entities.
// It is used to filter entities based on their components.
// It receives arbitrary filters that are used to filter entities.
// It contains a cache that is used to avoid re-evaluating the search.
// So it is not recommended to create a new search every time you want
// to filter entities with the same search.
type Search struct {
	archMatches map[Namespace]*cache
	filter      filter.ComponentFilter
}

// NewSearch creates a new search.
// It receives arbitrary filters that are used to filter entities.
func NewSearch(filter filter.ComponentFilter) *Search {
	return &Search{
		archMatches: make(map[Namespace]*cache),
		filter:      filter,
	}
}

type SearchCallBackFn func(entity.ID) bool

// Each iterates over all entities that match the search.
// If you would like to stop the iteration, return false to the callback. To continue iterating, return true.
func (q *Search) Each(eCtx EngineContext, callback SearchCallBackFn) error {
	reader := eCtx.StoreReader()
	result := q.evaluateSearch(eCtx.GetEngine().Namespace(), reader)
	iter := iterators.NewEntityIterator(0, reader, result)
	for iter.HasNext() {
		entities, err := iter.Next()
		if err != nil {
			return err
		}
		for _, id := range entities {
			cont := callback(id)
			if !cont {
				return nil
			}
		}
	}
	return nil
}

// Count returns the number of entities that match the search.
func (q *Search) Count(eCtx EngineContext) (int, error) {
	namespace := eCtx.GetEngine().Namespace()
	reader := eCtx.StoreReader()
	result := q.evaluateSearch(namespace, reader)
	iter := iterators.NewEntityIterator(0, reader, result)
	ret := 0
	for iter.HasNext() {
		entities, err := iter.Next()
		if err != nil {
			return 0, err
		}
		ret += len(entities)
	}
	return ret, nil
}

// First returns the first entity that matches the search.
func (q *Search) First(eCtx EngineContext) (id entity.ID, err error) {
	namespace := eCtx.GetEngine().Namespace()
	reader := eCtx.StoreReader()
	result := q.evaluateSearch(namespace, reader)
	iter := iterators.NewEntityIterator(0, reader, result)
	if !iter.HasNext() {
		return iterators.BadID, eris.Wrap(err, "")
	}
	for iter.HasNext() {
		var entities []entity.ID
		entities, err = iter.Next()
		if err != nil {
			return 0, err
		}
		if len(entities) > 0 {
			return entities[0], nil
		}
	}
	return iterators.BadID, eris.Wrap(err, "")
}

func (q *Search) MustFirst(eCtx EngineContext) entity.ID {
	id, err := q.First(eCtx)
	if err != nil {
		panic("no entity matches the search")
	}
	return id
}

func (q *Search) evaluateSearch(namespace Namespace, sm store.Reader) []archetype.ID {
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
