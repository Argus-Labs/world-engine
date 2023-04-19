package filter

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/component"
)

func containsComponent(components []component.IComponentType, c component.IComponentType) bool {
	for _, comp := range components {
		if comp == c {
			return true
		}
	}
	return false
}
