package gamestate

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/codec"
	"pkg.world.dev/world-engine/cardinal/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
)

type FinalizedState struct {
	locked bool

	storage             PrimitiveStorage[string]
	compNameToComponent map[types.ComponentName]types.ComponentMetadata
	// archetype component set is never modified, therefore we can safely cache it.
	archIDToComps map[types.ArchetypeID][]types.ComponentMetadata // archID -> []comps
}

var _ Reader = &FinalizedState{}

func NewFinalizedState(storage PrimitiveStorage[string]) (*FinalizedState, error) {
	r := &FinalizedState{
		locked: false,

		storage:             storage,
		compNameToComponent: map[types.ComponentName]types.ComponentMetadata{},
		archIDToComps:       map[types.ArchetypeID][]types.ComponentMetadata{},
	}
	return r, nil
}

// init performs the initial load from Redis and locks ECB such that no new components can be registered.
func (m *FinalizedState) init() error {
	if m.locked {
		return eris.New("finalized state is already initialized")
	}

	if err := m.loadArchetype(); err != nil {
		return err
	}

	// Lock FinalizedState to prevent new components from being registered
	m.locked = true

	return nil
}

func (m *FinalizedState) isComponentRegistered(compName types.ComponentName) bool {
	_, ok := m.compNameToComponent[compName]
	return ok
}

func (m *FinalizedState) registerComponent(comp types.ComponentMetadata) error {
	if m.locked {
		return eris.New("unable to register components after FinalizedState is initialized")
	}

	if _, ok := m.compNameToComponent[comp.Name()]; ok {
		return eris.New("component already registered")
	}
	m.compNameToComponent[comp.Name()] = comp

	return nil
}

func (m *FinalizedState) GetComponentForEntity(comp types.Component, id types.EntityID) (any, error) {
	if err := m.checkInitialized(); err != nil {
		return nil, err
	}

	bz, err := m.GetComponentForEntityInRawJSON(comp, id)
	if err != nil {
		return nil, err
	}

	cType, ok := m.compNameToComponent[comp.Name()]
	if !ok {
		return nil, ErrComponentNotRegistered
	}

	return cType.Decode(bz)
}

func (m *FinalizedState) GetComponentForEntityInRawJSON(comp types.Component, id types.EntityID) (
	json.RawMessage, error,
) {
	if err := m.checkInitialized(); err != nil {
		return nil, err
	}

	err := m.checkComponentRegistered(comp)
	if err != nil {
		return nil, err
	}

	return m.storage.GetBytes(context.Background(), storageComponentKey(comp.Name(), id))
}

// GetEntitiesForArchID returns all the entities that currently belong to the given archetype EntityID.
func (m *FinalizedState) GetEntitiesForArchID(archID types.ArchetypeID) ([]types.EntityID, error) {
	if err := m.checkInitialized(); err != nil {
		return nil, err
	}

	active, err := m.getActiveEntities(archID)
	if err != nil {
		return nil, err
	}
	return active.ids, nil
}

// FindArchetypes returns a list of archetype IDs that fulfill the given component filter.
func (m *FinalizedState) FindArchetypes(filter filter.ComponentFilter) ([]types.ArchetypeID, error) {
	if err := m.checkInitialized(); err != nil {
		return nil, err
	}

	archetypes := make([]types.ArchetypeID, 0)

	err := m.loadArchetype()
	if err != nil {
		return nil, err
	}

	for archID, comps := range m.archIDToComps {
		if !filter.MatchesComponents(types.ConvertComponentMetadatasToComponents(comps)) {
			continue
		}
		archetypes = append(archetypes, archID)
	}

	return archetypes, nil
}

// GetAllComponentsForEntityInRawJSON returns all components for the given entity in JSON format.
func (m *FinalizedState) GetAllComponentsForEntityInRawJSON(id types.EntityID) (map[string]json.RawMessage, error) {
	if err := m.checkInitialized(); err != nil {
		return nil, err
	}

	comps, err := m.getComponentTypesForEntity(id)
	if err != nil {
		return nil, err
	}

	result := map[string]json.RawMessage{}

	for _, comp := range comps {
		value, err := m.GetComponentForEntityInRawJSON(comp, id)
		if err != nil {
			return nil, err
		}
		result[comp.Name()] = value
	}

	return result, nil
}

