package redis

import (
	"context"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"
)

type Storage struct {
	worldID                string
	componentStoragePrefix component.TypeID
	Client                 *redis.Client
	Log                    zerolog.Logger
}

// Options makes DevEx cleaner by proxying the actual redis options struct
// With this, the developer doesn't need to import Redis libraries on their game logic implementation.
type Options struct {
	// host:port address.
	Addr string

	// Use the specified Username to authenticate the current connection
	// with one of the connections defined in the ACL list when connecting
	// to a Redis 6.0 instance, or greater, that is using the Redis ACL system.
	Username string

	// Optional password. Must match the password specified in the
	// requirepass server configuration option (if connecting to a Redis 5.0 instance, or lower),
	// or the User Password when connecting to a Redis 6.0 instance, or greater,
	// that is using the Redis ACL system.
	Password string

	// Database to be selected after connecting to the server.
	DB int
}

func NewStorage(options Options, worldID string) Storage {
	return Storage{
		worldID: worldID,
		Client: redis.NewClient(&redis.Options{
			Addr:     options.Addr,
			Username: options.Username,
			Password: options.Password,
			DB:       options.DB,
		}),
		Log: zerolog.New(os.Stdout),
	}
}

// ---------------------------------------------------------------------------
// 							COMPONENT INDEX STORAGE
// ---------------------------------------------------------------------------

var _ storage.ComponentIndexStorage = &Storage{}

func (r *Storage) GetComponentIndexStorage(cid component.TypeID) storage.ComponentIndexStorage {
	r.componentStoragePrefix = cid
	return r
}

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

func (r *Storage) GetComponentStorage(cid component.TypeID) storage.ComponentStorage {
	r.componentStoragePrefix = cid
	return r
}

// ---------------------------------------------------------------------------
// 							COMPONENT STORAGE
// ---------------------------------------------------------------------------

func (r *Storage) PushComponent(component component.IComponentType, index storage.ArchetypeIndex) error {
	ctx := context.Background()
	key := r.componentDataKey(index)
	componentBz, err := component.New()
	if err != nil {
		return err
	}
	res := r.Client.LPush(ctx, key, componentBz)
	return res.Err()
}

func (r *Storage) Component(archetypeIndex storage.ArchetypeIndex, componentIndex storage.ComponentIndex) ([]byte, error) {
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
	return bz, nil
}

func (r *Storage) SetComponent(archetypeIndex storage.ArchetypeIndex, componentIndex storage.ComponentIndex, compBz []byte) error {
	ctx := context.Background()
	key := r.componentDataKey(archetypeIndex)
	res := r.Client.LSet(ctx, key, int64(componentIndex), compBz)
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

// ---------------------------------------------------------------------------
// 							ARCHETYPE STORAGE
// ---------------------------------------------------------------------------

var _ storage.ArchetypeStorage = &Storage{}

func (r *Storage) PushArchetype(index storage.ArchetypeIndex, layout []component.IComponentType) {
	ctx := context.Background()
	a := &types.Archetype{
		ArchetypeIndex: uint64(index),
		EntityIds:      nil,
		Components:     []*anypb.Any{},
	}
	err := r.setArchetype(ctx, a)
	if err != nil {
		// TODO(technicallyty): handle
	}
}

func (r *Storage) Archetype(index storage.ArchetypeIndex) *types.Archetype {
	ctx := context.Background()
	key := r.archetypeStorageKey(index)
	res := r.Client.Get(ctx, key)
	// TODO(technicallyty): handle error
	if res.Err() != nil {

	}
	bz, err := res.Bytes()
	if err != nil {

	}
	a := new(types.Archetype)
	err = proto.Unmarshal(bz, a)
	if err != nil {

	}
	return a
}

func (r *Storage) RemoveEntity(index storage.ArchetypeIndex, entityIndex int) entity.Entity {
	arch := r.Archetype(index)
	removed := arch.EntityIds[entityIndex]
	length := len(arch.EntityIds)
	arch.EntityIds[entityIndex] = arch.EntityIds[length-1]
	arch.EntityIds = arch.EntityIds[:length-1]
	return entity.Entity(removed)
}

func (r *Storage) PushEntity(index storage.ArchetypeIndex, e entity.Entity) {
	ctx := context.Background()
	arch := r.Archetype(index)
	arch.EntityIds = append(arch.EntityIds, uint64(e.ID()))
	err := r.setArchetype(ctx, arch)
	if err != nil {
		// TODO(technicallyty): handle
	}
}

func (r *Storage) setArchetype(ctx context.Context, a *types.Archetype) error {
	key := r.archetypeStorageKey(storage.ArchetypeIndex(a.ArchetypeIndex))
	bz, err := proto.Marshal(a)
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
	loc := storage.NewLocation(index, componentIndex)
	bz, err := storage.Encode(loc)
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
	bz, err := storage.Encode(*location)
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
	loc, err := storage.Decode[types.Location](bz)
	if err != nil {
		return nil, err
	}
	return &loc, nil
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
// 							ENTRY STORAGE
// ---------------------------------------------------------------------------

var _ storage.EntryStorage = &Storage{}

func (r *Storage) SetEntry(id entity.ID, entry *types.Entry) error {
	ctx := context.Background()
	key := r.entryStorageKey(id)
	bz, err := storage.Encode(entry)
	if err != nil {
		return err
	}
	res := r.Client.Set(ctx, key, bz, 0)
	if err := res.Err(); err != nil {
		return err
	}
	return nil
}

func (r *Storage) GetEntry(id entity.ID) (*types.Entry, error) {
	ctx := context.Background()
	key := r.entryStorageKey(id)
	res := r.Client.Get(ctx, key)
	if err := res.Err(); err != nil {
		return nil, err
	}
	bz, err := res.Bytes()
	if err != nil {
		return nil, err
	}
	decodedEntry, err := storage.Decode[types.Entry](bz)
	if err != nil {
		return nil, err
	}
	return &decodedEntry, nil
}

func (r *Storage) SetEntity(id entity.ID, e storage.Entity) error {
	entry, err := r.GetEntry(id)
	if err != nil {
		return err
	}
	entry.ID = uint64(e.ID())
	err = r.SetEntry(id, entry)
	if err != nil {
		return err
	}

	return nil
}

func (r *Storage) SetLocation(id entity.ID, location *types.Location) error {
	entry, err := r.GetEntry(id)
	if err != nil {
		return err
	}
	entry.Location = location
	err = r.SetEntry(id, entry)
	if err != nil {
		return err
	}

	return nil
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

// ---------------------------------------------------------------------------
// 						Archetype Component Index
// ---------------------------------------------------------------------------

// TODO(technicallyty): impl

var _ storage.ArchetypeComponentIndex = &Storage{}

func (r *Storage) Push(layout *storage.Layout) {
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
