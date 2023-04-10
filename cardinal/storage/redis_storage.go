package storage

import (
	"context"
	"fmt"
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

func (r *redisStorage) ComponentIndex(ai ArchetypeIndex) (ComponentIndex, bool) {
	//TODO implement me
	panic("implement me")
}

func (r *redisStorage) SetIndex(index ArchetypeIndex, index2 ComponentIndex) {
	//TODO implement me
	panic("implement me")
}

func (r *redisStorage) IncrementIndex(index ArchetypeIndex) {
	//TODO implement me
	panic("implement me")
}

func (r *redisStorage) DecrementIndex(index ArchetypeIndex) {
	//TODO implement me
	panic("implement me")
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

func (r *redisStorage) componentDataKey(index ArchetypeIndex) string {
	return fmt.Sprintf("WORLD-%s:CID-%d:A-%d", r.worldID, r.componentStoragePrefix, index)
}
