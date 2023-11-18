package ecs_test

import (
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/cardinaltestutils"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

type FooComponent struct {
	Data string
}

func (FooComponent) Name() string {
	return "foo"
}

func TestSearchEarlyTermination(t *testing.T) {
	world := cardinaltestutils.NewTestWorld(t).Instance()
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[FooComponent](world))

	total := 10
	count := 0
	stop := 5
	wCtx := ecs.NewWorldContext(world)
	_, err := component.CreateMany(wCtx, total, FooComponent{})
	testutils.AssertNilErrorWithTrace(t, err)
	q, err := world.NewSearch(ecs.Exact(FooComponent{}))
	testutils.AssertNilErrorWithTrace(t, err)
	testutils.AssertNilErrorWithTrace(t, q.Each(wCtx, func(id entity.ID) bool {
		count++
		return count != stop
	}))
	assert.Equal(t, count, stop)

	count = 0
	q, err = world.NewSearch(ecs.Exact(FooComponent{}))
	testutils.AssertNilErrorWithTrace(t, err)
	testutils.AssertNilErrorWithTrace(t, q.Each(wCtx, func(id entity.ID) bool {
		count++
		return true
	}))
	assert.Equal(t, count, total)
}
