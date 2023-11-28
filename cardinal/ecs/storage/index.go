package storage

import "pkg.world.dev/world-engine/cardinal/types/archetype"

type ArchetypeIterator struct {
	Current int
	Values  []archetype.ID
}

func (it *ArchetypeIterator) HasNext() bool {
	return it.Current < len(it.Values)
}

func (it *ArchetypeIterator) Next() archetype.ID {
	val := it.Values[it.Current]
	it.Current++
	return val
}
