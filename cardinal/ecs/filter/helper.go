package filter

import (
	"pkg.world.dev/world-engine/cardinal/ecs/icomponent"
)

func containsComponent(components []icomponent.IComponentType, c icomponent.IComponentType) bool {
	for _, comp := range components {
		if comp == c {
			return true
		}
	}
	return false
}
