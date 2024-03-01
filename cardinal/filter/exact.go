package filter

import (
	"pkg.world.dev/world-engine/cardinal/types"
)

type exact struct {
	components []types.Component
}

// Exact matches archetypes that contain exactly the same components specified.
func Exact(components ...types.Component) ComponentFilter {
	return exact{
		components: components,
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
