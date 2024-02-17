package storage_test

import (
	"testing"

	"pkg.world.dev/world-engine/cardinal/types"

	"pkg.world.dev/world-engine/assert"
)

type TestComponent1 struct {
	number int
}

func (TestComponent1) Name() string {
	return "test_component1"
}

type TestComponent struct {
	word string
}

func (TestComponent) Name() string {
	return "test_component"
}

func TestSetAndGetSchema(t *testing.T) {
	testComponent1 := TestComponent1{number: 2}
	testComponent := TestComponent{word: "hello"}
	schema1, err := types.SerializeComponentSchema(testComponent1)
	assert.NilError(t, err)
	schema, err := types.SerializeComponentSchema(testComponent)
	assert.NilError(t, err)
	rs := GetRedisStorage(t)
	err = rs.SetSchema(testComponent1.Name(), schema1)
	assert.NilError(t, err)
	err = rs.SetSchema(testComponent.Name(), schema)
	assert.NilError(t, err)
	otherSchema1, err := rs.GetSchema(testComponent1.Name())
	assert.NilError(t, err)
	valid, err := types.IsComponentValid(testComponent1, otherSchema1)
	assert.NilError(t, err)
	assert.Assert(t, valid)
	otherSchema, err := rs.GetSchema(testComponent.Name())
	assert.NilError(t, err)
	valid, err = types.IsComponentValid(testComponent1, otherSchema)
	assert.NilError(t, err)
	assert.Assert(t, !valid)
}
