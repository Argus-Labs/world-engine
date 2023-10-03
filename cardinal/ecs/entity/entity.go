package entity

import "pkg.world.dev/world-engine/cardinal/public"

type Entity struct {
	ID  public.EntityID
	Loc public.ILocation
}

func (e *Entity) EntityID() public.EntityID {
	return e.ID
}

func (e *Entity) GetArchID() public.ArchetypeID {
	return e.Loc.GetArchID()
}

// Location is a location of an Entity in the storage.
type Location struct {
	ArchID    public.ArchetypeID
	CompIndex public.ComponentIndex
}

func (l *Location) GetArchID() public.ArchetypeID {
	return l.ArchID
}

func (l *Location) GetCompIndex() public.ComponentIndex {
	return l.CompIndex
}

func (l *Location) SetCompIndex(index public.ComponentIndex) {
	l.CompIndex = index
}

func (l *Location) SetArchID(id public.ArchetypeID) {
	l.ArchID = id
}

// NewLocation creates a new EntityLocation.
func NewLocation(archetype public.ArchetypeID, component public.ComponentIndex) public.ILocation {
	res := Location{
		ArchID:    archetype,
		CompIndex: component,
	}
	return &res
}
