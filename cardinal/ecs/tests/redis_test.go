package tests

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/sign"

	"pkg.world.dev/world-engine/cardinal/ecs/storage"

	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

var _ encoding.BinaryMarshaler = Foo{}

type Foo struct {
	X int `json:"X"`
	Y int `json:"Y"`
}

func (f Foo) MarshalBinary() (data []byte, err error) {
	return json.Marshal(f)
}

var componentDataKey = func(worldId string, compId component.TypeID, archID int) string {
	return fmt.Sprintf("WORLD-%s:CID-%d:A-%d", worldId, compId, archID)
}

func TestList(t *testing.T) {
	type SomeComp struct {
		Foo int
	}
	ctx := context.Background()

	rs := getRedisStorage(t)
	store := storage.NewWorldStorage(&rs)
	x := storage.NewMockComponentType(SomeComp{}, SomeComp{Foo: 20})
	compStore := store.CompStore.Storage(x)

	err := compStore.PushComponent(x, 0)
	assert.NilError(t, err)
	err = compStore.PushComponent(x, 1)
	assert.NilError(t, err)

	err = compStore.MoveComponent(0, 0, 1)
	assert.NilError(t, err)

	bz, _ := compStore.Component(1, 1)
	foo, err := storage.Decode[SomeComp](bz)
	assert.NilError(t, err)
	assert.Equal(t, foo.Foo, 20)

	key := componentDataKey(WorldId, x.ID(), 0)
	res := rs.Client.LRange(ctx, key, 0, -1)
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
	x := storage.NewMockComponentType(SomeComp{}, SomeComp{Foo: 20})

	rs := getRedisStorage(t)
	store := storage.NewWorldStorage(&rs)

	idxStore := store.CompStore.GetComponentIndexStorage(x)
	archID, compIdx := storage.ArchetypeID(0), storage.ComponentIndex(1)
	assert.NilError(t, idxStore.SetIndex(archID, compIdx))
	gotIdx, ok, err := idxStore.ComponentIndex(archID)
	assert.NilError(t, err)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx)
	_, err = idxStore.IncrementIndex(archID)
	assert.NilError(t, err)

	gotIdx, ok, err = idxStore.ComponentIndex(archID)
	assert.NilError(t, err)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx+1)

	idxStore.DecrementIndex(archID)

	gotIdx, ok, err = idxStore.ComponentIndex(archID)
	assert.NilError(t, err)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx)

	compIdx = storage.ComponentIndex(25)
	idxStore.SetIndex(archID, compIdx)
	gotIdx, ok, err = idxStore.ComponentIndex(archID)
	assert.NilError(t, err)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx)
}

func TestRedis_Location(t *testing.T) {
	//ctx := context.Background()
	rs := getRedisStorage(t)
	store := storage.NewWorldStorage(&rs)

	loc := storage.NewLocation(0, 1)
	eid := storage.EntityID(3)
	store.EntityLocStore.SetLocation(eid, loc)
	gotLoc, _ := store.EntityLocStore.GetLocation(eid)
	assert.Equal(t, loc, gotLoc)

	aid, _ := store.EntityLocStore.ArchetypeID(eid)
	assert.Equal(t, loc.ArchID, aid)

	contains, _ := store.EntityLocStore.ContainsEntity(eid)
	assert.Equal(t, contains, true)

	notContains, _ := store.EntityLocStore.ContainsEntity(storage.EntityID(420))
	assert.Equal(t, notContains, false)

	compIdx, _ := store.EntityLocStore.ComponentIndexForEntity(eid)
	assert.Equal(t, loc.CompIndex, compIdx)

	newEID := storage.EntityID(40)
	archID2, compIdx2 := storage.ArchetypeID(10), storage.ComponentIndex(15)
	store.EntityLocStore.Insert(newEID, archID2, compIdx2)

	newLoc, _ := store.EntityLocStore.GetLocation(newEID)
	assert.Equal(t, newLoc.ArchID, archID2)
	assert.Equal(t, newLoc.CompIndex, compIdx2)

	assert.NilError(t, store.EntityLocStore.Remove(newEID))

	has, _ := store.EntityLocStore.ContainsEntity(newEID)
	assert.Equal(t, has, false)
}

