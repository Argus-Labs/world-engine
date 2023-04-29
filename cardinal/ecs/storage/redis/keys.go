package redis

import (
	"fmt"

	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

/*
	KEYS:
define keys for redis storage
	return fmt.Sprintf("WORLD-%s:CID-%d:A-%d", r.worldID, r.componentStoragePrefix, Index)
- 	COMPONENT DATA: 	COMPD:WORLD-1:CN-ECS.V1.ENERGY:A-5:C-1 	-> component struct bytes
-	COMPONENT INDEX: 	CIDX:WORLD-1:CN-ECS.V1.ENERGY:A-4 		-> Component Index
- 	ENTITY LOCATION: 	LOC:WORLD-1:E-1 						-> Location
- 	ENTITY LOCATION LEN LOCL:WORLD-1							-> Int
- 	ARCHETYPE STORAGE:  ARCH:WORLD-1:A-0 						-> archetype struct bytes
- 	ARCHETYPE IDX:		ARCH:WORLD-1:IDX						-> uint64
- 	ENTRY STORAGE:      ENTRY:WORLD-1:ID  						-> entry struct bytes
- 	ENTITY MGR: 		ENTITY:WORLD-1:NEXTID 					-> uint64 id
*/

func (r *Storage) componentDataKey(ai storage.ArchetypeIndex, ci storage.ComponentIndex) string {
	return fmt.Sprintf("COMPD:WORLD-%s:CN-%s:A-%d:C-%d", r.worldID, r.componentStoragePrefix, ai, ci)
}

func (r *Storage) componentIndexKey(index storage.ArchetypeIndex) string {
	return fmt.Sprintf("CIDX:WORLD-%s:CID-%s:A-%d", r.worldID, r.componentStoragePrefix, index)
}

func (r *Storage) entityLocationKey(e entity.ID) string {
	return fmt.Sprintf("LOC:WORLD-%s:E-%d", r.worldID, e)
}

func (r *Storage) entityLocationLenKey() string {
	return fmt.Sprintf("LOCL:WORLD-%s", r.worldID)
}

func (r *Storage) archetypeStorageKey(ai storage.ArchetypeIndex) string {
	return fmt.Sprintf("ARCH:WORLD-%s:A-%d", r.worldID, ai)
}

func (r *Storage) archetypeIndexKey() string {
	return fmt.Sprintf("ARCH:WORLD-%s:IDX", r.worldID)
}

func (r *Storage) entryStorageKey(id entity.ID) string {
	return fmt.Sprintf("ENTRY:WORLD-%s:%d", r.worldID, id)
}

func (r *Storage) nextEntityIDKey() string {
	return fmt.Sprintf("ENTITY:WORLD-%s:NEXTID", r.worldID)
}
