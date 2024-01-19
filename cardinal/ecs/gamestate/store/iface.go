package store

import (
	"context"
	"encoding/json"

	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/cardinal/types/archetype"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
	"pkg.world.dev/world-engine/cardinal/types/message"
)

type Reader interface {
	// One Component One Entity
	GetComponentForEntity(cType component.ComponentMetadata, id entity.ID) (any, error)
	GetComponentForEntityInRawJSON(cType component.ComponentMetadata, id entity.ID) (json.RawMessage, error)

	// Many Components One Entity
	GetComponentTypesForEntity(id entity.ID) ([]component.ComponentMetadata, error)

	// One Archetype Many Components
	GetComponentTypesForArchID(archID archetype.ID) []component.ComponentMetadata
	GetArchIDForComponents(components []component.ComponentMetadata) (archetype.ID, error)

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
	CreateEntity(comps ...component.ComponentMetadata) (entity.ID, error)
	CreateManyEntities(num int, comps ...component.ComponentMetadata) ([]entity.ID, error)

	// One Component One Entity
	SetComponentForEntity(cType component.ComponentMetadata, id entity.ID, value any) error
	AddComponentToEntity(cType component.ComponentMetadata, id entity.ID) error
	RemoveComponentFromEntity(cType component.ComponentMetadata, id entity.ID) error

	// Misc
	InjectLogger(logger *zerolog.Logger)
	Close() error
	RegisterComponents([]component.ComponentMetadata) error
}

type TickStorage interface {
	GetTickNumbers() (start, end uint64, err error)
	StartNextTick(txs []message.Message, queues *txpool.TxQueue) error
	FinalizeTick(ctx context.Context) error
	Recover(txs []message.Message) (*txpool.TxQueue, error)
}

// IGameStateManager represents all the methods required to track Component, Entity, and Archetype information
// which powers the ECS storage layer.
type IGameStateManager interface {
	TickStorage
	Reader
	Writer
	ToReadOnly() Reader
}
