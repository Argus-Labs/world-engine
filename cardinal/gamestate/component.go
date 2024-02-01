package gamestate

import (
	"pkg.world.dev/world-engine/cardinal/types"
	"sort"

	"github.com/rotisserie/eris"
)

// compKey is a tuple of a component ComponentID and an entity EntityID. It used as a map key to keep
// track of component data in-memory.
type compKey struct {
	typeID   types.ComponentID
	entityID types.EntityID
}

// sortComponentSet re-orders the given components so their IDs are strictly increasing. If any component is duplicated
// an error is returned.
func sortComponentSet(components []types.ComponentMetadata) error {
	sort.Slice(
		components, func(i, j int) bool {
			return components[i].ID() < components[j].ID()
		},
	)
	for i := 1; i < len(components); i++ {
		if components[i] == components[i-1] {
			return eris.New("duplicate components is not allowed")
		}
	}

	return nil
}

func isComponentSetMatch(a, b []types.ComponentMetadata) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID() != b[i].ID() {
			return false
		}
	}
	return true
}
