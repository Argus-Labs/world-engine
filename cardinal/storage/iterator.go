package storage

import (
	"github.com/argus-labs/cardinal/entity"
)

// EntityIterator is an iterator for entity lists in archetypes.
type EntityIterator struct {
	current      int
	archAccessor ArchetypeAccessor
	indices      []ArchetypeIndex
}

// NewEntityIterator returns an iterator for Entitys.
func NewEntityIterator(current int, archAccessor ArchetypeAccessor, indices []ArchetypeIndex) EntityIterator {
	return EntityIterator{
		current:      current,
		archAccessor: archAccessor,
		indices:      indices,
	}
}

// HasNext returns true if there are more entity list to iterate over.
func (it *EntityIterator) HasNext() bool {
	return it.current < len(it.indices)
}

// Next returns the next entity list.
func (it *EntityIterator) Next() []entity.Entity {
	archetypeIndex := it.indices[it.current]
	it.current++
	return it.archAccessor.Archetype(archetypeIndex).Entities()
}
