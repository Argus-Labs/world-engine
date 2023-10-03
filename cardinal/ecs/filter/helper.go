package filter

import (
	"pkg.world.dev/world-engine/cardinal/interfaces"
)

func containsComponent(components []interfaces.IComponentType, c interfaces.IComponentType) bool {
	for _, comp := range components {
		if comp == c {
			return true
		}
	}
	return false
}
