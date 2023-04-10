package storage

import (
	"context"
	"os"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/argus-labs/cardinal/component"
)

type redisStorage struct {
	worldID                string
	componentStoragePrefix component.TypeID
	c                      *redis.Client
	log                    zerolog.Logger
}

var _ ComponentIndexStorage = &redisStorage{}

func (r *redisStorage) GetComponentIndexStorage(cid component.TypeID) ComponentIndexStorage {
	r.componentStoragePrefix = cid
	return r
}

func (r *redisStorage) ComponentIndex(ai ArchetypeIndex) (ComponentIndex, bool) {
	ctx := context.Background()
	key := r.componentIndexKey(ai)
	res := r.c.Get(ctx, key)
	if err := res.Err(); err != nil {
		r.log.Err(err) // TODO(technicallyty): handle this
	}
	result, err := res.Result()
	if err != nil {
		r.log.Err(err)
		// TODO(technicallyty): handle this
	}
	if len(result) == 0 {
		return 0, false
	}
	ret, err := res.Int()
	if err != nil {
		// TODO(technicallyty): handle this
	}
	return ComponentIndex(ret), true
}

func (r *redisStorage) SetIndex(index ArchetypeIndex, index2 ComponentIndex) {
	ctx := context.Background()
	key := r.componentIndexKey(index)
	res := r.c.Set(ctx, key, int64(index2), 0)
	if err := res.Err(); err != nil {
		// TODO(technicallyty): handle this
	}
}

func (r *redisStorage) IncrementIndex(index ArchetypeIndex) {
	ctx := context.Background()
	idx, ok := r.ComponentIndex(index)
	if !ok {
		// TODO(technicallyty): handle this
	}
	key := r.componentIndexKey(index)
	newIdx := idx + 1
	r.c.Set(ctx, key, int64(newIdx), 0)
}

func (r *redisStorage) DecrementIndex(index ArchetypeIndex) {
	ctx := context.Background()
	idx, ok := r.ComponentIndex(index)
	if !ok {
		// TODO(technicallyty): handle this
	}
	key := r.componentIndexKey(index)
	newIdx := idx - 1
	r.c.Set(ctx, key, int64(newIdx), 0)
}

var _ = redisStorage{}

var _ ComponentStorageManager = &redisStorage{}

func NewRedisStorage(c *redis.Client, worldID string) WorldStorage {

	redisStorage := redisStorage{
		worldID: worldID,
		c:       c,
		log:     zerolog.New(os.Stdout),
	}
	return WorldStorage{CompStore: Components{
		store:            &redisStorage,
		componentIndices: &redisStorage,
	}}
}

// ---------------------------------------------------------------------------
// 							COMPONENT STORAGE MANAGER
// ---------------------------------------------------------------------------

func (r *redisStorage) GetComponentStorage(cid component.TypeID) ComponentStorage {
	r.componentStoragePrefix = cid
	return r
}

func (r redisStorage) InitializeComponentStorage(cid component.TypeID) {
	// initialize a new list within redis... is this even necessary? we shall see..
}

// ---------------------------------------------------------------------------
// 							COMPONENT STORAGE
// ---------------------------------------------------------------------------

func (r *redisStorage) PushComponent(component component.IComponentType, index ArchetypeIndex) error {
	ctx := context.Background()
	key := r.componentDataKey(index)
	componentBz, err := component.New()
	if err != nil {
		return err
	}
	r.c.LPush(ctx, key, componentBz)
	return nil
}

func (r *redisStorage) Component(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) []byte {
	ctx := context.Background()
	key := r.componentDataKey(archetypeIndex)
	res := r.c.LIndex(ctx, key, int64(componentIndex))
	if err := res.Err(); err != nil {
		r.log.Err(err)
		return nil
	}
	bz, err := res.Bytes()
	if err != nil {
		r.log.Err(err)
	}
	return bz
}

func (r *redisStorage) SetComponent(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex, compBz []byte) {
	ctx := context.Background()
	key := r.componentDataKey(archetypeIndex)
	res := r.c.LSet(ctx, key, int64(componentIndex), compBz)
	if err := res.Err(); err != nil {
		r.log.Err(err)
		// TODO(technicallyty): refactor to return error from this interface method.
	}
}

func (r *redisStorage) MoveComponent(source ArchetypeIndex, index ComponentIndex, dst ArchetypeIndex) {
	ctx := context.Background()
	sKey := r.componentDataKey(source)
	dKey := r.componentDataKey(dst)
	res := r.c.LIndex(ctx, sKey, int64(index))
	if err := res.Err(); err != nil {
		r.log.Err(err)
		// TODO(technicallyty): refactor to return error from this interface method.
	}
	// Redis doesn't provide a good way to delete as specific indexes
	// so we use this hack of setting the value to DELETE, and then deleting by that value.
	statusRes := r.c.LSet(ctx, sKey, int64(index), "DELETE")
	if err := statusRes.Err(); err != nil {
		r.log.Err(err)
	}
	componentBz, err := res.Bytes()
	if err != nil {
		r.log.Err(err)
		// TODO(technicallyty): refactor to return error from this interface method.
	}
	r.c.LPush(ctx, dKey, componentBz)
	cmd := r.c.LRem(ctx, sKey, 1, "DELETE")
	if err := cmd.Err(); err != nil {
		r.log.Err(err)
		// TODO(technicallyty): refactor to return error from this interface method.
	}
}

func (r *redisStorage) SwapRemove(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) []byte {
	ctx := context.Background()
	r.delete(ctx, archetypeIndex, componentIndex)
	return nil
}

func (r *redisStorage) delete(ctx context.Context, index ArchetypeIndex, componentIndex ComponentIndex) {
	sKey := r.componentDataKey(index)
	statusRes := r.c.LSet(ctx, sKey, int64(componentIndex), "DELETE")
	if err := statusRes.Err(); err != nil {
		r.log.Err(err)
	}
	cmd := r.c.LRem(ctx, sKey, 1, "DELETE")
	if err := cmd.Err(); err != nil {
		r.log.Err(err)
	}
}

func (r *redisStorage) Contains(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) bool {
	ctx := context.Background()
	key := r.componentDataKey(archetypeIndex)
	res := r.c.LIndex(ctx, key, int64(componentIndex))
	if err := res.Err(); err != nil {
		r.log.Err(err)
	}
	result, err := res.Result()
	if err != nil {
		r.log.Err(err)
	}
	return len(result) > 0
}
