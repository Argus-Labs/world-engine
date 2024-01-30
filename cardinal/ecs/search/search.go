package search

import (
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/gamestate"
	"pkg.world.dev/world-engine/cardinal/ecs/iterators"
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
	archMatches map[string]*cache
	filter      filter.ComponentFilter
	namespace   string
	reader      gamestate.Reader
}

// NewSearch creates a new search.
// It receives arbitrary filters that are used to filter entities.
func NewSearch(filter filter.ComponentFilter, namespace string, reader gamestate.Reader) *Search {
	return &Search{
		archMatches: make(map[string]*cache),
		filter:      filter,
		namespace:   namespace,
		reader:      reader,
	}
}

type CallbackFn func(entity.ID) bool

// Each iterates over all entities that match the search.
// If you would like to stop the iteration, return false to the callback. To continue iterating, return true.
func (s *Search) Each(callback CallbackFn) error {
	result := s.evaluateSearch()
	iter := iterators.NewEntityIterator(0, s.reader, result)
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
func (s *Search) Count() (int, error) {
	result := s.evaluateSearch()
	iter := iterators.NewEntityIterator(0, s.reader, result)
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
func (s *Search) First() (id entity.ID, err error) {
	result := s.evaluateSearch()
	iter := iterators.NewEntityIterator(0, s.reader, result)
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

func (s *Search) MustFirst() entity.ID {
	id, err := s.First()
	if err != nil {
		panic("no entity matches the search")
	}
	return id
}

func (s *Search) evaluateSearch() []archetype.ID {
	if _, ok := s.archMatches[s.namespace]; !ok {
		s.archMatches[s.namespace] = &cache{
			archetypes: make([]archetype.ID, 0),
			seen:       0,
		}
	}
	cache := s.archMatches[s.namespace]
	for it := s.reader.SearchFrom(s.filter, cache.seen); it.HasNext(); {
		cache.archetypes = append(cache.archetypes, it.Next())
	}
	cache.seen = s.reader.ArchetypeCount()
	return cache.archetypes
}
