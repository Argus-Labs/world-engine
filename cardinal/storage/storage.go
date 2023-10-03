// Package storage uses Redis to ecs world data to a persistent storage layer. Data saved to this layer includes
// entity and component data, tick information, nonce information, archetype ID to component set mappings, and
// entity location information.

package storage

import (
	"io"

	"pkg.world.dev/world-engine/cardinal/public"
)

type WorldStorage struct {
	CompStore        Components
	EntityLocStore   public.EntityLocationStorage
	ArchCompIdxStore public.ArchetypeComponentIndex
	ArchAccessor     public.ArchetypeAccessor
	EntityMgr        public.EntityManager
	StateStore       public.StateStorage
	TickStore        public.TickStorage
	NonceStore       public.NonceStorage
	IO               io.Closer
}

func NewWorldStorage(omni public.OmniStorage) WorldStorage {
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
