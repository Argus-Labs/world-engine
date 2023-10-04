package filter

import (
	"pkg.world.dev/world-engine/cardinal/ecs/icomponent"
)

type exact struct {
	components []icomponent.IComponentType
}

// Exact matches archetypes that contain exactly the same components specified.
func Exact(components ...icomponent.IComponentType) ComponentFilter {
	return exact{
		components: components,
	}
}

func (f exact) MatchesComponents(components []icomponent.IComponentType) bool {
	if len(components) != len(f.components) {
		return false
	}
	for _, componentType := range components {
		if !containsComponent(f.components, componentType) {
			return false
		}
	}
	return true
}
