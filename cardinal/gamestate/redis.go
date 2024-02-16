package gamestate

import (
	"context"

	"pkg.world.dev/world-engine/cardinal/codec"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/types"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
)

type RedisStorage struct {
	cachedClient  redis.Cmdable
	currentClient redis.Cmdable
}

var _ PrimitiveStorage = &RedisStorage{}

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

func (r *RedisStorage) StartTransaction(_ context.Context) (PrimitiveStorage, error) {
	pipeline := r.currentClient.TxPipeline()
	redisTransaction := NewRedisPrimitiveStorage(pipeline)
	redisTransaction.cachedClient = r.currentClient
	return &redisTransaction, nil
}

func (r *RedisStorage) EndTransaction(ctx context.Context) (PrimitiveStorage, error) {
	pipeline, ok := r.currentClient.(redis.Pipeliner)
	if !ok {
		return nil, eris.New("current redis storage is not a pipeline/transaction")
	}
	_, err := pipeline.Exec(ctx)
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	result := NewRedisPrimitiveStorage(r.cachedClient)
	return &result, nil
}

func NewRedisPrimitiveStorage(client redis.Cmdable) RedisStorage {
	// when in transaction "mode" cachedClient will hold the original storage
	// and currentClient will hold the "transaction storage" for now it will just hold two copies of storage.
	return RedisStorage{
		cachedClient:  client,
		currentClient: client,
	}
}

// pipeFlushToRedis return a pipeliner with all pending state changes to redis ready to be committed in an atomic
// transaction. If an error is returned, no redis changes will have been made.
func (m *EntityCommandBuffer) makePipeOfRedisCommands(ctx context.Context) (PrimitiveStorage, error) {
	pipe, err := m.storage.StartTransaction(ctx)
	if err != nil {
		return nil, err
	}

	if m.typeToComponent == nil {
		// component.ComponentID -> ComponentMetadata mappings are required to serialized data for the DB
		return nil, eris.New("must call RegisterComponents before flushing to DB")
	}

	operations := []struct {
		name   string
		method func(ctx context.Context, pipe PrimitiveStorage) error
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
func (m *EntityCommandBuffer) addEntityIDToArchIDToPipe(ctx context.Context, pipe PrimitiveStorage) error {
	for id, originArchID := range m.entityIDToOriginArchID {
		key := redisArchetypeIDForEntityID(id)
		archID, ok := m.entityIDToArchID[id]
		if !ok {
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
func (m *EntityCommandBuffer) addNextEntityIDToPipe(ctx context.Context, pipe PrimitiveStorage) error {
	// There are no pending entity id creations, so there's nothing to commit
	if m.pendingEntityIDs == 0 {
		return nil
	}
	key := redisNextEntityIDKey()
	nextID := m.nextEntityIDSaved + m.pendingEntityIDs
	return eris.Wrap(pipe.Set(ctx, key, nextID), "")
}

// addComponentChangesToPipe adds updated component values for entities to the redis pipe.
func (m *EntityCommandBuffer) addComponentChangesToPipe(ctx context.Context, pipe PrimitiveStorage) error {
	for key, isMarkedForDeletion := range m.compValuesToDelete {
		if !isMarkedForDeletion {
			continue
		}
		redisKey := redisComponentKey(key.typeID, key.entityID)
		if err := pipe.Delete(ctx, redisKey); err != nil {
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
		if err = pipe.Set(ctx, redisKey, bz); err != nil {
			return eris.Wrap(err, "")
		}
	}
	return nil
}

// preloadArchIDs loads the mapping of archetypes IDs to sets of IComponentTypes from storage.
func (m *EntityCommandBuffer) loadArchIDs() error {
	archIDToComps, ok, err := getArchIDToCompTypesFromRedis(m.storage, m.typeToComponent)
	if err != nil {
		return err
	}
	if !ok {
		// Nothing is saved in the DB. Leave the m.archIDToComps field unchanged
		return nil
	}
	if len(m.archIDToComps) > 0 {
		return eris.New("assigned archetype ArchetypeID is about to be overwritten by something from storage")
	}
	m.archIDToComps = archIDToComps
	return nil
}

// addPendingArchIDsToPipe adds any newly created archetype IDs (as well as the associated sets of components) to the
// redis pipe.
func (m *EntityCommandBuffer) addPendingArchIDsToPipe(ctx context.Context, pipe PrimitiveStorage) error {
	if len(m.pendingArchIDs) == 0 {
		return nil
	}

	bz, err := m.encodeArchIDToCompTypes()
	if err != nil {
		return err
	}

	return eris.Wrap(pipe.Set(ctx, redisArchIDsToCompTypesKey(), bz), "")
}

// addActiveEntityIDsToPipe adds information about which entities are assigned to which archetype IDs to the reids pipe.
func (m *EntityCommandBuffer) addActiveEntityIDsToPipe(ctx context.Context, pipe PrimitiveStorage) error {
	for archID, active := range m.activeEntities {
		if !active.modified {
			continue
		}
		bz, err := codec.Encode(active.ids)
		if err != nil {
			return err
		}
		key := redisActiveEntityIDKey(archID)
		err = pipe.Set(ctx, key, bz)
		if err != nil {
			return eris.Wrap(err, "")
		}
	}
	return nil
}

func (m *EntityCommandBuffer) encodeArchIDToCompTypes() ([]byte, error) {
	forStorage := map[types.ArchetypeID][]types.ComponentID{}
	for archID, comps := range m.archIDToComps {
		typeIDs := []types.ComponentID{}
		for _, comp := range comps {
			typeIDs = append(typeIDs, comp.ID())
		}
		forStorage[archID] = typeIDs
	}
	return codec.Encode(forStorage)
}

func getArchIDToCompTypesFromRedis(
	storage PrimitiveStorage,
	typeToComp map[types.ComponentID]types.ComponentMetadata,
) (m map[types.ArchetypeID][]types.ComponentMetadata, ok bool, err error) {
	ctx := context.Background()
	key := redisArchIDsToCompTypesKey()
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
	result := map[types.ArchetypeID][]types.ComponentMetadata{}
	for archID, compTypeIDs := range fromStorage {
		var currComps []types.ComponentMetadata
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
