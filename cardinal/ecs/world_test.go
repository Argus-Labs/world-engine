package ecs_test

import (
	"context"
	"testing"

	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/inmem"
)

func TestAddSystems(t *testing.T) {
	count := 0
	sys := func(ecs.WorldContext) error {
		count++
		return nil
	}

	w := inmem.NewECSWorldForTest(t)
	w.AddSystems(sys, sys, sys)
	err := w.LoadGameState()
	assert.NilError(t, err)

	err = w.Tick(context.Background())
	assert.NilError(t, err)

	assert.Equal(t, count, 3)
}

func TestSystemExecutionOrder(t *testing.T) {
	w := inmem.NewECSWorldForTest(t)
	order := make([]int, 0, 3)
	w.AddSystems(func(ecs.WorldContext) error {
		order = append(order, 1)
		return nil
	}, func(ecs.WorldContext) error {
		order = append(order, 2)
		return nil
	}, func(ecs.WorldContext) error {
		order = append(order, 3)
		return nil
	})
	err := w.LoadGameState()
	assert.NilError(t, err)
	assert.NilError(t, w.Tick(context.Background()))
	expectedOrder := []int{1, 2, 3}
	for i, elem := range order {
		assert.Equal(t, elem, expectedOrder[i])
	}
}

func TestSetNamespace(t *testing.T) {
	id := "foo"
	w := inmem.NewECSWorldForTest(t, ecs.WithNamespace(id))
	assert.Equal(t, w.Namespace(), id)
}
