package search

import (
	"slices"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
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
         │ NewSearch()      ├──────────────▶   Entity(...)   ├──────────────▶   Where(...)   │────────┘
  ┌──────┴─────────┐        │    ┌─────────┴─────┐           │ ┌────────────┴──┐             │
  │  Return Type:  ├────────┘    │ Return Type:  ├───────────┘ │ Return Type:  ├─────────────┘
  │searchBuilder   │             │ EntitySearch  │             │ EntitySearch  │     │
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
     │EntitySearch  ├──────────────┤   Type:   │          │                    │                     │
     │or Searchable │              │Searchable │          │                    └─────────────────────┘
     │              │              │           │          │
     │              │              │           │          │
     └──────┬───────┘              └───┬───────┘          │
            │  And, Or, Not Set funcs  │                  │
            │  that take search types  ├──────────────────┘
            │                          │
            │                          │
            │                          │
            │                          │
            └──────────────────────────┘
*/

type searchBuilder interface {
	Entity(componentFilter filter.ComponentFilter) EntitySearch
}

//revive:disable-next-line
type EntitySearch interface {
	Searchable
	Where(componentFilter filterFn) EntitySearch
}

type Searchable interface {
	evaluateSearch(eCtx Context) []types.ArchetypeID
	Each(eCtx Context, callback CallbackFn) error
	First(eCtx Context) (types.EntityID, error)
	MustFirst(eCtx Context) types.EntityID
	Count(eCtx Context) (int, error)
	Collect(eCtx Context) ([]types.EntityID, error)
}

// NewSearch creates a new search.
// It receives arbitrary filters that are used to filter entities.
func NewSearch() searchBuilder {
	return NewLegacySearch(nil).(searchBuilder)
}

// TODO: should deprecate this in the future.
func NewLegacySearch(componentFilter filter.ComponentFilter) EntitySearch {
	return &Search{
		archMatches:             &cache{},
		filter:                  componentFilter,
		componentPropertyFilter: nil,
	}
}

func (s *Search) Entity(componentFilter filter.ComponentFilter) EntitySearch {
	s.filter = componentFilter
	return s
}

// Once the where clause method is activated the search will ONLY return results
// if a where clause returns true and no error.
func (s *Search) Where(componentFilter filterFn) EntitySearch {
	var componentPropertyFilter filterFn
	if s.componentPropertyFilter != nil {
		componentPropertyFilter = AndFilter(s.componentPropertyFilter, componentFilter)
	} else {
		componentPropertyFilter = componentFilter
	}
	return &Search{
		archMatches:             &cache{},
		filter:                  s.filter,
		componentPropertyFilter: componentPropertyFilter,
	}
}

// Each iterates over all entities that match the search.
// If you would like to stop the iteration, return false to the callback. To continue iterating, return true.
func (s *Search) Each(eCtx Context, callback CallbackFn) (err error) {
	defer func() { defer panicOnFatalError(eCtx, err) }()

	result := s.evaluateSearch(eCtx)
	iter := iterators.NewEntityIterator(0, eCtx.StoreReader(), result)
	for iter.HasNext() {
		entities, err := iter.Next()
		if err != nil {
			return err
		}
		for _, id := range entities {
			var filterValue bool
			if s.componentPropertyFilter != nil {
				filterValue, err = s.componentPropertyFilter(eCtx, id)
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

func fastSortIDs(ids []types.EntityID) {
	slices.Sort(ids)
}

func (s *Search) Collect(eCtx Context) ([]types.EntityID, error) {
	acc := make([]types.EntityID, 0)
	err := s.Each(eCtx, func(id types.EntityID) bool {
		acc = append(acc, id)
		return true
	})
	if err != nil {
		return nil, err
	}
	fastSortIDs(acc)
	return acc, nil
}

// Count returns the number of entities that match the search.
func (s *Search) Count(eCtx Context) (ret int, err error) {
	defer func() { defer panicOnFatalError(eCtx, err) }()

	result := s.evaluateSearch(eCtx)
	iter := iterators.NewEntityIterator(0, eCtx.StoreReader(), result)
	for iter.HasNext() {
		entities, err := iter.Next()
		if err != nil {
			return 0, err
		}
		for _, id := range entities {
			var filterValue bool
			if s.componentPropertyFilter != nil {
				filterValue, err = s.componentPropertyFilter(eCtx, id)
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
func (s *Search) First(eCtx Context) (id types.EntityID, err error) {
	defer func() { defer panicOnFatalError(eCtx, err) }()

	result := s.evaluateSearch(eCtx)
	iter := iterators.NewEntityIterator(0, eCtx.StoreReader(), result)
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
				filterValue, err = s.componentPropertyFilter(eCtx, id)
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

func (s *Search) MustFirst(eCtx Context) types.EntityID {
	id, err := s.First(eCtx)
	if err != nil {
		panic("no entity matches the search")
	}
	return id
}

func (s *Search) evaluateSearch(eCtx Context) []types.ArchetypeID {
	cache := s.archMatches
	for it := eCtx.StoreReader().SearchFrom(s.filter, cache.seen); it.HasNext(); {
		cache.archetypes = append(cache.archetypes, it.Next())
	}
	cache.seen = eCtx.StoreReader().ArchetypeCount()
	return cache.archetypes
}
