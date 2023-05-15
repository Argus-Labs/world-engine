package storage

// Location is a location of an Ent in the storage.
type Location struct {
	ArchIndex ArchetypeIndex
	CompIndex ComponentIndex
}

// NewLocation creates a new EntityLocation.
func NewLocation(archetype ArchetypeIndex, component ComponentIndex) Location {
	return Location{
		ArchIndex: archetype,
		CompIndex: component,
	}
}
