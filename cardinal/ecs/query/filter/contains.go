package filter

import (
	"pkg.world.dev/world-engine/cardinal/ecs/icomponent"
)

type contains struct {
	components []icomponent.IComponentType
}

// Contains matches archetypes that contain all the components specified.
func Contains(components ...icomponent.IComponentType) ComponentFilter {
	return &contains{components: components}
}

func (f *contains) MatchesComponents(components []icomponent.IComponentType) bool {
	for _, componentType := range f.components {
		if !containsComponent(components, componentType) {
			return false
		}
	}
	return true
}
