package ecs_test

import (
	"testing"

	"pkg.world.dev/world-engine/cardinal/ecs/filter"

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
	engine := testutils.NewTestFixture(t, nil).Engine
	assert.NilError(t, ecs.RegisterComponent[FooComponent](engine))

	total := 10
	count := 0
	stop := 5
	eCtx := ecs.NewEngineContext(engine)
	_, err := ecs.CreateMany(eCtx, total, FooComponent{})
	assert.NilError(t, err)
	q := engine.NewSearch(filter.Exact(FooComponent{}))
	assert.NilError(
		t, q.Each(
			eCtx, func(id entity.ID) bool {
				count++
				return count != stop
			},
		),
	)
	assert.Equal(t, count, stop)

	count = 0
	q = engine.NewSearch(filter.Exact(FooComponent{}))
	assert.NilError(
		t, q.Each(
			eCtx, func(id entity.ID) bool {
				count++
				return true
			},
		),
	)
	assert.Equal(t, count, total)
}
