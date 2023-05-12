package storage

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/component"
)

type WorldAccessor interface {
	Component(componentType component.IComponentType, index ArchetypeIndex, componentIndex ComponentIndex) ([]byte, error)
	SetComponent(component.IComponentType, []byte, ArchetypeIndex, ComponentIndex) error
	GetLayout(index ArchetypeIndex) []component.IComponentType
	GetArchetypeForComponents([]component.IComponentType) ArchetypeIndex
	TransferArchetype(ArchetypeIndex, ArchetypeIndex, ComponentIndex) (ComponentIndex, error)
	Entry(id EntityID) (Entry, error)
	Remove(id EntityID) error
	Valid(id EntityID) (bool, error)
	Archetype(ArchetypeIndex) ArchetypeStorage
	SetEntryLocation(id EntityID, location Location) error
}

var _ EntityManager = &entityMgrImpl{}

func NewEntityManager() EntityManager {
	return &entityMgrImpl{destroyed: make([]EntityID, 0, 256), nextID: 0}
}

type entityMgrImpl struct {
	destroyed []EntityID
	nextID    EntityID
}

func (e *entityMgrImpl) GetNextEntityID() EntityID {
	e.nextID++
	return e.nextID
}

func (e *entityMgrImpl) shrink() {
	e.destroyed = e.destroyed[:len(e.destroyed)-1]
}

func (e *entityMgrImpl) NewEntity() (EntityID, error) {
	if len(e.destroyed) == 0 {
		id := e.GetNextEntityID()
		return id, nil
	}
	newEntity := e.destroyed[(len(e.destroyed) - 1)]
	e.shrink()
	return newEntity, nil
}

func (e *entityMgrImpl) Destroy(id EntityID) {
	e.destroyed = append(e.destroyed, id)
}
