package gamestate

import (
	"context"
	"encoding/json"

	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/txpool"
)

type Reader interface {
	// One Component One Entity
	GetComponentForEntity(cType types.ComponentMetadata, id types.EntityID) (any, error)
	GetComponentForEntityInRawJSON(cType types.ComponentMetadata, id types.EntityID) (json.RawMessage, error)

	// Many Components One Entity
	GetComponentTypesForEntity(id types.EntityID) ([]types.ComponentMetadata, error)

	// One Archetype Many Components
	GetComponentTypesForArchID(archID types.ArchetypeID) ([]types.ComponentMetadata, error)
	GetArchIDForComponents(components []types.ComponentMetadata) (types.ArchetypeID, error)

	// One Archetype Many Entities
	GetEntitiesForArchID(archID types.ArchetypeID) ([]types.EntityID, error)

	// Misc
	SearchFrom(filter filter.ComponentFilter, start int) *iterators.ArchetypeIterator
	ArchetypeCount() int
}

type Writer interface {
	// One Entity
	RemoveEntity(id types.EntityID) error

	// Many Components
	CreateEntity(comps ...types.ComponentMetadata) (types.EntityID, error)
	CreateManyEntities(num int, comps ...types.ComponentMetadata) ([]types.EntityID, error)

	// One Component One Entity
	SetComponentForEntity(cType types.ComponentMetadata, id types.EntityID, value any) error
	AddComponentToEntity(cType types.ComponentMetadata, id types.EntityID) error
	RemoveComponentFromEntity(cType types.ComponentMetadata, id types.EntityID) error

	// Misc
	InjectLogger(logger *zerolog.Logger)
	Close() error
	RegisterComponents([]types.ComponentMetadata) error
}

type TickStorage interface {
	GetTickNumbers() (start, end uint64, err error)
	StartNextTick(txs []types.Message, pool *txpool.TxPool) error
	FinalizeTick(ctx context.Context) error
	Recover(txs []types.Message) (*txpool.TxPool, error)
}

// Manager represents all the methods required to track Component, Entity, and Archetype information
// which powers the ECS dbStorage layer.
type Manager interface {
	TickStorage
	Reader
	Writer
	ToReadOnly() Reader
}
