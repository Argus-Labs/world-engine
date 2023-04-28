package redis

import (
	"context"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"
)

// ---------------------------------------------------------------------------
// 							ARCHETYPE STORAGE
// ---------------------------------------------------------------------------

var _ storage.ArchetypeStorage = &Storage{}

func (r *Storage) PushArchetype(index storage.ArchetypeIndex, layout []component.IComponentType) error {
	ctx := context.Background()
	anys := make([]*anypb.Any, len(layout))
	for i, comp := range layout {
		a, err := anypb.New(comp)
		if err != nil {
			return err
		}
		anys[i] = a
	}
	a := &types.Archetype{
		ArchetypeIndex: uint64(index),
		Components:     anys,
	}
	err := r.setArchetype(ctx, a)
	if err != nil {
		return err
	}
	return nil
}

func (r *Storage) Archetype(index storage.ArchetypeIndex) (*types.Archetype, error) {
	ctx := context.Background()
	key := r.archetypeStorageKey(index)
	res := r.Client.Get(ctx, key)
	if res.Err() != nil {
		return nil, res.Err()
	}
	bz, err := res.Bytes()
	if err != nil {
		return nil, err
	}
	a := new(types.Archetype)
	err = unmarshalProto(bz, a)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *Storage) RemoveEntity(index storage.ArchetypeIndex, entityIndex int) (entity.Entity, error) {
	arch, err := r.Archetype(index)
	if err != nil {
		return 0, err
	}
	removed := arch.EntityIds[entityIndex]
	length := len(arch.EntityIds)
	arch.EntityIds[entityIndex] = arch.EntityIds[length-1]
	arch.EntityIds = arch.EntityIds[:length-1]
	return entity.Entity(removed), nil
}

func (r *Storage) PushEntity(index storage.ArchetypeIndex, e entity.Entity) error {
	ctx := context.Background()
	arch, err := r.Archetype(index)
	if err != nil {
		return err
	}
	arch.EntityIds = append(arch.EntityIds, uint64(e.ID()))
	err = r.setArchetype(ctx, arch)
	if err != nil {
		return err
	}
	return nil
}

func (r *Storage) setArchetype(ctx context.Context, a *types.Archetype) error {
	key := r.archetypeStorageKey(storage.ArchetypeIndex(a.ArchetypeIndex))
	bz, err := marshalProto(a)
	if err != nil {
		return err
	}
	res := r.Client.Set(ctx, key, bz, 0)
	return res.Err()
}

func (r *Storage) GetNextArchetypeIndex() (uint64, error) {
	ctx := context.Background()
	key := r.archetypeIndexKey()
	res := r.Client.Get(ctx, key)
	if res.Err() != nil {
		return 0, res.Err()
	}
	idx, err := res.Uint64()
	if err != nil {
		return 0, err
	}
	setRes := r.Client.Set(ctx, key, idx+1, 0)
	if setRes.Err() != nil {
		return 0, setRes.Err()
	}
	return idx, nil
}

// ---------------------------------------------------------------------------
// 						Archetype Component Index
// ---------------------------------------------------------------------------

// TODO(technicallyty): impl

var _ storage.ArchetypeComponentIndex = &Storage{}

func (r *Storage) Push(layout []component.IComponentType) {
	//TODO implement me
	panic("implement me")
}

func (r *Storage) SearchFrom(filter filter.LayoutFilter, start int) *storage.ArchetypeIterator {
	//TODO implement me
	panic("implement me")
}

func (r *Storage) Search(layoutFilter filter.LayoutFilter) *storage.ArchetypeIterator {
	//TODO implement me
	panic("implement me")
}
