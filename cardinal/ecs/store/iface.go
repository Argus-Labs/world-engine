package store

import (
	"encoding/json"
	"pkg.world.dev/world-engine/cardinal/ecs/message"

	"pkg.world.dev/world-engine/cardinal/ecs/component/metadata"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/types/archetype"
)

type Reader interface {
	// One Component One Entity
	GetComponentForEntity(cType metadata.ComponentMetadata, id entity.ID) (any, error)
	GetComponentForEntityInRawJSON(cType metadata.ComponentMetadata, id entity.ID) (json.RawMessage, error)

	// Many Components One Entity
	GetComponentTypesForEntity(id entity.ID) ([]metadata.ComponentMetadata, error)

	// One Archetype Many Components
	GetComponentTypesForArchID(archID archetype.ID) []metadata.ComponentMetadata
	GetArchIDForComponents(components []metadata.ComponentMetadata) (archetype.ID, error)

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
	CreateEntity(comps ...metadata.ComponentMetadata) (entity.ID, error)
	CreateManyEntities(num int, comps ...metadata.ComponentMetadata) ([]entity.ID, error)

	// One Component One Entity
	SetComponentForEntity(cType metadata.ComponentMetadata, id entity.ID, value any) error
	AddComponentToEntity(cType metadata.ComponentMetadata, id entity.ID) error
	RemoveComponentFromEntity(cType metadata.ComponentMetadata, id entity.ID) error

	// Misc
	InjectLogger(logger *ecslog.Logger)
	Close() error
	RegisterComponents([]metadata.ComponentMetadata) error
}

type TickStorage interface {
	GetTickNumbers() (start, end uint64, err error)
	StartNextTick(txs []message.Message, queues *message.TxQueue) error
	FinalizeTick() error
	Recover(txs []message.Message) (*message.TxQueue, error)
}

// IManager represents all the methods required to track Component, Entity, and Archetype information
// which powers the ECS storage layer.
type IManager interface {
	TickStorage
	Reader
	Writer
	ToReadOnly() Reader
}
