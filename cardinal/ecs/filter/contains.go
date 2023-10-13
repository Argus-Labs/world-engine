package filter

import (
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

type contains struct {
	components []component.IComponentMetaData
}

// Contains matches archetypes that contain all the components specified.
func Contains(components ...component.IComponentMetaData) ComponentFilter {
	return &contains{components: components}
}

func (f *contains) MatchesComponents(components []component.IComponentMetaData) bool {
	for _, componentType := range f.components {
		if !containsComponent(components, componentType) {
			return false
		}
	}
	return true
}
