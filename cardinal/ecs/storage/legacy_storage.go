package storage

import (
	"fmt"

	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/interfaces"
)

var _ interfaces.ComponentStorageManager = &ComponentsSliceStorage{}

// ComponentsSliceStorage is a structure that contains component data in slices.
// Component data is indexed by component type ID, archetype Index, and finally component Index.
type ComponentsSliceStorage struct {
	componentStorages []*ComponentSliceStorage
}

func (c ComponentsSliceStorage) GetComponentIndexStorage(cid interfaces.ComponentTypeID) interfaces.ComponentIndexStorage {
	//TODO implement me
	panic("implement me")
}

func NewComponentsSliceStorage() interfaces.ComponentStorageManager {
	return &ComponentsSliceStorage{componentStorages: make([]*ComponentSliceStorage, 512)}
}

func (c ComponentsSliceStorage) GetComponentStorage(cid interfaces.ComponentTypeID) interfaces.ComponentStorage {
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

var _ interfaces.ComponentStorage = &ComponentSliceStorage{}

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
func (cs *ComponentSliceStorage) PushComponent(component interfaces.IComponentType, archetypeID interfaces.ArchetypeID) error {
	// TODO: optimize to avoid allocation
	compBz, err := component.New()
	if err != nil {
		return err
	}
	cs.storages[archetypeID] = append(cs.storages[archetypeID], compBz)
	return nil
}

// Component returns the bytes of data of the component in the archetype.
func (cs *ComponentSliceStorage) Component(archetypeID interfaces.ArchetypeID, componentIndex interfaces.ComponentIndex) ([]byte, error) {
	return cs.storages[archetypeID][componentIndex], nil
}

// SetComponent sets the bytes of data of the component in the archetype.
func (cs *ComponentSliceStorage) SetComponent(archetypeID interfaces.ArchetypeID, componentIndex interfaces.ComponentIndex, compBz []byte) error {
	cs.storages[archetypeID][componentIndex] = compBz
	return nil
}

// MoveComponent moves the bytes of data of the component in the archetype.
func (cs *ComponentSliceStorage) MoveComponent(source interfaces.ArchetypeID, index interfaces.ComponentIndex, dst interfaces.ArchetypeID) error {
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
func (cs *ComponentSliceStorage) SwapRemove(archetypeID interfaces.ArchetypeID, componentIndex interfaces.ComponentIndex) ([]byte, error) {
	componentValue := cs.storages[archetypeID][componentIndex]
	cs.storages[archetypeID][componentIndex] = cs.storages[archetypeID][len(cs.storages[archetypeID])-1]
	cs.storages[archetypeID] = cs.storages[archetypeID][:len(cs.storages[archetypeID])-1]
	return componentValue, nil
}

// Contains returns true if the storage contains the component.
func (cs *ComponentSliceStorage) Contains(archetypeID interfaces.ArchetypeID, componentIndex interfaces.ComponentIndex) (bool, error) {
	if cs.storages[archetypeID] == nil {
		return false, nil
	}
	if len(cs.storages[archetypeID]) <= int(componentIndex) {
		return false, nil
	}
	return cs.storages[archetypeID][componentIndex] != nil, nil
}

var _ interfaces.ComponentIndexStorage = &ComponentIndexMap{}

type ComponentIndexMap struct {
	idxs map[interfaces.ArchetypeID]interfaces.ComponentIndex
}

func NewComponentIndexMap() interfaces.ComponentIndexStorage {
	return &ComponentIndexMap{idxs: make(map[interfaces.ArchetypeID]interfaces.ComponentIndex)}
}

func (c ComponentIndexMap) ComponentIndex(ai interfaces.ArchetypeID) (interfaces.ComponentIndex, bool, error) {
	idx, ok := c.idxs[ai]
	return idx, ok, nil
}

func (c *ComponentIndexMap) SetIndex(archID interfaces.ArchetypeID, compIndex interfaces.ComponentIndex) error {
	c.idxs[archID] = compIndex
	return nil
}

// IncrementIndex increments the index for this archetype by 1. If the index doesn't
// currently exist, it is initialized to 0 and 0 is returned.
func (c *ComponentIndexMap) IncrementIndex(archID interfaces.ArchetypeID) (interfaces.ComponentIndex, error) {
	if _, ok := c.idxs[archID]; !ok {
		c.idxs[archID] = 0
	} else {
		c.idxs[archID]++
	}
	return c.idxs[archID], nil
}

func (c *ComponentIndexMap) DecrementIndex(archID interfaces.ArchetypeID) error {
	c.idxs[archID]--
	return nil
}

type locationValid struct {
	loc   interfaces.ILocation
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
func NewLocationMap() interfaces.EntityLocationStorage {
	return &LocationMap{
		locations: make([]*locationValid, 1, 256),
		len:       0,
	}
}

// ContainsEntity returns true if the storage contains the given entity ID.
func (lm *LocationMap) ContainsEntity(id interfaces.EntityID) (bool, error) {
	val := lm.locations[id]
	return val != nil && val.valid, nil
}

// Remove removes the given entity ID from the storage.
func (lm *LocationMap) Remove(id interfaces.EntityID) error {
	lm.locations[id].valid = false
	lm.len--
	return nil
}

// Insert inserts the given entity ID and archetype Index to the storage.
func (lm *LocationMap) Insert(id interfaces.EntityID, archetype interfaces.ArchetypeID, component interfaces.ComponentIndex) error {
	if int(id) == len(lm.locations) {
		loc := entity.NewLocation(archetype, component)
		lm.locations = append(lm.locations, &locationValid{loc, true})
		lm.len++
	} else {
		val := lm.locations[id]
		val.loc.SetArchID(archetype)
		val.loc.SetCompIndex(component)
		if !val.valid {
			lm.len++
			val.valid = true
		}
	}
	return nil
}

// SetLocation sets the given entity ID and archetype Index to the storage.
func (lm *LocationMap) SetLocation(id interfaces.EntityID, loc interfaces.ILocation) error {
	lm.Insert(id, loc.GetArchID(), loc.GetCompIndex())
	return nil
}

// GetLocation returns the location of the given entity ID.
func (lm *LocationMap) GetLocation(id interfaces.EntityID) (interfaces.ILocation, error) {
	return lm.locations[id].loc, nil
}

// ArchetypeID returns the archetype of the given entity ID.
func (lm *LocationMap) ArchetypeID(id interfaces.EntityID) (interfaces.ArchetypeID, error) {
	return lm.locations[id].loc.GetArchID(), nil
}

// ComponentIndexForEntity returns the component of the given entity ID.
func (lm *LocationMap) ComponentIndexForEntity(id interfaces.EntityID) (interfaces.ComponentIndex, error) {
	return lm.locations[id].loc.GetCompIndex(), nil
}

// Index is a structure that indexes archetypes by their component types.
type Index struct {
	compGroups [][]interfaces.IComponentType
	iterator   *ArchetypeIterator
}

// NewArchetypeComponentIndex creates a new search Index.
func NewArchetypeComponentIndex() interfaces.ArchetypeComponentIndex {
	return &Index{
		compGroups: [][]interfaces.IComponentType{},
		iterator: &ArchetypeIterator{
			Current: 0,
		},
	}
}

// Push adds an archetype to the search Index.
func (idx *Index) Push(comps []interfaces.IComponentType) {
	idx.compGroups = append(idx.compGroups, comps)
}

// SearchFrom searches for archetypes that match the given filter from the given Index.
func (idx *Index) SearchFrom(f interfaces.IComponentFilter, start int) interfaces.IArchtypeIterator {
	idx.iterator.Current = 0
	idx.iterator.Values = []interfaces.ArchetypeID{}
	for i := start; i < len(idx.compGroups); i++ {
		if f.MatchesComponents(idx.compGroups[i]) {
			idx.iterator.Values = append(idx.iterator.Values, interfaces.ArchetypeID(i))
		}
	}
	return idx.iterator
}

// Search searches for archetypes that match the given filter.
func (idx *Index) Search(filter interfaces.IComponentFilter) interfaces.IArchtypeIterator {
	return idx.SearchFrom(filter, 0)
}

func (idx *Index) Marshal() ([]byte, error) {
	compGroups := [][]interfaces.ComponentTypeID{}
	for _, comps := range idx.compGroups {
		currIDs := []interfaces.ComponentTypeID{}
		for _, component := range comps {
			currIDs = append(currIDs, component.ID())
		}
		compGroups = append(compGroups, currIDs)
	}
	return codec.Encode(compGroups)
}

func (idx *Index) UnmarshalWithComps(bytes []byte, comps []interfaces.IComponentType) error {
	compGroups, err := codec.Decode[[][]interfaces.ComponentTypeID](bytes)
	if err != nil {
		return err
	}
	idsToComps := newIDsToComponents(comps)

	for _, compGroup := range compGroups {
		currComps, err := idsToComps.convert(compGroup)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrorComponentMismatchWithSavedState, err)
		}
		idx.compGroups = append(idx.compGroups, currComps)
	}
	return nil
}
