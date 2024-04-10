package search

import (
	"errors"

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

type SearchBuiler interface {
	Searchable
	Entity(componentFilter filter.ComponentFilter) SearchBuiler
	Where(componentFilter filterFn) SearchBuiler
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
func NewSearch(wCtx engine.Context) *Search {
	return NewLegacySearch(wCtx, nil)
}

// TODO: should deprecate this in the future.
func NewLegacySearch(wCtx engine.Context, componentFilter filter.ComponentFilter) *Search {
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

func (s *Search) Entity(componentFilter filter.ComponentFilter) SearchBuiler {
	s.filter = componentFilter
	return s
}

func (s *Search) Where(componentFilter filterFn) SearchBuiler {
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
	acc := make([]types.EntityID, 0, 0)
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

type OrSearch struct {
	searches []Searchable
}

func (orSearch *OrSearch) evaluateSearch() []types.ArchetypeID {
	acc := make([]types.ArchetypeID, 0, 0)
	for _, search := range orSearch.searches {
		acc = append(acc, search.evaluateSearch()...)
	}
	return acc
}

func (orSearch *OrSearch) Each(callback CallbackFn) error {
	var err error = nil
	for _, search := range orSearch.searches {
		err = errors.Join(err, search.Each(callback))
		if err != nil {
			return err
		}
	}
	return nil
}

func (orSearch *OrSearch) collect() ([]types.EntityID, error) {
	resMap := make(map[types.EntityID]bool)
	for _, search := range orSearch.searches {
		ids, err := search.collect()
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			resMap[id] = true
		}
	}
	res := make([]types.EntityID, 0, 0)
	for id, _ := range resMap {
		res = append(res, id)
	}

	return res, nil
}

func (orSearch *OrSearch) First() (types.EntityID, error) {
	ids, err := orSearch.collect()
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, eris.New("No search results")
	}
	return ids[0], nil
}

func (orSearch *OrSearch) MustFirst() types.EntityID {
	id, err := orSearch.First()
	if err != nil {
		panic("no search results")
	}
	return id
}

func (orSearch *OrSearch) Count() (int, error) {
	ids, err := orSearch.collect()
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (orSearch *OrSearch) getEctx() engine.Context {
	return orSearch.searches[0].getEctx()
}

type AndSearch struct {
	searches []Searchable
}

func (andSearch *AndSearch) Each(callback CallbackFn) error {
	ids := make(map[types.EntityID]int)
	for _, search := range andSearch.searches {
		subIds, err := search.collect()
		if err != nil {
			return err
		}
		for _, subid := range subIds {
			v, ok := ids[subid]
			if !ok {
				ids[subid] = 1
			} else {
				ids[subid] = v + 1
			}
		}
	}
	for k, v := range ids {
		if v == len(andSearch.searches) {
			if !callback(k) {
				return nil
			}
		}
	}
	return nil
}

func (andSearch *AndSearch) collect() ([]types.EntityID, error) {
	results := make([]types.EntityID, 0, 0)
	err := andSearch.Each(func(id types.EntityID) bool {
		results = append(results, id)
		return false
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (andSearch *AndSearch) First() (types.EntityID, error) {
	ids, err := andSearch.collect()
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, eris.New("No search results")
	}
	return 0, nil
}

func (andSearch *AndSearch) MustFirst() types.EntityID {
	id, err := andSearch.First()
	if err != nil {
		panic("No search results")
	}
	return id
}

func (andSearch *AndSearch) Count() (int, error) {
	ids, err := andSearch.collect()
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (andSearch *AndSearch) evaluateSearch() []types.ArchetypeID {
	searchCounts := make(map[types.ArchetypeID]int)
	for _, search := range andSearch.searches {
		ids := search.evaluateSearch()
		for _, id := range ids {
			v, ok := searchCounts[id]
			if !ok {
				searchCounts[id] = 0
			} else {
				searchCounts[id] = v + 1
			}
		}
	}
	acc := make([]types.ArchetypeID, 0, 0)
	for key, searchCount := range searchCounts {
		if searchCount == len(andSearch.searches) {
			acc = append(acc, key)
		}
	}
	return acc
}

func (andSearch AndSearch) getEctx() engine.Context {
	return andSearch.searches[0].getEctx()
}

type NotSearch struct {
	search Searchable
}

func (notSearch *NotSearch) Each(callback CallbackFn) error {
	ids, err := notSearch.collect()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if !callback(id) {
			return nil
		}
	}
	return nil
}

func (notSearch *NotSearch) collect() ([]types.EntityID, error) {
	allsearch := NewSearch(notSearch.getEctx()).Entity(filter.All())
	allids, err := allsearch.collect()
	if err != nil {
		return nil, err
	}
	excludedIdsMap := make(map[types.EntityID]bool)
	excludedids, err := notSearch.search.collect()
	if err != nil {
		return nil, err
	}
	for _, id := range excludedids {
		excludedIdsMap[id] = true
	}
	result := make([]types.EntityID, 0, 0)
	for _, id := range allids {
		_, ok := excludedIdsMap[id]
		if !ok {
			result = append(result, id)
		}
	}
	return result, nil
}

func (notSearch *NotSearch) First() (types.EntityID, error) {
	ids, err := notSearch.collect()
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, eris.New("No results found")
	}
	return ids[0], nil
}

func (notSearch *NotSearch) MustFirst() types.EntityID {
	id, err := notSearch.First()
	if err != nil {
		panic("No search results")
	}
	return id
}

func (notSearch *NotSearch) Count() (int, error) {
	ids, err := notSearch.collect()
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (notSearch *NotSearch) getEctx() engine.Context {
	return notSearch.search.getEctx()
}

func (notSearch *NotSearch) evaluateSearch() []types.ArchetypeID {
	searchBuilder := NewSearch(notSearch.getEctx())
	allResults := searchBuilder.Entity(filter.All()).evaluateSearch()
	allResultsMap := make(map[types.ArchetypeID]bool)
	for _, result := range allResults {
		allResultsMap[result] = true
	}
	subResults := notSearch.evaluateSearch()
	finalResult := make([]types.ArchetypeID, 0, 0)
	for _, subResult := range subResults {
		_, ok := allResultsMap[subResult]
		if ok {
			finalResult = append(finalResult, subResult)
		}
	}
	return finalResult
}

func Or(searches ...Searchable) Searchable {
	return &OrSearch{searches: searches}
}

func And(searches ...Searchable) Searchable {
	return &AndSearch{searches: searches}
}

func Not(search Searchable) Searchable {
	return &NotSearch{search: search}
}
