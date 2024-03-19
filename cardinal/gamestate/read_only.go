package gamestate

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/codec"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
)

var _ Reader = &readOnlyManager{}

var (
	ErrNoArchIDMappingFound = errors.New("no mapping of archID to components found")
)

type readOnlyManager struct {
	storage         PrimitiveStorage[string]
	typeToComponent VolatileStorage[types.ComponentID, types.ComponentMetadata]
	archIDToComps   VolatileStorage[types.ArchetypeID, []types.ComponentMetadata]
}

func (m *EntityCommandBuffer) ToReadOnly() Reader {
	return &readOnlyManager{
		storage:         m.dbStorage,
		typeToComponent: m.typeToComponent,
		archIDToComps:   m.archIDToComps,
	}
}

// refreshArchIDToCompTypes loads the map of archetype IDs to []ComponentMetadata from redis. This mapping is write
// only, i.e. if an archetype arch id is in this map, it will ALWAYS refer to the same set of components.
// It's ok to save this to memory instead of reading from redit each time.
func (r *readOnlyManager) refreshArchIDToCompTypes() error {
	archIDToComps, ok, err := getArchIDToCompTypesFromRedis(r.storage, r.typeToComponent)
	if err != nil {
		return err
	} else if !ok {
		return eris.Wrap(ErrNoArchIDMappingFound, "")
	}
	r.archIDToComps = archIDToComps
	return nil
}

func (r *readOnlyManager) GetComponentForEntity(
	cType types.ComponentMetadata, id types.EntityID,
) (any, error) {
	bz, err := r.GetComponentForEntityInRawJSON(cType, id)
	if err != nil {
		return nil, err
	}
	return cType.Decode(bz)
}

func (r *readOnlyManager) GetComponentForEntityInRawJSON(
	cType types.ComponentMetadata, id types.EntityID,
) (json.RawMessage, error) {
	ctx := context.Background()
	key := storageComponentKey(cType.ID(), id)
	res, err := r.storage.GetBytes(ctx, key)
	return res, eris.Wrap(err, "")
}

func (r *readOnlyManager) getComponentsForArchID(archID types.ArchetypeID) ([]types.ComponentMetadata, error) {
	if comps, err := r.archIDToComps.Get(archID); err == nil {
		return comps, nil
	}
	if err := r.refreshArchIDToCompTypes(); err != nil {
		return nil, err
	}
	comps, err := r.archIDToComps.Get(archID)
	if err != nil {
		return nil, eris.Wrapf(err, "unable to find components for arch EntityID %d", archID)
	}
	return comps, nil
}

func (r *readOnlyManager) GetComponentTypesForEntity(id types.EntityID) ([]types.ComponentMetadata, error) {
	ctx := context.Background()

	archIDKey := storageArchetypeIDForEntityID(id)
	num, err := r.storage.GetInt(ctx, archIDKey)
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	archID := types.ArchetypeID(num)

	return r.getComponentsForArchID(archID)
}

func (r *readOnlyManager) GetComponentTypesForArchID(archID types.ArchetypeID) ([]types.ComponentMetadata, error) {
	comps, err := r.getComponentsForArchID(archID)
	if err != nil {
		return nil, err
	}
	return comps, nil
}

func (r *readOnlyManager) GetArchIDForComponents(
	components []types.ComponentMetadata,
) (types.ArchetypeID, error) {
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
		archIDs, err := r.archIDToComps.Keys()
		if err != nil {
			return 0, err
		}
		for _, archID := range archIDs {
			currComps, err := r.archIDToComps.Get(archID)
			if err != nil {
				return 0, err
			}
			if isComponentSetMatch(currComps, components) {
				return archID, nil
			}
		}
	}
	return 0, eris.New("arch EntityID for components not found")
}

func (r *readOnlyManager) GetEntitiesForArchID(archID types.ArchetypeID) ([]types.EntityID, error) {
	ctx := context.Background()
	key := storageActiveEntityIDKey(archID)
	bz, err := r.storage.GetBytes(ctx, key)
	if err != nil {
		// No entities were found for this archetype EntityID
		return nil, eris.Wrap(err, "")
	}
	ids, err := codec.Decode[[]types.EntityID](bz)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *readOnlyManager) SearchFrom(filter filter.ComponentFilter, start int) *iterators.ArchetypeIterator {
	itr := &iterators.ArchetypeIterator{}
	if err := r.refreshArchIDToCompTypes(); err != nil {
		return itr
	}
	for i := start; i < r.archIDToComps.Len(); i++ {
		archID := types.ArchetypeID(i)
		// TODO: error swallowed here.
		// https://linear.app/arguslabs/issue/WORLD-943/cardinal-swallowing-errors-in-searchfrom
		componentMetadatas, _ := r.archIDToComps.Get(archID)
		if !filter.MatchesComponents(types.ConvertComponentMetadatasToComponents(componentMetadatas)) {
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
	return r.archIDToComps.Len()
}
