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
	componentPropertyFilter filterFn
}

// interfaces restrict order of operations.

// Once new search is instantiated the first and only thing you can do is call Entity()
// 1. Call Entity()
// 2. Optionally Call Where() multiple times, Once Entity is called, you can no longer call Entity again.
// 3. Call Each, First, MuchFirst, or Count to evaluate the search or Optionally compose searches with Set operators.
// 4. Once set operators are used the interface settles into Searchable which means, Entity and Where are no longer
// callable.

// In the following example a set operator is used on two different searches.
// Ex1: search.And(NewSearch(wCtx).Entity(..).Where(...), NewSearch(wCtx).Entity(..).Where(...)).Each(...)

// The following example is of a search without set operator
// Ex2: search.And(NewSearch(wCtx).Entity(..).Where(...).Where(...).Each(...)

/*
                                                                                     ┌─────────────────
                                                                                     │                │
                                                                                     ▼                │
         ┌──────────────────┐              ┌─────────────────┐              ┌────────────────┐        │
         │                  │              │                 │              │                │        │
         │ NewSearch(wCtx)  ├──────────────▶   Entity(...)   ├──────────────▶   Where(...)   │────────┘
  ┌──────┴─────────┐        │    ┌─────────┴─────┐           │ ┌────────────┴──┐             │
  │  Return Type:  ├────────┘    │ Return Type:  ├───────────┘ │ Return Type:  ├─────────────┘
  │preSearchBuilder│             │ SearchBuilder │             │ SearchBuilder │     │
  └────────────────┘             └───────────────┘             └───────────────┘     │
                                                                       │             │
                                                                       │             └────┐
                                                                       │                  │
            ┌──────────────────────────────────────────────────────────┘                  │    ┌─────────────┐
            │                                                                             │    │   Methods   │
            │                                                                             ▼    │ return void │
            │                                                                  ┌───────────────┤ or integer  │
            ▼                                                                  │               └─────┬───────┘
     ┌──────────────┐              ┌───────────┐                               │     Each(...),      │
     │              │              │           │                               │     First(...),     │
     │              ◀──────────────┤           │          ┌───────────────────▶│   MustFirst(...),   │
     │ Input type:  │              │  Return   │          │                    │     Count(...)      │
     │SearchBuilder ├──────────────┤   Type:   │          │                    │                     │
     │or Searchable │              │Searchable │          │                    └─────────────────────┘
     │              │              │           │          │
     │              │              │           │          │
     └──────┬───────┘              └───┬───────┘          │
            │  And, Or, Not functions  │                  │
            │  that take search types  ├──────────────────┘
            │                          │
            │                          │
            │                          │
            │                          │
            └──────────────────────────┘
*/

type preSearchBuilder interface {
	Entity(componentFilter filter.ComponentFilter) SearchBuilder
}

//revive:disable-next-line
type SearchBuilder interface {
	Searchable
	Where(componentFilter filterFn) SearchBuilder
}

type Searchable interface {
	evaluateSearch() []types.ArchetypeID
	getEctx() engine.Context
	Each(callback CallbackFn) error
	First() (types.EntityID, error)
	MustFirst() types.EntityID
	Count() (int, error)
	collect() ([]types.EntityID, error)
}

// NewSearch creates a new search.
// It receives arbitrary filters that are used to filter entities.
func NewSearch(wCtx engine.Context) preSearchBuilder {
	return NewLegacySearch(wCtx, nil).(preSearchBuilder)
}

// TODO: should deprecate this in the future.
func NewLegacySearch(wCtx engine.Context, componentFilter filter.ComponentFilter) SearchBuilder {
	return &Search{
		archMatches:             &cache{},
		filter:                  componentFilter,
		namespace:               wCtx.Namespace(),
		reader:                  wCtx.StoreReader(),
		wCtx:                    wCtx,
		componentPropertyFilter: nil,
	}
}

func (s *Search) getEctx() engine.Context {
	return s.wCtx
}

func (s *Search) Entity(componentFilter filter.ComponentFilter) SearchBuilder {
	s.filter = componentFilter
	return s
}

func (s *Search) Where(componentFilter filterFn) SearchBuilder {
	var componentPropertyFilter filterFn
	if s.componentPropertyFilter != nil {
		componentPropertyFilter = AndFilter(s.componentPropertyFilter, componentFilter)
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
				filterValue, err = s.componentPropertyFilter(s.wCtx, id)
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

func (s *Search) collect() ([]types.EntityID, error) {
	acc := make([]types.EntityID, 0)
	err := s.Each(func(id types.EntityID) bool {
		acc = append(acc, id)
		return true
	})
	if err != nil {
		return nil, err
	}
	return acc, nil
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
				filterValue, err = s.componentPropertyFilter(s.wCtx, id)
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
				filterValue, err = s.componentPropertyFilter(s.wCtx, id)
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
