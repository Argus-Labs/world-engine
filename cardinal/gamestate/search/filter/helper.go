package filter

import (
	"pkg.world.dev/world-engine/cardinal/types"
)

// CreateComponentMatcher creates a function given a slice of components. This function will
// take a parameter that is a single component and return true if it is in the slice of components
// or false otherwise
func CreateComponentMatcher(components []types.Component) func(types.Component) bool {
	mapStringToComponent := make(map[string]types.Component, len(components))
	for _, component := range components {
		mapStringToComponent[component.Name()] = component
	}
	return func(cType types.Component) bool {
		_, ok := mapStringToComponent[cType.Name()]
		return ok
	}
}
