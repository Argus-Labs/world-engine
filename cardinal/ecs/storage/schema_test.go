package storage_test

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/ecs/internal/testutil"
	"pkg.world.dev/world-engine/cardinal/types/component"
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
	schema1, err := component.SerializeComponentSchema(testComponent1)
	assert.NilError(t, err)
	schema, err := component.SerializeComponentSchema(testComponent)
	assert.NilError(t, err)
	rs := testutil.GetRedisStorage(t)
	err = rs.Schema.SetSchema(testComponent1.Name(), schema1)
	assert.NilError(t, err)
	err = rs.Schema.SetSchema(testComponent.Name(), schema)
	assert.NilError(t, err)
	otherSchema1, err := rs.Schema.GetSchema(testComponent1.Name())
	assert.NilError(t, err)
	valid, err := component.IsComponentValid(testComponent1, otherSchema1)
	assert.NilError(t, err)
	assert.Assert(t, valid)
	otherSchema, err := rs.Schema.GetSchema(testComponent.Name())
	assert.NilError(t, err)
	valid, err = component.IsComponentValid(testComponent1, otherSchema)
	assert.NilError(t, err)
	assert.Assert(t, !valid)
}
