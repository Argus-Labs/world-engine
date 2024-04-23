package gamestate

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"pkg.world.dev/world-engine/cardinal/codec"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/types"
)

var _ PrimitiveStorage[string] = &RedisStorage{}

type RedisStorage struct {
	currentClient redis.Cmdable
}

func (r *RedisStorage) GetFloat64(ctx context.Context, key string) (float64, error) {
	res, err := r.currentClient.Get(ctx, key).Float64()
	if err != nil {
		return 0, eris.Wrap(err, "")
	}
	return res, nil
}
func (r *RedisStorage) GetFloat32(ctx context.Context, key string) (float32, error) {
	res, err := r.currentClient.Get(ctx, key).Float32()
	if err != nil {
		return 0, eris.Wrap(err, "")
	}
	return res, nil
}
func (r *RedisStorage) GetUInt64(ctx context.Context, key string) (uint64, error) {
	res, err := r.currentClient.Get(ctx, key).Uint64()
	if err != nil {
		return 0, eris.Wrap(err, "")
	}
	return res, nil
}

func (r *RedisStorage) GetInt64(ctx context.Context, key string) (int64, error) {
	res, err := r.currentClient.Get(ctx, key).Int64()
	if err != nil {
		return 0, eris.Wrap(err, "")
	}
	return res, nil
}

func (r *RedisStorage) GetInt(ctx context.Context, key string) (int, error) {
	res, err := r.currentClient.Get(ctx, key).Int()
	if err != nil {
		return 0, eris.Wrap(err, "")
	}
	return res, nil
}

func (r *RedisStorage) GetBool(ctx context.Context, key string) (bool, error) {
	res, err := r.currentClient.Get(ctx, key).Bool()
	if err != nil {
		return false, eris.Wrap(err, "")
	}
	return res, nil
}

func (r *RedisStorage) GetBytes(ctx context.Context, key string) ([]byte, error) {
	bz, err := r.currentClient.Get(ctx, key).Bytes()
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	return bz, nil
}

func (r *RedisStorage) Set(ctx context.Context, key string, value any) error {
	return eris.Wrap(r.currentClient.Set(ctx, key, value, 0).Err(), "")
}

// Underlying type is a string. Unfortunately this is the way redis works and this is the most generic return value.
func (r *RedisStorage) Get(ctx context.Context, key string) (any, error) {
	var res any
	var err error
	res, err = r.currentClient.Get(ctx, key).Result()
	return res, eris.Wrap(err, "")
}

func (r *RedisStorage) Incr(ctx context.Context, key string) error {
	return eris.Wrap(r.currentClient.Incr(ctx, key).Err(), "")
}

func (r *RedisStorage) Decr(ctx context.Context, key string) error {
	return eris.Wrap(r.currentClient.Decr(ctx, key).Err(), "")
}

func (r *RedisStorage) Delete(ctx context.Context, key string) error {
	return eris.Wrap(r.currentClient.Del(ctx, key).Err(), "")
}

func (r *RedisStorage) Close(ctx context.Context) error {
	return eris.Wrap(r.currentClient.Shutdown(ctx).Err(), "")
}

func (r *RedisStorage) Keys(ctx context.Context) ([]string, error) {
	return r.currentClient.Keys(ctx, "*").Result()
}

func (r *RedisStorage) Clear(ctx context.Context) error {
	return eris.Wrap(r.currentClient.FlushAll(ctx).Err(), "")
}

func (r *RedisStorage) StartTransaction(_ context.Context) (Transaction[string], error) {
	pipeline := r.currentClient.TxPipeline()
	redisTransaction := NewRedisPrimitiveStorage(pipeline)
	return &redisTransaction, nil
}

func (r *RedisStorage) EndTransaction(ctx context.Context) error {
	pipeline, ok := r.currentClient.(redis.Pipeliner)
	if !ok {
		return eris.New("current redis dbStorage is not a pipeline/transaction")
	}
	_, err := pipeline.Exec(ctx)
	return eris.Wrap(err, "")
}

