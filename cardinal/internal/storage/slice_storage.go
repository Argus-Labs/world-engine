package storage

import (
	"github.com/argus-labs/cardinal/component"
)

var _ Engine = &SliceStorage{}

// SliceStorage is a structure that stores the bytes of data of each component.
// It stores the bytes in the two-dimensional slice.
// First dimension is the archetype index.
// Second dimension is the component index.
// The component index is used to access the component data in the archetype.
type SliceStorage struct {
	storages [][][]byte
}

// NewSliceStorage creates a new empty structure that stores the bytes of data of each component.
func NewSliceStorage() *SliceStorage {
	return &SliceStorage{
		storages: make([][][]byte, 256),
	}
}

// PushComponent stores the new data of the component in the archetype.
func (cs *SliceStorage) PushComponent(component component.IComponentType, archetypeIndex ArchetypeIndex) error {
	if v := cs.storages[archetypeIndex]; v == nil {
		cs.storages[archetypeIndex] = nil
	}
	// TODO: optimize to avoid allocation
	compBz, err := component.New()
	if err != nil {
		return err
	}
	cs.storages[archetypeIndex] = append(cs.storages[archetypeIndex], compBz)
	return nil
}

// Component returns the bytes of data of the component in the archetype.
func (cs *SliceStorage) Component(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) []byte {
	return cs.storages[archetypeIndex][componentIndex]
}

// SetComponent sets the bytes of data of the component in the archetype.
func (cs *SliceStorage) SetComponent(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex, compBz []byte) {
	cs.storages[archetypeIndex][componentIndex] = compBz
}

// MoveComponent moves the bytes of data of the component in the archetype.
func (cs *SliceStorage) MoveComponent(source ArchetypeIndex, index ComponentIndex, dst ArchetypeIndex) {
	srcSlice := cs.storages[source]
	dstSlice := cs.storages[dst]

	value := srcSlice[index]
	srcSlice[index] = srcSlice[len(srcSlice)-1]
	srcSlice = srcSlice[:len(srcSlice)-1]
	cs.storages[source] = srcSlice

	dstSlice = append(dstSlice, value)
	cs.storages[dst] = dstSlice
}

// SwapRemove removes the bytes of data of the component in the archetype.
func (cs *SliceStorage) SwapRemove(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) []byte {
	componentValue := cs.storages[archetypeIndex][componentIndex]
	cs.storages[archetypeIndex][componentIndex] = cs.storages[archetypeIndex][len(cs.storages[archetypeIndex])-1]
	cs.storages[archetypeIndex] = cs.storages[archetypeIndex][:len(cs.storages[archetypeIndex])-1]
	return componentValue
}

// Contains returns true if the storage contains the component.
func (cs *SliceStorage) Contains(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) bool {
	if cs.storages[archetypeIndex] == nil {
		return false
	}
	if len(cs.storages[archetypeIndex]) <= int(componentIndex) {
		return false
	}
	return cs.storages[archetypeIndex][componentIndex] != nil
}
