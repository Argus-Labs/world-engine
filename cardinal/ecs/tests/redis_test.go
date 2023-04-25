package tests

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage/redis"
	types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
)

var _ encoding.BinaryMarshaler = Foo{}

const WorldId string = "1"

type Foo struct {
	X int `json:"X"`
	Y int `json:"Y"`
}

func (f Foo) MarshalBinary() (data []byte, err error) {
	return json.Marshal(f)
}

var componentDataKey = func(worldId string, compId component.TypeID, archIdx int) string {
	return fmt.Sprintf("WORLD-%s:CID-%d:A-%d", worldId, compId, archIdx)
}

func TestList(t *testing.T) {

	type SomeComp struct {
		Foo int
	}
	ctx := context.Background()

	rs := getRedisStorage(t)
	store := getWorldStorage(rs)
	x := storage.NewMockComponentType(SomeComp{}, SomeComp{Foo: 20})
	compStore := store.CompStore.Storage(x)

	err := compStore.PushComponent(x, 0)
	assert.NilError(t, err)
	err = compStore.PushComponent(x, 1)
	assert.NilError(t, err)

	err = compStore.MoveComponent(0, 0, 1)
	assert.NilError(t, err)

	bz, err := compStore.Component(1, 1)
	assert.NilError(t, err)
	foo, err := storage.Decode[SomeComp](bz)
	assert.NilError(t, err)
	assert.Equal(t, foo.Foo, 20)

	key := componentDataKey(WorldId, x.ID(), 0)
	res := rs.Client.LRange(ctx, key, 0, -1)
	result, err := res.Result()
	assert.NilError(t, err)
	assert.Check(t, len(result) == 0)

	contains, err := compStore.Contains(1, 0)
	assert.NilError(t, err)
	assert.Equal(t, contains, true)
}

func TestRedis_CompIndex(t *testing.T) {
	type SomeComp struct {
		Foo int
	}
	x := storage.NewMockComponentType(SomeComp{}, SomeComp{Foo: 20})

	rs := getRedisStorage(t)
	store := getWorldStorage(rs)

	idxStore := store.CompStore.GetComponentIndexStorage(x)
	archIdx, compIdx := storage.ArchetypeIndex(0), storage.ComponentIndex(1)
	err := idxStore.SetIndex(archIdx, compIdx)
	assert.NilError(t, err)
	gotIdx, ok, err := idxStore.ComponentIndex(archIdx)
	assert.NilError(t, err)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx)
	err = idxStore.IncrementIndex(archIdx)
	assert.NilError(t, err)

	gotIdx, ok, err = idxStore.ComponentIndex(archIdx)
	assert.NilError(t, err)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx+1)

	err = idxStore.DecrementIndex(archIdx)
	assert.NilError(t, err)

	gotIdx, ok, err = idxStore.ComponentIndex(archIdx)
	assert.NilError(t, err)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx)

	compIdx = storage.ComponentIndex(25)
	err = idxStore.SetIndex(archIdx, compIdx)
	assert.NilError(t, err)
	gotIdx, ok, err = idxStore.ComponentIndex(archIdx)
	assert.NilError(t, err)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx)
}

func TestRedis_Location(t *testing.T) {
	rs := getRedisStorage(t)
	store := getWorldStorage(rs)

	loc := storage.NewLocation(0, 1)
	eid := entity.ID(3)
	err := store.EntityLocStore.Set(eid, loc)
	assert.NilError(t, err)

	gotLoc, err := store.EntityLocStore.Location(eid)
	assert.NilError(t, err)
	diff := cmp.Diff(loc, gotLoc, protocmp.Transform())
	assert.Equal(t, len(diff), 0, diff)

	aid, err := store.EntityLocStore.ArchetypeIndex(eid)
	assert.NilError(t, err)
	assert.Equal(t, loc.ArchetypeIndex, aid)

	contains, err := store.EntityLocStore.ContainsEntity(eid)
	assert.NilError(t, err)
	assert.Equal(t, contains, true)

	notContains, err := store.EntityLocStore.ContainsEntity(entity.ID(420))
	assert.NilError(t, err)
	assert.Equal(t, notContains, false)

	compIdx, err := store.EntityLocStore.ComponentIndexForEntity(eid)
	assert.NilError(t, err)
	assert.Equal(t, loc.ComponentIndex, compIdx)

	newEID := entity.ID(40)
	archIdx2, compIdx2 := storage.ArchetypeIndex(10), storage.ComponentIndex(15)
	err = store.EntityLocStore.Insert(newEID, archIdx2, compIdx2)
	assert.NilError(t, err)

	newLoc, err := store.EntityLocStore.Location(newEID)
	assert.NilError(t, err)
	assert.Equal(t, newLoc.ArchetypeIndex, archIdx2)
	assert.Equal(t, newLoc.ComponentIndex, compIdx2)

	err = store.EntityLocStore.Remove(newEID)
	assert.NilError(t, err)

	has, err := store.EntityLocStore.ContainsEntity(newEID)
	assert.NilError(t, err)
	assert.Equal(t, has, false)
}

func TestRedis_EntryStorage(t *testing.T) {
	rs := getRedisStorage(t)
	store := getWorldStorage(rs)

	eid := entity.ID(12)
	loc := &types.Location{
		ArchetypeIndex: 15,
		ComponentIndex: 12,
		Valid:          true,
	}
	e := storage.NewEntry(eid, loc)
	err := store.EntryStore.SetEntry(eid, e)
	assert.NilError(t, err)

	gotEntry, err := store.EntryStore.GetEntry(eid)
	assert.NilError(t, err)
	diff := cmp.Diff(e, gotEntry, protocmp.Transform())
	assert.Equal(t, len(diff), 0, diff)

	newLoc := &types.Location{
		ArchetypeIndex: 39,
		ComponentIndex: 82,
		Valid:          false,
	}
	err = store.EntryStore.SetLocation(eid, newLoc)
	assert.NilError(t, err)

	gotEntry, err = store.EntryStore.GetEntry(eid)
	assert.NilError(t, err)

	diff = cmp.Diff(gotEntry.Location, newLoc, protocmp.Transform())
	assert.Equal(t, len(diff), 0, diff)

	newEnt := entity.NewEntity(400)
	err = store.EntryStore.SetEntity(eid, newEnt)
	assert.NilError(t, err)
	gotEntry, err = store.EntryStore.GetEntry(eid)
	assert.NilError(t, err)
	assert.Equal(t, gotEntry.ID, uint64(newEnt.ID()))
}

func getRedisStorage(t *testing.T) *redis.Storage {
	s := miniredis.RunT(t)
	rs := redis.NewStorage(redis.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}, WorldId)

	return &rs
}

func getWorldStorage(r *redis.Storage) *storage.WorldStorage {
	return &storage.WorldStorage{
		CompStore:        storage.Components{Store: r, ComponentIndices: r},
		EntityLocStore:   r,
		ArchCompIdxStore: r,
		ArchAccessor:     r,
		EntryStore:       r,
		EntityMgr:        r,
	}
}
