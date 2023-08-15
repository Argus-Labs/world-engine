package tests

import (
	"context"
	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/inmem"
	"testing"
)

func TestAddSystems(t *testing.T) {
	count := 0
	sys := func(w *ecs.World, txq *ecs.TransactionQueue) error {
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

func TestSetNamespace(t *testing.T) {
	id := "foo"
	w := inmem.NewECSWorldForTest(t, ecs.WithNamespace(id))
	assert.Equal(t, w.Namespace(), id)
}
