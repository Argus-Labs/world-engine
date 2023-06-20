package storage

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"
)

type ComponentStorage interface {
	PushComponent(component component.IComponentType, archID ArchetypeID) error
	Component(archetypeID ArchetypeID, componentIndex ComponentIndex) ([]byte, error)
	SetComponent(ArchetypeID, ComponentIndex, []byte) error
	MoveComponent(ArchetypeID, ComponentIndex, ArchetypeID) error
	SwapRemove(archetypeID ArchetypeID, componentIndex ComponentIndex) ([]byte, error)
	Contains(archetypeID ArchetypeID, componentIndex ComponentIndex) (bool, error)
}

type ComponentStorageManager interface {
	GetComponentStorage(cid component.TypeID) ComponentStorage
	GetComponentIndexStorage(cid component.TypeID) ComponentIndexStorage
}

type ComponentIndexStorage interface {
	// ComponentIndex returns the current index for this ArchetypeID. If the index doesn't currently
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
	SetLocation(EntityID, Location) error
	GetLocation(EntityID) (Location, error)
	ArchetypeID(id EntityID) (ArchetypeID, error)
	ComponentIndexForEntity(EntityID) (ComponentIndex, error)
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
	Push(layout *Layout)
	SearchFrom(filter filter.LayoutFilter, start int) *ArchetypeIterator
	Search(layoutFilter filter.LayoutFilter) *ArchetypeIterator
}

type ArchetypeAccessor interface {
	ComponentMarshaler
	PushArchetype(archID ArchetypeID, layout *Layout)
	Archetype(archID ArchetypeID) ArchetypeStorage
	Count() int
}

type ArchetypeStorage interface {
	Layout() *Layout
	Entities() []EntityID
	SwapRemove(entityIndex ComponentIndex) EntityID
	LayoutMatches(components []component.IComponentType) bool
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
	GetTickNumbers() (start, end int, err error)
	StartNextTick(txs []transaction.ITransaction, queues map[transaction.TypeID][]any) error
	FinalizeTick() error
	Recover(txs []transaction.ITransaction) (map[transaction.TypeID][]any, error)
}
