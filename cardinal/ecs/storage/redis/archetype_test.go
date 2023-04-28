package redis

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	testtypes "github.com/argus-labs/world-engine/cardinal/ecs/storage/testtypes/v1"
)

func TestPushArchetype(t *testing.T) {
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
}

func getTestStorage(t *testing.T) Storage {
	s := miniredis.RunT(t)
	return NewStorage(Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}, "1")
}
