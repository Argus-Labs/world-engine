package storage

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
)

var _ encoding.BinaryMarshaler = Foo{}

type Foo struct {
	X int `json:"X"`
	Y int `json:"Y"`
}

func (f Foo) MarshalBinary() (data []byte, err error) {
	return json.Marshal(f)
}

func TestRedis(t *testing.T) {
	ctx := context.Background()

	rdb := getRedisClient(t)

	foo := &Foo{
		X: 35,
		Y: 40,
	}
	key := "foo"
	err := rdb.Set(ctx, key, foo, time.Duration(0)).Err()
	assert.NilError(t, err)

	cmd := rdb.Get(ctx, key)
	if err := cmd.Err(); err != nil {
		t.Fatal(err)
	}

	bz, err := cmd.Bytes()
	assert.NilError(t, err)

	var f Foo
	err = json.Unmarshal(bz, &f)
	assert.NilError(t, err)
	assert.Equal(t, f.X, foo.X)
	assert.Equal(t, f.Y, foo.Y)

	ss := rdb.Get(ctx, "fooiasjdflkasdjf")
	if ss.Err() != nil {
		fmt.Println("error!")
	}
}

var componentDataKey = func(worldId string, compId component.TypeID, archIdx int) string {
	return fmt.Sprintf("WORLD-%s:CID-%d:A-%d", worldId, compId, archIdx)
}

func TestList(t *testing.T) {

	type SomeComp struct {
		Foo int
	}
	ctx := context.Background()
	rdb := getRedisClient(t)
	worldId := "1"
	store := NewRedisStorage(rdb, worldId)
	x := NewMockComponentType(SomeComp{}, SomeComp{Foo: 20})
	compStore := store.CompStore.Storage(x)

	err := compStore.PushComponent(x, 0)
	assert.NilError(t, err)
	err = compStore.PushComponent(x, 1)
	assert.NilError(t, err)

	compStore.MoveComponent(0, 0, 1)

	bz, _ := compStore.Component(1, 1)
	foo, err := Decode[SomeComp](bz)
	assert.NilError(t, err)
	assert.Equal(t, foo.Foo, 20)

	key := componentDataKey(worldId, x.ID(), 0)
	res := rdb.LRange(ctx, key, 0, -1)
	result, err := res.Result()
	assert.NilError(t, err)
	assert.Check(t, len(result) == 0)

	contains, _ := compStore.Contains(1, 0)
	assert.Equal(t, contains, true)
}

func TestRedis_CompIndex(t *testing.T) {
	type SomeComp struct {
		Foo int
	}
	ctx := context.Background()
	_ = ctx
	rdb := getRedisClient(t)
	x := NewMockComponentType(SomeComp{}, SomeComp{Foo: 20})
	worldId := "1"
	store := NewRedisStorage(rdb, worldId)

	idxStore := store.CompStore.GetComponentIndexStorage(x)
	archIdx, compIdx := ArchetypeIndex(0), ComponentIndex(1)
	idxStore.SetIndex(archIdx, compIdx)
	gotIdx, ok, err := idxStore.ComponentIndex(archIdx)
	assert.NilError(t, err)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx)
	idxStore.IncrementIndex(archIdx)

	gotIdx, ok, err = idxStore.ComponentIndex(archIdx)
	assert.NilError(t, err)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx+1)

	idxStore.DecrementIndex(archIdx)

	gotIdx, ok, err = idxStore.ComponentIndex(archIdx)
	assert.NilError(t, err)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx)

	compIdx = ComponentIndex(25)
	idxStore.SetIndex(archIdx, compIdx)
	gotIdx, ok, err = idxStore.ComponentIndex(archIdx)
	assert.NilError(t, err)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx)
}

func TestRedis_Location(t *testing.T) {
	//ctx := context.Background()
	rdb := getRedisClient(t)
	worldId := "1"
	store := NewRedisStorage(rdb, worldId)
	loc := NewLocation(0, 1)
	eid := entity.ID(3)
	store.EntityLocStore.Set(eid, loc)
	gotLoc, _ := store.EntityLocStore.Location(eid)
	assert.Equal(t, *loc, *gotLoc)

	aid := store.EntityLocStore.ArchetypeIndex(eid)
	assert.Equal(t, loc.ArchIndex, aid)

	contains, _ := store.EntityLocStore.ContainsEntity(eid)
	assert.Equal(t, contains, true)

	notContains, _ := store.EntityLocStore.ContainsEntity(entity.ID(420))
	assert.Equal(t, notContains, false)

	compIdx := store.EntityLocStore.ComponentIndexForEntity(eid)
	assert.Equal(t, loc.CompIndex, compIdx)

	newEID := entity.ID(40)
	archIdx2, compIdx2 := ArchetypeIndex(10), ComponentIndex(15)
	store.EntityLocStore.Insert(newEID, archIdx2, compIdx2)

	newLoc, _ := store.EntityLocStore.Location(newEID)
	assert.Equal(t, newLoc.ArchIndex, archIdx2)
	assert.Equal(t, newLoc.CompIndex, compIdx2)

	store.EntityLocStore.Remove(newEID)

	has, _ := store.EntityLocStore.ContainsEntity(newEID)
	assert.Equal(t, has, false)
}

func TestRedis_EntryStorage(t *testing.T) {
	ctx := context.Background()
	_ = ctx
	rdb := getRedisClient(t)
	worldId := "1"
	store := NewRedisStorage(rdb, worldId)

	eid := entity.ID(12)
	loc := &Location{
		ArchIndex: 15,
		CompIndex: 12,
		Valid:     true,
	}
	e := NewEntry(eid, entity.NewEntity(eid), loc)
	err := store.EntryStore.SetEntry(eid, e)
	assert.NilError(t, err)

	gotEntry, _ := store.EntryStore.GetEntry(eid)
	assert.DeepEqual(t, e, gotEntry)

	newLoc := Location{
		ArchIndex: 39,
		CompIndex: 82,
		Valid:     false,
	}
	store.EntryStore.SetLocation(eid, newLoc)

	gotEntry, _ = store.EntryStore.GetEntry(eid)
	assert.DeepEqual(t, *gotEntry.Loc, newLoc)

	newEnt := entity.NewEntity(400)
	store.EntryStore.SetEntity(eid, newEnt)
	gotEntry, _ = store.EntryStore.GetEntry(eid)
	assert.DeepEqual(t, gotEntry.Ent, newEnt)
}

func getRedisClient(t *testing.T) *redis.Client {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	return rdb
}
