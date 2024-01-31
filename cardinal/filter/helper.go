package filter

import "pkg.world.dev/world-engine/cardinal/types/component"

// MatchComponentMetadata returns true if the given slice of components contains the given component.
// Components are the same if they have the same Name.
func MatchComponentMetadata(
	components []component.ComponentMetadata,
	cType component.ComponentMetadata,
) bool {
	for _, c := range components {
		if cType.Name() == c.Name() {
			return true
		}
	}
	return false
}

// MatchComponent returns true if the given slice of components contains the given component.
// Components are the same if they have the same Name.
func MatchComponent(
	components []component.Component,
	cType component.Component,
) bool {
	for _, c := range components {
		if cType.Name() == c.Name() {
			return true
		}
	}
	return false
}
