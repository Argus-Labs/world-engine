package search

import (
	"errors"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type OrSearch struct {
	searches []Searchable
}

type AndSearch struct {
	searches []Searchable
}

type NotSearch struct {
	search Searchable
}

func (orSearch *OrSearch) evaluateSearch(eCtx engine.Context) []types.ArchetypeID {
	acc := make([]types.ArchetypeID, 0)
	for _, search := range orSearch.searches {
		acc = append(acc, search.evaluateSearch(eCtx)...)
	}
	return acc
}

func (orSearch *OrSearch) Each(eCtx engine.Context, callback CallbackFn) error {
	var err error
	for _, search := range orSearch.searches {
		err = errors.Join(err, search.Each(eCtx, callback))
		if err != nil {
			return err
		}
	}
	return nil
}

func (orSearch *OrSearch) collect(eCtx engine.Context) ([]types.EntityID, error) {
	resMap := make(map[types.EntityID]bool)
	for _, search := range orSearch.searches {
		ids, err := search.collect(eCtx)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			resMap[id] = true
		}
	}
	res := make([]types.EntityID, 0)
	for id := range resMap {
		res = append(res, id)
	}

	return res, nil
}

func (orSearch *OrSearch) First(eCtx engine.Context) (types.EntityID, error) {
	ids, err := orSearch.collect(eCtx)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, eris.New("No search results")
	}
	return ids[0], nil
}

func (orSearch *OrSearch) MustFirst(eCtx engine.Context) types.EntityID {
	id, err := orSearch.First(eCtx)
	if err != nil {
		panic("no search results")
	}
	return id
}

func (orSearch *OrSearch) Count(eCtx engine.Context) (int, error) {
	ids, err := orSearch.collect(eCtx)
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (andSearch *AndSearch) Each(eCtx engine.Context, callback CallbackFn) error {
	ids := make(map[types.EntityID]int)
	for _, search := range andSearch.searches {
		subIDs, err := search.collect(eCtx)
		if err != nil {
			return err
		}
		for _, subid := range subIDs {
			ids[subid]++
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

func (andSearch *AndSearch) collect(eCtx engine.Context) ([]types.EntityID, error) {
	results := make([]types.EntityID, 0)
	err := andSearch.Each(eCtx, func(id types.EntityID) bool {
		results = append(results, id)
		return true
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (andSearch *AndSearch) First(eCtx engine.Context) (types.EntityID, error) {
	ids, err := andSearch.collect(eCtx)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, eris.New("No search results")
	}
	return ids[0], nil
}

func (andSearch *AndSearch) MustFirst(eCtx engine.Context) types.EntityID {
	id, err := andSearch.First(eCtx)
	if err != nil {
		panic("No search results")
	}
	return id
}

func (andSearch *AndSearch) Count(eCtx engine.Context) (int, error) {
	ids, err := andSearch.collect(eCtx)
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (andSearch *AndSearch) evaluateSearch(eCtx engine.Context) []types.ArchetypeID {
	searchCounts := make(map[types.ArchetypeID]int)
	for _, search := range andSearch.searches {
		ids := search.evaluateSearch(eCtx)
		for _, id := range ids {
			searchCounts[id]++
		}
	}
	acc := make([]types.ArchetypeID, 0)
	for key, searchCount := range searchCounts {
		if searchCount == len(andSearch.searches) {
			acc = append(acc, key)
		}
	}
	return acc
}

func (notSearch *NotSearch) Each(eCtx engine.Context, callback CallbackFn) error {
	ids, err := notSearch.collect(eCtx)
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

func (notSearch *NotSearch) collect(eCtx engine.Context) ([]types.EntityID, error) {
	allsearch := NewSearch().Entity(filter.All())
	allids, err := allsearch.collect(eCtx)
	if err != nil {
		return nil, err
	}
	excludedIDsMap := make(map[types.EntityID]bool)
	excludedids, err := notSearch.search.collect(eCtx)
	if err != nil {
		return nil, err
	}
	for _, id := range excludedids {
		excludedIDsMap[id] = true
	}
	result := make([]types.EntityID, 0)
	for _, id := range allids {
		_, ok := excludedIDsMap[id]
		if !ok {
			result = append(result, id)
		}
	}
	return result, nil
}

func (notSearch *NotSearch) First(eCtx engine.Context) (types.EntityID, error) {
	ids, err := notSearch.collect(eCtx)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, eris.New("No results found")
	}
	return ids[0], nil
}

func (notSearch *NotSearch) MustFirst(eCtx engine.Context) types.EntityID {
	id, err := notSearch.First(eCtx)
	if err != nil {
		panic("No search results")
	}
	return id
}

func (notSearch *NotSearch) Count(eCtx engine.Context) (int, error) {
	ids, err := notSearch.collect(eCtx)
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (notSearch *NotSearch) evaluateSearch(eCtx engine.Context) []types.ArchetypeID {
	searchBuilder := NewSearch()
	allResults := searchBuilder.Entity(filter.All()).evaluateSearch(eCtx)
	allResultsMap := make(map[types.ArchetypeID]bool)
	for _, result := range allResults {
		allResultsMap[result] = true
	}
	subResults := notSearch.search.evaluateSearch(eCtx)
	finalResult := make([]types.ArchetypeID, 0)
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
