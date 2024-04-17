package filter

import (
	"pkg.world.dev/world-engine/cardinal/types"
)

type contains struct {
	components []types.Component
}

// Contains matches archetypes that contain all the components specified.
func Contains(components ...ComponentWrapper) ComponentFilter {
	acc := make([]types.Component, 0, len(components))
	for _, wrapper := range components {
		acc = append(acc, wrapper.Component)
	}
	return &contains{components: acc}
}

func (f *contains) MatchesComponents(components []types.Component) bool {
	matchComponent := CreateComponentMatcher(components)
	for _, componentType := range f.components {
		if !matchComponent(componentType) {
			return false
		}
	}
	return true
}
