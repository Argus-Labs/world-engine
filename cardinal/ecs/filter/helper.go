package filter

import (
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

func containsComponent(components []component.IComponentMetaData, c component.IComponentMetaData) bool {
	for _, comp := range components {
		if comp == c {
			return true
		}
	}
	return false
}
