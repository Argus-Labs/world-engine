package filter

import (
	"pkg.world.dev/world-engine/cardinal/types"
)

type exact struct {
	components []types.Component
}

// Exact matches archetypes that contain exactly the same components specified.
func Exact(components ...ComponentWrapper) ComponentFilter {
	acc := make([]types.Component, 0, len(components))
	for _, wrapper := range components {
		acc = append(acc, wrapper.Component)
	}
	return exact{
		components: acc,
	}
}

func (f exact) MatchesComponents(components []types.Component) bool {
	if len(components) != len(f.components) {
		return false
	}
	matchComponent := CreateComponentMatcher(f.components)
	for _, componentType := range components {
		if !matchComponent(componentType) {
			return false
		}
	}
	return true
}
