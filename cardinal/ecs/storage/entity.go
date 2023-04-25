package storage

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
	types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"
)

// Entity is identifier of an Ent.
// Entity is just a wrapper of uint64.
type Entity = entity.Entity

// Null represents an invalid Ent which is zero.
var Null = entity.Null

type WorldAccessor interface {
	Component(componentType component.IComponentType, index ArchetypeIndex, componentIndex ComponentIndex) ([]byte, error)
	SetComponent(component.IComponentType, []byte, ArchetypeIndex, ComponentIndex) error
	GetLayout(index ArchetypeIndex) []component.IComponentType
	GetArchetypeForComponents([]component.IComponentType) ArchetypeIndex
	TransferArchetype(ArchetypeIndex, ArchetypeIndex, ComponentIndex) (ComponentIndex, error)
	Entry(entity.Entity) (*types.Entry, error)
	Remove(entity.Entity) error
	Valid(entity.Entity) (bool, error)
	Archetype(ArchetypeIndex) *types.Archetype
	SetEntryLocation(id entity.ID, location *types.Location) error
}
