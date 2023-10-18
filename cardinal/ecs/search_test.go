package ecs_test

import (
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
)

type FooComponent struct {
	Data string
}

func (FooComponent) Name() string {
	return "foo"
}

func TestSearchEarlyTermination(t *testing.T) {

	world := ecs.NewTestWorld(t)
	assert.NilError(t, ecs.RegisterComponent[FooComponent](world))

	total := 10
	count := 0
	stop := 5
	_, err := cardinal.CreateMany(world, total, FooComponent{})
	assert.NilError(t, err)
	q, err := world.NewSearch(ecs.Exact(FooComponent{}))
	assert.NilError(t, err)
	q.Each(world, func(id entity.ID) bool {
		count++
		if count == stop {
			return false
		}
		return true
	})
	assert.Equal(t, count, stop)

	count = 0
	q, err = world.NewSearch(ecs.Exact(FooComponent{}))
	assert.NilError(t, err)
	q.Each(world, func(id entity.ID) bool {
		count++
		return true
	})
	assert.Equal(t, count, total)
}
