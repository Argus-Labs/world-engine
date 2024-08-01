package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/types"
)

type searchIterator struct {
	// current is the index of the current archetype id being iterated over
	current int
	// archIDs is the list of archetype ids that we want to iterate over
	archIDs []types.ArchetypeID
	// stateReader is an interface that allows us to read the current entity state
	stateReader gamestate.Reader
}

// newSearchIterator returns an iterator that returns the list of entities for the given archetype ids.
func newSearchIterator(stateReader gamestate.Reader, archIDs []types.ArchetypeID) searchIterator {
	return searchIterator{
		current:     0,
		archIDs:     archIDs,
		stateReader: stateReader,
	}
}

// HasNext evaluates to true if there are still archetypes to iterate over.
func (it *searchIterator) HasNext() bool {
	return it.current < len(it.archIDs)
}

// Next returns the next entity list based on the list of archetypes in archIds.
func (it *searchIterator) Next() ([]types.EntityID, error) {
	archetypeID := it.archIDs[it.current]
	it.current++
	return it.stateReader.GetEntitiesForArchID(archetypeID)
}
