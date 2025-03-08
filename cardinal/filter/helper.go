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

// or false otherwise.
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

func ConvertComponentMetadatasToComponentWrappers(comps []types.ComponentMetadata) []ComponentWrapper {
	ret := make([]ComponentWrapper, len(comps))
	for i, comp := range comps {
		ret[i] = ComponentWrapper{Component: comp}
	}
	return ret
}
