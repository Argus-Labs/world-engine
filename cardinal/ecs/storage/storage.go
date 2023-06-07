package storage

type WorldStorage struct {
	CompStore        Components
	EntityLocStore   EntityLocationStorage
	ArchCompIdxStore ArchetypeComponentIndex
	ArchAccessor     ArchetypeAccessor
	EntityMgr        EntityManager
	StateStore       StateStorage
}

func NewWorldStorage(
	cs Components,
	els EntityLocationStorage,
	acis ArchetypeComponentIndex,
	aa ArchetypeAccessor,
	em EntityManager,
	ss StateStorage,
) WorldStorage {
	return WorldStorage{
		CompStore:        cs,
		EntityLocStore:   els,
		ArchCompIdxStore: acis,
		ArchAccessor:     aa,
		EntityMgr:        em,
		StateStore:       ss,
	}
}
