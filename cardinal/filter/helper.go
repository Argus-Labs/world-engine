package filter

import (
	"pkg.world.dev/world-engine/cardinal/types"
)

// MatchComponentMetadata returns true if the given slice of components contains the given component.
// Components are the same if they have the same Name.
func MatchComponentMetadata(
	components []types.ComponentMetadata,
	cType types.ComponentMetadata,
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
	components []types.Component,
	cType types.Component,
) bool {
	for _, c := range components {
		if cType.Name() == c.Name() {
			return true
		}
	}
	return false
}