// getActiveEntities returns the entities that are currently assigned to the given archetype EntityID.
func (m *FinalizedState) getActiveEntities(archID types.ArchetypeID) (*activeEntities, error) {
	bz, err := m.storage.GetBytes(context.Background(), storageActiveEntityIDKey(archID))
	if err != nil {
		return nil, err
	}

	var ids []types.EntityID
	if err != nil {
		// todo: this is redis specific, should be changed to a general error on storage
		// todo: RedisStorage needs to be modified to return this general error when a redis.Nil is detected.
		if !eris.Is(eris.Cause(err), redis.Nil) {
			return nil, err
		}
	} else {
		ids, err = codec.Decode[[]types.EntityID](bz)
		if err != nil {
			return nil, err
		}
	}

	entities := activeEntities{
		ids:      ids,
		modified: false,
	}

	return &entities, nil
}

// GetLastFinalizedTick returns the last tick that was successfully finalized.
// If the latest finalized tick is 0, it means that no tick has been finalized yet.
func (m *FinalizedState) GetLastFinalizedTick() (int64, error) {
	if err := m.checkInitialized(); err != nil {
		return 0, err
	}

	tick, err := m.storage.GetInt64(context.Background(), storageLastFinalizedTickKey())
	if err != nil {
		// If the returned error is redis.Nil, it means that the key does not exist yet. In this case, we can infer
		// that the latest finalized tick is 0. If the return is not redis.Nil, it means that an actual error occurred.
		if eris.Is(err, redis.Nil) {
			tick = -1
		} else {
			return 0, eris.Wrap(err, "failed to get latest finalized tick")
		}
	}

	return tick, nil
}

func (m *FinalizedState) ArchetypeCount() (int, error) {
	if err := m.checkInitialized(); err != nil {
		return 0, err
	}

	if err := m.loadArchetype(); err != nil {
		return 0, err
	}

	return len(m.archIDToComps), nil
}

// getArchetypeForEntity returns the archetype EntityID for the given entity EntityID.
func (m *FinalizedState) getArchetypeForEntity(id types.EntityID) (types.ArchetypeID, error) {
	// If the entity ID is not in the in-memory cache, fetch the archetype ID from Redis.
	num, err := m.storage.GetInt(context.Background(), storageArchetypeIDForEntityID(id))
	if err != nil {
		// todo: Make redis.Nil a general error on storage
		if eris.Is(err, redis.Nil) {
			return 0, eris.Wrap(redis.Nil, ErrEntityDoesNotExist.Error())
		}
		return 0, err
	}

	return types.ArchetypeID(num), nil
}

// getComponentTypesForEntity returns all the component types that are currently on the given entity. Only types
// are returned. To get the actual component data, use GetComponentForEntity.
func (m *FinalizedState) getComponentTypesForEntity(id types.EntityID) ([]types.ComponentMetadata, error) {
	archID, err := m.getArchetypeForEntity(id)
	if err != nil {
		return nil, nil
	}

	err = m.loadArchetype()
	if err != nil {
		return nil, err
	}

	return m.archIDToComps[archID], nil
}

// loadArchetype returns a mapping that contains the corresponding components for a given archetype ID.
// In contrast to ECB, it's perfectly fine to reload the archetype cache since we are not tracking the working here.
func (m *FinalizedState) loadArchetype() error {
	bz, err := m.storage.GetBytes(context.Background(), storageArchIDsToCompTypesKey())
	if err != nil {
		// If no archetypes have been set, just terminate early.
		if eris.Is(eris.Cause(err), redis.Nil) {
			return nil
		}
		return err
	}

	archetypes, err := codec.Decode[map[types.ArchetypeID][]types.ComponentName](bz)
	if err != nil {
		return err
	}

	result := map[types.ArchetypeID][]types.ComponentMetadata{}
	for archID, compNames := range archetypes {
		var currComps []types.ComponentMetadata

		// Validate component schemas
		for _, compName := range compNames {
			fmt.Println(compName)
			fmt.Println(m.compNameToComponent)
			currComp, ok := m.compNameToComponent[compName]
			if !ok {
				return ErrComponentMismatchWithSavedState
			}
			currComps = append(currComps, currComp)
		}

		result[archID] = currComps
	}

	m.archIDToComps = result

	return nil
}

func (m *FinalizedState) checkInitialized() error {
	if !m.locked {
		return eris.New("finalized state is not initialized")
	}
	return nil
}

func (m *FinalizedState) checkComponentRegistered(comp types.Component) error {
	_, ok := m.compNameToComponent[comp.Name()]
	if !ok {
		return eris.Wrap(ErrComponentNotRegistered, fmt.Sprintf("component %q is not registered", comp.Name()))
	}
	return nil
}
