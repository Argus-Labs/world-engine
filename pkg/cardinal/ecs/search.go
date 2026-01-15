package ecs

import (
	"iter"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
)

// SearchParam contains paramters for a search query.
// We use expr lang for the where clause to filter the entities, please refer to its documentation
// for more details: https://expr-lang.org/docs/getting-started.
type SearchParam struct {
	Find   []string    // List of component names to search for. Empty = all entities.
	Match  SearchMatch // A match type to use for the search. Ignored when Find is empty.
	Where  string      // Optional expr language string to filter the results.
	Limit  int         // Maximum number of results to return (default: 50, 0 = use default, max: 10000)
	Offset int         // Number of results to skip before returning (default: 0, min: 0)
}

// validateAndGetFilter validates the search parameters and returns an expr VM program compiled
// from the where clause.
func (s *SearchParam) validateAndGetFilter() (*vm.Program, error) {
	// Only validate Match when Find is non-empty (Match is ignored when Find is empty)
	if len(s.Find) > 0 {
		if s.Match != MatchExact && s.Match != MatchContains {
			return nil, eris.Errorf("invalid `match` value: must be either '%s' or '%s'", MatchExact, MatchContains)
		}
	}

	// Validate Limit
	if s.Limit < MinQueryLimit {
		return nil, eris.Errorf("limit must be >= %d, got %d", MinQueryLimit, s.Limit)
	}
	if s.Limit > MaxQueryLimit {
		return nil, eris.Errorf("limit must be <= %d, got %d", MaxQueryLimit, s.Limit)
	}

	// Validate Offset
	if s.Offset < MinQueryOffset {
		return nil, eris.Errorf("offset must be >= %d, got %d", MinQueryOffset, s.Offset)
	}

	var filter *vm.Program

	// If no expression is provided, return a nil program
	if len(s.Where) == 0 {
		return filter, nil
	}

	// Compile the expression and check that the return type is boolean.
	filter, err := expr.Compile(s.Where, expr.AsBool())
	if err != nil {
		return nil, eris.Wrap(err, "failed to parse where clause")
	}

	return filter, nil
}

// SearchMatch is the type of match to use for the search.
type SearchMatch string

const (
	// MatchExact matches entities that have exactly the specified components.
	MatchExact SearchMatch = "exact"
	// MatchContains matches entities that contains the specified components, but may have other
	// components as well.
	MatchContains SearchMatch = "contains"
)

// Query limit constants.
const (
	// DefaultQueryLimit is the default maximum number of results returned when Limit is 0 or not specified.
	DefaultQueryLimit = 50
	// MaxQueryLimit is the maximum allowed limit value to prevent excessive memory usage.
	MaxQueryLimit = 10000
	// MinQueryLimit is the minimum allowed limit value (0 means use default).
	MinQueryLimit = 0
	// MinQueryOffset is the minimum allowed offset value.
	MinQueryOffset = 0
)

// buildEntityResult creates a result map from an entity ID and its components.
func buildEntityResult(eid EntityID, components []Component) map[string]any {
	result := make(map[string]any)
	// We have to cast id from EntityID to int here or else we can't query the data because for some
	// reason expr can't compare EntityID with integers in the expression.
	result["_id"] = uint32(eid)

	for _, component := range components {
		result[component.Name()] = component
	}

	return result
}

// matchesFilter checks if an entity matches the filter expression.
func matchesFilter(filter *vm.Program, result map[string]any) (bool, error) {
	// Run the filter expression. We set the entity map as the environment for `Run` so the vm
	// program has access to the entity data to filter.
	output, err := expr.Run(filter, result)
	if err != nil {
		return false, eris.Wrap(err, "failed to run filter expression")
	}

	isMatchFilter, ok := output.(bool)
	// Because we compile the expr once without passing in the environment, as it's only available
	// while iterating, expr.Compile can't fully check if the expression returns a bool,
	// especially when we filter for a struct field e.g. health.hp > 200, expr can't determine the
	// type of health.hp during compilation.
	if !ok {
		return false, eris.New("invalid where clause")
	}

	return isMatchFilter, nil
}

