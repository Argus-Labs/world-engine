package storage

import (
	"fmt"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
)

/*
	KEYS:
define keys for redis storage
	return fmt.Sprintf("WORLD-%s:CID-%d:A-%d", r.WorldID, r.ComponentStoragePrefix, id)
- 	COMPONENT DATA: 	COMPD:WORLD-1:CID-<comp-id>:A-<arch-id>
	List of []bytes: Each item in this list deserializes to a specific component.TypeID. This component data is tied
	to a specific entity. An entity's Location information tells what index into this list belongs to the entity.

-	ARCHETYPE INDEX: 	ARCHIDX:WORLD-1:A-<arch-id>
	Integer: The next available index for this archetype that can be assigned to an entity.

- 	ENTITY LOCATION: 	LOC:WORLD-1:E-<entity-id>		-> Location
	[]bytes: Deserializes to a storage.Location. The Location tells what archetype this entity belongs to and what index
	into that archetype contains this entity's component data.

- 	ENTITY LOCATION LEN LOCL:WORLD-1			-> Int
	Integer: The number of entities that exist in this world.

- 	ENTITY MGR: 		ENTITY:WORLD-1:NEXTID 	-> uint64 id
	Integer: The entity ID that should be given to the next created entity. This is unique across
	the world.

-	STATE STORAGE:      STATE:WORLD-1:<sub-key> -> arbitrary bytes to save world state
	[]bytes: Arbitrary slice of bytes used for saving and loading state. Currently being used for
	saving the set of component.TypeIDs that belong to each Archetype.
*/

func (r *RedisStorage) componentDataKey(archID ArchetypeID, compID component.TypeID) string {
	return fmt.Sprintf("COMPD:WORLD-%s:CID-%d:A-%d", r.WorldID, compID, archID)
}

func (r *RedisStorage) archetypeIndexKey(id ArchetypeID) string {
	return fmt.Sprintf("ARCHIDX:WORLD-%s:A-%d", r.WorldID, id)
}

func (r *RedisStorage) entityLocationKey(id EntityID) string {
	return fmt.Sprintf("LOC:WORLD-%s:E-%d", r.WorldID, id)
}

func (r *RedisStorage) entityLocationLenKey() string {
	return fmt.Sprintf("LOCL:WORLD-%s", r.WorldID)
}

func (r *RedisStorage) nextEntityIDKey() string {
	return fmt.Sprintf("ENTITY:WORLD-%s:NEXTID", r.WorldID)
}

func (r *RedisStorage) stateStorageKey(subKey string) string {
	return fmt.Sprintf("STATE:WORLD-%s:%s", r.WorldID, subKey)
}

func (r *RedisStorage) tickKey() string {
	return "TICK"
}

func (r *RedisStorage) pendingTransactionsKey() string {
	return "PENDING:TRANSACTIONS"
}
