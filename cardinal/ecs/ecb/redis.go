package ecb

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	component_metadata "pkg.world.dev/world-engine/cardinal/ecs/component/metadata"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

// pipeFlushToRedis return a pipeliner with all pending state changes to redis ready to be committed in an atomic
// transaction. If an error is returned, no redis changes will have been made.
func (m *Manager) makePipeOfRedisCommands(ctx context.Context) (redis.Pipeliner, error) {
	pipe := m.client.TxPipeline()

	if m.typeToComponent == nil {
		// component.TypeID -> ComponentMetadata mappings are required to serialized data for the DB
		return nil, errors.New("must call RegisterComponents before flushing to DB")
	}

	if err := m.addComponentChangesToPipe(ctx, pipe); err != nil {
		return nil, fmt.Errorf("failed to add component changes to pipe: %w", err)
	}
	if err := m.addNextEntityIDToPipe(ctx, pipe); err != nil {
		return nil, fmt.Errorf("failed to add entity id changes to pipe: %w", err)
	}
	if err := m.addPendingArchIDsToPipe(ctx, pipe); err != nil {
		return nil, fmt.Errorf("failed to add archID to component type map to pipe: %w", err)
	}
	if err := m.addEntityIDToArchIDToPipe(ctx, pipe); err != nil {
		return nil, fmt.Errorf("failed to add entity ID to archID mapping to pipe: %w", err)
	}
	if err := m.addActiveEntityIDsToPipe(ctx, pipe); err != nil {
		return nil, fmt.Errorf("failed to add changes to active entity ids to pipe: %w", err)
	}

	return pipe, nil
}

// addEntityIDToArchIDToPipe adds the information related to mapping an entity ID to its assigned archetype ID.
func (m *Manager) addEntityIDToArchIDToPipe(ctx context.Context, pipe redis.Pipeliner) error {
	for id, originArchID := range m.entityIDToOriginArchID {
		key := redisArchetypeIDForEntityID(id)
		archID, ok := m.entityIDToArchID[id]
		if !ok {
			// this entity has been removed
			if err := pipe.Del(ctx, key).Err(); err != nil {
				return err
			}
			continue
		}
		// This entity somehow ended up back at its original archetype. There's nothing to do.
		if archID == originArchID {
			continue
		}

		// Otherwise, the archetype actually needs to be updated
		archIDAsNum := int(archID)
		if err := pipe.Set(ctx, key, archIDAsNum, 0).Err(); err != nil {
			return err
		}
	}

	return nil
}

// addNextEntityIDToPipe adds any changes to the next available entity ID to the given redis pipe.
func (m *Manager) addNextEntityIDToPipe(ctx context.Context, pipe redis.Pipeliner) error {
	// There are no pending entity id creations, so there's nothing to commit
	if m.pendingEntityIDs == 0 {
		return nil
	}
	key := redisNextEntityIDKey()
	nextID := m.nextEntityIDSaved + m.pendingEntityIDs
	return pipe.Set(ctx, key, nextID, 0).Err()
}

// addComponentChangesToPipe adds updated component values for entities to the redis pipe.
func (m *Manager) addComponentChangesToPipe(ctx context.Context, pipe redis.Pipeliner) error {
	for key, isMarkedForDeletion := range m.compValuesToDelete {
		if !isMarkedForDeletion {
			continue
		}
		redisKey := redisComponentKey(key.typeID, key.entityID)
		if err := pipe.Del(ctx, redisKey).Err(); err != nil {
			return err
		}
	}

	for key, value := range m.compValues {
		cType := m.typeToComponent[key.typeID]
		bz, err := cType.Encode(value)
		if err != nil {
			return err
		}

		redisKey := redisComponentKey(key.typeID, key.entityID)
		if err = pipe.Set(ctx, redisKey, bz, 0).Err(); err != nil {
			return err
		}
	}
	return nil
}

// preloadArchIDs loads the mapping of archetypes IDs to sets of IComponentTypes from storage.
func (m *Manager) loadArchIDs() error {
	archIDToComps, ok, err := getArchIDToCompTypesFromRedis(m.client, m.typeToComponent)
	if err != nil {
		return err
	}
	if !ok {
		// Nothing is saved in the DB. Leave the m.archIDToComps field unchanged
		return nil
	}
	if len(m.archIDToComps) > 0 {
		return errors.New("assigned archetype ID is about to be overwritten by something from storage")
	}
	m.archIDToComps = archIDToComps
	return nil
}

// addPendingArchIDsToPipe adds any newly created archetype IDs (as well as the associated sets of components) to the
// redis pipe.
func (m *Manager) addPendingArchIDsToPipe(ctx context.Context, pipe redis.Pipeliner) error {
	if len(m.pendingArchIDs) == 0 {
		return nil
	}

	bz, err := m.encodeArchIDToCompTypes()
	if err != nil {
		return err
	}

	return pipe.Set(ctx, redisArchIDsToCompTypesKey(), bz, 0).Err()
}

// addActiveEntityIDsToPipe adds information about which entities are assigned to which archetype IDs to the reids pipe.
func (m *Manager) addActiveEntityIDsToPipe(ctx context.Context, pipe redis.Pipeliner) error {
	for archID, active := range m.activeEntities {
		if !active.modified {
			continue
		}
		bz, err := codec.Encode(active.ids)
		if err != nil {
			return err
		}
		key := redisActiveEntityIDKey(archID)
		err = pipe.Set(ctx, key, bz, 0).Err()
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) encodeArchIDToCompTypes() ([]byte, error) {
	forStorage := map[archetype.ID][]component_metadata.TypeID{}
	for archID, comps := range m.archIDToComps {
		typeIDs := []component_metadata.TypeID{}
		for _, comp := range comps {
			typeIDs = append(typeIDs, comp.ID())
		}
		forStorage[archID] = typeIDs
	}
	return codec.Encode(forStorage)
}

func getArchIDToCompTypesFromRedis(client *redis.Client,
	typeToComp map[component_metadata.TypeID]component_metadata.ComponentMetadata,
) (m map[archetype.ID][]component_metadata.ComponentMetadata, ok bool, err error) {
	ctx := context.Background()
	key := redisArchIDsToCompTypesKey()
	bz, err := client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}

	fromStorage, err := codec.Decode[map[archetype.ID][]component_metadata.TypeID](bz)
	if err != nil {
		return nil, false, err
	}

	// result is the mapping of Arch ID -> IComponent sets
	result := map[archetype.ID][]component_metadata.ComponentMetadata{}
	for archID, compTypeIDs := range fromStorage {
		currComps := []component_metadata.ComponentMetadata{}
		for _, compTypeID := range compTypeIDs {
			currComp, found := typeToComp[compTypeID]
			if !found {
				return nil, false, storage.ErrComponentMismatchWithSavedState
			}
			currComps = append(currComps, currComp)
		}

		result[archID] = currComps
	}
	return result, true, nil
}