func TestCanSaveAndRecoverArbitraryData(t *testing.T) {
	rs := getRedisStorage(t)
	type SomeData struct {
		One   string
		Two   int
		Three float64
		Map   map[string]int
	}

	wantData := &SomeData{
		One:   "hello",
		Two:   100,
		Three: 3.1415,
		Map: map[string]int{
			"alpha": 100,
			"beta":  200,
			"gamma": 300,
		},
	}
	buf, err := storage.Encode(wantData)
	assert.NilError(t, err)

	const key = "foobar"
	err = rs.Save(key, buf)
	assert.NilError(t, err)

	gotBytes, ok, err := rs.Load(key)
	assert.Equal(t, true, ok)
	assert.NilError(t, err)

	gotData, err := storage.Decode[*SomeData](gotBytes)
	assert.NilError(t, err)
	assert.DeepEqual(t, gotData, wantData)
}

func TestCanSaveAndRecoverSignatures(t *testing.T) {
	rs := getRedisStorage(t)
	type TxIn struct {
		Str string
	}
	type TxOut struct {
		Str string
	}

	tx := ecs.NewTransactionType[TxIn, TxOut]("tx_a")
	tx.SetID(55)

	key, err := crypto.GenerateKey()
	assert.NilError(t, err)

	wantVal := TxIn{"the_data"}
	personaTag := "xyzzy"
	wantSig, err := sign.NewSignedPayload(key, personaTag, "namespace", 66, wantVal)
	assert.NilError(t, err)
	wantTxID := transaction.TxID{personaTag, 100}

	queue := transaction.TxMap{
		tx.ID(): []transaction.TxAny{
			{
				Value: wantVal,
				ID:    wantTxID,
				Sig:   wantSig,
			},
		},
	}

	txSlice := []transaction.ITransaction{tx}

	rs.StartNextTick(txSlice, queue)

	gotQueue, err := rs.Recover(txSlice)
	assert.NilError(t, err)

	assert.Equal(t, 1, len(gotQueue))
	slice, ok := gotQueue[tx.ID()]
	assert.Check(t, ok)
	assert.Equal(t, 1, len(slice))
	assert.Equal(t, wantTxID, slice[0].ID)
	gotSig := slice[0].Sig
	assert.DeepEqual(t, wantSig, gotSig)

	gotVal, ok := slice[0].Value.(TxIn)
	assert.Check(t, ok)
	assert.Equal(t, wantVal, gotVal)

}

func TestLargeArbitraryDataProducesError(t *testing.T) {
	rs := getRedisStorage(t)
	// Make a 6 Mb slice. This should not fit in a redis bucket
	largePayload := make([]byte, 6*1024*1024)
	err := rs.Save("foobar", largePayload)
	assert.ErrorIs(t, err, storage.ErrorBufferTooLargeForRedisValue)
}

func TestGettingIndexStorageShouldNotImpactIncrement(t *testing.T) {
	rs := getRedisStorage(t)

	archID := storage.ArchetypeID(99)

	err := rs.SetIndex(archID, 0)
	assert.NilError(t, err)

	compIndex, err := rs.IncrementIndex(archID)
	assert.NilError(t, err)
	assert.Equal(t, storage.ComponentIndex(1), compIndex)

	compIndex, err = rs.IncrementIndex(archID)
	assert.NilError(t, err)
	assert.Equal(t, storage.ComponentIndex(2), compIndex)

	// Get the component index storage for some random component type.
	// This should have no impact on incrementing the index of archID
	_ = rs.GetComponentIndexStorage(component.TypeID(100))

	compIndex, err = rs.IncrementIndex(archID)
	assert.NilError(t, err)
	assert.Equal(t, storage.ComponentIndex(3), compIndex)
}
