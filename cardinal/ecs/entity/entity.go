package entity

import (
	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/entityid"
)

type Entity struct {
	ID  entityid.ID
	Loc Location
}

func (e Entity) EntityID() entityid.ID {
	return e.ID
}

// Location is a location of an Entity in the storage.
type Location struct {
	ArchID    archetype.ID
	CompIndex component.Index
}

// NewLocation creates a new EntityLocation.
func NewLocation(archetype archetype.ID, component component.Index) Location {
	return Location{
		ArchID:    archetype,
		CompIndex: component,
	}
}
