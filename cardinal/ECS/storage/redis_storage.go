package storage

import (
	"context"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/argus-labs/cardinal/ECS/component"
	"github.com/argus-labs/cardinal/ECS/entity"
)

// Archetypes can just be stored in program memory. It just a structure that allows us to quickly
// decipher combinations of components. There is no point in storing such information in a backend.
// at the very least, we may want to store the list of entities that an archetype has.
//
// Archetype -> group of entities for specific set of components. makes it easy to find entities based on comps.
// example:
//
//
//
// Normal Planet Archetype(1): EnergyComponent, OwnableComponent
// Entities (1), (2), (3)....
//
// In Go memory -> []Archetypes {arch1 (maps to above)}
//
// We need to consider if this needs to be stored in a backend at all. We _should_ be able to build archetypes from
// system restarts as they don't really contain any information about the location of anything stored in a backend.
//
// Something to consider -> we should do something i.e. RegisterComponents, and have it deterministically assign TypeID's to components.
// We could end up with some issues (needs to be determined).

type redisStorage struct {
	worldID                string
	componentStoragePrefix component.TypeID
	c                      *redis.Client
	log                    zerolog.Logger
	archetypeCache         ArchetypeAccessor
}

var _ = redisStorage{}

func NewRedisStorage(c *redis.Client, worldID string) WorldStorage {

	redisStorage := redisStorage{
		worldID: worldID,
		c:       c,
		log:     zerolog.New(os.Stdout),
	}
	return WorldStorage{
		CompStore: Components{
			store:            &redisStorage,
			componentIndices: &redisStorage,
		},
		EntityLocStore:   &redisStorage,
		ArchAccessor:     NewArchetypeAccessor(),
		ArchCompIdxStore: NewArchetypeComponentIndex(),
		EntryStore:       &redisStorage,
		EntityMgr:        &redisStorage,
	}
}

// ---------------------------------------------------------------------------
// 							COMPONENT INDEX STORAGE
// ---------------------------------------------------------------------------

var _ ComponentIndexStorage = &redisStorage{}

func (r *redisStorage) GetComponentIndexStorage(cid component.TypeID) ComponentIndexStorage {
	r.componentStoragePrefix = cid
	return r
}

func (r *redisStorage) ComponentIndex(ai ArchetypeIndex) (ComponentIndex, bool, error) {
	ctx := context.Background()
	key := r.componentIndexKey(ai)
	res := r.c.Get(ctx, key)
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
	return ComponentIndex(ret), true, nil
}

func (r *redisStorage) SetIndex(index ArchetypeIndex, index2 ComponentIndex) error {
	ctx := context.Background()
	key := r.componentIndexKey(index)
	res := r.c.Set(ctx, key, int64(index2), 0)
	return res.Err()
}

func (r *redisStorage) IncrementIndex(index ArchetypeIndex) error {
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
	res := r.c.Set(ctx, key, int64(newIdx), 0)
	return res.Err()
}

func (r *redisStorage) DecrementIndex(index ArchetypeIndex) error {
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
	res := r.c.Set(ctx, key, int64(newIdx), 0)
	return res.Err()
}

// ---------------------------------------------------------------------------
// 							COMPONENT STORAGE MANAGER
// ---------------------------------------------------------------------------

var _ ComponentStorageManager = &redisStorage{}

func (r *redisStorage) GetComponentStorage(cid component.TypeID) ComponentStorage {
	r.componentStoragePrefix = cid
	return r
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
	res := r.c.LPush(ctx, key, componentBz)
	return res.Err()
}

func (r *redisStorage) Component(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) ([]byte, error) {
	ctx := context.Background()
	key := r.componentDataKey(archetypeIndex)
	res := r.c.LIndex(ctx, key, int64(componentIndex))
	if err := res.Err(); err != nil {
		return nil, err
	}
	bz, err := res.Bytes()
	if err != nil {
		return nil, err
	}
	return bz, nil
}

func (r *redisStorage) SetComponent(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex, compBz []byte) error {
	ctx := context.Background()
	key := r.componentDataKey(archetypeIndex)
	res := r.c.LSet(ctx, key, int64(componentIndex), compBz)
	return res.Err()
}

func (r *redisStorage) MoveComponent(source ArchetypeIndex, index ComponentIndex, dst ArchetypeIndex) error {
	ctx := context.Background()
	sKey := r.componentDataKey(source)
	dKey := r.componentDataKey(dst)
	res := r.c.LIndex(ctx, sKey, int64(index))
	if err := res.Err(); err != nil {
		return err
	}
	// Redis doesn't provide a good way to delete as specific indexes
	// so we use this hack of setting the value to DELETE, and then deleting by that value.
	statusRes := r.c.LSet(ctx, sKey, int64(index), "DELETE")
	if err := statusRes.Err(); err != nil {
		return err
	}
	componentBz, err := res.Bytes()
	if err != nil {
		return err
	}
	pushRes := r.c.LPush(ctx, dKey, componentBz)
	if err := pushRes.Err(); err != nil {
		return err
	}
	cmd := r.c.LRem(ctx, sKey, 1, "DELETE")
	return cmd.Err()
}

