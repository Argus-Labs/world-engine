package search

import (
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type CallbackFn func(types.EntityID) bool

type cache struct {
	archetypes []types.ArchetypeID
	seen       int
}

// Search represents a search for entities.
// It is used to filter entities based on their components.
// It receives arbitrary filters that are used to filter entities.
// It contains a cache that is used to avoid re-evaluating the search.
// So it is not recommended to create a new search every time you want
// to filter entities with the same search.
type Search struct {
	archMatches             *cache
	filter                  filter.ComponentFilter
	namespace               string
	reader                  gamestate.Reader
	wCtx                    engine.Context
	componentPropertyFilter PredicateEvaluator
}

// NewSearch creates a new search.
// It receives arbitrary filters that are used to filter entities.
func NewSearch(wCtx engine.Context) *Search {
	return NewSearchWithFilter(wCtx, nil)
}

func NewSearchWithFilter(wCtx engine.Context, componentFilter filter.ComponentFilter) *Search {
	return &Search{
		archMatches:             &cache{},
		filter:                  componentFilter,
		namespace:               wCtx.Namespace(),
		reader:                  wCtx.StoreReader(),
		wCtx:                    wCtx,
		componentPropertyFilter: nil,
	}
}

func (s *Search) Contains(component ...componentWrapper) *Search {
	if s.filter != nil {
		panic("Search already contains a filter.")
	}
	acc := make([]types.Component, 0, len(component))
	for _, comp := range component {
		acc = append(acc, comp.component)
	}
	return &Search{
		archMatches:             &cache{},
		filter:                  filter.Contains(acc...),
		namespace:               s.namespace,
		reader:                  s.reader,
		wCtx:                    s.wCtx,
		componentPropertyFilter: s.componentPropertyFilter,
	}
}

func (s *Search) All() *Search {
	return &Search{
		archMatches:             &cache{},
		filter:                  filter.All(),
		namespace:               s.namespace,
		reader:                  s.reader,
		wCtx:                    s.wCtx,
		componentPropertyFilter: s.componentPropertyFilter,
	}
}

func (s *Search) Exact(component ...componentWrapper) *Search {
	if s.filter != nil {
		panic("Search already contains a filter.")
	}
	acc := make([]types.Component, 0, len(component))
	for _, comp := range component {
		acc = append(acc, comp.component)
	}
	return &Search{
		archMatches:             &cache{},
		filter:                  filter.Exact(acc...),
		namespace:               s.namespace,
		reader:                  s.reader,
		wCtx:                    s.wCtx,
		componentPropertyFilter: s.componentPropertyFilter,
	}
}

func (s *Search) Where(componentFilter PredicateEvaluator) *Search {
	var componentPropertyFilter PredicateEvaluator
	if s.componentPropertyFilter != nil {
		componentPropertyFilter = &andFilterComponent{filterComponents: []PredicateEvaluator{
			s.componentPropertyFilter, componentFilter,
		}}
	} else {
		componentPropertyFilter = componentFilter
	}
	return &Search{
		archMatches:             &cache{},
		filter:                  s.filter,
		namespace:               s.namespace,
		reader:                  s.reader,
		wCtx:                    s.wCtx,
		componentPropertyFilter: componentPropertyFilter,
	}
}

// Each iterates over all entities that match the search.
// If you would like to stop the iteration, return false to the callback. To continue iterating, return true.
func (s *Search) Each(callback CallbackFn) (err error) {
	defer func() { defer panicOnFatalError(s.wCtx, err) }()

	result := s.evaluateSearch()
	iter := iterators.NewEntityIterator(0, s.reader, result)
	for iter.HasNext() {
		entities, err := iter.Next()
		if err != nil {
			return err
		}
		for _, id := range entities {
			var filterValue bool
			if s.componentPropertyFilter != nil {
				filterValue, err = s.componentPropertyFilter.Evaluate(s.wCtx, id)
				if err != nil {
					continue
				}
			} else {
				filterValue = true
			}

			if filterValue {
				cont := callback(id)
				if !cont {
					return nil
				}
			}
		}
	}
	return nil
}

// Count returns the number of entities that match the search.
func (s *Search) Count() (ret int, err error) {
	defer func() { defer panicOnFatalError(s.wCtx, err) }()

	result := s.evaluateSearch()
	iter := iterators.NewEntityIterator(0, s.reader, result)
	for iter.HasNext() {
		entities, err := iter.Next()
		if err != nil {
			return 0, err
		}
		for _, id := range entities {
			var filterValue bool
			if s.componentPropertyFilter != nil {
				filterValue, err = s.componentPropertyFilter.Evaluate(s.wCtx, id)
				if err != nil {
					continue
				}
			} else {
				filterValue = true
			}
			if filterValue {
				ret++
			}
		}
	}
	return ret, nil
}

// First returns the first entity that matches the search.
func (s *Search) First() (id types.EntityID, err error) {
	defer func() { defer panicOnFatalError(s.wCtx, err) }()

	result := s.evaluateSearch()
	iter := iterators.NewEntityIterator(0, s.reader, result)
	if !iter.HasNext() {
		return iterators.BadID, eris.Wrap(err, "")
	}
	for iter.HasNext() {
		entities, err := iter.Next()
		if err != nil {
			return 0, err
		}
		for _, id := range entities {
			var filterValue bool
			if s.componentPropertyFilter != nil {
				filterValue, err = s.componentPropertyFilter.Evaluate(s.wCtx, id)
				if err != nil {
					continue
				}
			} else {
				filterValue = true
			}
			if filterValue {
				return id, nil
			}
		}
	}
	return iterators.BadID, eris.Wrap(err, "")
}

func (s *Search) MustFirst() types.EntityID {
	id, err := s.First()
	if err != nil {
		panic("no entity matches the search")
	}
	return id
}

func (s *Search) evaluateSearch() []types.ArchetypeID {
	cache := s.archMatches
	for it := s.reader.SearchFrom(s.filter, cache.seen); it.HasNext(); {
		cache.archetypes = append(cache.archetypes, it.Next())
	}
	cache.seen = s.reader.ArchetypeCount()
	return cache.archetypes
}

func (s *Search) And(otherSearch *Search) *Search {
	var componentPropertyFilter PredicateEvaluator
	if s.componentPropertyFilter != nil && otherSearch.componentPropertyFilter != nil {
		componentPropertyFilter = &andFilterComponent{filterComponents: []PredicateEvaluator{
			s.componentPropertyFilter, otherSearch.componentPropertyFilter,
		}}
	} else {
		componentPropertyFilter = otherSearch.componentPropertyFilter
	}

	return &Search{
		archMatches:             &cache{},
		filter:                  filter.And(s.filter, otherSearch.filter),
		namespace:               s.namespace,
		reader:                  s.reader,
		wCtx:                    s.wCtx,
		componentPropertyFilter: componentPropertyFilter,
	}
}

func (s *Search) Or(otherSearch *Search) *Search {
	var componentPropertyFilter PredicateEvaluator
	if s.componentPropertyFilter != nil && otherSearch.componentPropertyFilter != nil {
		componentPropertyFilter = &orFilterComponent{filterComponents: []PredicateEvaluator{
			s.componentPropertyFilter, otherSearch.componentPropertyFilter,
		}}
	} else {
		componentPropertyFilter = otherSearch.componentPropertyFilter
	}

	return &Search{
		archMatches:             &cache{},
		filter:                  filter.Or(s.filter, otherSearch.filter),
		namespace:               s.namespace,
		reader:                  s.reader,
		wCtx:                    s.wCtx,
		componentPropertyFilter: componentPropertyFilter,
	}
}

func (s *Search) Not() *Search {
	var componentPropertyFilter PredicateEvaluator
	if s.componentPropertyFilter != nil {
		componentPropertyFilter = &notFilterComponent{filterComponent: s.componentPropertyFilter}
	} else {
		componentPropertyFilter = nil
	}
	return &Search{
		archMatches:             &cache{},
		filter:                  filter.Not(s.filter),
		namespace:               s.namespace,
		reader:                  s.reader,
		wCtx:                    s.wCtx,
		componentPropertyFilter: componentPropertyFilter}
}
