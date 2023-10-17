package entity

import (
	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/component_metadata"
)

type ID uint64

type Entity struct {
	ID  ID
	Loc Location
}

func (e Entity) EntityID() ID {
	return e.ID
}

// Location is a location of an Entity in the storage.
type Location struct {
	ArchID    archetype.ID
	CompIndex component_metadata.Index
}

// NewLocation creates a new EntityLocation.
func NewLocation(archetype archetype.ID, component component_metadata.Index) Location {
	return Location{
		ArchID:    archetype,
		CompIndex: component,
	}
}
