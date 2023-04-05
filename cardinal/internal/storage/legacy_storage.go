package storage

import (
	"github.com/argus-labs/cardinal/component"
)

var _ Storage = &ComponentsSliceStorage{}

type ComponentsSliceStorage struct {
	componentStorages []*ComponentSliceStorage
}

func NewComponentsSliceStorage() Storage {
	return &ComponentsSliceStorage{componentStorages: make([]*ComponentSliceStorage, 512)}
}

func (c ComponentsSliceStorage) GetComponentStorage(cid component.TypeID) ComponentStorage {
	s := c.componentStorages[cid]
	// we need to **explicitly** return nil here if the ComponentSliceStorage pointer is nil.
	// the storage slice is pre-allocated with pointer values,
	// which can make life difficult for consumers of this function
	// when checking if the returned interface is nil.
	if s == nil {
		return nil
	}
	return s
}

func (c *ComponentsSliceStorage) InitializeComponentStorage(cid component.TypeID) {
	c.componentStorages[cid] = NewSliceStorage()
}

var _ ComponentStorage = &ComponentSliceStorage{}

// ComponentSliceStorage is a structure that stores the bytes of data of each component.
// It stores the bytes in the two-dimensional slice.
// First dimension is the archetype index.
// Second dimension is the component index.
// The component index is used to access the component data in the archetype.
type ComponentSliceStorage struct {
	storages [][][]byte
}

// NewSliceStorage creates a new empty structure that stores the bytes of data of each component.
func NewSliceStorage() *ComponentSliceStorage {
	return &ComponentSliceStorage{
		storages: make([][][]byte, 256),
	}
}

// PushComponent stores the new data of the component in the archetype.
func (cs *ComponentSliceStorage) PushComponent(component component.IComponentType, archetypeIndex ArchetypeIndex) error {
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
func (cs *ComponentSliceStorage) Component(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) []byte {
	return cs.storages[archetypeIndex][componentIndex]
}

// SetComponent sets the bytes of data of the component in the archetype.
func (cs *ComponentSliceStorage) SetComponent(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex, compBz []byte) {
	cs.storages[archetypeIndex][componentIndex] = compBz
}

// MoveComponent moves the bytes of data of the component in the archetype.
func (cs *ComponentSliceStorage) MoveComponent(source ArchetypeIndex, index ComponentIndex, dst ArchetypeIndex) {
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
func (cs *ComponentSliceStorage) SwapRemove(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) []byte {
	componentValue := cs.storages[archetypeIndex][componentIndex]
	cs.storages[archetypeIndex][componentIndex] = cs.storages[archetypeIndex][len(cs.storages[archetypeIndex])-1]
	cs.storages[archetypeIndex] = cs.storages[archetypeIndex][:len(cs.storages[archetypeIndex])-1]
	return componentValue
}

// Contains returns true if the storage contains the component.
func (cs *ComponentSliceStorage) Contains(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) bool {
	if cs.storages[archetypeIndex] == nil {
		return false
	}
	if len(cs.storages[archetypeIndex]) <= int(componentIndex) {
		return false
	}
	return cs.storages[archetypeIndex][componentIndex] != nil
}

var _ ComponentIndexStorage = &ComponentIndexMap{}

type ComponentIndexMap struct {
	idxs map[ArchetypeIndex]ComponentIndex
}

func NewComponentIndexMap() ComponentIndexStorage {
	return &ComponentIndexMap{idxs: make(map[ArchetypeIndex]ComponentIndex)}
}

func (c ComponentIndexMap) ComponentIndex(ai ArchetypeIndex) (ComponentIndex, bool) {
	idx, ok := c.idxs[ai]
	return idx, ok
}

func (c *ComponentIndexMap) SetIndex(index ArchetypeIndex, index2 ComponentIndex) {
	c.idxs[index] = index2
}

func (c *ComponentIndexMap) IncrementIndex(index ArchetypeIndex) {
	c.idxs[index]++
}

func (c *ComponentIndexMap) DecrementIndex(index ArchetypeIndex) {
	c.idxs[index]--
}
