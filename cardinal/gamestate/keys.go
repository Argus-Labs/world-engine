package gamestate

import (
	"fmt"

	"pkg.world.dev/world-engine/cardinal/types"
)

// storageComponentKey is the key that maps an entity ID and a specific component ID to the value of that component.
func storageComponentKey(typeID types.ComponentID, id types.EntityID) string {
	return fmt.Sprintf("ECB:COMPONENT-VALUE:TYPE-ID-%d:ENTITY-ID-%d", typeID, id)
}

// storageNextEntityIDKey is the key that stores the next available entity ID that can be assigned to a newly created
// entity.
func storageNextEntityIDKey() string {
	return "ECB:NEXT-ENTITY-ID"
}

// storageArchetypeIDForEntityID is the key that maps a specific entity ID to its archetype ID.
// Note, this key and storageActiveEntityIDKey represent the same information.
// This maps entity.ID -> archetype.ID.
func storageArchetypeIDForEntityID(id types.EntityID) string {
	return fmt.Sprintf("ECB:ARCHETYPE-ID:ENTITY-ID-%d", id)
}

// storageActiveEntityIDKey is the key that maps an archetype ID to all the entities that currently belong
// to the archetype ID.
// Note, this key and storageArchetypeIDForEntityID represent the same information.
// This maps archetype.ID -> []entity.ID.
func storageActiveEntityIDKey(archID types.ArchetypeID) string {
	return fmt.Sprintf("ECB:ACTIVE-ENTITY-IDS:ARCHETYPE-ID-%d", archID)
}

// storageArchIDsToCompTypesKey is the key that stores the map of archetype IDs to its relevant set of component types
// (in the form of []component.ID). To recover the actual ComponentMetadata information, a slice of active
// ComponentMetadata must be used.
func storageArchIDsToCompTypesKey() string {
	return "ECB:ARCHETYPE-ID-TO-COMPONENT-TYPES"
}

func storageStartTickKey() string {
	return "ECB:START-TICK"
}

func storageEndTickKey() string {
	return "ECB:END-TICK"
}

func storagePendingTransactionKey() string {
	return "ECB:PENDING-TRANSACTIONS"
}
