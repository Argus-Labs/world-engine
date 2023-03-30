package storage

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"testing"

	"gotest.tools/v3/assert"
)

func TestStorage_Bytes(t *testing.T) {
	type Component struct{ ID string }
	var (
		componentType = NewMockComponentType[any](Component{}, Component{ID: "foo"})
	)

	store := NewStorage()

	tests := []struct {
		ID       string
		expected string
	}{
		{ID: "a", expected: "a"},
		{ID: "b", expected: "b"},
		{ID: "c", expected: "c"},
	}

	var archIdx ArchetypeIndex = 0
	var compIdx ComponentIndex = 0
	for _, test := range tests {
		err := store.PushComponent(componentType, archIdx)
		assert.NilError(t, err)
		bz := encodeComponent(t, Component{ID: test.ID})
		fmt.Println(string(bz))
		store.SetComponent(archIdx, compIdx, bz)
		compIdx++
	}

	compIdx = 0
	for _, test := range tests {
		bz := store.Component(archIdx, compIdx)
		var buf bytes.Buffer
		buf.Write(bz)
		dec := gob.NewDecoder(&buf)
		c := &Component{}
		err := dec.Decode(c)
		assert.NilError(t, err)
		assert.Equal(t, c.ID, test.expected)
		compIdx++
	}

	removed := store.SwapRemove(archIdx, 1)
	assert.Assert(t, removed != nil, "removed component should not be nil")
	comp := decodeComponent[Component](t, removed)
	assert.Equal(t, comp.ID, "b", "removed component should have ID 'b'")

	tests2 := []struct {
		archIdx    ArchetypeIndex
		cmpIdx     ComponentIndex
		expectedID string
	}{
		{archIdx: 0, cmpIdx: 0, expectedID: "a"},
		{archIdx: 0, cmpIdx: 1, expectedID: "c"},
	}

	for _, test := range tests2 {
		compBz := store.Component(test.archIdx, test.cmpIdx)
		comp := decodeComponent[Component](t, compBz)
		assert.Equal(t, comp.ID, test.expectedID)
		compIdx++
	}
}

func decodeComponent[T any](t *testing.T, bz []byte) T {
	var buf bytes.Buffer
	buf.Write(bz)
	dec := gob.NewDecoder(&buf)
	comp := new(T)
	err := dec.Decode(comp)
	assert.NilError(t, err)
	return *comp
}

func encodeComponent[T any](t *testing.T, comp T) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(comp)
	assert.NilError(t, err)
	return buf.Bytes()
}