// normalizeLimit applies default limit and enforces maximum limit.
func normalizeLimit(limit int) int {
	if limit == 0 {
		limit = DefaultQueryLimit
	}
	if limit > MaxQueryLimit {
		limit = MaxQueryLimit
	}
	return limit
}

// processEntity processes a single entity and adds it to results if it matches the criteria.
// Returns (shouldContinue, done) where shouldContinue indicates if we should continue processing
// and done indicates if we've reached the limit.
func processEntity(
	result map[string]any,
	filter *vm.Program,
	offset int,
	skipped *int,
	results *[]map[string]any,
	collected *int,
	limit int,
) (bool, error) {
	// Apply Where filter if present
	if filter != nil {
		matches, err := matchesFilter(filter, result)
		if err != nil {
			return false, err
		}
		if !matches {
			return true, nil // Continue processing
		}
	}

	// Apply offset: skip first N matching entities
	if *skipped < offset {
		*skipped++
		return true, nil // Continue processing
	}

	// Add to results
	*results = append(*results, result)
	*collected++

	// Check if we've reached the limit
	return *collected < limit, nil
}

// NewSearch returns a map of entities that match the given search parameters.
func (w *World) NewSearch(params SearchParam) ([]map[string]any, error) {
	filter, err := params.validateAndGetFilter()
	if err != nil {
		return nil, eris.Wrap(err, "invalid search params")
	}

	archetypeIDs, err := findMatchingArchetypes(w, params.Find, params.Match)
	if err != nil {
		return nil, eris.Wrap(err, "failed to get archetypes from components")
	}

	limit := normalizeLimit(params.Limit)
	results := make([]map[string]any, 0, limit)
	skipped := 0
	collected := 0

	for _, id := range archetypeIDs {
		arch := w.state.archetypes[id]

		for eid, components := range archIter(arch) {
			result := buildEntityResult(eid, components)

			shouldContinue, err := processEntity(
				result, filter, params.Offset, &skipped, &results, &collected, limit,
			)
			if err != nil {
				return nil, err
			}
			if !shouldContinue {
				return results, nil
			}
		}
	}

	return results, nil
}

// findMatchingArchetypes returns the archetypes that match the given components and match type.
// When compNames is empty, returns all archetype IDs.
func findMatchingArchetypes(w *World, compNames []string, match SearchMatch) ([]archetypeID, error) {
	ws := w.state

	// If compNames is empty, return all archetype IDs
	if len(compNames) == 0 {
		archIDs := make([]archetypeID, 0, len(ws.archetypes))
		for id := range ws.archetypes {
			archIDs = append(archIDs, id)
		}
		return archIDs, nil
	}

	// Build component bitmap from names
	component := bitmap.Bitmap{}
	for _, name := range compNames {
		id, exists := ws.components.catalog[name]
		if !exists {
			return nil, eris.Errorf("component %s not registered", name)
		}
		component.Set(id)
	}

	// Find matching archetypes based on match type
	var archIDs []int
	switch match {
	case MatchExact:
		aid, ok := ws.archExact(component)
		if ok {
			archIDs = []int{aid}
		}
	case MatchContains:
		archIDs = ws.archContains(component)
	}
	return archIDs, nil
}

// archIter returns an iterator of the archetypes entities and its components.
func archIter(a *archetype) iter.Seq2[EntityID, []Component] {
	return func(yield func(EntityID, []Component) bool) {
		for row := range a.entities {
			eid := a.entities[row]

			components := make([]Component, 0, a.compCount)
			for _, column := range a.columns {
				component := column.getAbstract(row)
				components = append(components, component)
			}

			if !yield(eid, components) {
				return
			}
		}
	}
}
