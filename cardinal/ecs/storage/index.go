package storage

type ArchetypeIterator struct {
	Current int
	Values  []ArchetypeIndex
}

func (it *ArchetypeIterator) HasNext() bool {
	return it.Current < len(it.Values)
}

func (it *ArchetypeIterator) Next() ArchetypeIndex {
	val := it.Values[it.Current]
	it.Current++
	return val
}
