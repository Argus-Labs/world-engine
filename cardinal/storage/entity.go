package storage

import (
	"github.com/argus-labs/cardinal/component"
	"github.com/argus-labs/cardinal/entity"
)

// Entity is identifier of an entity.
// Entity is just a wrapper of uint64.
type Entity = entity.Entity

// Null represents an invalid entity which is zero.
var Null = entity.Null

type WorldAccessor interface {
	Component(componentType component.IComponentType, index ArchetypeIndex, componentIndex ComponentIndex) []byte
	SetComponent(cType component.IComponentType, component []byte, index ArchetypeIndex, componentIndex ComponentIndex)
	GetLayout(index ArchetypeIndex) []component.IComponentType
	GetArchetypeForComponents([]component.IComponentType) ArchetypeIndex
	TransferArchetype(i1, i2 ArchetypeIndex, index ComponentIndex) ComponentIndex
	Entry(entity.Entity) *Entry
	Remove(entity.Entity)
	Valid(entity.Entity) bool
	Archetype(ArchetypeIndex) ArchetypeStorage
}

type EntityManager interface {
	Destroy(Entity)
	Length() int
	Get(int) Entity
	Shrink()
	GetNextEntityID() entity.ID
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

func (e *entityMgrImpl) Shrink() {
	e.destroyed = e.destroyed[:len(e.destroyed)-1]
}

// TODO(technicallyty): unsafe asf
func (e entityMgrImpl) Get(i int) Entity {
	return e.destroyed[i]
}

func (e entityMgrImpl) Length() int {
	return len(e.destroyed)
}

func (e *entityMgrImpl) Destroy(et Entity) {
	e.destroyed = append(e.destroyed, et)
}
