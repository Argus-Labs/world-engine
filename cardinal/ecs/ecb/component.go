package ecb

import (
	"sort"

	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

// compKey is a tuple of a component TypeID and an entity ID. It used as a map key to keep
// track of component data in-memory.
type compKey struct {
	typeID   component.TypeID
	entityID entity.ID
}

// sortComponentSet re-orders the given components so their IDs are strictly increasing. If any component is duplicated
// an error is returned.
func sortComponentSet(components []component.ComponentMetadata) error {
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

func isComponentSetMatch(a, b []component.ComponentMetadata) bool {
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
