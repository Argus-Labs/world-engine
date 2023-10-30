package store

import (
	"encoding/json"

	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	component_metadata "pkg.world.dev/world-engine/cardinal/ecs/component/metadata"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
)

type Reader interface {
	// One Component One Entity
	GetComponentForEntity(cType component_metadata.ComponentMetadata, id entity.ID) (any, error)
	GetComponentForEntityInRawJSON(cType component_metadata.ComponentMetadata, id entity.ID) (json.RawMessage, error)

	// Many Components One Entity
	GetComponentTypesForEntity(id entity.ID) ([]component_metadata.ComponentMetadata, error)

	// One Archetype Many Components
	GetComponentTypesForArchID(archID archetype.ID) []component_metadata.ComponentMetadata
	GetArchIDForComponents(components []component_metadata.ComponentMetadata) (archetype.ID, error)

	// One Archetype Many Entities
	GetEntitiesForArchID(archID archetype.ID) ([]entity.ID, error)

	// Misc
	SearchFrom(filter filter.ComponentFilter, start int) *storage.ArchetypeIterator
	ArchetypeCount() int
}

type Writer interface {
	// One Entity
	RemoveEntity(id entity.ID) error

	// Many Components
	CreateEntity(comps ...component_metadata.ComponentMetadata) (entity.ID, error)
	CreateManyEntities(num int, comps ...component_metadata.ComponentMetadata) ([]entity.ID, error)

	// One Component One Entity
	SetComponentForEntity(cType component_metadata.ComponentMetadata, id entity.ID, value any) error
	AddComponentToEntity(cType component_metadata.ComponentMetadata, id entity.ID) error
	RemoveComponentFromEntity(cType component_metadata.ComponentMetadata, id entity.ID) error

	// Misc
	InjectLogger(logger *ecslog.Logger)
	Close() error
	RegisterComponents([]component_metadata.ComponentMetadata) error
}

type TickStorage interface {
	GetTickNumbers() (start, end uint64, err error)
	StartNextTick(txs []transaction.ITransaction, queues *transaction.TxQueue) error
	FinalizeTick() error
	Recover(txs []transaction.ITransaction) (*transaction.TxQueue, error)
}

// IManager represents all the methods required to track Component, Entity, and Archetype information
// which powers the ECS storage layer.
type IManager interface {
	TickStorage
	Reader
	Writer
	ToReadOnly() Reader
}
