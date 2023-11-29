package ecb

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/store"
	"pkg.world.dev/world-engine/cardinal/types/archetype"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

var _ store.Reader = &readOnlyManager{}

var (
	ErrNoArchIDMappingFound = errors.New("no mapping of archID to components found")
)

type readOnlyManager struct {
	client          *redis.Client
	typeToComponent map[component.TypeID]component.ComponentMetadata
	archIDToComps   map[archetype.ID][]component.ComponentMetadata
}

func (m *Manager) ToReadOnly() store.Reader {
	return &readOnlyManager{
		client:          m.client,
		typeToComponent: m.typeToComponent,
	}
}

// refreshArchIDToCompTypes loads the map of archetype IDs to []ComponentMetadata from redis. This mapping is write
// only, i.e. if an archetype ID is in this map, it will ALWAYS refer to the same set of components. It's ok to save
// this to memory instead of reading from redit each time. If an archetype ID is not found in this map.
func (r *readOnlyManager) refreshArchIDToCompTypes() error {
	archIDToComps, ok, err := getArchIDToCompTypesFromRedis(r.client, r.typeToComponent)
	if err != nil {
		return err
	} else if !ok {
		return eris.Wrap(ErrNoArchIDMappingFound, "")
	}
	r.archIDToComps = archIDToComps
	return nil
}

func (r *readOnlyManager) GetComponentForEntity(
	cType component.ComponentMetadata, id entity.ID,
) (any, error) {
	bz, err := r.GetComponentForEntityInRawJSON(cType, id)
	if err != nil {
		return nil, err
	}
	return cType.Decode(bz)
}

func (r *readOnlyManager) GetComponentForEntityInRawJSON(
	cType component.ComponentMetadata, id entity.ID,
) (json.RawMessage, error) {
	ctx := context.Background()
	key := redisComponentKey(cType.ID(), id)
	res, err := r.client.Get(ctx, key).Bytes()
	return res, eris.Wrap(err, "")
}

func (r *readOnlyManager) getComponentsForArchID(archID archetype.ID) ([]component.ComponentMetadata, error) {
	if comps, ok := r.archIDToComps[archID]; ok {
		return comps, nil
	}
	if err := r.refreshArchIDToCompTypes(); err != nil {
		return nil, err
	}
	comps, ok := r.archIDToComps[archID]
	if !ok {
		return nil, eris.Errorf("unable to find components for arch ID %d", archID)
	}
	return comps, nil
}

func (r *readOnlyManager) GetComponentTypesForEntity(id entity.ID) ([]component.ComponentMetadata, error) {
	ctx := context.Background()

	archIDKey := redisArchetypeIDForEntityID(id)
	num, err := r.client.Get(ctx, archIDKey).Int()
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	archID := archetype.ID(num)

	return r.getComponentsForArchID(archID)
}

func (r *readOnlyManager) GetComponentTypesForArchID(archID archetype.ID) []component.ComponentMetadata {
	comps, err := r.getComponentsForArchID(archID)
	if err != nil {
		panic(eris.ToString(err, true))
	}
	return comps
}

func (r *readOnlyManager) GetArchIDForComponents(
	components []component.ComponentMetadata,
) (archetype.ID, error) {
	if err := sortComponentSet(components); err != nil {
		return 0, err
	}

	// It's slow to refresh the archIDToComps map from redis, and mappings never change (once initially set).
	// Skip the refreshing from redis in the first pass. Maybe the component set in question is already in our
	// in-memory map. If we fail to find it on the first pass, refresh the map from redis.
	for _, refreshMapFromRedis := range []bool{false, true} {
		if refreshMapFromRedis {
			if err := r.refreshArchIDToCompTypes(); err != nil {
				return 0, err
			}
		}

		for archID, currComps := range r.archIDToComps {
			if isComponentSetMatch(currComps, components) {
				return archID, nil
			}
		}
	}
	return 0, eris.New("arch ID for components not found")
}

func (r *readOnlyManager) GetEntitiesForArchID(archID archetype.ID) ([]entity.ID, error) {
	ctx := context.Background()
	key := redisActiveEntityIDKey(archID)
	bz, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		// No entities were found for this archetype ID
		return nil, eris.Wrap(err, "")
	}
	ids, err := codec.Decode[[]entity.ID](bz)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *readOnlyManager) SearchFrom(filter filter.ComponentFilter, start int) *storage.ArchetypeIterator {
	itr := &storage.ArchetypeIterator{}
	if err := r.refreshArchIDToCompTypes(); err != nil {
		return itr
	}
	for i := start; i < len(r.archIDToComps); i++ {
		archID := archetype.ID(i)
		if !filter.MatchesComponents(r.archIDToComps[archID]) {
			continue
		}
		itr.Values = append(itr.Values, archID)
	}
	return itr
}

func (r *readOnlyManager) ArchetypeCount() int {
	if err := r.refreshArchIDToCompTypes(); err != nil {
		return 0
	}
	return len(r.archIDToComps)
}
