package storage

import (
	"pkg.world.dev/world-engine/cardinal/interfaces"
)

type ArchetypeIterator struct {
	Current int
	Values  []interfaces.ArchetypeID
}

func (it *ArchetypeIterator) GetCurrent() int {
	return it.Current
}

func (it *ArchetypeIterator) GetValues() []interfaces.ArchetypeID {
	return it.Values
}

func (it *ArchetypeIterator) HasNext() bool {
	return it.Current < len(it.Values)
}

func (it *ArchetypeIterator) Next() interfaces.ArchetypeID {
	val := it.Values[it.Current]
	it.Current++
	return val
}
