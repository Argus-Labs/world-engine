package redis

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/redis/go-redis/v9"
	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	testtypes "github.com/argus-labs/world-engine/cardinal/ecs/storage/testtypes/v1"
	types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"
)

var (
	compareOpt = cmpopts.IgnoreUnexported(testtypes.EnergyComponent{}, testtypes.OwnableComponent{}, types.Entry{}, types.Location{})
)

func TestComponents_PushGet(t *testing.T) {
	store := getTestStorage(t)
	energy1 := &testtypes.EnergyComponent{
		Amount: 150,
		Cap:    30_000,
	}
	ai := storage.ArchetypeIndex(0)
	compIdx1, err := store.PushComponent(energy1, ai)
	assert.NilError(t, err)
	assert.Equal(t, compIdx1, storage.ComponentIndex(0))

	energy2 := &testtypes.EnergyComponent{
		Amount: 350,
		Cap:    40_000,
	}
	compIdx2, err := store.PushComponent(energy2, ai)
	assert.NilError(t, err)
	assert.Equal(t, compIdx2, storage.ComponentIndex(1))

	gotComp1, err := store.Component(ai, compIdx1)
	assert.NilError(t, err)
	gotEnergy1 := gotComp1.(*testtypes.EnergyComponent)
	assert.DeepEqual(t, gotEnergy1, energy1, compareOpt)

	gotComp2, err := store.Component(ai, compIdx2)
	assert.NilError(t, err)
	gotEnergy2 := gotComp2.(*testtypes.EnergyComponent)
	assert.DeepEqual(t, gotEnergy2, energy2, compareOpt)
}

func TestComponents_Set(t *testing.T) {
	store := getTestStorage(t)
	ai := storage.ArchetypeIndex(0)
	energy := &testtypes.EnergyComponent{Amount: 40, Cap: 400}
	ci, err := store.PushComponent(energy, ai)
	assert.NilError(t, err)

	comp, err := store.Component(ai, ci)
	assert.NilError(t, err)
	gotEnergy := comp.(*testtypes.EnergyComponent)
	assert.DeepEqual(t, gotEnergy, energy, compareOpt)
}

func TestComponent_Move(t *testing.T) {
	store := getTestStorage(t)
	src, dst := storage.ArchetypeIndex(0), storage.ArchetypeIndex(1)

	energy := &testtypes.EnergyComponent{Amount: 150, Cap: 40000}
	compIdx, err := store.PushComponent(energy, src)
	assert.NilError(t, err)

	err = store.MoveComponent(src, compIdx, dst)
	assert.NilError(t, err)

	gotComp, err := store.Component(dst, compIdx)
	assert.NilError(t, err)
	gotEnergy := gotComp.(*testtypes.EnergyComponent)

	assert.DeepEqual(t, gotEnergy, energy, compareOpt)
}

func TestRemoveComponent(t *testing.T) {
	store := getTestStorage(t)
	ai := storage.ArchetypeIndex(0)
	energy := &testtypes.EnergyComponent{Amount: 40, Cap: 400}

	compIdx, err := store.PushComponent(energy, ai)
	assert.NilError(t, err)

	err = store.RemoveComponent(ai, compIdx)
	assert.NilError(t, err)

	_, err = store.Component(ai, compIdx)
	assert.Error(t, err, redis.Nil.Error())
}

func TestIdx(t *testing.T) {
	ctx := context.Background()
	store := getTestStorage(t)
	ai := storage.ArchetypeIndex(15)
	key := store.componentIndexKey(ai)

	gg := store.Client.Get(ctx, key)
	num, err := gg.Uint64()
	fmt.Println(err)
	fmt.Println(num)

	res := store.Client.Incr(ctx, key)
	fmt.Println(res.Err())
	gg = store.Client.Get(ctx, key)
	fmt.Println(gg.Uint64())
}
