package storage_test

import (
	"fmt"
	"testing"

	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

func TestStorage_Bytes(t *testing.T) {
	type Component struct{ ID string }
	var (
		componentType = storage.NewMockComponentType[any](Component{}, Component{ID: "foo"})
	)

	store := storage.NewSliceStorage()

	tests := []struct {
		ID       string
		expected string
	}{
		{ID: "a", expected: "a"},
		{ID: "b", expected: "b"},
		{ID: "c", expected: "c"},
	}

	var archIdx storage.ArchetypeID = 0
	var compIdx storage.ComponentIndex = 0
	for _, test := range tests {
		err := store.PushComponent(componentType, archIdx)
		assert.NilError(t, err)
		bz, err := codec.Encode(Component{ID: test.ID})
		assert.NilError(t, err)
		fmt.Println(string(bz))
		store.SetComponent(archIdx, compIdx, bz)
		compIdx++
	}

	compIdx = 0
	for _, test := range tests {
		bz, _ := store.Component(archIdx, compIdx)
		c, err := codec.Decode[*Component](bz)
		assert.NilError(t, err)
		assert.Equal(t, c.ID, test.expected)
		compIdx++
	}

	removed, _ := store.SwapRemove(archIdx, 1)
	assert.Assert(t, removed != nil, "removed component should not be nil")
	comp, err := codec.Decode[Component](removed)
	assert.NilError(t, err)
	assert.Equal(t, comp.ID, "b", "removed component should have ID 'b'")

	tests2 := []struct {
		archIdx    storage.ArchetypeID
		cmpIdx     storage.ComponentIndex
		expectedID string
	}{
		{archIdx: 0, cmpIdx: 0, expectedID: "a"},
		{archIdx: 0, cmpIdx: 1, expectedID: "c"},
	}

	for _, test := range tests2 {
		compBz, _ := store.Component(test.archIdx, test.cmpIdx)
		comp, err := codec.Decode[Component](compBz)
		assert.NilError(t, err)
		assert.Equal(t, comp.ID, test.expectedID)
		compIdx++
	}
}
