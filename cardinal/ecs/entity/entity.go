package entity

import (
	"pkg.world.dev/world-engine/cardinal/interfaces"
)

type Entity struct {
	ID  interfaces.EntityID
	Loc interfaces.ILocation
}

func (e *Entity) EntityID() interfaces.EntityID {
	return e.ID
}

func (e *Entity) GetArchID() interfaces.ArchetypeID {
	return e.Loc.GetArchID()
}

// Location is a location of an Entity in the storage.
type Location struct {
	ArchID    interfaces.ArchetypeID
	CompIndex interfaces.ComponentIndex
}

func (l *Location) GetArchID() interfaces.ArchetypeID {
	return l.ArchID
}

func (l *Location) GetCompIndex() interfaces.ComponentIndex {
	return l.CompIndex
}

func (l *Location) SetCompIndex(index interfaces.ComponentIndex) {
	l.CompIndex = index
}

func (l *Location) SetArchID(id interfaces.ArchetypeID) {
	l.ArchID = id
}

// NewLocation creates a new EntityLocation.
func NewLocation(archetype interfaces.ArchetypeID, component interfaces.ComponentIndex) interfaces.ILocation {
	res := Location{
		ArchID:    archetype,
		CompIndex: component,
	}
	return &res
}
