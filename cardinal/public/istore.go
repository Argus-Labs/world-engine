package public

import (
	"encoding/json"
	"io"
)

type IStoreManager interface {
	Close() error
	InjectLogger(logger IWorldLogger)
	GetEntity(id EntityID) (IEntity, error)
	RemoveEntity(id EntityID) error
	CreateEntity(comps ...IComponentType) (EntityID, error)
	CreateManyEntities(num int, comps ...IComponentType) ([]EntityID, error)
	SetComponentForEntity(cType IComponentType, id EntityID, value any) error
	GetComponentTypesForArchID(archID ArchetypeID) []IComponentType
	GetComponentTypesForEntity(id EntityID) ([]IComponentType, error)
	GetComponentForEntity(cType IComponentType, id EntityID) (any, error)
	GetComponentForEntityInRawJson(cType IComponentType, id EntityID) (json.RawMessage, error)
	GetArchIDForComponents(components []IComponentType) (ArchetypeID, error)
	GetArchAccessor() ArchetypeAccessor
	GetArchCompIdxStore() ArchetypeComponentIndex
	AddComponentToEntity(cType IComponentType, id EntityID) error
	RemoveComponentFromEntity(cType IComponentType, id EntityID) error
}

type OmniStorage interface {
	ComponentStorageManager
	ComponentIndexStorage
	EntityLocationStorage
	EntityManager
	StateStorage
	TickStorage
	NonceStorage
	io.Closer
}

type ComponentStorage interface {
	PushComponent(component IComponentType, archID ArchetypeID) error
	Component(archetypeID ArchetypeID, componentIndex ComponentIndex) ([]byte, error)
	SetComponent(ArchetypeID, ComponentIndex, []byte) error
	MoveComponent(ArchetypeID, ComponentIndex, ArchetypeID) error
	SwapRemove(archetypeID ArchetypeID, componentIndex ComponentIndex) ([]byte, error)
	Contains(archetypeID ArchetypeID, componentIndex ComponentIndex) (bool, error)
}

type ComponentStorageManager interface {
	GetComponentStorage(cid ComponentTypeID) ComponentStorage
	GetComponentIndexStorage(cid ComponentTypeID) ComponentIndexStorage
}

type ComponentIndexStorage interface {
	// ComponentIndex returns the current index for this archetype.ID. If the index doesn't currently
	// exist then "0, false, nil" is returned
	ComponentIndex(ArchetypeID) (ComponentIndex, bool, error)
	// SetIndex sets the index of the given archerype index. This is the next index that will be assigned to a new
	// entity in this archetype.
	SetIndex(ArchetypeID, ComponentIndex) error
	// IncrementIndex increments an index for this archetype and returns the new value. If the index
	// does not yet exist, set the index to 0 and return 0.
	IncrementIndex(ArchetypeID) (ComponentIndex, error)
	// DecrementIndex decrements an index for this archetype by 1. The index is allowed to go into
	// negative numbers.
	DecrementIndex(ArchetypeID) error
}

type EntityLocationStorage interface {
	ContainsEntity(EntityID) (bool, error)
	Remove(EntityID) error
	Insert(EntityID, ArchetypeID, ComponentIndex) error
	SetLocation(EntityID, ILocation) error
	GetLocation(EntityID) (ILocation, error)
	ArchetypeID(id EntityID) (ArchetypeID, error)
	ComponentIndexForEntity(EntityID) (ComponentIndex, error)
	Len() (int, error)
}

// ComponentMarshaler is an interface that can marshal and unmarshal itself to bytes. Since
// IComponentType are interfaces (and not easily serilizable) a list of instantiated
// components is required to unmarshal the data.
type ComponentMarshaler interface {
	Marshal() ([]byte, error)
	UnmarshalWithComps([]byte, []IComponentType) error
}

type ArchetypeComponentIndex interface {
	ComponentMarshaler
	Push(comps []IComponentType)
	SearchFrom(filter IComponentFilter, start int) IArchtypeIterator
	Search(compfilter IComponentFilter) IArchtypeIterator
}

type ArchetypeAccessor interface {
	ComponentMarshaler
	PushArchetype(archID ArchetypeID, comps []IComponentType)
	Archetype(archID ArchetypeID) ArchetypeStorage
	Count() int
}

type ArchetypeStorage interface {
	Components() []IComponentType
	Entities() []EntityID
	SwapRemove(entityIndex ComponentIndex) EntityID
	ComponentsMatch(components []IComponentType) bool
	PushEntity(entity EntityID)
	Count() int
}

type EntityManager interface {
	Destroy(EntityID)
	NewEntity() (EntityID, error)
}

type StateStorage interface {
	Save(key string, data []byte) error
	Load(key string) (data []byte, ok bool, err error)
}

type TickStorage interface {
	GetTickNumbers() (start, end uint64, err error)
	StartNextTick(txs []ITransaction, queues ITxQueue) error
	FinalizeTick() error
	Recover(txs []ITransaction) (ITxQueue, error)
}

type NonceStorage interface {
	GetNonce(key string) (uint64, error)
	SetNonce(key string, nonce uint64) error
}
