package cmdbuffer

import (
	"fmt"

	"pkg.world.dev/world-engine/cardinal/ecs/entity"
)

// activeEntities represents a group of entities, and
type activeEntities struct {
	ids      []entity.ID
	modified bool
}

// swapRemove removes the given entity ID from this list of active entities. This is used when moving
// an entity from one archetype to another, and then deleting an entity altogether.
func (a *activeEntities) swapRemove(idToRemove entity.ID) error {
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
		return fmt.Errorf("cannot find entity id %d", idToRemove)
	}
	lastIndex := len(a.ids) - 1
	if indexOfID < lastIndex {
		a.ids[indexOfID] = a.ids[lastIndex]
	}
	a.ids = a.ids[:len(a.ids)-1]
	return nil
}
