package storage

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
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
	ContainsEntity(entity.ID) (bool, error)
	Remove(entity.ID) error
	Insert(entity.ID, ArchetypeIndex, ComponentIndex) error
	Set(entity.ID, *Location) error
	Location(entity.ID) (*Location, error)
	ArchetypeIndex(id entity.ID) (ArchetypeIndex, error)
	ComponentIndexForEntity(entity.ID) (ComponentIndex, error)
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
	Entities() []entity.Entity
	SwapRemove(entityIndex int) entity.Entity
	LayoutMatches(components []component.IComponentType) bool
	PushEntity(entity entity.Entity)
	Count() int
}

type EntryStorage interface {
	SetEntry(entity.ID, *Entry) error
	GetEntry(entity.ID) (*Entry, error)
	SetEntity(entity.ID, Entity) error
	SetLocation(entity.ID, Location) error
}

type EntityManager interface {
	Destroy(Entity)
	NewEntity() (Entity, error)
}
