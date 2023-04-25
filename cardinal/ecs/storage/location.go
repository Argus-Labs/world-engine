package storage

import types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"

// Location is a location of an Ent in the storage.
type Location struct {
	ArchIndex ArchetypeIndex
	CompIndex ComponentIndex
	Valid     bool
}

// NewLocation creates a new EntityLocation.
func NewLocation(archetype ArchetypeIndex, component ComponentIndex) *types.Location {
	return &types.Location{
		ArchetypeIndex: uint64(archetype),
		ComponentIndex: uint64(component),
		Valid:          true,
	}
}
