package search

import (
	"pkg.world.dev/world-engine/cardinal/types"
)

type HasEntitiesForArchetype interface {
	GetEntitiesForArchID(archID types.ArchetypeID) ([]types.EntityID, error)
}

// EntityIterator is an iterator for Ent lists in archetypes.
type EntityIterator struct {
	current      int
	archAccessor HasEntitiesForArchetype
	indices      []types.ArchetypeID
}

// NewEntityIterator returns an iterator for Entitys.
func NewEntityIterator(current int, archAccessor HasEntitiesForArchetype, indices []types.ArchetypeID) EntityIterator {
	return EntityIterator{
		current:      current,
		archAccessor: archAccessor,
		indices:      indices,
	}
}

// HasNext returns true if there are more Ent list to iterate over.
func (it *EntityIterator) HasNext() bool {
	return it.current < len(it.indices)
}

// Next returns the next Ent list.
func (it *EntityIterator) Next() ([]types.EntityID, error) {
	archetypeID := it.indices[it.current]
	it.current++
	return it.archAccessor.GetEntitiesForArchID(archetypeID)
}
