package component_test

import (
	"pkg.world.dev/world-engine/cardinal/engine/storage"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/codec"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

func TestComponents(t *testing.T) {
	type ComponentData struct {
		ID string
	}
	var (
		ca = storage.NewMockComponentType(ComponentData{}, ComponentData{ID: "foo"})
		cb = storage.NewMockComponentType(ComponentData{}, ComponentData{ID: "bar"})
	)

	components := storage.NewComponents(storage.NewComponentsSliceStorage(), storage.NewComponentIndexMap())

	tests := []*struct {
		comps   []component.IComponentType
		archID  archetype.ID
		compIdx component.Index
		ID      string
	}{
		{
			[]component.IComponentType{ca},
			0,
			0,
			"a",
		},
		{
			[]component.IComponentType{ca, cb},
			1,
			1,
			"b",
		},
	}

	for _, tt := range tests {
		var err error
		tt.compIdx, err = components.PushComponents(tt.comps, tt.archID)
		assert.NilError(t, err)
	}

	for _, tt := range tests {
		for _, comp := range tt.comps {
			st := components.Storage(comp)
			ok, err := st.Contains(tt.archID, tt.compIdx)
			assert.NilError(t, err)
			if !ok {
				t.Errorf("storage should contain the component at %d, %d", tt.archID, tt.compIdx)
			}
			bz, _ := st.Component(tt.archID, tt.compIdx)
			dat, err := codec.Decode[ComponentData](bz)
			assert.NilError(t, err)
			dat.ID = tt.ID

			compBz, err := codec.Encode(dat)
			assert.NilError(t, err)

			err = st.SetComponent(tt.archID, tt.compIdx, compBz)
			assert.NilError(t, err)
		}
	}

	target := tests[0]
	storage := components.Storage(ca)

	srcArchIdx := target.archID
	var dstArchIdx archetype.ID = 1

	assert.NilError(t, storage.MoveComponent(srcArchIdx, target.compIdx, dstArchIdx))
	assert.NilError(t, components.Move(srcArchIdx, dstArchIdx))

	ok, err := storage.Contains(srcArchIdx, target.compIdx)
	if ok {
		t.Errorf("storage should not contain the component at %d, %d", target.archID, target.compIdx)
	}
	if idx, _, _ := components.ComponentIndices.ComponentIndex(srcArchIdx); idx != -1 {
		t.Errorf("component Index should be -1 at %d but %d", srcArchIdx, idx)
	}

	newCompIdx, _, _ := components.ComponentIndices.ComponentIndex(dstArchIdx)

	ok, err = storage.Contains(dstArchIdx, newCompIdx)
	if !ok {
		t.Errorf("storage should contain the component at %d, %d", dstArchIdx, target.compIdx)
	}

	bz, _ := storage.Component(dstArchIdx, newCompIdx)
	dat, err := codec.Decode[ComponentData](bz)
	assert.NilError(t, err)
	if dat.ID != target.ID {
		t.Errorf("component should have ID '%s', got ID '%s'", target.ID, dat.ID)
	}
}

func TestErrorWhenAccessingComponentNotOnEntity(t *testing.T) {
	world := ecs.NewTestWorld(t)
	foundComp := ecs.NewComponentType[string]("foundComp")
	notFoundComp := ecs.NewComponentType[string]("notFoundComp")

	assert.NilError(t, world.RegisterComponents(foundComp, notFoundComp))

	id, err := world.Create(foundComp)
	assert.NilError(t, err)
	_, err = notFoundComp.Get(world, id)
	assert.ErrorIs(t, err, storage.ErrorComponentNotOnEntity)
}

func TestMultipleCallsToCreateSupported(t *testing.T) {
	type ValueComponent struct {
		Val int
	}
	world := ecs.NewTestWorld(t)
	valComp := ecs.NewComponentType[ValueComponent]("ValueComponent")
	assert.NilError(t, world.RegisterComponents(valComp))

	id, err := world.Create(valComp)
	assert.NilError(t, err)

	assert.NilError(t, valComp.Set(world, id, ValueComponent{99}))

	val, err := valComp.Get(world, id)
	assert.NilError(t, err)
	assert.Equal(t, 99, val.Val)

	_, err = world.Create(valComp)

	val, err = valComp.Get(world, id)
	assert.NilError(t, err)
	assert.Equal(t, 99, val.Val)
}
