package gamestate

import (
	"fmt"
	"pkg.world.dev/world-engine/cardinal/types"
)

// redisComponentKey is the key that maps an entity EntityID and a specific component EntityID to the value of that component.
func redisComponentKey(typeID types.ComponentID, id types.EntityID) string {
	return fmt.Sprintf("ECB:COMPONENT-VALUE:TYPE-EntityID-%d:ENTITY-EntityID-%d", typeID, id)
}

// redisNextEntityIDKey is the key that stores the next available entity EntityID that can be assigned to a newly created
// entity.
func redisNextEntityIDKey() string {
	return "ECB:NEXT-ENTITY-EntityID"
}

// redisArchetypeIDForEntityID is the key that maps a specific entity EntityID to its archetype EntityID.
// Note, this key and redisActiveEntityIDKey represent the same information.
// This maps entity.EntityID -> archetype.ArchetypeID.
func redisArchetypeIDForEntityID(id types.EntityID) string {
	return fmt.Sprintf("ECB:ARCHETYPE-EntityID:ENTITY-EntityID-%d", id)
}

// redisActiveEntityIDKey is the key that maps an archetype EntityID to all the entities that currently belong
// to the archetype EntityID.
// Note, this key and redisArchetypeIDForEntityID represent the same information.
// This maps archetype.ArchetypeID -> []entity.EntityID.
func redisActiveEntityIDKey(archID types.ArchetypeID) string {
	return fmt.Sprintf("ECB:ACTIVE-ENTITY-IDS:ARCHETYPE-EntityID-%d", archID)
}

// redisArchIDsToCompTypesKey is the key that stores the map of archetype IDs to its relevant set of component types
// (in the form of []component.EntityID). To recover the actual ComponentMetadata information, a slice of active
// ComponentMetadata must be used.
func redisArchIDsToCompTypesKey() string {
	return "ECB:ARCHETYPE-EntityID-TO-COMPONENT-TYPES"
}

func redisStartTickKey() string {
	return "ECB:START-TICK"
}

func redisEndTickKey() string {
	return "ECB:END-TICK"
}

func redisPendingTransactionKey() string {
	return "ECB:PENDING-TRANSACTIONS"
}
