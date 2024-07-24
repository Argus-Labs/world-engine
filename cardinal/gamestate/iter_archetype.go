package gamestate

import (
	"pkg.world.dev/world-engine/cardinal/types"
)

type ArchetypeIterator struct {
	Current int
	Values  []types.ArchetypeID
}

func (it *ArchetypeIterator) HasNext() bool {
	return it.Current < len(it.Values)
}

func (it *ArchetypeIterator) Next() types.ArchetypeID {
	val := it.Values[it.Current]
	it.Current++
	return val
}
