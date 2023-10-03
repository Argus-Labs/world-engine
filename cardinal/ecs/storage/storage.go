// Package storage uses Redis to ecs world data to a persistent storage layer. Data saved to this layer includes
// entity and component data, tick information, nonce information, archetype ID to component set mappings, and
// entity location information.

package storage

import (
	"io"

	"pkg.world.dev/world-engine/cardinal/interfaces"
)

type WorldStorage struct {
	CompStore        Components
	EntityLocStore   interfaces.EntityLocationStorage
	ArchCompIdxStore interfaces.ArchetypeComponentIndex
	ArchAccessor     interfaces.ArchetypeAccessor
	EntityMgr        interfaces.EntityManager
	StateStore       interfaces.StateStorage
	TickStore        interfaces.TickStorage
	NonceStore       interfaces.NonceStorage
	IO               io.Closer
}

func NewWorldStorage(omni interfaces.OmniStorage) WorldStorage {
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
