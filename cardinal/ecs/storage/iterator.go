package storage

// EntityIterator is an iterator for Ent lists in archetypes.
type EntityIterator struct {
	current      int
	archAccessor ArchetypeStorage
	indices      []ArchetypeIndex
}

// NewEntityIterator returns an iterator for entities.
func NewEntityIterator(current int, archAccessor ArchetypeStorage, indices []ArchetypeIndex) EntityIterator {
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

// Next returns the next entity list.
func (it *EntityIterator) Next() []uint64 {
	archetypeIndex := it.indices[it.current]
	it.current++
	return it.archAccessor.Archetype(archetypeIndex).GetEntityIds()
}
