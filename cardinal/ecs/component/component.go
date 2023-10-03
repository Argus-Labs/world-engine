package component

import "pkg.world.dev/world-engine/cardinal/interfaces"

// Contains returns true if the given slice of components contains the given component. Components are the same if they
// have the same ID.
func Contains(components []interfaces.IComponentType, cType interfaces.IComponentType) bool {
	for _, c := range components {
		if cType.ID() == c.ID() {
			return true
		}
	}
	return false
}
