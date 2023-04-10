package mem

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

	"github.com/argus-labs/cardinal/component"
	"github.com/argus-labs/cardinal/storage"
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
	store := storage.NewRedisStorage(rdb, worldId)
	x := storage.NewMockComponentType(SomeComp{}, SomeComp{Foo: 20})
	compStore := store.CompStore.Storage(x)

	err := compStore.PushComponent(x, 0)
	assert.NilError(t, err)
	err = compStore.PushComponent(x, 1)
	assert.NilError(t, err)

	compStore.MoveComponent(0, 0, 1)

	bz := compStore.Component(1, 1)
	foo, err := storage.DecodeComponent[SomeComp](bz)
	assert.NilError(t, err)
	assert.Equal(t, foo.Foo, 20)

	key := componentDataKey(worldId, x.ID(), 0)
	res := rdb.LRange(ctx, key, 0, -1)
	result, err := res.Result()
	assert.NilError(t, err)
	assert.Check(t, len(result) == 0)

	contains := compStore.Contains(1, 0)
	assert.Equal(t, contains, true)
}

func TestRedis_CompIndex(t *testing.T) {
	type SomeComp struct {
		Foo int
	}
	ctx := context.Background()
	_ = ctx
	rdb := getRedisClient(t)
	x := storage.NewMockComponentType(SomeComp{}, SomeComp{Foo: 20})
	worldId := "1"
	store := storage.NewRedisStorage(rdb, worldId)

	idxStore := store.CompStore.GetComponentIndexStorage(x)
	archIdx, compIdx := storage.ArchetypeIndex(0), storage.ComponentIndex(1)
	idxStore.SetIndex(archIdx, compIdx)
	gotIdx, ok := idxStore.ComponentIndex(archIdx)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx)
	idxStore.IncrementIndex(archIdx)

	gotIdx, ok = idxStore.ComponentIndex(archIdx)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx+1)

	idxStore.DecrementIndex(archIdx)

	gotIdx, ok = idxStore.ComponentIndex(archIdx)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx)

	compIdx = storage.ComponentIndex(25)
	idxStore.SetIndex(archIdx, compIdx)
	gotIdx, ok = idxStore.ComponentIndex(archIdx)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx)
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
