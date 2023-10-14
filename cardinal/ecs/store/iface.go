package store

import (
	"encoding/json"

	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/component_metadata"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

type Reader interface {
	// One Component One Entity
	GetComponentForEntity(cType component_metadata.IComponentMetaData, id entity.ID) (any, error)
	GetComponentForEntityInRawJson(cType component_metadata.IComponentMetaData, id entity.ID) (json.RawMessage, error)

	// Many Components One Entity
	GetComponentTypesForEntity(id entity.ID) ([]component_metadata.IComponentMetaData, error)

	// One Archetype Many Components
	GetComponentTypesForArchID(archID archetype.ID) []component_metadata.IComponentMetaData
	GetArchIDForComponents(components []component_metadata.IComponentMetaData) (archetype.ID, error)

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
	CreateEntity(comps ...component_metadata.IComponentMetaData) (entity.ID, error)
	CreateManyEntities(num int, comps ...component_metadata.IComponentMetaData) ([]entity.ID, error)

	// One Component One Entity
	SetComponentForEntity(cType component_metadata.IComponentMetaData, id entity.ID, value any) error
	AddComponentToEntity(cType component_metadata.IComponentMetaData, id entity.ID) error
	RemoveComponentFromEntity(cType component_metadata.IComponentMetaData, id entity.ID) error

	// Misc
	InjectLogger(logger *ecslog.Logger)
	Close() error
	RegisterComponents([]component_metadata.IComponentMetaData) error
}

// IManager represents all the methods required to track Component, Entity, and Archetype information
// which powers the ECS storage layer.
type IManager interface {
	Reader
	Writer
}
