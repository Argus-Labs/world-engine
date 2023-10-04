package storage

import (
	"fmt"

	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/icomponent"
	"pkg.world.dev/world-engine/cardinal/ecs/query/filter"
)

var _ ComponentStorageManager = &ComponentsSliceStorage{}

// ComponentsSliceStorage is a structure that contains component data in slices.
// Component data is indexed by component type ID, archetype Index, and finally component Index.
type ComponentsSliceStorage struct {
	componentStorages []*ComponentSliceStorage
}

func (c ComponentsSliceStorage) GetComponentIndexStorage(cid icomponent.TypeID) ComponentIndexStorage {
	//TODO implement me
	panic("implement me")
}

func NewComponentsSliceStorage() ComponentStorageManager {
	return &ComponentsSliceStorage{componentStorages: make([]*ComponentSliceStorage, 512)}
}

func (c ComponentsSliceStorage) GetComponentStorage(cid icomponent.TypeID) ComponentStorage {
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
func (cs *ComponentSliceStorage) PushComponent(component icomponent.IComponentType, archetypeID archetype.ID) error {
	// TODO: optimize to avoid allocation
	compBz, err := component.New()
	if err != nil {
		return err
	}
	cs.storages[archetypeID] = append(cs.storages[archetypeID], compBz)
	return nil
}

// Component returns the bytes of data of the component in the archetype.
func (cs *ComponentSliceStorage) Component(archetypeID archetype.ID, componentIndex icomponent.Index) ([]byte, error) {
	return cs.storages[archetypeID][componentIndex], nil
}

// SetComponent sets the bytes of data of the component in the archetype.
func (cs *ComponentSliceStorage) SetComponent(archetypeID archetype.ID, componentIndex icomponent.Index, compBz []byte) error {
	cs.storages[archetypeID][componentIndex] = compBz
	return nil
}

// MoveComponent moves the bytes of data of the component in the archetype.
func (cs *ComponentSliceStorage) MoveComponent(source archetype.ID, index icomponent.Index, dst archetype.ID) error {
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
func (cs *ComponentSliceStorage) SwapRemove(archetypeID archetype.ID, componentIndex icomponent.Index) ([]byte, error) {
	componentValue := cs.storages[archetypeID][componentIndex]
	cs.storages[archetypeID][componentIndex] = cs.storages[archetypeID][len(cs.storages[archetypeID])-1]
	cs.storages[archetypeID] = cs.storages[archetypeID][:len(cs.storages[archetypeID])-1]
	return componentValue, nil
}

// Contains returns true if the storage contains the component.
func (cs *ComponentSliceStorage) Contains(archetypeID archetype.ID, componentIndex icomponent.Index) (bool, error) {
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
	idxs map[archetype.ID]icomponent.Index
}

func NewComponentIndexMap() ComponentIndexStorage {
	return &ComponentIndexMap{idxs: make(map[archetype.ID]icomponent.Index)}
}

func (c ComponentIndexMap) ComponentIndex(ai archetype.ID) (icomponent.Index, bool, error) {
	idx, ok := c.idxs[ai]
	return idx, ok, nil
}

func (c *ComponentIndexMap) SetIndex(archID archetype.ID, compIndex icomponent.Index) error {
	c.idxs[archID] = compIndex
	return nil
}

// IncrementIndex increments the index for this archetype by 1. If the index doesn't
// currently exist, it is initialized to 0 and 0 is returned.
func (c *ComponentIndexMap) IncrementIndex(archID archetype.ID) (icomponent.Index, error) {
	if _, ok := c.idxs[archID]; !ok {
		c.idxs[archID] = 0
	} else {
		c.idxs[archID]++
	}
	return c.idxs[archID], nil
}

func (c *ComponentIndexMap) DecrementIndex(archID archetype.ID) error {
	c.idxs[archID]--
	return nil
}

type locationValid struct {
	loc   entity.Location
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
func (lm *LocationMap) ContainsEntity(id entity.ID) (bool, error) {
	val := lm.locations[id]
	return val != nil && val.valid, nil
}

// Remove removes the given entity ID from the storage.
func (lm *LocationMap) Remove(id entity.ID) error {
	lm.locations[id].valid = false
	lm.len--
	return nil
}

// Insert inserts the given entity ID and archetype Index to the storage.
func (lm *LocationMap) Insert(id entity.ID, archetype archetype.ID, component icomponent.Index) error {
	if int(id) == len(lm.locations) {
		loc := entity.NewLocation(archetype, component)
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
func (lm *LocationMap) SetLocation(id entity.ID, loc entity.Location) error {
	lm.Insert(id, loc.ArchID, loc.CompIndex)
	return nil
}

// GetLocation returns the location of the given entity ID.
func (lm *LocationMap) GetLocation(id entity.ID) (entity.Location, error) {
	return lm.locations[id].loc, nil
}

// ArchetypeID returns the archetype of the given entity ID.
func (lm *LocationMap) ArchetypeID(id entity.ID) (archetype.ID, error) {
	return lm.locations[id].loc.ArchID, nil
}

// ComponentIndexForEntity returns the component of the given entity ID.
func (lm *LocationMap) ComponentIndexForEntity(id entity.ID) (icomponent.Index, error) {
	return lm.locations[id].loc.CompIndex, nil
}

// Index is a structure that indexes archetypes by their component types.
type Index struct {
	compGroups [][]icomponent.IComponentType
	iterator   *ArchetypeIterator
}

// NewArchetypeComponentIndex creates a new search Index.
func NewArchetypeComponentIndex() ArchetypeComponentIndex {
	return &Index{
		compGroups: [][]icomponent.IComponentType{},
		iterator: &ArchetypeIterator{
			Current: 0,
		},
	}
}

// Push adds an archetype to the search Index.
func (idx *Index) Push(comps []icomponent.IComponentType) {
	idx.compGroups = append(idx.compGroups, comps)
}

// SearchFrom searches for archetypes that match the given filter from the given Index.
func (idx *Index) SearchFrom(f filter.ComponentFilter, start int) *ArchetypeIterator {
	idx.iterator.Current = 0
	idx.iterator.Values = []archetype.ID{}
	for i := start; i < len(idx.compGroups); i++ {
		if f.MatchesComponents(idx.compGroups[i]) {
			idx.iterator.Values = append(idx.iterator.Values, archetype.ID(i))
		}
	}
	return idx.iterator
}

// Search searches for archetypes that match the given filter.
func (idx *Index) Search(filter filter.ComponentFilter) *ArchetypeIterator {
	return idx.SearchFrom(filter, 0)
}

func (idx *Index) Marshal() ([]byte, error) {
	compGroups := [][]icomponent.TypeID{}
	for _, comps := range idx.compGroups {
		currIDs := []icomponent.TypeID{}
		for _, component := range comps {
			currIDs = append(currIDs, component.ID())
		}
		compGroups = append(compGroups, currIDs)
	}
	return codec.Encode(compGroups)
}

func (idx *Index) UnmarshalWithComps(bytes []byte, comps []icomponent.IComponentType) error {
	compGroups, err := codec.Decode[[][]icomponent.TypeID](bytes)
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
