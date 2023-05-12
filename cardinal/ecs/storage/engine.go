package storage

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
)

type ComponentStorage interface {
	PushComponent(component component.IComponentType, index ArchetypeIndex) error
	Component(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) ([]byte, error)
	SetComponent(ArchetypeIndex, ComponentIndex, []byte) error
	MoveComponent(ArchetypeIndex, ComponentIndex, ArchetypeIndex) error
	SwapRemove(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) ([]byte, error)
	Contains(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) (bool, error)
}

type ComponentStorageManager interface {
	GetComponentStorage(cid component.TypeID) ComponentStorage
	GetComponentIndexStorage(cid component.TypeID) ComponentIndexStorage
}

type ComponentIndexStorage interface {
	// ComponentIndex returns the current index for this ArchetypeIndex. If the index doesn't currently
	// exist then "0, false, nil" is returned
	ComponentIndex(ArchetypeIndex) (ComponentIndex, bool, error)
	// SetIndex sets the index to the given calue.
	SetIndex(ArchetypeIndex, ComponentIndex) error
	// IncrementIndex increments an index for this archetype and returns the new value. If the index
	// does not yet exist, set the index to 0 and return 0.
	IncrementIndex(ArchetypeIndex) (ComponentIndex, error)
	// DecrementIndex decrements an index for this archetype by 1. The index is allowed to go into
	// negative numbers.
	DecrementIndex(ArchetypeIndex) error
}

type EntityLocationStorage interface {
	ContainsEntity(EntityID) (bool, error)
	Remove(EntityID) error
	Insert(EntityID, ArchetypeIndex, ComponentIndex) error
	Set(EntityID, Location) error
	Location(EntityID) (Location, error)
	ArchetypeIndex(id EntityID) (ArchetypeIndex, error)
	ComponentIndexForEntity(EntityID) (ComponentIndex, error)
	Len() (int, error)
}

type ArchetypeComponentIndex interface {
	Push(layout *Layout)
	SearchFrom(filter filter.LayoutFilter, start int) *ArchetypeIterator
	Search(layoutFilter filter.LayoutFilter) *ArchetypeIterator
}

type ArchetypeAccessor interface {
	PushArchetype(index ArchetypeIndex, layout *Layout)
	Archetype(index ArchetypeIndex) ArchetypeStorage
	Count() int
}

type ArchetypeStorage interface {
	Layout() *Layout
	Entities() []EntityID
	SwapRemove(entityIndex ComponentIndex) EntityID
	LayoutMatches(components []component.IComponentType) bool
	PushEntity(entity EntityID)
	Count() int
}

type EntryStorage interface {
	SetEntry(EntityID, Entry) error
	GetEntry(EntityID) (Entry, error)
	SetLocation(EntityID, Location) error
}

type EntityManager interface {
	Destroy(EntityID)
	NewEntity() (EntityID, error)
}
