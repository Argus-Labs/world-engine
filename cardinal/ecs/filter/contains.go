package filter

import (
	"pkg.world.dev/world-engine/cardinal/interfaces"
)

type contains struct {
	components []interfaces.IComponentType
}

// Contains matches archetypes that contain all the components specified.
func Contains(components ...interfaces.IComponentType) interfaces.IComponentFilter {
	return &contains{components: components}
}

func (f *contains) MatchesComponents(components []interfaces.IComponentType) bool {
	for _, componentType := range f.components {
		if !containsComponent(components, componentType) {
			return false
		}
	}
	return true
}
