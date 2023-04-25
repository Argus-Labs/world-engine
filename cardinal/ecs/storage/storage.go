package storage

type WorldStorage struct {
	CompStore        Components
	EntityLocStore   EntityLocationStorage
	ArchCompIdxStore ArchetypeComponentIndex
	ArchAccessor     ArchetypeStorage
	EntryStore       EntryStorage
	EntityMgr        EntityManager
}

func NewWorldStorage(
	cs Components,
	els EntityLocationStorage,
	acis ArchetypeComponentIndex,
	aa ArchetypeStorage,
	es EntryStorage,
	em EntityManager) WorldStorage {
	return WorldStorage{
		CompStore:        cs,
		EntityLocStore:   els,
		ArchCompIdxStore: acis,
		ArchAccessor:     aa,
		EntryStore:       es,
		EntityMgr:        em,
	}
}
