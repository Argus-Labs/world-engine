package storage

import (
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"
)

type ComponentStorage interface {
	PushComponent(component.IComponentType, ArchetypeIndex) (ComponentIndex, error)
	PushRawComponent(*anypb.Any, ArchetypeIndex) error
	Component(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) (component.IComponentType, error)
	SetComponent(ArchetypeIndex, ComponentIndex, component.IComponentType) error
	// MoveComponent moves the component from one Arch-Component to another.
	MoveComponent(ArchetypeIndex, ComponentIndex, ArchetypeIndex) error
	RemoveComponent(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) error
	Contains(ArchetypeIndex, ComponentIndex) (bool, error)
}

type ComponentStorageManager interface {
	GetComponentStorage(string) ComponentStorage
	GetComponentIndexStorage(component.IComponentType) ComponentIndexStorage
}

type ComponentIndexStorage interface {
	ComponentIndex(ArchetypeIndex) (ComponentIndex, bool, error)
	SetIndex(ArchetypeIndex, ComponentIndex) error
	IncrementIndex(ArchetypeIndex) error
	DecrementIndex(ArchetypeIndex) error
}

type EntityLocationStorage interface {
	ContainsEntity(entity.ID) (bool, error)
	Remove(entity.ID) error
	Insert(entity.ID, ArchetypeIndex, ComponentIndex) error
	Set(entity.ID, *types.Location) error
	Location(entity.ID) (*types.Location, error)
	ArchetypeIndex(id entity.ID) (ArchetypeIndex, error)
	ComponentIndexForEntity(entity.ID) (ComponentIndex, error)
	Len() (int, error)
}

type ArchetypeComponentIndex interface {
	Push(layout []component.IComponentType)
	SearchFrom(filter filter.LayoutFilter, start int) *ArchetypeIterator
	Search(layoutFilter filter.LayoutFilter) *ArchetypeIterator
}

type ArchetypeStorage interface {
	PushArchetype(index ArchetypeIndex, layout []component.IComponentType) error
	Archetype(index ArchetypeIndex) (*types.Archetype, error)
	RemoveEntityAt(index ArchetypeIndex, entityIndex int) (entity.Entity, error)
	RemoveEntity(ArchetypeIndex, entity.Entity) error
	PushEntity(ArchetypeIndex, entity.Entity) error
	GetNextArchetypeIndex() (uint64, error)
}

type EntryStorage interface {
	SetEntry(*types.Entry) error
	GetEntry(entity.ID) (*types.Entry, error)
	SetEntity(entity.ID, Entity) error
	SetLocation(entity.ID, *types.Location) error
}

type EntityManager interface {
	Destroy(Entity)
	NewEntity() (Entity, error)
}
