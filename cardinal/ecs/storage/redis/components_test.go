package redis

import (
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
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
	compIdx := storage.ComponentIndex(0)
	setStorePrefix(store, energy1)
	err := store.PushComponent(energy1, ai, compIdx)
	assert.NilError(t, err)

	ownable := &testtypes.OwnableComponent{
		Owner: "steve",
	}
	setStorePrefix(store, ownable)
	err = store.PushComponent(ownable, ai, compIdx)
	assert.NilError(t, err)

	setStorePrefix(store, energy1)
	gotComp1, err := store.Component(ai, compIdx)
	assert.NilError(t, err)
	gotEnergy1 := gotComp1.(*testtypes.EnergyComponent)
	assert.DeepEqual(t, gotEnergy1, energy1, compareOpt)

	setStorePrefix(store, ownable)
	gotComp2, err := store.Component(ai, compIdx)
	assert.NilError(t, err)
	gotOwnable := gotComp2.(*testtypes.OwnableComponent)
	assert.DeepEqual(t, gotOwnable, ownable, compareOpt)
}

func TestComponents_Set(t *testing.T) {
	store := getTestStorage(t)
	ai := storage.ArchetypeIndex(0)
	ci := storage.ComponentIndex(0)
	energy := &testtypes.EnergyComponent{Amount: 40, Cap: 400}
	setStorePrefix(store, energy)
	err := store.PushComponent(energy, ai, ci)
	assert.NilError(t, err)

	energy.Amount = 500
	store.SetComponent(ai, ci, energy)

	comp, err := store.Component(ai, ci)
	assert.NilError(t, err)
	gotEnergy := comp.(*testtypes.EnergyComponent)
	assert.DeepEqual(t, gotEnergy, energy, compareOpt)
}

func TestComponent_Move(t *testing.T) {
	store := getTestStorage(t)
	src, dst := storage.ArchetypeIndex(0), storage.ArchetypeIndex(1)
	compIdx := storage.ComponentIndex(0)

	energy := &testtypes.EnergyComponent{Amount: 150, Cap: 40000}
	setStorePrefix(store, energy)
	err := store.PushComponent(energy, src, compIdx)
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
	ci := storage.ComponentIndex(0)
	energy := &testtypes.EnergyComponent{Amount: 40, Cap: 400}

	err := store.PushComponent(energy, ai, ci)
	assert.NilError(t, err)

	err = store.RemoveComponent(ai, ci)
	assert.NilError(t, err)

	_, err = store.Component(ai, ci)
	assert.Error(t, err, redis.Nil.Error())
}

func setStorePrefix(store *Storage, msg proto.Message) {
	store.componentStoragePrefix = string(msg.ProtoReflect().Descriptor().FullName())
}
