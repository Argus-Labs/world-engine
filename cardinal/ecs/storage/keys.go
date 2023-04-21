package storage

import (
	"fmt"

	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
)

/*
	KEYS:
define keys for redis storage
	return fmt.Sprintf("WORLD-%s:CID-%d:A-%d", r.WorldID, r.ComponentStoragePrefix, Index)
- 	COMPONENT DATA: 	COMPD:WORLD-1:CID-0:A-5 -> component struct bytes
-	COMPONENT INDEX: 	CIDX:WORLD-1:CID-0:A-4 	-> Component Index
- 	ENTITY LOCATION: 	LOC:WORLD-1:E-1 		-> Location
- 	ENTITY LOCATION LEN LOCL:WORLD-1			-> Int
- 	ARCH COMP INDEX:    ACI:WORLD-1
- 	ENTRY STORAGE:      ENTRY:WORLD-1:ID  		-> entry struct bytes
- 	ENTITY MGR: 		ENTITY:WORLD-1:NEXTID 	-> uint64 id
*/

func (r *RedisStorage) componentDataKey(index ArchetypeIndex) string {
	return fmt.Sprintf("COMPD:WORLD-%s:CID-%d:A-%d", r.WorldID, r.ComponentStoragePrefix, index)
}

func (r *RedisStorage) componentIndexKey(index ArchetypeIndex) string {
	return fmt.Sprintf("CIDX:WORLD-%s:CID-%d:A-%d", r.WorldID, r.ComponentStoragePrefix, index)
}

func (r *RedisStorage) entityLocationKey(e entity.ID) string {
	return fmt.Sprintf("LOC:WORLD-%s:E-%d", r.WorldID, e)
}

func (r *RedisStorage) entityLocationLenKey() string {
	return fmt.Sprintf("LOCL:WORLD-%s", r.WorldID)
}

func (r *RedisStorage) archetypeStorageKey(ai ArchetypeIndex) string {
	return fmt.Sprintf("ARCH:WORLD-%s:A-%d", r.WorldID, ai)
}

func (r *RedisStorage) entryStorageKey(id entity.ID) string {
	return fmt.Sprintf("ENTRY:WORLD-%s:%d", r.WorldID, id)
}

func (r *RedisStorage) nextEntityIDKey() string {
	return fmt.Sprintf("ENTITY:WORLD-%s:NEXTID", r.WorldID)
}
