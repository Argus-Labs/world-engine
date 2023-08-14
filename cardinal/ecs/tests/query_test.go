package tests

import (
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	"gotest.tools/v3/assert"
	"testing"
)

func TestQueryEarlyTermination(t *testing.T) {
	type FooComponent struct {
		Data string
	}
	foo := ecs.NewComponentType[FooComponent]()
	world := inmem.NewECSWorldForTest(t)
	assert.NilError(t, world.RegisterComponents(foo))

	total := 10
	count := 0
	stop := 5
	_, err := world.CreateMany(total, foo)
	assert.NilError(t, err)
	ecs.NewQuery(filter.Exact(foo)).Each(world, func(id storage.EntityID) bool {
		count++
		if count == stop {
			return false
		}
		return true
	})
	assert.Equal(t, count, stop)

	count = 0
	ecs.NewQuery(filter.Exact(foo)).Each(world, func(id storage.EntityID) bool {
		count++
		return true
	})
	assert.Equal(t, count, total)
}
