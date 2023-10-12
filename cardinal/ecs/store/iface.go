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
	// One Component One Entity
	GetComponentForEntity(cType component.IComponentType, id entity.ID) (any, error)
	GetComponentForEntityInRawJson(cType component.IComponentType, id entity.ID) (json.RawMessage, error)

	// Many Components One Entity
	GetComponentTypesForEntity(id entity.ID) ([]component.IComponentType, error)

	// One Archetype Many Components
	GetComponentTypesForArchID(archID archetype.ID) []component.IComponentType
	GetArchIDForComponents(components []component.IComponentType) (archetype.ID, error)

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
	CreateEntity(comps ...component.IComponentType) (entity.ID, error)
	CreateManyEntities(num int, comps ...component.IComponentType) ([]entity.ID, error)

	// One Component One Entity
	SetComponentForEntity(cType component.IComponentType, id entity.ID, value any) error
	AddComponentToEntity(cType component.IComponentType, id entity.ID) error
	RemoveComponentFromEntity(cType component.IComponentType, id entity.ID) error

	// Misc
	InjectLogger(logger *ecslog.Logger)
	Close() error
	RegisterComponents([]component.IComponentType) error
}

// IManager represents all the methods required to track Component, Entity, and Archetype information
// which powers the ECS storage layer.
type IManager interface {
	Reader
	Writer
}
