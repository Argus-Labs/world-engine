package ecs

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type wrongComponentA struct {
	Value string
}

func (wrongComponentA) Name() string {
	return testutils.ComponentA{}.Name()
}

func (c wrongComponentA) MarshalWire() ([]byte, error) {
	return []byte(c.Value), nil
}

func (wrongComponentA) UnmarshalWire(data []byte) (any, error) {
	return wrongComponentA{Value: string(data)}, nil
}

func TestCreateWithComponents_ValidatesBeforeMutation(t *testing.T) {
	t.Parallel()

	world := NewWorld()
	_, err := RegisterComponent[testutils.ComponentA](world)
	require.NoError(t, err)

	_, err = CreateWithComponents(
		world,
		testutils.ComponentA{X: 1},
		testutils.ComponentB{ID: 2},
	)
	require.Error(t, err)
	assert.Empty(t, world.LiveEntityIDs(), "unregistered component must not create a partial entity")

	_, err = CreateWithComponents(
		world,
		testutils.ComponentA{X: 1},
		testutils.ComponentA{X: 2},
	)
	require.Error(t, err)
	assert.Empty(t, world.LiveEntityIDs(), "duplicate component must not create a partial entity")

	_, err = CreateWithComponents(world, wrongComponentA{Value: "wrong concrete type"})
	require.Error(t, err)
	assert.Empty(t, world.LiveEntityIDs(), "type mismatch must not create a partial entity")

	_, err = RegisterComponent[testutils.ComponentB](world)
	require.NoError(t, err)
	componentA := testutils.ComponentA{X: 1, Y: 2, Z: 3}
	componentB := testutils.ComponentB{ID: 4, Label: "valid", Enabled: true}
	eid, err := CreateWithComponents(world, componentA, componentB)
	require.NoError(t, err)
	assert.Equal(t, []EntityID{eid}, world.LiveEntityIDs())
	assert.Equal(t, componentA, mustGetComponent[testutils.ComponentA](t, world, eid))
	assert.Equal(t, componentB, mustGetComponent[testutils.ComponentB](t, world, eid))
}

func mustGetComponent[C Component](t *testing.T, world *World, eid EntityID) C {
	t.Helper()
	component, err := Get[C](world, eid)
	require.NoError(t, err)
	return component
}
