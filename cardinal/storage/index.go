package storage

type ArchetypeIterator struct {
	current int
	values  []ArchetypeIndex
}

func (it *ArchetypeIterator) HasNext() bool {
	return it.current < len(it.values)
}

func (it *ArchetypeIterator) Next() ArchetypeIndex {
	val := it.values[it.current]
	it.current++
	return val
}
