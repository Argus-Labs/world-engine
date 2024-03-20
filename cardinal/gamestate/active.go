package gamestate

import (
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/types"
)

// activeEntities represents a group of entities.
type activeEntities struct {
	ids      []types.EntityID
	modified bool
}

// swapRemove removes the given entity EntityID from this list of active entities. This is used when moving
// an entity from one archetype to another, and then deleting an entity altogether.
func (a *activeEntities) swapRemove(idToRemove types.EntityID) error {
	// TODO: The finding and removing of these entity ids can be sped up. We're going with a simple implementation
	// here to get to an MVP
	indexOfID := -1
	for i, id := range a.ids {
		if idToRemove == id {
			indexOfID = i
			break
		}
	}
	if indexOfID == -1 {
		return eris.Errorf("cannot find entity id %s", idToRemove)
	}
	lastIndex := len(a.ids) - 1
	if indexOfID < lastIndex {
		a.ids[indexOfID] = a.ids[lastIndex]
	}
	a.ids = a.ids[:len(a.ids)-1]
	return nil
}
