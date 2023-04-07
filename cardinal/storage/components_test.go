package storage

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/argus-labs/cardinal/component"
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
			if !st.Contains(tt.archIdx, tt.compIdx) {
				t.Errorf("storage should contain the component at %d, %d", tt.archIdx, tt.compIdx)
			}
			bz := st.Component(tt.archIdx, tt.compIdx)
			dat := decodeComponent[ComponentData](t, bz)
			dat.ID = tt.ID
			st.SetComponent(tt.archIdx, tt.compIdx, encodeComponent[ComponentData](t, dat))
		}
	}

	target := tests[0]
	storage := components.Storage(ca)

	srcArchIdx := target.archIdx
	var dstArchIdx ArchetypeIndex = 1

	storage.MoveComponent(srcArchIdx, target.compIdx, dstArchIdx)
	components.Move(srcArchIdx, dstArchIdx)

	if storage.Contains(srcArchIdx, target.compIdx) {
		t.Errorf("storage should not contain the component at %d, %d", target.archIdx, target.compIdx)
	}
	if idx, _ := components.componentIndices.ComponentIndex(srcArchIdx); idx != -1 {
		t.Errorf("component index should be -1 at %d but %d", srcArchIdx, idx)
	}

	newCompIdx, _ := components.componentIndices.ComponentIndex(dstArchIdx)
	if !storage.Contains(dstArchIdx, newCompIdx) {
		t.Errorf("storage should contain the component at %d, %d", dstArchIdx, target.compIdx)
	}

	bz := storage.Component(dstArchIdx, newCompIdx)
	dat := decodeComponent[ComponentData](t, bz)
	if dat.ID != target.ID {
		t.Errorf("component should have ID '%s', got ID '%s'", target.ID, dat.ID)
	}
}
