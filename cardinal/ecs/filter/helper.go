package filter

import "pkg.world.dev/world-engine/cardinal/ecs/component_metadata"

// MatchComponentMetaData returns true if the given slice of components contains the given component. Components are the same if they
// have the same ID.
func MatchComponentMetaData(components []component_metadata.IComponentMetaData, cType component_metadata.IComponentMetaData) bool {
	for _, c := range components {
		if cType.ID() == c.ID() {
			return true
		}
	}
	return false
}