func NewRedisPrimitiveStorage(client redis.Cmdable) RedisStorage {
	return RedisStorage{
		currentClient: client,
	}
}

// pipeFlushToRedis return a pipeliner with all pending state changes to redis ready to be committed in an atomic
// transaction. If an error is returned, no redis changes will have been made.
func (m *EntityCommandBuffer) makePipeOfRedisCommands(ctx context.Context) (PrimitiveStorage[string], error) {
	pipe, err := m.dbStorage.StartTransaction(ctx)
	if err != nil {
		return nil, err
	}

	if m.typeToComponent == nil {
		// component.ComponentID -> ComponentMetadata mappings are required to serialized data for the DB
		return nil, eris.New("must call RegisterComponents before flushing to DB")
	}

	operations := []struct {
		name   string
		method func(ctx context.Context, pipe PrimitiveStorage[string]) error
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

// addEntityIDToArchIDToPipe adds the information related to mapping an EntityID to its assigned archetype ArchetypeID.
func (m *EntityCommandBuffer) addEntityIDToArchIDToPipe(ctx context.Context, pipe PrimitiveStorage[string]) error {
	ids, err := m.entityIDToOriginArchID.Keys()
	if err != nil {
		return err
	}
	for _, id := range ids {
		originArchID, err := m.entityIDToOriginArchID.Get(id)
		if err != nil {
			return err
		}
		key := storageArchetypeIDForEntityID(id)
		archID, err := m.entityIDToArchID.Get(id)
		if err != nil {
			// this entity has been removed
			if err := pipe.Delete(ctx, key); err != nil {
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
		if err := pipe.Set(ctx, key, archIDAsNum); err != nil {
			return eris.Wrap(err, "")
		}
	}

	return nil
}

// addNextEntityIDToPipe adds any changes to the next available entity ArchetypeID to the given redis pipe.
func (m *EntityCommandBuffer) addNextEntityIDToPipe(ctx context.Context, pipe PrimitiveStorage[string]) error {
	// There are no pending entity id creations, so there's nothing to commit
	if m.pendingEntityIDs == 0 {
		return nil
	}
	key := storageNextEntityIDKey()
	nextID := m.nextEntityIDSaved + m.pendingEntityIDs
	return eris.Wrap(pipe.Set(ctx, key, nextID), "")
}

// addComponentChangesToPipe adds updated component values for entities to the redis pipe.
func (m *EntityCommandBuffer) addComponentChangesToPipe(ctx context.Context, pipe PrimitiveStorage[string]) error {
	keysToDelete, err := m.compValuesToDelete.Keys()
	if err != nil {
		return err
	}
	for _, key := range keysToDelete {
		isMarkedForDeletion, err := m.compValuesToDelete.Get(key)
		if err != nil {
			return err
		}
		if !isMarkedForDeletion {
			continue
		}
		redisKey := storageComponentKey(key.typeID, key.entityID)
		if err := pipe.Delete(ctx, redisKey); err != nil {
			return eris.Wrap(err, "")
		}
	}
	if err = m.compValuesToDelete.Clear(); err != nil {
		return eris.Wrap(err, "failed to clear to-be-deleted component values store")
	}
	keys, err := m.compValues.Keys()
	if err != nil {
		return err
	}
	for _, key := range keys {
		cType, err := m.typeToComponent.Get(key.typeID)
		if err != nil {
			return err
		}
		value, err := m.compValues.Get(key)
		if err != nil {
			return err
		}
		bz, err := cType.Encode(value)
		if err != nil {
			return err
		}

		redisKey := storageComponentKey(key.typeID, key.entityID)
		if err = pipe.Set(ctx, redisKey, bz); err != nil {
			return eris.Wrap(err, "")
		}
	}
	return nil
}

// preloadArchIDs loads the mapping of archetypes IDs to sets of IComponentTypes from dbStorage.
func (m *EntityCommandBuffer) loadArchIDs() error {
	archIDToComps, ok, err := getArchIDToCompTypesFromRedis(m.dbStorage, m.typeToComponent)
	if err != nil {
		return err
	}
	if !ok {
		// Nothing is saved in the DB. Leave the m.archIDToComps field unchanged
		return nil
	}
	if m.archIDToComps.Len() > 0 {
		return eris.New("assigned archetype ArchetypeID is about to be overwritten by something from dbStorage")
	}
	m.archIDToComps = archIDToComps
	return nil
}

// addPendingArchIDsToPipe adds any newly created archetype IDs (as well as the associated sets of components) to the
// redis pipe.
func (m *EntityCommandBuffer) addPendingArchIDsToPipe(ctx context.Context, pipe PrimitiveStorage[string]) error {
	if len(m.pendingArchIDs) == 0 {
		return nil
	}

	bz, err := m.encodeArchIDToCompTypes()
	if err != nil {
		return err
	}

	return eris.Wrap(pipe.Set(ctx, storageArchIDsToCompTypesKey(), bz), "")
}

// addActiveEntityIDsToPipe adds information about which entities are assigned to which archetype IDs to the reids pipe.
func (m *EntityCommandBuffer) addActiveEntityIDsToPipe(ctx context.Context, pipe PrimitiveStorage[string]) error {
	archIDs, err := m.activeEntities.Keys()
	if err != nil {
		return err
	}
	for _, archID := range archIDs {
		active, err := m.activeEntities.Get(archID)
		if err != nil {
			return err
		}
		if !active.modified {
			continue
		}
		bz, err := codec.Encode(active.ids)
		if err != nil {
			return err
		}
		key := storageActiveEntityIDKey(archID)
		err = pipe.Set(ctx, key, bz)
		if err != nil {
			return eris.Wrap(err, "")
		}
	}
	return nil
}

func (m *EntityCommandBuffer) encodeArchIDToCompTypes() ([]byte, error) {
	forStorage := map[types.ArchetypeID][]types.ComponentID{}
	archIDs, err := m.archIDToComps.Keys()
	if err != nil {
		return nil, err
	}
	for _, archID := range archIDs {
		typeIDs := []types.ComponentID{}
		comps, err := m.archIDToComps.Get(archID)
		if err != nil {
			return nil, err
		}
		for _, comp := range comps {
			typeIDs = append(typeIDs, comp.ID())
		}
		forStorage[archID] = typeIDs
	}
	return codec.Encode(forStorage)
}

func getArchIDToCompTypesFromRedis(
	storage PrimitiveStorage[string],
	typeToComp VolatileStorage[types.ComponentID, types.ComponentMetadata],
) (m VolatileStorage[types.ArchetypeID, []types.ComponentMetadata], ok bool, err error) {
	ctx := context.Background()
	key := storageArchIDsToCompTypesKey()
	bz, err := storage.GetBytes(ctx, key)
	err = eris.Wrap(err, "")
	if eris.Is(eris.Cause(err), redis.Nil) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}

	fromStorage, err := codec.Decode[map[types.ArchetypeID][]types.ComponentID](bz)
	if err != nil {
		return nil, false, err
	}

	// result is the mapping of Arch ArchetypeID -> IComponent sets
	result := NewMapStorage[types.ArchetypeID, []types.ComponentMetadata]()
	for archID, compTypeIDs := range fromStorage {
		var currComps []types.ComponentMetadata
		for _, compTypeID := range compTypeIDs {
			currComp, err := typeToComp.Get(compTypeID)
			if err != nil {
				return nil, false, eris.Wrap(iterators.ErrComponentMismatchWithSavedState, "")
			}
			currComps = append(currComps, currComp)
		}

		err = result.Set(archID, currComps)
		if err != nil {
			return nil, false, err
		}
	}
	return result, true, nil
}
