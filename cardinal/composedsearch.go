package cardinal

import (
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/types"
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

func (orSearch *OrSearch) evaluateSearch(wCtx WorldContext) []types.ArchetypeID {
	acc := make([]types.ArchetypeID, 0)
	for _, search := range orSearch.searches {
		acc = append(acc, search.evaluateSearch(wCtx)...)
	}
	return acc
}

func (orSearch *OrSearch) Each(wCtx WorldContext, callback CallbackFn) error {
	// deduplicate
	idCount := make(map[types.EntityID]int)
	for _, search := range orSearch.searches {
		subids, err := search.Collect(wCtx)
		if err != nil {
			return err
		}
		for _, id := range subids {
			idCount[id]++
		}
	}
	idSlice := make([]types.EntityID, 0, len(idCount))
	for id := range idCount {
		idSlice = append(idSlice, id)
	}

	// sort
	fastSortIDs(idSlice)

	// execute
	for _, id := range idSlice {
		if !callback(id) {
			return nil
		}
	}
	return nil
}

func (orSearch *OrSearch) Collect(wCtx WorldContext) ([]types.EntityID, error) {
	// deduplicate
	idExists := make(map[types.EntityID]bool)
	res := make([]types.EntityID, 0)
	for _, search := range orSearch.searches {
		ids, err := search.Collect(wCtx)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			idExists[id] = true
		}
	}
	for id := range idExists {
		res = append(res, id)
	}

	// sort
	fastSortIDs(res)

	return res, nil
}

func (orSearch *OrSearch) First(wCtx WorldContext) (types.EntityID, error) {
	ids, err := orSearch.Collect(wCtx)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, eris.New("No search results")
	}
	return ids[0], nil
}

func (orSearch *OrSearch) MustFirst(wCtx WorldContext) types.EntityID {
	id, err := orSearch.First(wCtx)
	if err != nil {
		panic("no search results")
	}
	return id
}

func (orSearch *OrSearch) Count(wCtx WorldContext) (int, error) {
	ids, err := orSearch.Collect(wCtx)
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (andSearch *AndSearch) Each(wCtx WorldContext, callback CallbackFn) error {
	// count
	idCount := make(map[types.EntityID]int)
	for _, search := range andSearch.searches {
		subIDs, err := search.Collect(wCtx)
		if err != nil {
			return err
		}
		for _, subid := range subIDs {
			idCount[subid]++
		}
	}

	// filter
	idSlice := make([]types.EntityID, 0, len(idCount))
	for id, count := range idCount {
		if count == len(andSearch.searches) {
			idSlice = append(idSlice, id)
		}
	}

	// sort
	fastSortIDs(idSlice)

	// execute
	for _, id := range idSlice {
		if !callback(id) {
			return nil
		}
	}
	return nil
}

func (andSearch *AndSearch) Collect(wCtx WorldContext) ([]types.EntityID, error) {
	// filter
	results := make([]types.EntityID, 0)
	err := andSearch.Each(wCtx, func(id types.EntityID) bool {
		results = append(results, id)
		return true
	})
	if err != nil {
		return nil, err
	}

	// sort
	fastSortIDs(results)

	return results, nil
}

func (andSearch *AndSearch) First(wCtx WorldContext) (types.EntityID, error) {
	ids, err := andSearch.Collect(wCtx)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, eris.New("No search results")
	}
	return ids[0], nil
}

func (andSearch *AndSearch) MustFirst(wCtx WorldContext) types.EntityID {
	id, err := andSearch.First(wCtx)
	if err != nil {
		panic("No search results")
	}
	return id
}

func (andSearch *AndSearch) Count(wCtx WorldContext) (int, error) {
	ids, err := andSearch.Collect(wCtx)
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (andSearch *AndSearch) evaluateSearch(wCtx WorldContext) []types.ArchetypeID {
	searchCounts := make(map[types.ArchetypeID]int)
	for _, search := range andSearch.searches {
		ids := search.evaluateSearch(wCtx)
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

func (notSearch *NotSearch) Each(wCtx WorldContext, callback CallbackFn) error {
	// sort
	ids, err := notSearch.Collect(wCtx)
	if err != nil {
		return err
	}

	// execute
	for _, id := range ids {
		if !callback(id) {
			return nil
		}
	}

	return nil
}

func (notSearch *NotSearch) Collect(wCtx WorldContext) ([]types.EntityID, error) {
	// Get all ids
	allsearch := NewSearch().Entity(filter.All())
	allids, err := allsearch.Collect(wCtx)
	if err != nil {
		return nil, err
	}
	// Get ids to exclude
	excludedIDsMap := make(map[types.EntityID]bool)
	excludedids, err := notSearch.search.Collect(wCtx)
	if err != nil {
		return nil, err
	}
	for _, id := range excludedids {
		excludedIDsMap[id] = true
	}

	// subtract excluded ids from all ids
	result := make([]types.EntityID, 0)
	for _, id := range allids {
		_, ok := excludedIDsMap[id]
		if !ok {
			result = append(result, id)
		}
	}

	// sort ids
	fastSortIDs(result)

	return result, nil
}

func (notSearch *NotSearch) First(wCtx WorldContext) (types.EntityID, error) {
	ids, err := notSearch.Collect(wCtx)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, eris.New("No results found")
	}
	return ids[0], nil
}

func (notSearch *NotSearch) MustFirst(wCtx WorldContext) types.EntityID {
	id, err := notSearch.First(wCtx)
	if err != nil {
		panic("No search results")
	}
	return id
}

func (notSearch *NotSearch) Count(wCtx WorldContext) (int, error) {
	ids, err := notSearch.Collect(wCtx)
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

func (notSearch *NotSearch) evaluateSearch(wCtx WorldContext) []types.ArchetypeID {
	searchBuilder := NewSearch()
	allResults := searchBuilder.Entity(filter.All()).evaluateSearch(wCtx)
	allResultsMap := make(map[types.ArchetypeID]bool)
	for _, result := range allResults {
		allResultsMap[result] = true
	}
	subResults := notSearch.search.evaluateSearch(wCtx)
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
