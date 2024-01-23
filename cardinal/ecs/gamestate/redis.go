package gamestate

import (
	"context"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	"pkg.world.dev/world-engine/cardinal/ecs/iterators"
	"pkg.world.dev/world-engine/cardinal/types/archetype"
	"pkg.world.dev/world-engine/cardinal/types/component"
)

// pipeFlushToRedis return a pipeliner with all pending state changes to redis ready to be committed in an atomic
// transaction. If an error is returned, no redis changes will have been made.
func (m *EntityCommandBuffer) makePipeOfRedisCommands(ctx context.Context) (redis.Pipeliner, error) {
	pipe := m.client.TxPipeline()

	if m.typeToComponent == nil {
		// component.TypeID -> ComponentMetadata mappings are required to serialized data for the DB
		return nil, eris.New("must call RegisterComponents before flushing to DB")
	}

	operations := []struct {
		name   string
		method func(ctx context.Context, pipe redis.Pipeliner) error
	}{
		{"component_changes", m.addComponentChangesToPipe},
		{"next_entity_id", m.addNextEntityIDToPipe},
		{"pending_arch_ids", m.addPendingArchIDsToPipe},
		{"entity_id_to_arch_id", m.addEntityIDToArchIDToPipe},
		{"active_entity_ids", m.addActiveEntityIDsToPipe},
	}

	for _, operation := range operations {
		var pipeSpan tracer.Span
		pipeSpan, ctx = tracer.StartSpanFromContext(ctx, "tick.span."+operation.name)
		if err := operation.method(ctx, pipe); err != nil {
			pipeSpan.Finish(tracer.WithError(err))
			return nil, eris.Wrapf(err, "failed to run step %q", operation.name)
		}
		pipeSpan.Finish()
	}
	return pipe, nil
}

// addEntityIDToArchIDToPipe adds the information related to mapping an entity ID to its assigned archetype ID.
func (m *EntityCommandBuffer) addEntityIDToArchIDToPipe(ctx context.Context, pipe redis.Pipeliner) error {
	for id, originArchID := range m.entityIDToOriginArchID {
		key := redisArchetypeIDForEntityID(id)
		archID, ok := m.entityIDToArchID[id]
		if !ok {
			// this entity has been removed
			if err := pipe.Del(ctx, key).Err(); err != nil {
				return eris.Wrap(err, "")
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
			return eris.Wrap(err, "")
		}
	}

	return nil
}

// addNextEntityIDToPipe adds any changes to the next available entity ID to the given redis pipe.
func (m *EntityCommandBuffer) addNextEntityIDToPipe(ctx context.Context, pipe redis.Pipeliner) error {
	// There are no pending entity id creations, so there's nothing to commit
	if m.pendingEntityIDs == 0 {
		return nil
	}
	key := redisNextEntityIDKey()
	nextID := m.nextEntityIDSaved + m.pendingEntityIDs
	return eris.Wrap(pipe.Set(ctx, key, nextID, 0).Err(), "")
}

// addComponentChangesToPipe adds updated component values for entities to the redis pipe.
func (m *EntityCommandBuffer) addComponentChangesToPipe(ctx context.Context, pipe redis.Pipeliner) error {
	for key, isMarkedForDeletion := range m.compValuesToDelete {
		if !isMarkedForDeletion {
			continue
		}
		redisKey := redisComponentKey(key.typeID, key.entityID)
		if err := pipe.Del(ctx, redisKey).Err(); err != nil {
			return eris.Wrap(err, "")
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
			return eris.Wrap(err, "")
		}
	}
	return nil
}

// preloadArchIDs loads the mapping of archetypes IDs to sets of IComponentTypes from storage.
func (m *EntityCommandBuffer) loadArchIDs() error {
	archIDToComps, ok, err := getArchIDToCompTypesFromRedis(m.client, m.typeToComponent)
	if err != nil {
		return err
	}
	if !ok {
		// Nothing is saved in the DB. Leave the m.archIDToComps field unchanged
		return nil
	}
	if len(m.archIDToComps) > 0 {
		return eris.New("assigned archetype ID is about to be overwritten by something from storage")
	}
	m.archIDToComps = archIDToComps
	return nil
}

// addPendingArchIDsToPipe adds any newly created archetype IDs (as well as the associated sets of components) to the
// redis pipe.
func (m *EntityCommandBuffer) addPendingArchIDsToPipe(ctx context.Context, pipe redis.Pipeliner) error {
	if len(m.pendingArchIDs) == 0 {
		return nil
	}

	bz, err := m.encodeArchIDToCompTypes()
	if err != nil {
		return err
	}

	return eris.Wrap(pipe.Set(ctx, redisArchIDsToCompTypesKey(), bz, 0).Err(), "")
}

// addActiveEntityIDsToPipe adds information about which entities are assigned to which archetype IDs to the reids pipe.
func (m *EntityCommandBuffer) addActiveEntityIDsToPipe(ctx context.Context, pipe redis.Pipeliner) error {
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
			return eris.Wrap(err, "")
		}
	}
	return nil
}

func (m *EntityCommandBuffer) encodeArchIDToCompTypes() ([]byte, error) {
	forStorage := map[archetype.ID][]component.TypeID{}
	for archID, comps := range m.archIDToComps {
		typeIDs := []component.TypeID{}
		for _, comp := range comps {
			typeIDs = append(typeIDs, comp.ID())
		}
		forStorage[archID] = typeIDs
	}
	return codec.Encode(forStorage)
}

func getArchIDToCompTypesFromRedis(
	client *redis.Client,
	typeToComp map[component.TypeID]component.ComponentMetadata,
) (m map[archetype.ID][]component.ComponentMetadata, ok bool, err error) {
	ctx := context.Background()
	key := redisArchIDsToCompTypesKey()
	bz, err := client.Get(ctx, key).Bytes()
	err = eris.Wrap(err, "")
	if eris.Is(eris.Cause(err), redis.Nil) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}

	fromStorage, err := codec.Decode[map[archetype.ID][]component.TypeID](bz)
	if err != nil {
		return nil, false, err
	}

	// result is the mapping of Arch ID -> IComponent sets
	result := map[archetype.ID][]component.ComponentMetadata{}
	for archID, compTypeIDs := range fromStorage {
		var currComps []component.ComponentMetadata
		for _, compTypeID := range compTypeIDs {
			currComp, found := typeToComp[compTypeID]
			if !found {
				return nil, false, eris.Wrap(iterators.ErrComponentMismatchWithSavedState, "")
			}
			currComps = append(currComps, currComp)
		}

		result[archID] = currComps
	}
	return result, true, nil
}
