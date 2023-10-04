package storage_test

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	"pkg.world.dev/world-engine/cardinal/ecs/component_types"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/internal/testutil"
	"pkg.world.dev/world-engine/cardinal/ecs/itransaction"

	"github.com/ethereum/go-ethereum/crypto"

	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/sign"
)

var _ encoding.BinaryMarshaler = Foo{}

type Foo struct {
	X int `json:"X"`
	Y int `json:"Y"`
}

func (f Foo) MarshalBinary() (data []byte, err error) {
	return json.Marshal(f)
}

var componentDataKey = func(worldId string, compId component_types.TypeID, archID int) string {
	return fmt.Sprintf("WORLD-%s:CID-%d:A-%d", worldId, compId, archID)
}

func TestList(t *testing.T) {
	type SomeComp struct {
		Foo int
	}
	ctx := context.Background()

	rs := testutil.GetRedisStorage(t)
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
	foo, err := codec.Decode[SomeComp](bz)
	assert.NilError(t, err)
	assert.Equal(t, foo.Foo, 20)

	key := componentDataKey(testutil.WorldId, x.ID(), 0)
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

	rs := testutil.GetRedisStorage(t)
	store := storage.NewWorldStorage(&rs)

	idxStore := store.CompStore.GetComponentIndexStorage(x)
	archID, compIdx := archetype.ID(0), component_types.Index(1)
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

	compIdx = component_types.Index(25)
	idxStore.SetIndex(archID, compIdx)
	gotIdx, ok, err = idxStore.ComponentIndex(archID)
	assert.NilError(t, err)
	assert.Check(t, ok == true)
	assert.Check(t, gotIdx == compIdx)
}

func TestRedis_Location(t *testing.T) {
	//ctx := context.Background()
	rs := testutil.GetRedisStorage(t)
	store := storage.NewWorldStorage(&rs)

	loc := entity.NewLocation(0, 1)
	eid := entity.ID(3)
	store.EntityLocStore.SetLocation(eid, loc)
	gotLoc, _ := store.EntityLocStore.GetLocation(eid)
	assert.Equal(t, loc, gotLoc)

	aid, _ := store.EntityLocStore.ArchetypeID(eid)
	assert.Equal(t, loc.ArchID, aid)

	contains, _ := store.EntityLocStore.ContainsEntity(eid)
	assert.Equal(t, contains, true)

	notContains, _ := store.EntityLocStore.ContainsEntity(entity.ID(420))
	assert.Equal(t, notContains, false)

	compIdx, _ := store.EntityLocStore.ComponentIndexForEntity(eid)
	assert.Equal(t, loc.CompIndex, compIdx)

	newEID := entity.ID(40)
	archID2, compIdx2 := archetype.ID(10), component_types.Index(15)
	store.EntityLocStore.Insert(newEID, archID2, compIdx2)

	newLoc, _ := store.EntityLocStore.GetLocation(newEID)
	assert.Equal(t, newLoc.ArchID, archID2)
	assert.Equal(t, newLoc.CompIndex, compIdx2)

	assert.NilError(t, store.EntityLocStore.Remove(newEID))

	has, _ := store.EntityLocStore.ContainsEntity(newEID)
	assert.Equal(t, has, false)
}

func TestCanSaveAndRecoverArbitraryData(t *testing.T) {
	rs := testutil.GetRedisStorage(t)
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
	buf, err := codec.Encode(wantData)
	assert.NilError(t, err)

	const key = "foobar"
	err = rs.Save(key, buf)
	assert.NilError(t, err)

	gotBytes, ok, err := rs.Load(key)
	assert.Equal(t, true, ok)
	assert.NilError(t, err)

	gotData, err := codec.Decode[*SomeData](gotBytes)
	assert.NilError(t, err)
	assert.DeepEqual(t, gotData, wantData)
}

func TestMiniRedisCopy(t *testing.T) {
	rs := testutil.GetRedisStorage(t)
	ctx := context.Background()
	rs.Client.LPush(ctx, "testing", "original")
	rs.Client.Copy(ctx, "testing", "testing2", 0, true)
	rs.Client.LSet(ctx, "testing", 0, "changed")
	x := rs.Client.LRange(ctx, "testing", 0, 0)
	y := rs.Client.LRange(ctx, "testing2", 0, 0)
	assert.Assert(t, x.Val()[0] != y.Val()[0])
}

func TestCanSaveAndRecoverSignatures(t *testing.T) {
	rs := testutil.GetRedisStorage(t)
	type TxIn struct {
		Str string
	}
	type TxOut struct {
		Str string
	}

	tx := transaction.NewTransactionType[TxIn, TxOut]("tx_a")
	tx.SetID(55)

	key, err := crypto.GenerateKey()
	assert.NilError(t, err)

	wantVal := TxIn{"the_data"}
	personaTag := "xyzzy"
	wantSig, err := sign.NewSignedPayload(key, personaTag, "namespace", 66, wantVal)
	assert.NilError(t, err)
	wantTxHash := itransaction.TxHash(wantSig.HashHex())

	queue := transaction.NewTxQueue()
	queue.AddTransaction(tx.ID(), wantVal, wantSig)

	txSlice := []itransaction.ITransaction{tx}

	rs.StartNextTick(txSlice, queue)

	gotQueue, err := rs.Recover(txSlice)
	assert.NilError(t, err)

	slice := gotQueue.ForID(tx.ID())
	assert.Equal(t, 1, len(slice))
	assert.Equal(t, wantTxHash, slice[0].TxHash)
	gotSig := slice[0].Sig
	assert.DeepEqual(t, wantSig, gotSig)

	gotVal, ok := slice[0].Value.(TxIn)
	assert.Check(t, ok)
	assert.Equal(t, wantVal, gotVal)

}

func TestLargeArbitraryDataProducesError(t *testing.T) {
	rs := testutil.GetRedisStorage(t)
	// Make a 6 Mb slice. This should not fit in a redis bucket
	largePayload := make([]byte, 6*1024*1024)
	err := rs.Save("foobar", largePayload)
	assert.ErrorIs(t, err, storage.ErrorBufferTooLargeForRedisValue)
}

func TestGettingIndexStorageShouldNotImpactIncrement(t *testing.T) {
	rs := testutil.GetRedisStorage(t)

	archID := archetype.ID(99)

	err := rs.SetIndex(archID, 0)
	assert.NilError(t, err)

	compIndex, err := rs.IncrementIndex(archID)
	assert.NilError(t, err)
	assert.Equal(t, component_types.Index(1), compIndex)

	compIndex, err = rs.IncrementIndex(archID)
	assert.NilError(t, err)
	assert.Equal(t, component_types.Index(2), compIndex)

	// Get the component index storage for some random component type.
	// This should have no impact on incrementing the index of archID
	_ = rs.GetComponentIndexStorage(component_types.TypeID(100))

	compIndex, err = rs.IncrementIndex(archID)
	assert.NilError(t, err)
	assert.Equal(t, component_types.Index(3), compIndex)
}
