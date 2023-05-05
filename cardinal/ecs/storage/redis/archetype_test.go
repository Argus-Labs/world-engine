package redis

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/types/known/anypb"
	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	testtypes "github.com/argus-labs/world-engine/cardinal/ecs/storage/testtypes/v1"
)

func TestArchetypeStorage_SetGet(t *testing.T) {
	reg := storage.NewTypeRegistry()
	store := getTestStorage(t)
	energy := &testtypes.EnergyComponent{Amount: 15, Cap: 40}
	ownable := &testtypes.OwnableComponent{Owner: "me"}
	layout := []component.IComponentType{energy, ownable}
	reg.Register(layout...)
	idx := storage.ArchetypeIndex(10)
	err := store.PushArchetype(idx, layout)
	assert.NilError(t, err)

	arch, err := store.Archetype(idx)
	assert.NilError(t, err)
	assert.Equal(t, arch.ArchetypeIndex, uint64(idx))
	gotLayout := make([]component.IComponentType, 0, len(layout))
	for _, anyComp := range arch.Components {
		comp, err := anypb.UnmarshalNew(anyComp, store.unmarshalOptions())
		assert.NilError(t, err)
		gotLayout = append(gotLayout, comp)
	}
	for i, comp := range gotLayout {
		assert.DeepEqual(t, comp, layout[i], cmpopts.IgnoreUnexported(testtypes.EnergyComponent{}, testtypes.OwnableComponent{}))
	}
}

func TestArchetypeStorage_PushRemoveEntity(t *testing.T) {
	reg := storage.NewTypeRegistry()
	store := getTestStorage(t)
	energy := &testtypes.EnergyComponent{Amount: 15, Cap: 40}
	layout := []component.IComponentType{energy}
	reg.Register(layout...)
	idx := storage.ArchetypeIndex(1)
	err := store.PushArchetype(idx, layout)
	assert.NilError(t, err)

	ent := entity.Entity(15)
	err = store.PushEntity(idx, ent)
	assert.NilError(t, err)

	arch, err := store.Archetype(idx)
	assert.NilError(t, err)
	assert.Equal(t, len(arch.EntityIds), 1)
	assert.Equal(t, arch.EntityIds[0], uint64(ent))

	err = store.RemoveEntity(idx, ent)
	assert.NilError(t, err)

	gotArch, err := store.Archetype(idx)
	assert.NilError(t, err)
	assert.Equal(t, len(gotArch.EntityIds), 0)

	err = store.PushEntity(idx, ent)
	assert.NilError(t, err)

	id, err := store.RemoveEntityAt(idx, 0)
	assert.NilError(t, err)
	assert.Equal(t, id, ent)

	gotArch, err = store.Archetype(idx)
	assert.NilError(t, err)
	assert.Equal(t, len(gotArch.EntityIds), 0)
}

func TestArchetypeStorage_GetNextArchetypeIndex(t *testing.T) {
	store := getTestStorage(t)
	idx, err := store.GetNextArchetypeIndex()
	assert.NilError(t, err)
	assert.Equal(t, idx, uint64(0))

	idx, err = store.GetNextArchetypeIndex()
	assert.NilError(t, err)
	assert.Equal(t, idx, uint64(1))
}

func getTestStorage(t *testing.T) *Storage {
	s := miniredis.RunT(t)
	store := NewStorage(Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}, "1")
	return &store
}
