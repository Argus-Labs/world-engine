package filter

import "pkg.world.dev/world-engine/cardinal/types/component"

type contains struct {
	components []component.Component
}

// Contains matches archetypes that contain all the components specified.
func Contains(components ...component.Component) ComponentFilter {
	return &contains{components: components}
}

func (f *contains) MatchesComponents(components []component.Component) bool {
	for _, componentType := range f.components {
		if !MatchComponent(components, componentType) {
			return false
		}
	}
	return true
}
