package filter

import (
	"pkg.world.dev/world-engine/cardinal/types"
)

// MatchComponentMetadata returns true if the given slice of components contains the given component.
// Components are the same if they have the same Name.
func MatchComponentMetadata(
	components []types.ComponentMetadata,
	cType types.ComponentMetadata,
) bool {
	for _, c := range components {
		if cType.Name() == c.Name() {
			return true
		}
	}
	return false
}

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
