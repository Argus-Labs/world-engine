package storage

import (
	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/entityid"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
)

type ComponentStorage interface {
	PushComponent(component component.IComponentType, archID archetype.ID) error
	Component(archetypeID archetype.ID, componentIndex component.Index) ([]byte, error)
	SetComponent(archetype.ID, component.Index, []byte) error
	MoveComponent(archetype.ID, component.Index, archetype.ID) error
	SwapRemove(archetypeID archetype.ID, componentIndex component.Index) ([]byte, error)
	Contains(archetypeID archetype.ID, componentIndex component.Index) (bool, error)
}

type ComponentStorageManager interface {
	GetComponentStorage(cid component.TypeID) ComponentStorage
	GetComponentIndexStorage(cid component.TypeID) ComponentIndexStorage
}

type ComponentIndexStorage interface {
	// ComponentIndex returns the current index for this archetype.ID. If the index doesn't currently
	// exist then "0, false, nil" is returned
	ComponentIndex(archetype.ID) (component.Index, bool, error)
	// SetIndex sets the index of the given archerype index. This is the next index that will be assigned to a new
	// entity in this archetype.
	SetIndex(archetype.ID, component.Index) error
	// IncrementIndex increments an index for this archetype and returns the new value. If the index
	// does not yet exist, set the index to 0 and return 0.
	IncrementIndex(archetype.ID) (component.Index, error)
	// DecrementIndex decrements an index for this archetype by 1. The index is allowed to go into
	// negative numbers.
	DecrementIndex(archetype.ID) error
}

type EntityLocationStorage interface {
	ContainsEntity(entityid.ID) (bool, error)
	Remove(entityid.ID) error
	Insert(entityid.ID, archetype.ID, component.Index) error
	SetLocation(entityid.ID, entity.Location) error
	GetLocation(entityid.ID) (entity.Location, error)
	ArchetypeID(id entityid.ID) (archetype.ID, error)
	ComponentIndexForEntity(entityid.ID) (component.Index, error)
	Len() (int, error)
}

// ComponentMarshaler is an interface that can marshal and unmarshal itself to bytes. Since
// IComponentType are interfaces (and not easily serilizable) a list of instantiated
// components is required to unmarshal the data.
type ComponentMarshaler interface {
	Marshal() ([]byte, error)
	UnmarshalWithComps([]byte, []component.IComponentType) error
}

type ArchetypeComponentIndex interface {
	ComponentMarshaler
	Push(comps []component.IComponentType)
	SearchFrom(filter filter.ComponentFilter, start int) *ArchetypeIterator
	Search(compfilter filter.ComponentFilter) *ArchetypeIterator
}

type ArchetypeAccessor interface {
	ComponentMarshaler
	PushArchetype(archID archetype.ID, comps []component.IComponentType)
	Archetype(archID archetype.ID) ArchetypeStorage
	Count() int
}

type ArchetypeStorage interface {
	Components() []component.IComponentType
	Entities() []entityid.ID
	SwapRemove(entityIndex component.Index) entityid.ID
	ComponentsMatch(components []component.IComponentType) bool
	PushEntity(entity entityid.ID)
	Count() int
}

type EntityManager interface {
	Destroy(entityid.ID)
	NewEntity() (entityid.ID, error)
}

type StateStorage interface {
	Save(key string, data []byte) error
	Load(key string) (data []byte, ok bool, err error)
}

type TickStorage interface {
	GetTickNumbers() (start, end uint64, err error)
	StartNextTick(txs []transaction.ITransaction, queues *transaction.TxQueue) error
	FinalizeTick() error
	Recover(txs []transaction.ITransaction) (*transaction.TxQueue, error)
}

type NonceStorage interface {
	GetNonce(key string) (uint64, error)
	SetNonce(key string, nonce uint64) error
}
