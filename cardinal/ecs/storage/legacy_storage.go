package storage

import (
	"fmt"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
)

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
func (cs *ComponentSliceStorage) PushComponent(component component.IComponentType, archetypeID ArchetypeID) error {
	// TODO: optimize to avoid allocation
	compBz, err := component.New()
	if err != nil {
		return err
	}
	cs.storages[archetypeID] = append(cs.storages[archetypeID], compBz)
	return nil
}

// Component returns the bytes of data of the component in the archetype.
func (cs *ComponentSliceStorage) Component(archetypeID ArchetypeID, componentIndex ComponentIndex) ([]byte, error) {
	return cs.storages[archetypeID][componentIndex], nil
}

// SetComponent sets the bytes of data of the component in the archetype.
func (cs *ComponentSliceStorage) SetComponent(archetypeID ArchetypeID, componentIndex ComponentIndex, compBz []byte) error {
	cs.storages[archetypeID][componentIndex] = compBz
	return nil
}

// MoveComponent moves the bytes of data of the component in the archetype.
func (cs *ComponentSliceStorage) MoveComponent(source ArchetypeID, index ComponentIndex, dst ArchetypeID) error {
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
func (cs *ComponentSliceStorage) SwapRemove(archetypeID ArchetypeID, componentIndex ComponentIndex) ([]byte, error) {
	componentValue := cs.storages[archetypeID][componentIndex]
	cs.storages[archetypeID][componentIndex] = cs.storages[archetypeID][len(cs.storages[archetypeID])-1]
	cs.storages[archetypeID] = cs.storages[archetypeID][:len(cs.storages[archetypeID])-1]
	return componentValue, nil
}

// Contains returns true if the storage contains the component.
func (cs *ComponentSliceStorage) Contains(archetypeID ArchetypeID, componentIndex ComponentIndex) (bool, error) {
	if cs.storages[archetypeID] == nil {
		return false, nil
	}
	if len(cs.storages[archetypeID]) <= int(componentIndex) {
		return false, nil
	}
	return cs.storages[archetypeID][componentIndex] != nil, nil
}

var _ ComponentIndexStorage = &ComponentIndexMap{}

type ComponentIndexMap struct {
	idxs map[ArchetypeID]ComponentIndex
}

func NewComponentIndexMap() ComponentIndexStorage {
	return &ComponentIndexMap{idxs: make(map[ArchetypeID]ComponentIndex)}
}

func (c ComponentIndexMap) ComponentIndex(ai ArchetypeID) (ComponentIndex, bool, error) {
	idx, ok := c.idxs[ai]
	return idx, ok, nil
}

func (c *ComponentIndexMap) SetIndex(archID ArchetypeID, compIndex ComponentIndex) error {
	c.idxs[archID] = compIndex
	return nil
}

// IncrementIndex increments the index for this archetype by 1. If the index doesn't
// currently exist, it is initialized to 0 and 0 is returned.
func (c *ComponentIndexMap) IncrementIndex(archID ArchetypeID) (ComponentIndex, error) {
	if _, ok := c.idxs[archID]; !ok {
		c.idxs[archID] = 0
	} else {
		c.idxs[archID]++
	}
	return c.idxs[archID], nil
}

func (c *ComponentIndexMap) DecrementIndex(archID ArchetypeID) error {
	c.idxs[archID]--
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
func (lm *LocationMap) Insert(id EntityID, archetype ArchetypeID, component ComponentIndex) error {
	if int(id) == len(lm.locations) {
		loc := NewLocation(archetype, component)
		lm.locations = append(lm.locations, &locationValid{loc, true})
		lm.len++
	} else {
		val := lm.locations[id]
		val.loc.ArchID = archetype
		val.loc.CompIndex = component
		if !val.valid {
			lm.len++
			val.valid = true
		}
	}
	return nil
}

// SetLocation sets the given entity ID and archetype Index to the storage.
func (lm *LocationMap) SetLocation(id EntityID, loc Location) error {
	lm.Insert(id, loc.ArchID, loc.CompIndex)
	return nil
}

// GetLocation returns the location of the given entity ID.
func (lm *LocationMap) GetLocation(id EntityID) (Location, error) {
	return lm.locations[id].loc, nil
}

// ArchetypeID returns the archetype of the given entity ID.
func (lm *LocationMap) ArchetypeID(id EntityID) (ArchetypeID, error) {
	return lm.locations[id].loc.ArchID, nil
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
	idx.iterator.Values = []ArchetypeID{}
	for i := start; i < len(idx.layouts); i++ {
		if f.MatchesLayout(idx.layouts[i]) {
			idx.iterator.Values = append(idx.iterator.Values, ArchetypeID(i))
		}
	}
	return idx.iterator
}

// Search searches for archetypes that match the given filter.
func (idx *Index) Search(filter filter.LayoutFilter) *ArchetypeIterator {
	return idx.SearchFrom(filter, 0)
}

func (idx *Index) Marshal() ([]byte, error) {
	layouts := [][]component.TypeID{}
	for _, layout := range idx.layouts {
		currIDs := []component.TypeID{}
		for _, component := range layout {
			currIDs = append(currIDs, component.ID())
		}
		layouts = append(layouts, currIDs)
	}
	return Encode(layouts)
}

func (idx *Index) UnmarshalWithComps(bytes []byte, comps []component.IComponentType) error {
	layouts, err := Decode[[][]component.TypeID](bytes)
	if err != nil {
		return err
	}
	idsToComps := newIDsToComponents(comps)

	for _, layout := range layouts {
		currComps, err := idsToComps.convert(layout)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrorComponentMismatchWithSavedState, err)
		}
		idx.layouts = append(idx.layouts, currComps)
	}
	return nil
}
