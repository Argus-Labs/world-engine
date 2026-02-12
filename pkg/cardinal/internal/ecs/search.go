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
	Find   []string    // List of component names to search for. Must be empty when Match is MatchAll.
	Match  SearchMatch // A match type to use for the search.
	Where  string      // Optional expr language string to filter the results.
	Limit  uint32      // Maximum number of results to return (default: unlimited, 0 = unlimited)
	Offset uint32      // Number of results to skip before returning (default: 0)
}

// SearchMatch is the type of match to use for the search.
type SearchMatch string

const (
	// MatchExact matches entities that have exactly the specified components.
	MatchExact SearchMatch = "exact"
	// MatchContains matches entities that contains the specified components, but may have other
	// components as well.
	MatchContains SearchMatch = "contains"
	// MatchAll matches all entities regardless of components. Find must be empty when using this.
	MatchAll SearchMatch = "all"
)

// DefaultQueryLimit is the default maximum number of results returned when Limit is 0 (unlimited).
const DefaultQueryLimit = ^uint32(0) // Max uint32 (4294967295)

// NewSearch returns a map of entities that match the given search parameters.
//
//nolint:gocognit // Complexity from sequential filtering (where, offset, limit) kept together for readability.
func (w *World) NewSearch(params SearchParam) ([]map[string]any, error) {
	filter, err := params.validateAndGetFilter()
	if err != nil {
		return nil, eris.Wrap(err, "invalid search params")
	}

	archetypeIDs, err := findMatchingArchetypes(w, params.Find, params.Match)
	if err != nil {
		return nil, eris.Wrap(err, "failed to get archetypes from components")
	}

	limit := params.Limit
	if limit == 0 {
		limit = DefaultQueryLimit
	}

	results := make([]map[string]any, 0)
	skipped := uint32(0)
	collected := uint32(0)

	for _, id := range archetypeIDs {
		arch := w.state.archetypes[id]

		for eid, components := range archIter(arch) {
			result := buildEntityResult(eid, components)

			// Apply Where filter if present
			if filter != nil {
				matches, err := matchesFilter(filter, result)
				if err != nil {
					return nil, err
				}
				if !matches {
					continue // Skip this entity
				}
			}

			// Apply offset: skip first N matching entities
			if skipped < params.Offset {
				skipped++
				continue
			}

			// Add to results
			results = append(results, result)
			collected++

			// Early termination: stop when limit is reached
			if collected >= limit {
				return results, nil
			}
		}
	}

	return results, nil
}

// validateAndGetFilter validates the search parameters and returns an expr VM program compiled
// from the where clause.
func (s *SearchParam) validateAndGetFilter() (*vm.Program, error) {
	// Validate Match and Find relationship
	if s.Match == MatchAll {
		if len(s.Find) > 0 {
			return nil, eris.New("find must be empty when match is 'all'")
		}
	} else {
		if len(s.Find) == 0 {
			return nil, eris.New("find must not be empty when match is not 'all'")
		}
		if s.Match != MatchExact && s.Match != MatchContains {
			return nil, eris.Errorf("invalid `match` value: must be either '%s' or '%s'", MatchExact, MatchContains)
		}
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

// findMatchingArchetypes returns the archetypes that match the given components and match type.
func findMatchingArchetypes(w *World, compNames []string, match SearchMatch) ([]archetypeID, error) {
	ws := w.state

	// If match is MatchAll, return all archetype IDs
	if match == MatchAll {
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
	case MatchAll:
		// This case should never be reached as MatchAll is handled earlier in the function
		// but included for exhaustive switch coverage
		return nil, eris.New("MatchAll should be handled before this switch")
	}
	return archIDs, nil
}

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
