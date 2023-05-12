package storage

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
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
// Component data is indexed by component type ID, archetype Index, and finally component Index.
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
		c.componentStorages[cid] = NewSliceStorage()
		s = c.componentStorages[cid]
	}
	return s
}

var _ ComponentStorage = &ComponentSliceStorage{}

// ComponentSliceStorage is a structure that stores the bytes of data of each component.
// It stores the bytes in the two-dimensional slice.
// First dimension is the archetype Index.
// Second dimension is the component Index.
// The component Index is used to access the component data in the archetype.
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
func (cs *ComponentSliceStorage) Component(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) ([]byte, error) {
	return cs.storages[archetypeIndex][componentIndex], nil
}

// SetComponent sets the bytes of data of the component in the archetype.
func (cs *ComponentSliceStorage) SetComponent(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex, compBz []byte) error {
	cs.storages[archetypeIndex][componentIndex] = compBz
	return nil
}

// MoveComponent moves the bytes of data of the component in the archetype.
func (cs *ComponentSliceStorage) MoveComponent(source ArchetypeIndex, index ComponentIndex, dst ArchetypeIndex) error {
	srcSlice := cs.storages[source]
	dstSlice := cs.storages[dst]

	value := srcSlice[index]
	srcSlice[index] = srcSlice[len(srcSlice)-1]
	srcSlice = srcSlice[:len(srcSlice)-1]
	cs.storages[source] = srcSlice

	dstSlice = append(dstSlice, value)
	cs.storages[dst] = dstSlice
	return nil
}

// SwapRemove removes the bytes of data of the component in the archetype.
func (cs *ComponentSliceStorage) SwapRemove(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) ([]byte, error) {
	componentValue := cs.storages[archetypeIndex][componentIndex]
	cs.storages[archetypeIndex][componentIndex] = cs.storages[archetypeIndex][len(cs.storages[archetypeIndex])-1]
	cs.storages[archetypeIndex] = cs.storages[archetypeIndex][:len(cs.storages[archetypeIndex])-1]
	return componentValue, nil
}

// Contains returns true if the storage contains the component.
func (cs *ComponentSliceStorage) Contains(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) (bool, error) {
	if cs.storages[archetypeIndex] == nil {
		return false, nil
	}
	if len(cs.storages[archetypeIndex]) <= int(componentIndex) {
		return false, nil
	}
	return cs.storages[archetypeIndex][componentIndex] != nil, nil
}

var _ ComponentIndexStorage = &ComponentIndexMap{}

type ComponentIndexMap struct {
	idxs map[ArchetypeIndex]ComponentIndex
}

func NewComponentIndexMap() ComponentIndexStorage {
	return &ComponentIndexMap{idxs: make(map[ArchetypeIndex]ComponentIndex)}
}

func (c ComponentIndexMap) ComponentIndex(ai ArchetypeIndex) (ComponentIndex, bool, error) {
	idx, ok := c.idxs[ai]
	return idx, ok, nil
}

func (c *ComponentIndexMap) SetIndex(index ArchetypeIndex, index2 ComponentIndex) error {
	c.idxs[index] = index2
	return nil
}

// IncrementIndex increments the index for this archetype by 1. If the index doesn't
// currently exist, it is initialized to 0 and 0 is returned.
func (c *ComponentIndexMap) IncrementIndex(index ArchetypeIndex) (ComponentIndex, error) {
	if _, ok := c.idxs[index]; !ok {
		c.idxs[index] = 0
	} else {
		c.idxs[index]++
	}
	return c.idxs[index], nil
}

func (c *ComponentIndexMap) DecrementIndex(index ArchetypeIndex) error {
	c.idxs[index]--
	return nil
}

type locationValid struct {
	loc   Location
	valid bool
}

// LocationMap is a storage of Ent locations.
type LocationMap struct {
	locations []*locationValid
	len       int
}

func (lm *LocationMap) Len() (int, error) {
	return lm.len, nil
}

