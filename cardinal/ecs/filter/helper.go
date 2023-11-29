package filter

import "pkg.world.dev/world-engine/cardinal/types/component"

// MatchComponentMetaData returns true if the given slice of components contains the given component. Components are the
// same if they have the same ID.
func MatchComponentMetaData(
	components []component.ComponentMetadata,
	cType component.ComponentMetadata,
) bool {
	for _, c := range components {
		if cType.ID() == c.ID() {
			return true
		}
	}
	return false
}
