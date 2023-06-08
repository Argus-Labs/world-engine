package storage

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
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

type RedisStorage struct {
	WorldID                string
	ComponentStoragePrefix component.TypeID
	Client                 *redis.Client
	Log                    zerolog.Logger
	ArchetypeCache         ArchetypeAccessor
}

type Options = redis.Options

func NewRedisStorage(options Options, worldID string) RedisStorage {
	return RedisStorage{
		WorldID: worldID,
		Client:  redis.NewClient(&options),
		Log:     zerolog.New(os.Stdout),
	}
}

// ---------------------------------------------------------------------------
// 							COMPONENT INDEX STORAGE
// ---------------------------------------------------------------------------

var _ ComponentIndexStorage = &RedisStorage{}

func (r *RedisStorage) GetComponentIndexStorage(cid component.TypeID) ComponentIndexStorage {
	r.ComponentStoragePrefix = cid
	return r
}

// ComponentIndex returns the current component index for this archetype.
// If this archetype is missing, 0, false, nil will be returned. If you plan on using this index
// call IncrementIndex instead and use the returned index.
func (r *RedisStorage) ComponentIndex(ai ArchetypeID) (ComponentIndex, bool, error) {
	ctx := context.Background()
	key := r.archetypeIndexKey(ai)
	res := r.Client.Get(ctx, key)
	if err := res.Err(); err == redis.Nil {
		return 0, false, nil
	} else if err != nil {
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

func (r *RedisStorage) SetIndex(archID ArchetypeID, compIndex ComponentIndex) error {
	ctx := context.Background()
	key := r.archetypeIndexKey(archID)
	res := r.Client.Set(ctx, key, int64(compIndex), 0)
	return res.Err()
}

// IncrementIndex adds 1 to this archetype and returns the NEW value of the index. If this archetype
// doesn't exist, this index is initialized and 0 is returned.
func (r *RedisStorage) IncrementIndex(archID ArchetypeID) (ComponentIndex, error) {
	ctx := context.Background()
	idx, ok, err := r.ComponentIndex(archID)
	if err != nil {
		return 0, err
	} else if !ok {
		idx = 0
	} else {
		idx++
	}
	key := r.archetypeIndexKey(archID)
	res := r.Client.Set(ctx, key, int64(idx), 0)
	return idx, res.Err()
}

// DecrementIndex decreases the component index for this archetype by 1.
func (r *RedisStorage) DecrementIndex(archID ArchetypeID) error {
	ctx := context.Background()
	idx, ok, err := r.ComponentIndex(archID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("component index not found at archetype index %d", archID)
	}
	key := r.archetypeIndexKey(archID)
	newIdx := idx - 1
	res := r.Client.Set(ctx, key, int64(newIdx), 0)
	return res.Err()
}

// ---------------------------------------------------------------------------
// 							COMPONENT STORAGE MANAGER
// ---------------------------------------------------------------------------

var _ ComponentStorageManager = &RedisStorage{}

func (r *RedisStorage) GetComponentStorage(cid component.TypeID) ComponentStorage {
	r.ComponentStoragePrefix = cid
	return r
}

// ---------------------------------------------------------------------------
// 							COMPONENT STORAGE
// ---------------------------------------------------------------------------

func (r *RedisStorage) PushComponent(component component.IComponentType, archID ArchetypeID) error {
	ctx := context.Background()
	key := r.componentDataKey(archID, r.ComponentStoragePrefix)
	componentBz, err := component.New()
	if err != nil {
		return err
	}
	res := r.Client.LPush(ctx, key, componentBz)
	return res.Err()
}

func (r *RedisStorage) Component(archetypeID ArchetypeID, componentIndex ComponentIndex) ([]byte, error) {
	ctx := context.Background()
	key := r.componentDataKey(archetypeID, r.ComponentStoragePrefix)
	res := r.Client.LIndex(ctx, key, int64(componentIndex))
	if err := res.Err(); err == redis.Nil {
		return nil, ErrorComponentNotOnEntity
	} else if err != nil {
		return nil, err
	}
	bz, err := res.Bytes()
	if err != nil {
		return nil, err
	}
	return bz, nil
}

func (r *RedisStorage) SetComponent(archetypeID ArchetypeID, componentIndex ComponentIndex, compBz []byte) error {
	ctx := context.Background()
	key := r.componentDataKey(archetypeID, r.ComponentStoragePrefix)
	res := r.Client.LSet(ctx, key, int64(componentIndex), compBz)
	return res.Err()
}

func (r *RedisStorage) MoveComponent(source ArchetypeID, index ComponentIndex, dst ArchetypeID) error {
	ctx := context.Background()
	sKey := r.componentDataKey(source, r.ComponentStoragePrefix)
	dKey := r.componentDataKey(dst, r.ComponentStoragePrefix)
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

func (r *RedisStorage) SwapRemove(archetypeID ArchetypeID, componentIndex ComponentIndex) ([]byte, error) {
	ctx := context.Background()
	err := r.delete(ctx, archetypeID, componentIndex)
	return nil, err
}

func (r *RedisStorage) delete(ctx context.Context, archID ArchetypeID, componentIndex ComponentIndex) error {
	sKey := r.componentDataKey(archID, r.ComponentStoragePrefix)
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

func (r *RedisStorage) Contains(archetypeID ArchetypeID, componentIndex ComponentIndex) (bool, error) {
	ctx := context.Background()
	key := r.componentDataKey(archetypeID, r.ComponentStoragePrefix)
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

// ---------------------------------------------------------------------------
// 							ENTITY LOCATION STORAGE
// ---------------------------------------------------------------------------

var _ EntityLocationStorage = &RedisStorage{}

func (r *RedisStorage) ContainsEntity(id EntityID) (bool, error) {
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

func (r *RedisStorage) Remove(id EntityID) error {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	res := r.Client.Del(ctx, key)
	return res.Err()
}

func (r *RedisStorage) Insert(id EntityID, archID ArchetypeID, componentIndex ComponentIndex) error {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	loc := NewLocation(archID, componentIndex)
	bz, err := Encode(loc)
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

func (r *RedisStorage) SetLocation(id EntityID, location Location) error {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	bz, err := Encode(location)
	if err != nil {
		return err
	}
	res := r.Client.Set(ctx, key, bz, 0)
	if err := res.Err(); err != nil {
		return err
	}
	return nil
}

func (r *RedisStorage) GetLocation(id EntityID) (loc Location, err error) {
	ctx := context.Background()
	key := r.entityLocationKey(id)
	res := r.Client.Get(ctx, key)
	if err := res.Err(); err != nil {
		return loc, err
	}
	bz, err := res.Bytes()
	if err != nil {
		return loc, err
	}
	loc, err = Decode[Location](bz)
	if err != nil {
		return loc, err
	}
	return loc, nil
}

func (r *RedisStorage) ArchetypeID(id EntityID) (ArchetypeID, error) {
	loc, err := r.GetLocation(id)
	return loc.ArchID, err
}

func (r *RedisStorage) ComponentIndexForEntity(id EntityID) (ComponentIndex, error) {
	loc, err := r.GetLocation(id)
	return loc.CompIndex, err
}

func (r *RedisStorage) Len() (int, error) {
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

var _ EntityManager = &RedisStorage{}

func (r *RedisStorage) Destroy(e EntityID) {
	// this is just a no-op, not really needed for redis
	// since we're a bit more space efficient here
}

func (r *RedisStorage) NewEntity() (EntityID, error) {
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

	ent := EntityID(nextID)
	incRes := r.Client.Incr(ctx, key)
	if err := incRes.Err(); err != nil {
		return 0, err
	}
	return ent, nil
}

const (
	// Reids values are limited to 512 mb (https://redis.io/docs/data-types/tutorial).
	// The size of the values in this arbitrary key/value store is limited. If we find
	// that we want to save more data than this limit, we'll have to distribute the
	// data across multiple keys OR save it in a different way
	maxRedisValueSize = 5 * 1024 * 1025
)

var ErrorBufferTooLargeForRedisValue = errors.New("buffer is too large for redis value")

func (r *RedisStorage) Save(key string, buf []byte) error {
	if len(buf) > maxRedisValueSize {
		return ErrorBufferTooLargeForRedisValue
	}

	ctx := context.Background()
	redisKey := r.stateStorageKey(key)
	cmd := r.Client.Set(ctx, redisKey, buf, 0)
	if err := cmd.Err(); err != nil {
		return err
	}
	return nil
}

func (r *RedisStorage) Load(key string) (data []byte, ok bool, err error) {
	ctx := context.Background()
	redisKey := r.stateStorageKey(key)
	cmd := r.Client.Get(ctx, redisKey)
	buf, err := cmd.Bytes()
	if err != nil {
		return nil, false, err
	}
	return buf, true, nil
}
