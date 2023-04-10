package storage

import (
	"bytes"
	"encoding/gob"

	"github.com/argus-labs/cardinal/component"
	"github.com/argus-labs/cardinal/entity"
	"github.com/argus-labs/cardinal/filter"
)

func NewLegacyStorage() WorldStorage {
	componentsStore := NewComponents(NewComponentsSliceStorage(), NewComponentIndexMap())
	eloStore := NewLocationMap()
	archIdxStore := NewArchetypeComponentIndex()
	archAcc := NewArchetypeAccessor()
	entryStore := NewEntryStorage()
	entityMgr := NewEntityManager()

	return NewWorldStorage(componentsStore, eloStore, archIdxStore, archAcc, entryStore, entityMgr)
}

var _ ComponentStorageManager = &ComponentsSliceStorage{}

// ComponentsSliceStorage is a structure that contains component data in slices.
// Component data is indexed by component type ID, archetype index, and finally component index.
type ComponentsSliceStorage struct {
	componentStorages []*ComponentSliceStorage
}

func (c ComponentsSliceStorage) GetComponentIndexStorage(cid component.TypeID) ComponentIndexStorage {
	//TODO implement me
	panic("implement me")
}

func NewComponentsSliceStorage() ComponentStorageManager {
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

// LocationMap is a storage of entity locations.
type LocationMap struct {
	locations []*Location
	len       int
}

func (lm *LocationMap) Len() int {
	return lm.len
}

// NewLocationMap creates an empty storage.
func NewLocationMap() EntityLocationStorage {
	return &LocationMap{
		locations: make([]*Location, 1, 256),
		len:       0,
	}
}

// Contains returns true if the storage contains the given entity id.
func (lm *LocationMap) Contains(id entity.ID) bool {
	val := lm.locations[id]
	return val != nil && val.Valid
}

// Remove removes the given entity id from the storage.
func (lm *LocationMap) Remove(id entity.ID) {
	lm.locations[id].Valid = false
	lm.len--
}

// Insert inserts the given entity id and archetype index to the storage.
func (lm *LocationMap) Insert(id entity.ID, archetype ArchetypeIndex, component ComponentIndex) {
	if int(id) == len(lm.locations) {
		loc := NewLocation(archetype, component)
		lm.locations = append(lm.locations, loc)
		lm.len++
	} else {
		loc := lm.locations[id]
		loc.ArchIndex = archetype
		loc.CompIndex = component
		if !loc.Valid {
			lm.len++
			loc.Valid = true
		}
	}
}

// Set sets the given entity id and archetype index to the storage.
func (lm *LocationMap) Set(id entity.ID, loc *Location) {
	lm.Insert(id, loc.ArchIndex, loc.CompIndex)
}

// Location returns the location of the given entity id.
func (lm *LocationMap) Location(id entity.ID) *Location {
	return lm.locations[id]
}

// ArchetypeIndex returns the archetype of the given entity id.
func (lm *LocationMap) ArchetypeIndex(id entity.ID) ArchetypeIndex {
	return lm.locations[id].ArchIndex
}

// ComponentIndex returns the component of the given entity id.
func (lm *LocationMap) ComponentIndex(id entity.ID) ComponentIndex {
	return lm.locations[id].CompIndex
}

// Index is a structure that indexes archetypes by their component types.
type Index struct {
	layouts  [][]component.IComponentType
	iterator *ArchetypeIterator
}

// NewArchetypeComponentIndex creates a new search index.
func NewArchetypeComponentIndex() ArchetypeComponentIndex {
	return &Index{
		layouts: [][]component.IComponentType{},
		iterator: &ArchetypeIterator{
			current: 0,
		},
	}
}

// Push adds an archetype to the search index.
func (idx *Index) Push(layout *Layout) {
	idx.layouts = append(idx.layouts, layout.Components())
}

// SearchFrom searches for archetypes that match the given filter from the given index.
func (idx *Index) SearchFrom(f filter.LayoutFilter, start int) *ArchetypeIterator {
	idx.iterator.current = 0
	idx.iterator.values = []ArchetypeIndex{}
	for i := start; i < len(idx.layouts); i++ {
		if f.MatchesLayout(idx.layouts[i]) {
			idx.iterator.values = append(idx.iterator.values, ArchetypeIndex(i))
		}
	}
	return idx.iterator
}

// Search searches for archetypes that match the given filter.
func (idx *Index) Search(filter filter.LayoutFilter) *ArchetypeIterator {
	return idx.SearchFrom(filter, 0)
}

func DecodeComponent[T any](bz []byte) (T, error) {
	var buf bytes.Buffer
	buf.Write(bz)
	dec := gob.NewDecoder(&buf)
	comp := new(T)
	err := dec.Decode(comp)
	var t T
	if err != nil {
		return t, err
	}
	return *comp, nil
}
func EncodeComponent[T any](comp T) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(comp)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
