package store

import (
	"encoding/json"

	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

type Reader interface {
	// One Entity
	GetEntity(id entity.ID) (entity.Entity, error)

	// One Component One Entity
	GetComponentForEntity(cType component.IComponentMetaData, id entity.ID) (any, error)
	GetComponentForEntityInRawJson(cType component.IComponentMetaData, id entity.ID) (json.RawMessage, error)

	// Many Components One Entity
	GetComponentTypesForEntity(id entity.ID) ([]component.IComponentMetaData, error)

	// One Archetype Many Components
	GetComponentTypesForArchID(archID archetype.ID) []component.IComponentMetaData
	GetArchIDForComponents(components []component.IComponentMetaData) (archetype.ID, error)

	// One Archetype Many Entities
	GetEntitiesForArchID(archID archetype.ID) []entity.ID

	// Misc
	SearchFrom(filter filter.ComponentFilter, start int) *storage.ArchetypeIterator
	ArchetypeCount() int
}

type Writer interface {
	// One Entity
	RemoveEntity(id entity.ID) error

	// Many Components
	CreateEntity(comps ...component.IComponentMetaData) (entity.ID, error)
	CreateManyEntities(num int, comps ...component.IComponentMetaData) ([]entity.ID, error)

	// One Component One Entity
	SetComponentForEntity(cType component.IComponentMetaData, id entity.ID, value any) error
	AddComponentToEntity(cType component.IComponentMetaData, id entity.ID) error
	RemoveComponentFromEntity(cType component.IComponentMetaData, id entity.ID) error

	// Misc
	InjectLogger(logger *ecslog.Logger)
	Close() error
	RegisterComponents([]component.IComponentMetaData) error
}

// IManager represents all the methods required to track Component, Entity, and Archetype information
// which powers the ECS storage layer.
type IManager interface {
	Reader
	Writer
}
