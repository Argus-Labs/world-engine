// Package storage uses Redis to ecs world data to a persistent storage layer. Data saved to this layer includes
// entity and component data, tick information, nonce information, archetype ID to component set mappings, and
// entity location information.

package storage

import "io"

type WorldStorage struct {
	CompStore        Components
	EntityLocStore   EntityLocationStorage
	ArchCompIdxStore ArchetypeComponentIndex
	ArchAccessor     ArchetypeAccessor
	EntityMgr        EntityManager
	StateStore       StateStorage
	TickStore        TickStorage
	NonceStore       NonceStorage
	IO               io.Closer
}

type OmniStorage interface {
	ComponentStorageManager
	ComponentIndexStorage
	EntityLocationStorage
	EntityManager
	StateStorage
	TickStorage
	NonceStorage
	io.Closer
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
		IO:               omni,
	}
}
