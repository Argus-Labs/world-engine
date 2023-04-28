package redis

import (
	"context"
	"fmt"

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

func (r *Storage) SetIndex(index storage.ArchetypeIndex, index2 storage.ComponentIndex) error {
	ctx := context.Background()
	key := r.componentIndexKey(index)
	res := r.Client.Set(ctx, key, int64(index2), 0)
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

func (r *Storage) PushComponent(comp component.IComponentType, index storage.ArchetypeIndex) error {
	ctx := context.Background()
	key := r.componentDataKey(index)
	bz, err := r.encode(comp)
	if err != nil {
		return err
	}
	res := r.Client.LPush(ctx, key, bz)
	return res.Err()
}

func (r *Storage) Component(archetypeIndex storage.ArchetypeIndex, componentIndex storage.ComponentIndex) (component.IComponentType, error) {
	ctx := context.Background()
	key := r.componentDataKey(archetypeIndex)
	res := r.Client.LIndex(ctx, key, int64(componentIndex))
	if err := res.Err(); err != nil {
		return nil, err
	}
	bz, err := res.Bytes()
	if err != nil {
		return nil, err
	}
	msg, err := r.decode(bz)
	return msg, err
}

func (r *Storage) SetComponent(archetypeIndex storage.ArchetypeIndex, componentIndex storage.ComponentIndex, comp component.IComponentType) error {
	ctx := context.Background()
	key := r.componentDataKey(archetypeIndex)
	bz, err := r.encode(comp)
	if err != nil {
		return err
	}
	res := r.Client.LSet(ctx, key, int64(componentIndex), bz)
	return res.Err()
}

func (r *Storage) MoveComponent(source storage.ArchetypeIndex, index storage.ComponentIndex, dst storage.ArchetypeIndex) error {
	ctx := context.Background()
	sKey := r.componentDataKey(source)
	dKey := r.componentDataKey(dst)
	res := r.Client.LIndex(ctx, sKey, int64(index))
	if err := res.Err(); err != nil {
		return err
	}
	// Redis doesn't provide a good way to delete as specific indexes
	// so we use this hack of setting the value to DELETE, and then deleting by that value.
	statusRes := r.Client.LSet(ctx, sKey, int64(index), "DELETE")
	if err := statusRes.Err(); err != nil {
		return err
	}
	componentBz, err := res.Bytes()
	if err != nil {
		return err
	}
	pushRes := r.Client.LPush(ctx, dKey, componentBz)
	if err := pushRes.Err(); err != nil {
		return err
	}
	cmd := r.Client.LRem(ctx, sKey, 1, "DELETE")
	return cmd.Err()
}

func (r *Storage) SwapRemove(archetypeIndex storage.ArchetypeIndex, componentIndex storage.ComponentIndex) ([]byte, error) {
	ctx := context.Background()
	err := r.delete(ctx, archetypeIndex, componentIndex)
	return nil, err
}

func (r *Storage) delete(ctx context.Context, index storage.ArchetypeIndex, componentIndex storage.ComponentIndex) error {
	sKey := r.componentDataKey(index)
	statusRes := r.Client.LSet(ctx, sKey, int64(componentIndex), "DELETE")
	if err := statusRes.Err(); err != nil {
		return err
	}
	cmd := r.Client.LRem(ctx, sKey, 1, "DELETE")
	if err := cmd.Err(); err != nil {
		return err
	}
	return nil
}

func (r *Storage) Contains(archetypeIndex storage.ArchetypeIndex, componentIndex storage.ComponentIndex) (bool, error) {
	ctx := context.Background()
	key := r.componentDataKey(archetypeIndex)
	res := r.Client.LIndex(ctx, key, int64(componentIndex))
	if err := res.Err(); err != nil {
		return false, err
	}
	result, err := res.Result()
	if err != nil {
		return false, err
	}
	return len(result) > 0, nil
}

func (r *Storage) PushRawComponent(a *anypb.Any, idx storage.ArchetypeIndex) error {
	ctx := context.Background()
	key := r.componentDataKey(idx)
	bz, err := marshalProto(a)
	if err != nil {
		return err
	}
	res := r.Client.LPush(ctx, key, bz)
	return res.Err()
}
