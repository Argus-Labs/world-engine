package storage

import (
	"fmt"

	"github.com/argus-labs/cardinal/ECS/entity"
)

/*
	KEYS:
define keys for redis storage
	return fmt.Sprintf("WORLD-%s:CID-%d:A-%d", r.worldID, r.componentStoragePrefix, Index)
- 	COMPONENT DATA: 	COMPD:WORLD-1:CID-0:A-5 -> component struct bytes
-	COMPONENT INDEX: 	CIDX:WORLD-1:CID-0:A-4 	-> Component Index
- 	ENTITY LOCATION: 	LOC:WORLD-1:E-1 		-> Location
- 	ENTITY LOCATION LEN LOCL:WORLD-1			-> Int
- 	ARCH COMP INDEX:    ACI:WORLD-1
- 	ENTRY STORAGE:      ENTRY:WORLD-1:ID  		-> entry struct bytes
- 	ENTITY MGR: 		ENTITY:WORLD-1:NEXTID 	-> uint64 id
*/

func (r redisStorage) componentDataKey(index ArchetypeIndex) string {
	return fmt.Sprintf("COMPD:WORLD-%s:CID-%d:A-%d", r.worldID, r.componentStoragePrefix, index)
}

func (r redisStorage) componentIndexKey(index ArchetypeIndex) string {
	return fmt.Sprintf("CIDX:WORLD-%s:CID-%d:A-%d", r.worldID, r.componentStoragePrefix, index)
}

func (r redisStorage) entityLocationKey(e entity.ID) string {
	return fmt.Sprintf("LOC:WORLD-%s:E-%d", r.worldID, e)
}

func (r redisStorage) entityLocationLenKey() string {
	return fmt.Sprintf("LOCL:WORLD-%s", r.worldID)
}

func (r redisStorage) archetypeStorageKey(ai ArchetypeIndex) string {
	return fmt.Sprintf("ARCH:WORLD-%s:A-%d", r.worldID, ai)
}

func (r redisStorage) entryStorageKey(id entity.ID) string {
	return fmt.Sprintf("ENTRY:WORLD-%s:%d", r.worldID, id)
}

func (r redisStorage) nextEntityIDKey() string {
	return fmt.Sprintf("ENTITY:WORLD-%s:NEXTID", r.worldID)
}
