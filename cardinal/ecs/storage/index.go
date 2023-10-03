package storage

import "pkg.world.dev/world-engine/cardinal/public"

type ArchetypeIterator struct {
	Current int
	Values  []public.ArchetypeID
}

func (it *ArchetypeIterator) GetCurrent() int {
	return it.Current
}

func (it *ArchetypeIterator) GetValues() []public.ArchetypeID {
	return it.Values
}

func (it *ArchetypeIterator) HasNext() bool {
	return it.Current < len(it.Values)
}

func (it *ArchetypeIterator) Next() public.ArchetypeID {
	val := it.Values[it.Current]
	it.Current++
	return val
}
