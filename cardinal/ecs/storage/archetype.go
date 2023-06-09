package storage

import (
	"errors"
	"fmt"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
)

type ArchetypeID int

var _ ArchetypeAccessor = &archetypeStorageImpl{}

func NewArchetypeAccessor() ArchetypeAccessor {
	return &archetypeStorageImpl{archs: make([]*Archetype, 0)}
}

type archetypeStorageImpl struct {
	archs []*Archetype
}

func (a *archetypeStorageImpl) PushArchetype(archID ArchetypeID, layout *Layout) {
	a.archs = append(a.archs, &Archetype{
		ID:         archID,
		Entitys:    make([]EntityID, 0, 256),
		ArchLayout: layout,
	})
}

func (a *archetypeStorageImpl) Count() int {
	return len(a.archs)
}

func (a *archetypeStorageImpl) Archetype(archID ArchetypeID) ArchetypeStorage {
	return a.archs[archID]
}

// archForStorage is a helper struct that is used to serialize/deserialize the archetypeStorageImpl
// struct to bytes. The IComponentType interfaces do not serialize to bytes easily, so instead
// we just extract the TypeIDs and serialize the ids to bytes. On deserilization we need a
// slice of IComponentTypes with the correct TypeIDs so that we can recover the original
// archetypeStorageImpl.
type archForStorage struct {
	ID           ArchetypeID
	Entities     []EntityID
	ComponentIDs []component.TypeID
}

// Marshal converts the archetypeStorageImpl to bytes. Only the IDs from the IComponentTypes
// are serialized.
func (a *archetypeStorageImpl) Marshal() ([]byte, error) {
	archs := make([]archForStorage, len(a.archs))
	for i := range archs {
		archs[i].ID = a.archs[i].ID
		archs[i].Entities = a.archs[i].Entitys
		for _, c := range a.archs[i].Layout().Components() {
			archs[i].ComponentIDs = append(archs[i].ComponentIDs, c.ID())
		}
	}
	return Encode(archs)
}

var (
	// ErrorComponentMismatchWithSavedState is an error that is returned when a TypeID from
	// the saved state is not found in the passed in list of components.
	ErrorComponentMismatchWithSavedState = errors.New("registered components do not match with the saved state")
)

// idsToComponents converts slices of TypeIDs to the corresponding IComponentTypes
type idsToComponents struct {
	m map[component.TypeID]component.IComponentType
}

func newIDsToComponents(components []component.IComponentType) idsToComponents {
	m := map[component.TypeID]component.IComponentType{}
	for i, comp := range components {
		m[comp.ID()] = components[i]
	}
	return idsToComponents{m: m}
}

func (c idsToComponents) convert(ids []component.TypeID) (comps []component.IComponentType, ok error) {
	comps = []component.IComponentType{}
	for _, id := range ids {
		comp, ok := c.m[id]
		if !ok {
			return nil, fmt.Errorf("id %d not found in %v", id, c.m)
		}
		comps = append(comps, comp)
	}
	return comps, nil
}

// UnmarshalWithComps converts some bytes (generated with Marshal) and a list of components into
// an archetypeStorageImpl. The slice of components is required because the interfaces were not
// actually serialized to bytes, just their IDs.
func (a *archetypeStorageImpl) UnmarshalWithComps(bytes []byte, components []component.IComponentType) error {
	archetypesFromStorage, err := Decode[[]archForStorage](bytes)
	if err != nil {
		return err
	}
	idsToComps := newIDsToComponents(components)

	for _, arch := range archetypesFromStorage {
		currComps, err := idsToComps.convert(arch.ComponentIDs)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrorComponentMismatchWithSavedState, err)
		}
		a.PushArchetype(arch.ID, NewLayout(currComps))
		a.archs[len(a.archs)-1].Entitys = arch.Entities
	}
	return nil
}

// Archetype is a collection of Entities for a specific archetype of components.
// This structure allows to quickly find Entities based on their components.
type Archetype struct {
	ID         ArchetypeID
	Entitys    []EntityID
	ArchLayout *Layout
}

var _ ArchetypeStorage = &Archetype{}

// NewArchetype creates a new archetype.
func NewArchetype(archID ArchetypeID, layout *Layout) *Archetype {
	return &Archetype{
		ID:         archID,
		Entitys:    make([]EntityID, 0, 256),
		ArchLayout: layout,
	}
}

// Layout is a collection of archetypes for a specific ArchLayout of components.
func (archetype *Archetype) Layout() *Layout {
	return archetype.ArchLayout
}

// Entities returns all Entities in this archetype.
func (archetype *Archetype) Entities() []EntityID {
	return archetype.Entitys
}

// SwapRemove removes an Ent from the archetype and returns it.
func (archetype *Archetype) SwapRemove(entityIndex ComponentIndex) EntityID {
	removed := archetype.Entitys[entityIndex]
	archetype.Entitys[entityIndex] = archetype.Entitys[len(archetype.Entitys)-1]
	archetype.Entitys = archetype.Entitys[:len(archetype.Entitys)-1]
	return removed
}

// LayoutMatches returns true if the given ArchLayout matches this archetype.
func (archetype *Archetype) LayoutMatches(components []component.IComponentType) bool {
	if len(archetype.ArchLayout.Components()) != len(components) {
		return false
	}
	for _, componentType := range components {
		if !archetype.ArchLayout.HasComponent(componentType) {
			return false
		}
	}
	return true
}

// PushEntity adds an Ent to the archetype.
func (archetype *Archetype) PushEntity(id EntityID) {
	archetype.Entitys = append(archetype.Entitys, id)
}

// Count returns the number of Entitys in the archetype.
func (archetype *Archetype) Count() int {
	return len(archetype.Entitys)
}