// NewLocationMap creates an empty storage.
func NewLocationMap() EntityLocationStorage {
	return &LocationMap{
		locations: make([]*locationValid, 1, 256),
		len:       0,
	}
}

// ContainsEntity returns true if the storage contains the given entity ID.
func (lm *LocationMap) ContainsEntity(id EntityID) (bool, error) {
	val := lm.locations[id]
	return val != nil && val.valid, nil
}

// Remove removes the given entity ID from the storage.
func (lm *LocationMap) Remove(id EntityID) error {
	lm.locations[id].valid = false
	lm.len--
	return nil
}

// Insert inserts the given entity ID and archetype Index to the storage.
func (lm *LocationMap) Insert(id EntityID, archetype ArchetypeIndex, component ComponentIndex) error {
	if int(id) == len(lm.locations) {
		loc := NewLocation(archetype, component)
		lm.locations = append(lm.locations, &locationValid{loc, true})
		lm.len++
	} else {
		val := lm.locations[id]
		val.loc.ArchIndex = archetype
		val.loc.CompIndex = component
		if !val.valid {
			lm.len++
			val.valid = true
		}
	}
	return nil
}

// Set sets the given entity ID and archetype Index to the storage.
func (lm *LocationMap) Set(id EntityID, loc Location) error {
	lm.Insert(id, loc.ArchIndex, loc.CompIndex)
	return nil
}

// Location returns the location of the given entity ID.
func (lm *LocationMap) Location(id EntityID) (Location, error) {
	return lm.locations[id].loc, nil
}

// ArchetypeIndex returns the archetype of the given entity ID.
func (lm *LocationMap) ArchetypeIndex(id EntityID) (ArchetypeIndex, error) {
	return lm.locations[id].loc.ArchIndex, nil
}

// ComponentIndexForEntity returns the component of the given entity ID.
func (lm *LocationMap) ComponentIndexForEntity(id EntityID) (ComponentIndex, error) {
	return lm.locations[id].loc.CompIndex, nil
}

// Index is a structure that indexes archetypes by their component types.
type Index struct {
	layouts  [][]component.IComponentType
	iterator *ArchetypeIterator
}

// NewArchetypeComponentIndex creates a new search Index.
func NewArchetypeComponentIndex() ArchetypeComponentIndex {
	return &Index{
		layouts: [][]component.IComponentType{},
		iterator: &ArchetypeIterator{
			Current: 0,
		},
	}
}

// Push adds an archetype to the search Index.
func (idx *Index) Push(layout *Layout) {
	idx.layouts = append(idx.layouts, layout.Components())
}

// SearchFrom searches for archetypes that match the given filter from the given Index.
func (idx *Index) SearchFrom(f filter.LayoutFilter, start int) *ArchetypeIterator {
	idx.iterator.Current = 0
	idx.iterator.Values = []ArchetypeIndex{}
	for i := start; i < len(idx.layouts); i++ {
		if f.MatchesLayout(idx.layouts[i]) {
			idx.iterator.Values = append(idx.iterator.Values, ArchetypeIndex(i))
		}
	}
	return idx.iterator
}

// Search searches for archetypes that match the given filter.
func (idx *Index) Search(filter filter.LayoutFilter) *ArchetypeIterator {
	return idx.SearchFrom(filter, 0)
}

type entryStorageImpl struct {
	entries []Entry
}

func (e *entryStorageImpl) SetLocation(id EntityID, location Location) error {
	e.entries[id].Loc = location

	return nil
}

var _ EntryStorage = &entryStorageImpl{}

func NewEntryStorage() EntryStorage {
	return &entryStorageImpl{entries: make([]Entry, 1, 256)}
}

func (e *entryStorageImpl) SetEntry(id EntityID, entry Entry) error {
	if int(id) >= len(e.entries) {
		e.entries = append(e.entries, Entry{})
	}
	e.entries[id] = entry
	return nil
}

func (e entryStorageImpl) GetEntry(id EntityID) (Entry, error) {
	return e.entries[id], nil
}
