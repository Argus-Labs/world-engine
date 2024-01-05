package ecs_test

import (
	"testing"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

type FooComponent struct {
	Data string
}

func (FooComponent) Name() string {
	return "foo"
}

func TestSearchEarlyTermination(t *testing.T) {
	world := testutils.NewTestWorld(t).Instance()
	assert.NilError(t, ecs.RegisterComponent[FooComponent](world))

	total := 10
	count := 0
	stop := 5
	wCtx := ecs.NewWorldContext(world)
	_, err := ecs.CreateMany(wCtx, total, FooComponent{})
	assert.NilError(t, err)
	q, err := world.NewSearch(ecs.Exact(FooComponent{}))
	assert.NilError(t, err)
	assert.NilError(
		t, q.Each(
			wCtx, func(id entity.ID) bool {
				count++
				return count != stop
			},
		).Commit(),
	)
	assert.Equal(t, count, stop)

	count = 0
	q, err = world.NewSearch(ecs.Exact(FooComponent{}))
	assert.NilError(t, err)
	assert.NilError(
		t, q.Each(
			wCtx, func(id entity.ID) bool {
				count++
				return true
			},
		).Commit(),
	)
	assert.Equal(t, count, total)
	doubleCount := 0
	otherCounter := func(id entity.ID) bool {
		doubleCount++
		return true
	}
	assert.NilError(t, q.Each(wCtx, otherCounter).Each(wCtx, otherCounter).Commit())
	assert.Equal(t, 2*count, doubleCount)
}
