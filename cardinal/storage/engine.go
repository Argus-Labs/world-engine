package storage

import (
	"github.com/argus-labs/cardinal/component"
	"github.com/argus-labs/cardinal/entity"
	"github.com/argus-labs/cardinal/filter"
)

type ComponentStorage interface {
	PushComponent(component component.IComponentType, index ArchetypeIndex) error
	Component(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) []byte
	SetComponent(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex, compBz []byte)
	MoveComponent(source ArchetypeIndex, index ComponentIndex, dst ArchetypeIndex)
	SwapRemove(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) []byte
	Contains(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) bool
}

type ComponentStorageManager interface {
	GetComponentStorage(cid component.TypeID) ComponentStorage
	GetComponentIndexStorage(cid component.TypeID) ComponentIndexStorage
	InitializeComponentStorage(cid component.TypeID)
}

type ComponentIndexStorage interface {
	ComponentIndex(ai ArchetypeIndex) (ComponentIndex, bool)
	SetIndex(ArchetypeIndex, ComponentIndex)
	IncrementIndex(ArchetypeIndex)
	DecrementIndex(ArchetypeIndex)
}

type EntityLocationStorage interface {
	Contains(id entity.ID) bool
	Remove(id entity.ID)
	Insert(id entity.ID, index ArchetypeIndex, componentIndex ComponentIndex)
	Set(id entity.ID, location *Location)
	Location(id entity.ID) *Location
	ArchetypeIndex(id entity.ID) ArchetypeIndex
	ComponentIndex(id entity.ID) ComponentIndex
	Len() int
}

type ArchetypeComponentIndex interface {
	Push(layout *Layout)
	SearchFrom(filter filter.LayoutFilter, start int) *ArchetypeIterator
	Search(layoutFilter filter.LayoutFilter) *ArchetypeIterator
}

type ArchetypeAccessor interface {
	Archetypes() []*Archetype
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
