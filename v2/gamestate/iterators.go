package gamestate

import (
	"pkg.world.dev/world-engine/cardinal/v2/types"
)

type EntityIterator struct {
	// current is the index of the current archetype id being iterated over
	current int
	// archIDs is the list of archetype ids that we want to iterate over
	archIDs []types.ArchetypeID
	// stateReader is an interface that allows us to read the current entity state
	stateReader Reader
}

// NewEntityIterator returns an iterator that iterates through a list of entities for the given archetype iterators.
func NewEntityIterator(stateReader Reader, archIDs []types.ArchetypeID) EntityIterator {
	iterator := EntityIterator{
		current:     0,
		archIDs:     archIDs,
		stateReader: stateReader,
	}
	return iterator
}

// HasNext evaluates to true if there are still archetypes to iterate over.
func (it *EntityIterator) HasNext() bool {
	return it.current < len(it.archIDs)
}

// Next returns the next entity list based on the list of archetypes in archIds.
func (it *EntityIterator) Next() ([]types.EntityID, error) {
	archID := it.archIDs[it.current]
	it.current++
	return it.stateReader.GetEntitiesForArchID(archID)
}
