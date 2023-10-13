package component_test

import (
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/codec"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	storage2 "pkg.world.dev/world-engine/cardinal/ecs/storage"
)

func TestComponents(t *testing.T) {
	type ComponentData struct {
		ID string
	}
	var (
		ca = storage2.NewMockComponentType(ComponentData{}, ComponentData{ID: "foo"})
		cb = storage2.NewMockComponentType(ComponentData{}, ComponentData{ID: "bar"})
	)

	components := storage2.NewComponents(storage2.NewComponentsSliceStorage(), storage2.NewComponentIndexMap())

	tests := []*struct {
		comps   []component.IComponentMetaData
		archID  archetype.ID
		compIdx component.Index
		ID      string
	}{
		{
			[]component.IComponentMetaData{ca},
			0,
			0,
			"a",
		},
		{
			[]component.IComponentMetaData{ca, cb},
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

type foundComp struct{}
type notFoundComp struct{}

func (_ foundComp) Name() string {
	return "foundComp"
}

func (_ notFoundComp) Name() string {
	return "notFoundComp"
}

func TestErrorWhenAccessingComponentNotOnEntity(t *testing.T) {
	world := ecs.NewTestWorld(t)
	assert.NilError(t, ecs.RegisterComponent[foundComp](world))
	assert.NilError(t, ecs.RegisterComponent[notFoundComp](world))

	id, err := ecs.Create(world, foundComp{})
	assert.NilError(t, err)
	_, err = ecs.GetComponent[notFoundComp](world, id)
	//_, err = notFound.Get(world, id)
	assert.ErrorIs(t, err, storage2.ErrorComponentNotOnEntity)
}

type ValueComponent struct {
	Val int
}

func (ValueComponent) Name() string {
	return "ValueComponent"
}

func TestMultipleCallsToCreateSupported(t *testing.T) {

	world := ecs.NewTestWorld(t)
	assert.NilError(t, ecs.RegisterComponent[ValueComponent](world))

	id, err := ecs.Create(world, ValueComponent{})
	assert.NilError(t, err)

	assert.NilError(t, ecs.SetComponent[ValueComponent](world, id, &ValueComponent{99}))

	val, err := ecs.GetComponent[ValueComponent](world, id)
	assert.NilError(t, err)
	assert.Equal(t, 99, val.Val)

	_, err = ecs.Create(world, ValueComponent{})

	val, err = ecs.GetComponent[ValueComponent](world, id)
	//val, err = valComp.Get(world, id)
	assert.NilError(t, err)
	assert.Equal(t, 99, val.Val)
}
