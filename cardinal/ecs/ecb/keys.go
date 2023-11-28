package ecb

import (
	"fmt"

	"pkg.world.dev/world-engine/cardinal/types/archetype"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

// redisComponentKey is the key that maps an entity ID and a specific component ID to the value of that component.
func redisComponentKey(typeID component.TypeID, id entity.ID) string {
	return fmt.Sprintf("ECB:COMPONENT-VALUE:TYPE-ID-%d:ENTITY-ID-%d", typeID, id)
}

// redisNextEntityIDKey is the key that stores the next available entity ID that can be assigned to a newly created
// entity.
func redisNextEntityIDKey() string {
	return "ECB:NEXT-ENTITY-ID"
}

// redisArchetypeIDForEntityID is the key that maps a specific entity ID to its archetype ID.
// Note, this key and redisActiveEntityIDKey represent the same information.
// This maps entity.ID -> archetype.ID.
func redisArchetypeIDForEntityID(id entity.ID) string {
	return fmt.Sprintf("ECB:ARCHETYPE-ID:ENTITY-ID-%d", id)
}

// redisActiveEntityIDKey is the key that maps an archetype ID to all the entities that currently belong
// to the archetype ID.
// Note, this key and redisArchetypeIDForEntityID represent the same information.
// This maps archetype.ID -> []entity.ID.
func redisActiveEntityIDKey(archID archetype.ID) string {
	return fmt.Sprintf("ECB:ACTIVE-ENTITY-IDS:ARCHETYPE-ID-%d", archID)
}

// redisArchIDsToCompTypesKey is the key that stores the map of archetype IDs to its relevant set of component types
// (in the form of []component.ID). To recover the actual ComponentMetadata information, a slice of active
// ComponentMetadata must be used.
func redisArchIDsToCompTypesKey() string {
	return "ECB:ARCHETYPE-ID-TO-COMPONENT-TYPES"
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
