package redis

import (
	"context"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

// ---------------------------------------------------------------------------
// 							COMPONENT STORAGE MANAGER
// ---------------------------------------------------------------------------

var _ storage.ComponentStorageManager = &Storage{}

func (r *Storage) GetComponentStorage(cid string) storage.ComponentStorage {
	r.componentStoragePrefix = cid
	return r
}

func (r *Storage) GetNextIndex(ctx context.Context, index storage.ArchetypeIndex) (storage.ComponentIndex, error) {
	return r.getNextComponentIndex(ctx, index)
}

// ---------------------------------------------------------------------------
// 							COMPONENT STORAGE
// ---------------------------------------------------------------------------

func (r *Storage) Component(archIdx storage.ArchetypeIndex, compIdx storage.ComponentIndex) (component.IComponentType, error) {
	ctx := context.Background()
	key := r.componentDataKey(archIdx, compIdx)
	res := r.Client.Get(ctx, key)
	if err := res.Err(); err != nil {
		return nil, err
	}
	bz, err := res.Bytes()
	if err != nil {
		return nil, err
	}
	msg, err := r.decodeComponent(bz)
	return msg, err
}

func (r *Storage) SetComponent(archIdx storage.ArchetypeIndex, compIdx storage.ComponentIndex, comp component.IComponentType) error {
	ctx := context.Background()
	key := r.componentDataKey(archIdx, compIdx)
	bz, err := r.encodeComponent(comp)
	if err != nil {
		return err
	}
	res := r.Client.Set(ctx, key, bz, 0)
	return res.Err()
}

func (r *Storage) MoveComponent(srcArchIdx storage.ArchetypeIndex, compIdx storage.ComponentIndex, dstArchIdx storage.ArchetypeIndex) error {
	ctx := context.Background()
	sKey := r.componentDataKey(srcArchIdx, compIdx)
	dKey := r.componentDataKey(dstArchIdx, compIdx)

	// get the source component
	res := r.Client.Get(ctx, sKey)
	if err := res.Err(); err != nil {
		return err
	}

	err := r.delete(ctx, srcArchIdx, compIdx)
	if err != nil {
		return err
	}

	componentBz, err := res.Bytes()
	if err != nil {
		return err
	}
	setRes := r.Client.Set(ctx, dKey, componentBz, 0)
	if err := setRes.Err(); err != nil {
		return err
	}

	return nil
}

func (r *Storage) RemoveComponent(archetypeIndex storage.ArchetypeIndex, componentIndex storage.ComponentIndex) error {
	ctx := context.Background()
	err := r.delete(ctx, archetypeIndex, componentIndex)
	return err
}

func (r *Storage) delete(ctx context.Context, index storage.ArchetypeIndex, componentIndex storage.ComponentIndex) error {
	sKey := r.componentDataKey(index, componentIndex)
	cmd := r.Client.Del(ctx, sKey)
	if err := cmd.Err(); err != nil {
		return err
	}
	return nil
}

func (r *Storage) Contains(archetypeIndex storage.ArchetypeIndex, componentIndex storage.ComponentIndex) (bool, error) {
	ctx := context.Background()
	key := r.componentDataKey(archetypeIndex, componentIndex)
	res := r.Client.Get(ctx, key)
	if res.Err() != nil {
		if res.Err() == redis.Nil {
			return false, nil
		}
		return false, res.Err()
	}
	return true, nil
}

func (r *Storage) PushComponent(comp component.IComponentType, index storage.ArchetypeIndex, componentIndex storage.ComponentIndex) error {
	anyComp, err := anypb.New(comp)
	if err != nil {
		return err
	}
	return r.PushRawComponent(anyComp, index, componentIndex)
}

func (r *Storage) PushRawComponent(a *anypb.Any, archIdx storage.ArchetypeIndex, compIdx storage.ComponentIndex) error {
	ctx := context.Background()
	key := r.componentDataKey(archIdx, compIdx)
	bz, err := marshalProto(a)
	if err != nil {
		return err
	}
	res := r.Client.Set(ctx, key, bz, 0)
	return res.Err()
}

func (r *Storage) getNextComponentIndex(ctx context.Context, idx storage.ArchetypeIndex) (storage.ComponentIndex, error) {
	key := r.componentIndexKey(idx)
	res := r.Client.Get(ctx, key)
	compIdx, err := res.Uint64()
	if err != nil && err != redis.Nil {
		return 0, err
	}
	res2 := r.Client.Incr(ctx, key)
	if res.Err() != nil {
		return 0, res2.Err()
	}
	return storage.ComponentIndex(compIdx), nil
}