func (r *redisStorage) SwapRemove(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) ([]byte, error) {
	ctx := context.Background()
	err := r.delete(ctx, archetypeIndex, componentIndex)
	return nil, err
}

func (r *redisStorage) delete(ctx context.Context, index ArchetypeIndex, componentIndex ComponentIndex) error {
	sKey := r.componentDataKey(index)
	statusRes := r.c.LSet(ctx, sKey, int64(componentIndex), "DELETE")
	if err := statusRes.Err(); err != nil {
		return err
	}
	cmd := r.c.LRem(ctx, sKey, 1, "DELETE")
	if err := cmd.Err(); err != nil {
		return err
	}
	return nil
}

func (r *redisStorage) Contains(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) (bool, error) {
	ctx := context.Background()
	key := r.componentDataKey(archetypeIndex)
	res := r.c.LIndex(ctx, key, int64(componentIndex))
	if err := res.Err(); err != nil {
		return false, err
	}
	result, err := res.Result()
	if err != nil {
		return false, err
	}
	return len(result) > 0, nil
}

// ---------------------------------------------------------------------------
// 							ENTITY LOCATION STORAGE
// ---------------------------------------------------------------------------

var _ EntityLocationStorage = &redisStorage{}

func (r *redisStorage) ContainsEntity(id entity.ID) (bool, error) {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	res := r.c.Get(ctx, key)
	if err := res.Err(); err != nil {
		return false, err
	}
	locBz, err := res.Bytes()
	if err != nil {
		return false, err
	}
	return locBz != nil, nil
}

func (r *redisStorage) Remove(id entity.ID) error {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	res := r.c.Del(ctx, key)
	return res.Err()
}

func (r *redisStorage) Insert(id entity.ID, index ArchetypeIndex, componentIndex ComponentIndex) error {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	loc := NewLocation(index, componentIndex)
	bz, err := Encode(loc)
	if err != nil {
		return err
	}
	res := r.c.Set(ctx, key, bz, 0)
	if err := res.Err(); err != nil {
		return err
	}
	key = r.entityLocationLenKey()
	incRes := r.c.Incr(ctx, key)
	if err := incRes.Err(); err != nil {
		return err
	}
	return nil
}

func (r *redisStorage) Set(id entity.ID, location *Location) error {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	bz, err := Encode(*location)
	if err != nil {
		return err
	}
	res := r.c.Set(ctx, key, bz, 0)
	if err := res.Err(); err != nil {
		return err
	}
	return nil
}

func (r *redisStorage) Location(id entity.ID) (*Location, error) {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	res := r.c.Get(ctx, key)
	if err := res.Err(); err != nil {
		return nil, err
	}
	bz, err := res.Bytes()
	if err != nil {
		return nil, err
	}
	loc, err := Decode[Location](bz)
	if err != nil {
		return nil, err
	}
	return &loc, nil
}

func (r *redisStorage) ArchetypeIndex(id entity.ID) ArchetypeIndex {
	loc, _ := r.Location(id)
	return loc.ArchIndex
}

func (r *redisStorage) ComponentIndexForEntity(id entity.ID) ComponentIndex {
	loc, _ := r.Location(id)
	return loc.CompIndex
}

func (r *redisStorage) Len() (int, error) {
	ctx := context.Background()
	key := r.entityLocationLenKey()
	res := r.c.Get(ctx, key)
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
// 							ENTRY STORAGE
// ---------------------------------------------------------------------------

var _ EntryStorage = &redisStorage{}

func (r *redisStorage) SetEntry(id entity.ID, entry *Entry) error {
	ctx := context.Background()
	key := r.entryStorageKey(id)
	bz, err := Encode(entry)
	if err != nil {
		return err
	}
	res := r.c.Set(ctx, key, bz, 0)
	if err := res.Err(); err != nil {
		return err
	}
	return nil
}

func (r *redisStorage) GetEntry(id entity.ID) (*Entry, error) {
	ctx := context.Background()
	key := r.entryStorageKey(id)
	res := r.c.Get(ctx, key)
	if err := res.Err(); err != nil {
		return nil, err
	}
	bz, err := res.Bytes()
	if err != nil {
		return nil, err
	}
	decodedEntry, err := Decode[Entry](bz)
	if err != nil {
		return nil, err
	}
	return &decodedEntry, nil
}

func (r *redisStorage) SetEntity(id entity.ID, e Entity) {
	entry, _ := r.GetEntry(id)
	entry.Ent = e
	r.SetEntry(id, entry)
}

func (r *redisStorage) SetLocation(id entity.ID, location Location) {
	entry, _ := r.GetEntry(id)
	entry.Loc = &location
	r.SetEntry(id, entry)
}

// ---------------------------------------------------------------------------
// 							Entity Manager
// ---------------------------------------------------------------------------

var _ EntityManager = &redisStorage{}

func (r *redisStorage) Destroy(e Entity) {
	// this is just a no-op, not really needed for redis
	// since we're a bit more space efficient here
}

func (r *redisStorage) NewEntity() (Entity, error) {
	ctx := context.Background()
	key := r.nextEntityIDKey()
	res := r.c.Get(ctx, key)
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

	ent := Entity(nextID)
	incRes := r.c.Incr(ctx, key)
	if err := incRes.Err(); err != nil {
		return 0, err
	}
	return ent, nil
}
