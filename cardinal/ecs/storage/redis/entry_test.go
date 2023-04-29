package redis

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"
)

func TestEntry_SetGet(t *testing.T) {
	store := getTestStorage(t)
	e := &types.Entry{
		ID: 15,
		Location: &types.Location{
			ArchetypeIndex: 1,
			ComponentIndex: 2,
			Valid:          true,
		},
	}
	err := store.SetEntry(e)
	assert.NilError(t, err)

	gotEnt, err := store.GetEntry(entity.ID(e.ID))
	assert.NilError(t, err)
	assert.DeepEqual(t, gotEnt, e, compareOpt)
}

func TestEntry_SetLocation(t *testing.T) {
	store := getTestStorage(t)
	e := &types.Entry{
		ID: 15,
		Location: &types.Location{
			ArchetypeIndex: 1,
			ComponentIndex: 2,
			Valid:          true,
		},
	}
	err := store.SetEntry(e)
	assert.NilError(t, err)

	e.Location = &types.Location{
		ArchetypeIndex: 325,
		ComponentIndex: 43920,
		Valid:          false,
	}
	err = store.SetLocation(entity.ID(e.ID), e.Location)
	assert.NilError(t, err)

	gotEntry, err := store.GetEntry(entity.ID(e.ID))
	assert.NilError(t, err)
	assert.DeepEqual(t, gotEntry, e, compareOpt)
}

func TestEntry_SetEntity(t *testing.T) {
	store := getTestStorage(t)
	e := &types.Entry{
		ID: 15,
		Location: &types.Location{
			ArchetypeIndex: 1,
			ComponentIndex: 2,
			Valid:          true,
		},
	}
	err := store.SetEntry(e)
	assert.NilError(t, err)

	newEntity := storage.Entity(40)
	err = store.SetEntity(entity.ID(e.ID), newEntity)
	assert.NilError(t, err)

	e.ID = uint64(newEntity)
	gotEntry, err := store.GetEntry(newEntity.ID())
	assert.NilError(t, err)
	assert.DeepEqual(t, gotEntry, e, compareOpt)
}
