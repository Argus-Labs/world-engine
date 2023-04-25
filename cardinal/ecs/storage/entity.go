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
	Archetype(ArchetypeIndex) ArchetypeStorage
	SetEntryLocation(id entity.ID, location Location) error
}

var _ EntityManager = &entityMgrImpl{}

func NewEntityManager() EntityManager {
	return &entityMgrImpl{destroyed: make([]Entity, 0, 256), nextID: 0}
}

type entityMgrImpl struct {
	destroyed []Entity
	nextID    entity.ID
}

func (e *entityMgrImpl) GetNextEntityID() entity.ID {
	e.nextID++
	return e.nextID
}

func (e *entityMgrImpl) shrink() {
	e.destroyed = e.destroyed[:len(e.destroyed)-1]
}

func (e *entityMgrImpl) NewEntity() (Entity, error) {
	if len(e.destroyed) == 0 {
		id := e.GetNextEntityID()
		return entity.NewEntity(id), nil
	}
	newEntity := e.destroyed[(len(e.destroyed) - 1)]
	e.shrink()
	return newEntity, nil
}

func (e *entityMgrImpl) Destroy(et Entity) {
	e.destroyed = append(e.destroyed, et)
}
