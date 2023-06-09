package storage

// Location is a location of an Ent in the storage.
type Location struct {
	ArchID    ArchetypeID
	CompIndex ComponentIndex
}

// NewLocation creates a new EntityLocation.
func NewLocation(archetype ArchetypeID, component ComponentIndex) Location {
	return Location{
		ArchID:    archetype,
		CompIndex: component,
	}
}
