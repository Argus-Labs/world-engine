package storage

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
)

func TestComponents(t *testing.T) {
	type ComponentData struct {
		ID string
	}
	var (
		ca = NewMockComponentType(ComponentData{}, ComponentData{ID: "foo"})
		cb = NewMockComponentType(ComponentData{}, ComponentData{ID: "bar"})
	)

	components := NewComponents(NewComponentsSliceStorage(), NewComponentIndexMap())

	tests := []*struct {
		layout  *Layout
		archIdx ArchetypeIndex
		compIdx ComponentIndex
		ID      string
	}{
		{
			NewLayout([]component.IComponentType{ca}),
			0,
			0,
			"a",
		},
		{
			NewLayout([]component.IComponentType{ca, cb}),
			1,
			1,
			"b",
		},
	}

	for _, tt := range tests {
		var err error
		tt.compIdx, err = components.PushComponents(tt.layout.Components(), tt.archIdx)
		assert.NilError(t, err)
	}

	for _, tt := range tests {
		for _, comp := range tt.layout.Components() {
			st := components.Storage(comp)
			ok, err := st.Contains(tt.archIdx, tt.compIdx)
			assert.NilError(t, err)
			if !ok {
				t.Errorf("storage should contain the component at %d, %d", tt.archIdx, tt.compIdx)
			}
			bz, _ := st.Component(tt.archIdx, tt.compIdx)
			dat, err := Decode[ComponentData](bz)
			assert.NilError(t, err)
			dat.ID = tt.ID
			compBz, err := Encode(dat)
			assert.NilError(t, err)
			err = st.SetComponent(tt.archIdx, tt.compIdx, compBz)
			assert.NilError(t, err)
		}
	}

	target := tests[0]
	storage := components.Storage(ca)

	srcArchIdx := target.archIdx
	var dstArchIdx ArchetypeIndex = 1

	err := storage.MoveComponent(srcArchIdx, target.compIdx, dstArchIdx)
	assert.NilError(t, err)
	err = components.Move(srcArchIdx, dstArchIdx)
	assert.NilError(t, err)

	ok, err := storage.Contains(srcArchIdx, target.compIdx)
	assert.NilError(t, err)
	if ok {
		t.Errorf("storage should not contain the component at %d, %d", target.archIdx, target.compIdx)
	}
	if idx, _, _ := components.componentIndices.ComponentIndex(srcArchIdx); idx != -1 {
		t.Errorf("component Index should be -1 at %d but %d", srcArchIdx, idx)
	}

	newCompIdx, _, _ := components.componentIndices.ComponentIndex(dstArchIdx)
	ok, err = storage.Contains(dstArchIdx, newCompIdx)
	assert.NilError(t, err)
	if !ok {
		t.Errorf("storage should contain the component at %d, %d", dstArchIdx, target.compIdx)
	}

	bz, _ := storage.Component(dstArchIdx, newCompIdx)
	dat, err := Decode[ComponentData](bz)
	assert.NilError(t, err)
	if dat.ID != target.ID {
		t.Errorf("component should have ID '%s', got ID '%s'", target.ID, dat.ID)
	}
}
