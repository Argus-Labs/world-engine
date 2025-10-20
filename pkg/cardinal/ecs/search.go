package ecs

import (
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
)

// SearchParam contains paramters for a search query.
// We use expr lang for the where clause to filter the entities, please refer to its documentation
// for more details: https://expr-lang.org/docs/getting-started.
type SearchParam struct {
	Find  []string    // List of component names to search for
	Match SearchMatch // A match type to use for the search
	Where string      // Optional expr language string to filter the results.
}

// validateAndGetFilter validates the search parameters and returns an expr VM program compiled
// from the where clause.
func (s *SearchParam) validateAndGetFilter() (*vm.Program, error) {
	if len(s.Find) == 0 {
		return nil, eris.New("component list cannot be empty")
	}

	if s.Match != MatchExact && s.Match != MatchContains {
		return nil, eris.Errorf("invalid `match` value: must be either '%s' or '%s'", MatchExact, MatchContains)
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

// NewSearch returns a map of entities that match the given search parameters.
func (w *World) NewSearch(params SearchParam) ([]map[string]any, error) {
	filter, err := params.validateAndGetFilter()
	if err != nil {
		return nil, eris.Wrap(err, "invalid search params")
	}

	archs, err := getArchetypes(w, params.Find, params.Match)
	if err != nil {
		return nil, eris.Wrap(err, "failed to get archetypes from components")
	}

	results := make([]map[string]any, 0)
	for _, arch := range archs {
		if arch == nil {
			continue
		}

		isStopIterating := false
		arch.entities.Range(func(id uint32) {
			if isStopIterating {
				return
			}
			entityMap := EntityID(id).toMap(arch)

			// If there's no filter, include all entities.
			if filter == nil {
				results = append(results, entityMap)
				return
			}

			// Run the filter expression. We set the entity map as the environment for `Run` so the vm
			// program has access to the entity data to filter.
			output, innerErr := expr.Run(filter, entityMap)
			if innerErr != nil {
				err = eris.Wrap(innerErr, "failed to run filter expression")
				isStopIterating = true
				return
			}

			isMatchFilter, ok := output.(bool)
			// Because we compile the expr once without passing in the environment, as it's only available
			// while iterating, expr.Compile can't fully check if the expression returns a bool,x
			// especially when we filter for a struct field e.g. health.hp > 200, expr can't determine the
			// type of health.hp during compilation.
			if !ok {
				err = eris.New("invalid where clause")
				isStopIterating = true
				return
			}

			if isMatchFilter {
				results = append(results, entityMap)
			}
		})
	}

	return results, nil
}

// getArchetypes returns the archetypes that match the given components and match type.
func getArchetypes(w *World, compNames []string, match SearchMatch) ([]*archetype, error) {
	if len(compNames) == 0 {
		return nil, eris.New("component list cannot be empty")
	}

	component := bitmap.Bitmap{}
	for _, name := range compNames {
		id, exists := w.components.registry[name]
		if !exists {
			return nil, eris.Errorf("component %s not registered", name)
		}
		component.Set(id)
	}

	var archs []*archetype
	switch match {
	case MatchExact:
		arch := w.getState().archExact(component)
		if arch != nil {
			archs = []*archetype{arch}
		}
	case MatchContains:
		archs = w.getState().archContains(component)
	}
	return archs, nil
}

// entityToMap converts an entity to a map of its components. A "_id" key is added to the map
// to store the entity ID.
func (id EntityID) toMap(arch *archetype) map[string]any {
	data := make(map[string]any, arch.components.Count())

	// We have to cast id from EntityID to int here or else we can't query the data because for some
	// reason expr can't compare EntityID with integers in the expression.
	data["_id"] = uint32(id)

	for _, col := range arch.columns {
		c := toAbstractColumn(col)
		comp, ok := c.getAbstract(id)
		if !ok {
			continue
		}
		data[comp.Name()] = comp
	}

	return data
}
