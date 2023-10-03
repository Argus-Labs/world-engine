package storage

import "pkg.world.dev/world-engine/cardinal/public"

// EntityIterator is an iterator for Ent lists in archetypes.
type EntityIterator struct {
	current      int
	archAccessor public.ArchetypeAccessor
	indices      []public.ArchetypeID
}

// NewEntityIterator returns an iterator for Entitys.
func NewEntityIterator(current int, archAccessor public.ArchetypeAccessor, indices []public.ArchetypeID) EntityIterator {
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
func (it *EntityIterator) Next() []public.EntityID {
	archetypeID := it.indices[it.current]
	it.current++
	return it.archAccessor.Archetype(archetypeID).Entities()
}
