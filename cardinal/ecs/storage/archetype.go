package storage

import (
	"errors"
	"fmt"

	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	"pkg.world.dev/world-engine/cardinal/ecs/component_metadata"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
)

var _ ArchetypeAccessor = &archetypeStorageImpl{}

func NewArchetypeAccessor() ArchetypeAccessor {
	return &archetypeStorageImpl{archs: make([]*Archetype, 0)}
}

type archetypeStorageImpl struct {
	archs []*Archetype
}

func (a *archetypeStorageImpl) PushArchetype(archID archetype.ID, comps []component_metadata.IComponentMetaData) {
	a.archs = append(a.archs, &Archetype{
		ID:      archID,
		Entitys: make([]entity.ID, 0, 256),
		Comps:   comps,
	})
}

func (a *archetypeStorageImpl) Count() int {
	return len(a.archs)
}

func (a *archetypeStorageImpl) Archetype(archID archetype.ID) ArchetypeStorage {
	return a.archs[archID]
}

// archForStorage is a helper struct that is used to serialize/deserialize the archetypeStorageImpl
// struct to bytes. The IComponentMetaData interfaces do not serialize to bytes easily, so instead
// we just extract the TypeIDs and serialize the ids to bytes. On deserilization we need a
// slice of IComponentTypes with the correct TypeIDs so that we can recover the original
// archetypeStorageImpl.
type archForStorage struct {
	ID           archetype.ID
	Entities     []entity.ID
	ComponentIDs []component_metadata.TypeID
}

// Marshal converts the archetypeStorageImpl to bytes. Only the IDs from the IComponentTypes
// are serialized.
func (a *archetypeStorageImpl) Marshal() ([]byte, error) {
	archs := make([]archForStorage, len(a.archs))
	for i := range archs {
		archs[i].ID = a.archs[i].ID
		archs[i].Entities = a.archs[i].Entitys
		for _, c := range a.archs[i].Components() {
			archs[i].ComponentIDs = append(archs[i].ComponentIDs, c.ID())
		}
	}
	return codec.Encode(archs)
}

var (
	// ErrorComponentMismatchWithSavedState is an error that is returned when a TypeID from
	// the saved state is not found in the passed in list of components.
	ErrorComponentMismatchWithSavedState = errors.New("registered components do not match with the saved state")
)

// idsToComponents converts slices of TypeIDs to the corresponding IComponentTypes
type idsToComponents struct {
	m map[component_metadata.TypeID]component_metadata.IComponentMetaData
}

func newIDsToComponents(components []component_metadata.IComponentMetaData) idsToComponents {
	m := map[component_metadata.TypeID]component_metadata.IComponentMetaData{}
	for i, comp := range components {
		m[comp.ID()] = components[i]
	}
	return idsToComponents{m: m}
}

func (c idsToComponents) convert(ids []component_metadata.TypeID) (comps []component_metadata.IComponentMetaData, ok error) {
	comps = []component_metadata.IComponentMetaData{}
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
func (a *archetypeStorageImpl) UnmarshalWithComps(bytes []byte, components []component_metadata.IComponentMetaData) error {
	archetypesFromStorage, err := codec.Decode[[]archForStorage](bytes)
	if err != nil {
		return err
	}
	idsToComps := newIDsToComponents(components)

	for _, arch := range archetypesFromStorage {
		currComps, err := idsToComps.convert(arch.ComponentIDs)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrorComponentMismatchWithSavedState, err)
		}
		a.PushArchetype(arch.ID, currComps)
		a.archs[len(a.archs)-1].Entitys = arch.Entities
	}
	return nil
}

// Archetype is a collection of Entities for a specific archetype of components.
// This structure allows to quickly find Entities based on their components.
type Archetype struct {
	ID      archetype.ID
	Entitys []entity.ID
	Comps   []component_metadata.IComponentMetaData
}

var _ ArchetypeStorage = &Archetype{}

// NewArchetype creates a new archetype.
func NewArchetype(archID archetype.ID, components []component_metadata.IComponentMetaData) *Archetype {
	return &Archetype{
		ID:      archID,
		Entitys: make([]entity.ID, 0, 256),
		Comps:   components,
	}
}

// Components returns the slice of components associated with this archetype.
func (archetype *Archetype) Components() []component_metadata.IComponentMetaData {
	return archetype.Comps
}

// Entities returns all Entities in this archetype.
func (archetype *Archetype) Entities() []entity.ID {
	return archetype.Entitys
}

// SwapRemove removes an Ent from the archetype and returns it.
func (archetype *Archetype) SwapRemove(entityIndex component_metadata.Index) entity.ID {
	removed := archetype.Entitys[entityIndex]
	archetype.Entitys[entityIndex] = archetype.Entitys[len(archetype.Entitys)-1]
	archetype.Entitys = archetype.Entitys[:len(archetype.Entitys)-1]
	return removed
}

// ComponentsMatch returns true if the given components matches this archetype.
func (archetype *Archetype) ComponentsMatch(components []component_metadata.IComponentMetaData) bool {
	if len(archetype.Components()) != len(components) {
		return false
	}
	for _, componentType := range components {
		if !filter.MatchComponentMetaData(archetype.Comps, componentType) {
			return false
		}
	}
	return true
}

// PushEntity adds an Ent to the archetype.
func (archetype *Archetype) PushEntity(id entity.ID) {
	archetype.Entitys = append(archetype.Entitys, id)
}

// Count returns the number of Entitys in the archetype.
func (archetype *Archetype) Count() int {
	return len(archetype.Entitys)
}
