package filter

import (
	"pkg.world.dev/world-engine/cardinal/interfaces"
)

type exact struct {
	components []interfaces.IComponentType
}

// Exact matches archetypes that contain exactly the same components specified.
func Exact(components ...interfaces.IComponentType) interfaces.IComponentFilter {
	return exact{
		components: components,
	}
}

func (f exact) MatchesComponents(components []interfaces.IComponentType) bool {
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
