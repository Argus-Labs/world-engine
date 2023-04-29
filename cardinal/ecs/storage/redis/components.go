package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

// ---------------------------------------------------------------------------
// 							COMPONENT INDEX STORAGE
// ---------------------------------------------------------------------------

var _ storage.ComponentIndexStorage = &Storage{}

func (r *Storage) ComponentIndex(ai storage.ArchetypeIndex) (storage.ComponentIndex, bool, error) {
	ctx := context.Background()
	key := r.componentIndexKey(ai)
	res := r.Client.Get(ctx, key)
	if err := res.Err(); err != nil {
		return 0, false, err
	}
	result, err := res.Result()
	if err != nil {
		return 0, false, err
	}
	if len(result) == 0 {
		return 0, false, nil
	}
	ret, err := res.Int()
	if err != nil {
		return 0, false, err
	}
	return storage.ComponentIndex(ret), true, nil
}

func (r *Storage) SetIndex(ai storage.ArchetypeIndex, ci storage.ComponentIndex) error {
	ctx := context.Background()
	key := r.componentIndexKey(ai)
	res := r.Client.Set(ctx, key, int64(ci), 0)
	return res.Err()
}

func (r *Storage) IncrementIndex(index storage.ArchetypeIndex) error {
	ctx := context.Background()
	idx, ok, err := r.ComponentIndex(index)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("component index not found at archetype index %d", index)
	}
	key := r.componentIndexKey(index)
	newIdx := idx + 1
	res := r.Client.Set(ctx, key, int64(newIdx), 0)
	return res.Err()
}

func (r *Storage) DecrementIndex(index storage.ArchetypeIndex) error {
	ctx := context.Background()
	idx, ok, err := r.ComponentIndex(index)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("component index not found at archetype index %d", index)
	}
	key := r.componentIndexKey(index)
	newIdx := idx - 1
	res := r.Client.Set(ctx, key, int64(newIdx), 0)
	return res.Err()
}

// ---------------------------------------------------------------------------
// 							COMPONENT STORAGE MANAGER
// ---------------------------------------------------------------------------

var _ storage.ComponentStorageManager = &Storage{}

func (r *Storage) GetComponentStorage(cid string) storage.ComponentStorage {
	r.componentStoragePrefix = cid
	return r
}

func (r *Storage) GetComponentIndexStorage(ct component.IComponentType) storage.ComponentIndexStorage {
	r.componentStoragePrefix = string(ct.ProtoReflect().Descriptor().FullName())
	return r
}

// ---------------------------------------------------------------------------
// 							COMPONENT STORAGE
// ---------------------------------------------------------------------------

func (r *Storage) getComponentIndex(ctx context.Context, ai storage.ArchetypeIndex) (storage.ComponentIndex, error) {
	key := r.componentIndexKey(ai)
	res := r.Client.Get(ctx, key)
	var idx uint64
	if err := res.Err(); err != nil {
		if err == redis.Nil {
			idx = 0
		} else {
			return 0, err
		}
	} else {
		idx, err = res.Uint64()
		if err != nil {
			return 0, err
		}
	}
	setRes := r.Client.Set(ctx, key, idx+1, 0)
	if err := setRes.Err(); err != nil {
		return 0, err
	}

	return storage.ComponentIndex(idx), nil
}

func (r *Storage) PushComponent(comp component.IComponentType, archIdx storage.ArchetypeIndex) (storage.ComponentIndex, error) {
	ctx := context.Background()
	compIdx, err := r.getComponentIndex(ctx, archIdx)
	if err != nil {
		return 0, err
	}
	key := r.componentDataKey(archIdx, compIdx)
	bz, err := r.encodeComponent(comp)
	if err != nil {
		return 0, err
	}
	res := r.Client.Set(ctx, key, bz, 0)
	if res.Err() != nil {
		return 0, res.Err()
	}
	return compIdx, err
}

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

func (r *Storage) PushRawComponent(a *anypb.Any, idx storage.ArchetypeIndex) error {
	ctx := context.Background()
	compIdx, err := r.getComponentIndex(ctx, idx)
	if err != nil {
		return err
	}
	key := r.componentDataKey(idx, compIdx)
	bz, err := marshalProto(a)
	if err != nil {
		return err
	}
	res := r.Client.Set(ctx, key, bz, 0)
	return res.Err()
}
