package storage

type ArchetypeIterator struct {
	Current int
	Values  []ArchetypeID
}

func (it *ArchetypeIterator) HasNext() bool {
	return it.Current < len(it.Values)
}

func (it *ArchetypeIterator) Next() ArchetypeID {
	val := it.Values[it.Current]
	it.Current++
	return val
}
