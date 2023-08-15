package filter

import (
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

func containsComponent(components []component.IComponentType, c component.IComponentType) bool {
	for _, comp := range components {
		if comp == c {
			return true
		}
	}
	return false
}
