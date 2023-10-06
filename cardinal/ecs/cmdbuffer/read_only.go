package cmdbuffer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/store"
)

var _ store.IReader = &readOnlyManager{}

var (
	ErrorNoArchIDMappingFound = errors.New("no mapping of archID to components found")
)

type readOnlyManager struct {
	client          *redis.Client
	typeToComponent map[component.TypeID]component.IComponentType
	archIDToComps   map[archetype.ID][]component.IComponentType
}

func (m *Manager) NewReadOnlyStore() store.IReader {
	return &readOnlyManager{
		client:          m.client,
		typeToComponent: m.typeToComponent,
	}
}

// refreshArchIDToCompTypes loads the map of archetype IDs to []IComponentType from redis. This mapping is write only,
// i.e. if an archetype ID is in this map, it will ALWAYS refer to the same set of components. It's ok to save this to
// memory instead of reading from redit each time. If an archetype ID is not found in this map,
func (r *readOnlyManager) refreshArchIDToCompTypes() error {
	archIDToComps, ok, err := getArchIDToCompTypesFromRedis(r.client, r.typeToComponent)
	if err != nil {
		return err
	} else if !ok {
		return ErrorNoArchIDMappingFound
	}
	r.archIDToComps = archIDToComps
	return nil
}

// GetEntity converts an entity ID into an entity.Entity.
// TODO: This is only used in tests, so it should be removed from the StoreManager interface.
func (r *readOnlyManager) GetEntity(id entity.ID) (entity.Entity, error) {
	//TODO implement me
	panic("implement me")
}

func (r *readOnlyManager) GetComponentForEntity(cType component.IComponentType, id entity.ID) (any, error) {
	bz, err := r.GetComponentForEntityInRawJson(cType, id)
	if err != nil {
		return nil, err
	}
	return cType.Decode(bz)
}

func (r *readOnlyManager) GetComponentForEntityInRawJson(cType component.IComponentType, id entity.ID) (json.RawMessage, error) {
	ctx := context.Background()
	key := redisComponentKey(cType.ID(), id)
	return r.client.Get(ctx, key).Bytes()
}

func (r *readOnlyManager) getComponentsForArchID(archID archetype.ID) ([]component.IComponentType, error) {
	if comps, ok := r.archIDToComps[archID]; ok {
		return comps, nil
	}
	r.refreshArchIDToCompTypes()
	comps, ok := r.archIDToComps[archID]
	if !ok {
		return nil, fmt.Errorf("unable to find components for arch ID %d", archID)
	}
	return comps, nil

}

func (r *readOnlyManager) GetComponentTypesForEntity(id entity.ID) ([]component.IComponentType, error) {
	ctx := context.Background()

	archIDKey := redisArchetypeIDForEntityID(id)
	num, err := r.client.Get(ctx, archIDKey).Int()
	if err != nil {
		return nil, err
	}
	archID := archetype.ID(num)

	return r.getComponentsForArchID(archID)
}

func (r *readOnlyManager) GetComponentTypesForArchID(archID archetype.ID) []component.IComponentType {
	comps, err := r.getComponentsForArchID(archID)
	if err != nil {
		panic(err)
	}
	return comps
}

func (r *readOnlyManager) GetArchIDForComponents(components []component.IComponentType) (archetype.ID, error) {
	if err := sortComponentSet(components); err != nil {
		return 0, err
	}
	for _, tryRefresh := range []bool{false, true} {
		if tryRefresh {
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
	return 0, errors.New("arch ID for components not found")
}

func (r *readOnlyManager) GetEntitiesForArchID(archID archetype.ID) []entity.ID {
	ctx := context.Background()
	key := redisActiveEntityIDKey(archID)
	bz, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		// No entities were found for this archetype ID
		return nil
	}
	ids, err := codec.Decode[[]entity.ID](bz)
	if err != nil {
		// TODO: This method should allow for returning an error, but this impacts the store.IManager interface
		panic(err)
	}
	return ids
}

func (r *readOnlyManager) SearchFrom(filter filter.ComponentFilter, start int) *storage.ArchetypeIterator {
	itr := &storage.ArchetypeIterator{}
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
