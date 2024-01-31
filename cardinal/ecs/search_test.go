package ecs_test

import (
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/filter"
	"testing"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"pkg.world.dev/world-engine/cardinal/types/entity"
)

type FooComponent struct {
	Data string
}

func (FooComponent) Name() string {
	return "foo"
}

func TestSearchEarlyTermination(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, cardinal.RegisterComponent[FooComponent](world))
	assert.NilError(t, world.LoadGameState())

	total := 10
	count := 0
	stop := 5
	eCtx := cardinal.NewWorldContext(world)
	_, err := cardinal.CreateMany(eCtx, total, FooComponent{})
	assert.NilError(t, err)
	q := cardinal.NewSearch(eCtx, filter.Exact(FooComponent{}))
	assert.NilError(
		t, q.Each(
			func(id entity.ID) bool {
				count++
				return count != stop
			},
		),
	)
	assert.Equal(t, count, stop)

	count = 0
	q = cardinal.NewSearch(eCtx, filter.Exact(FooComponent{}))
	assert.NilError(
		t, q.Each(
			func(id entity.ID) bool {
				count++
				return true
			},
		),
	)
	assert.Equal(t, count, total)
}
