package utils

import "pkg.world.dev/world-engine/cardinal/ecs/icomponent"

// Contains returns true if the given slice of components contains the given component. Components are the same if they
// have the same ID.
func Contains(components []icomponent.IComponentType, cType icomponent.IComponentType) bool {
	for _, c := range components {
		if cType.ID() == c.ID() {
			return true
		}
	}
	return false
}
