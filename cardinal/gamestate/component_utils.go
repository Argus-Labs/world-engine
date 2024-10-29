package gamestate

import (
	"sort"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/types"
)

var errComponentMismatch = eris.New("component mismatched")

// compKey is a tuple of a component ComponentID and an entity EntityID. It used as a map key to keep
// track of component data in-memory.
type compKey struct {
	compName types.ComponentName
	entityID types.EntityID
}

// sortComponentSet sorts component names lexicographically.
func sortComponentSet(components []types.ComponentName) error {
	sort.Slice(
		components, func(i, j int) bool {
			return components[i] < components[j]
		},
	)
	for i := 1; i < len(components); i++ {
		if components[i] == components[i-1] {
			return eris.New("duplicate components is not allowed")
		}
	}

	return nil
}

func isComponentSetMatch(a, b []types.ComponentName) error {
	if len(a) != len(b) {
		return errComponentMismatch
	}

	if err := sortComponentSet(a); err != nil {
		return err
	}
	if err := sortComponentSet(b); err != nil {
		return err
	}

	for i := range a {
		if a[i] != b[i] {
			return errComponentMismatch
		}
	}

	return nil
}
