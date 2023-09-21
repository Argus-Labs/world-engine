package ecs_test

import (
	"testing"

	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/inmem"
)

func TestQueryEarlyTermination(t *testing.T) {
	type FooComponent struct {
		Data string
	}
	foo := ecs.NewComponentType[FooComponent]("foo")
	world := inmem.NewECSWorldForTest(t)
	assert.NilError(t, world.RegisterComponents(foo))

	total := 10
	count := 0
	stop := 5
	_, err := world.CreateMany(total, foo)
	assert.NilError(t, err)
	ecs.NewQuery(filter.Exact(foo)).Each(world, func(id entity.ID) bool {
		count++
		if count == stop {
			return false
		}
		return true
	})
	assert.Equal(t, count, stop)

	count = 0
	ecs.NewQuery(filter.Exact(foo)).Each(world, func(id entity.ID) bool {
		count++
		return true
	})
	assert.Equal(t, count, total)
}
