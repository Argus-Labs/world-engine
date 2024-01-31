package iterators

import (
	"pkg.world.dev/world-engine/cardinal/types/archetype"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

type HasEntitiesForArchetype interface {
	GetEntitiesForArchID(archID archetype.ID) ([]entity.ID, error)
}

// EntityIterator is an iterator for Ent lists in archetypes.
type EntityIterator struct {
	current      int
	archAccessor HasEntitiesForArchetype
	indices      []archetype.ID
}

// NewEntityIterator returns an iterator for Entitys.
func NewEntityIterator(current int, archAccessor HasEntitiesForArchetype, indices []archetype.ID) EntityIterator {
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
func (it *EntityIterator) Next() ([]entity.ID, error) {
	archetypeID := it.indices[it.current]
	it.current++
	return it.archAccessor.GetEntitiesForArchID(archetypeID)
}
