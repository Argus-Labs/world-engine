package storage

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"
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
	Push(layout *Layout)
	SearchFrom(filter filter.LayoutFilter, start int) *ArchetypeIterator
	Search(layoutFilter filter.LayoutFilter) *ArchetypeIterator
}

type ArchetypeStorage interface {
	PushArchetype(index ArchetypeIndex, layout []component.IComponentType)
	Archetype(index ArchetypeIndex) *types.Archetype
	RemoveEntity(index ArchetypeIndex, entityIndex int) entity.Entity
	PushEntity(ArchetypeIndex, entity.Entity)
	GetNextArchetypeIndex() (uint64, error)
}

// TODO(technicallyty): the below.
// LayoutMatches(components []component.IComponentType) bool <--- NEEDS TO BE ITS OWN FUNCTION.

type EntryStorage interface {
	SetEntry(entity.ID, *types.Entry) error
	GetEntry(entity.ID) (*types.Entry, error)
	SetEntity(entity.ID, Entity) error
	SetLocation(entity.ID, *types.Location) error
}

type EntityManager interface {
	Destroy(Entity)
	NewEntity() (Entity, error)
}
