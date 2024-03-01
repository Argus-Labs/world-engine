package filter

import (
	"pkg.world.dev/world-engine/cardinal/types"
)

type contains struct {
	components []types.Component
}

// Contains matches archetypes that contain all the components specified.
func Contains(components ...types.Component) ComponentFilter {
	return &contains{components: components}
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
