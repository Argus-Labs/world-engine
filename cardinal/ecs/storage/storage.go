package storage

type WorldStorage struct {
	CompStore        Components
	EntityLocStore   EntityLocationStorage
	ArchCompIdxStore ArchetypeComponentIndex
	ArchAccessor     ArchetypeAccessor
	EntityMgr        EntityManager
	StateStore       StateStorage
	TickStore        TickStorage
	NonceStore       NonceStorage
}

type OmniStorage interface {
	ComponentStorageManager
	ComponentIndexStorage
	EntityLocationStorage
	EntityManager
	StateStorage
	TickStorage
	NonceStorage
}

func NewWorldStorage(omni OmniStorage) WorldStorage {
	return WorldStorage{
		CompStore:        NewComponents(omni, omni),
		EntityLocStore:   omni,
		ArchCompIdxStore: NewArchetypeComponentIndex(),
		ArchAccessor:     NewArchetypeAccessor(),
		EntityMgr:        omni,
		StateStore:       omni,
		TickStore:        omni,
		NonceStore:       omni,
	}
}
