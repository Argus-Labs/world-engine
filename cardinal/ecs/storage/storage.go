package storage

type WorldStorage struct {
	CompStore        Components
	EntityLocStore   EntityLocationStorage
	ArchCompIdxStore ArchetypeComponentIndex
	ArchAccessor     ArchetypeAccessor
	EntityStore      EntityStorage
	EntityMgr        EntityManager
}

func NewWorldStorage(
	cs Components,
	els EntityLocationStorage,
	acis ArchetypeComponentIndex,
	aa ArchetypeAccessor,
	es EntityStorage,
	em EntityManager) WorldStorage {
	return WorldStorage{
		CompStore:        cs,
		EntityLocStore:   els,
		ArchCompIdxStore: acis,
		ArchAccessor:     aa,
		EntityStore:      es,
		EntityMgr:        em,
	}
}
