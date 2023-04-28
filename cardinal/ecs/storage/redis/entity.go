package redis

import (
	"context"

	"github.com/redis/go-redis/v9"

	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"
)

// ---------------------------------------------------------------------------
// 							ENTITY LOCATION STORAGE
// ---------------------------------------------------------------------------

var _ storage.EntityLocationStorage = &Storage{}

func (r *Storage) ContainsEntity(id entity.ID) (bool, error) {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	res := r.Client.Get(ctx, key)
	if err := res.Err(); err != nil {
		return false, err
	}
	locBz, err := res.Bytes()
	if err != nil {
		return false, err
	}
	return locBz != nil, nil
}

func (r *Storage) Remove(id entity.ID) error {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	res := r.Client.Del(ctx, key)
	return res.Err()
}

func (r *Storage) Insert(id entity.ID, index storage.ArchetypeIndex, componentIndex storage.ComponentIndex) error {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	loc := &types.Location{
		ArchetypeIndex: uint64(index),
		ComponentIndex: uint64(componentIndex),
		Valid:          true,
	}
	bz, err := marshalProto(loc)
	if err != nil {
		return err
	}
	res := r.Client.Set(ctx, key, bz, 0)
	if err := res.Err(); err != nil {
		return err
	}
	key = r.entityLocationLenKey()
	incRes := r.Client.Incr(ctx, key)
	if err := incRes.Err(); err != nil {
		return err
	}
	return nil
}

func (r *Storage) Set(id entity.ID, location *types.Location) error {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	bz, err := marshalProto(location)
	if err != nil {
		return err
	}
	res := r.Client.Set(ctx, key, bz, 0)
	if err := res.Err(); err != nil {
		return err
	}
	return nil
}

func (r *Storage) Location(id entity.ID) (*types.Location, error) {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	res := r.Client.Get(ctx, key)
	if err := res.Err(); err != nil {
		return nil, err
	}
	bz, err := res.Bytes()
	if err != nil {
		return nil, err
	}
	loc := new(types.Location)
	err = unmarshalProto(bz, loc)
	if err != nil {
		return nil, err
	}
	return loc, nil
}

func (r *Storage) ArchetypeIndex(id entity.ID) (storage.ArchetypeIndex, error) {
	loc, err := r.Location(id)
	return storage.ArchetypeIndex(loc.ArchetypeIndex), err
}

func (r *Storage) ComponentIndexForEntity(id entity.ID) (storage.ComponentIndex, error) {
	loc, err := r.Location(id)
	return storage.ComponentIndex(loc.ComponentIndex), err
}

func (r *Storage) Len() (int, error) {
	ctx := context.Background()
	key := r.entityLocationLenKey()
	res := r.Client.Get(ctx, key)
	if err := res.Err(); err != nil {
		return 0, err
	}
	length, err := res.Int()
	if err != nil {
		return 0, err
	}
	return length, nil
}

// ---------------------------------------------------------------------------
// 							Entity Manager
// ---------------------------------------------------------------------------

var _ storage.EntityManager = &Storage{}

func (r *Storage) Destroy(e storage.Entity) {
	// this is just a no-op, not really needed for redis
	// since we're a bit more space efficient here
}

func (r *Storage) NewEntity() (storage.Entity, error) {
	ctx := context.Background()
	key := r.nextEntityIDKey()
	res := r.Client.Get(ctx, key)
	var nextID uint64
	if err := res.Err(); err != nil {
		if res.Err() == redis.Nil {
			nextID = 0
		} else {
			return 0, err
		}
	} else {
		nextID, err = res.Uint64()
		if err != nil {
			return 0, err
		}
	}

	ent := storage.Entity(nextID)
	incRes := r.Client.Incr(ctx, key)
	if err := incRes.Err(); err != nil {
		return 0, err
	}
	return ent, nil
}
