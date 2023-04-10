package storage

import "fmt"

/*
	KEYS:
define keys for redis storage
	return fmt.Sprintf("WORLD-%s:CID-%d:A-%d", r.worldID, r.componentStoragePrefix, index)
- 	COMPONENT DATA: 	COMPD:WORLD-1:CID-0:A-5 -> component struct bytes
-	COMPONENT INDEX: 	CIDX:WORLD-1:CID-0:A-4 	-> Component Index

*/

func (r *redisStorage) componentDataKey(index ArchetypeIndex) string {
	return fmt.Sprintf("COMPD:WORLD-%s:CID-%d:A-%d", r.worldID, r.componentStoragePrefix, index)
}

func (r *redisStorage) componentIndexKey(index ArchetypeIndex) string {
	return fmt.Sprintf("CIDX:WORLD-%s:CID-%d:A-%d", r.worldID, r.componentStoragePrefix, index)
}
