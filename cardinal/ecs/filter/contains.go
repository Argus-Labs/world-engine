package filter

import (
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

type contains struct {
	components []component.IComponentType
}

// Contains matches archetypes that contain all the components specified.
func Contains(components ...component.IComponentType) ComponentFilter {
	return &contains{components: components}
}

func (f *contains) MatchesComponents(components []component.IComponentType) bool {
	for _, componentType := range f.components {
		if !containsComponent(components, componentType) {
			return false
		}
	}
	return true
}
