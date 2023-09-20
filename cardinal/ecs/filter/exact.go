package filter

import (
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

type exact struct {
	components []component.IComponentType
}

// Exact matches archetypes that contain exactly the same components specified.
func Exact(components ...component.IComponentType) ComponentFilter {
	return exact{
		components: components,
	}
}

func (f exact) MatchesComponents(components []component.IComponentType) bool {
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
