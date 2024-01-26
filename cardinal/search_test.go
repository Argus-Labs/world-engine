package cardinal_test

import (
	"testing"

	"pkg.world.dev/world-engine/cardinal/ecs/filter"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"pkg.world.dev/world-engine/cardinal"
)

type Alpha struct{}

func (Alpha) Name() string { return "alpha" }

type Beta struct{}

func (Beta) Name() string { return "beta" }

type Gamma struct{}

func (Gamma) Name() string { return "gamma" }

func TestSearchExample(t *testing.T) {
	fixture := testutils.NewTestFixture(t, nil)
	world := fixture.World
	assert.NilError(t, cardinal.RegisterComponent[Alpha](world))
	assert.NilError(t, cardinal.RegisterComponent[Beta](world))
	assert.NilError(t, cardinal.RegisterComponent[Gamma](world))
	assert.NilError(t, fixture.Engine.LoadGameState())

	worldCtx := testutils.WorldToWorldContext(world)
	_, err := cardinal.CreateMany(worldCtx, 10, Alpha{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Beta{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Gamma{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Alpha{}, Beta{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Alpha{}, Gamma{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Beta{}, Gamma{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Alpha{}, Beta{}, Gamma{})
	assert.NilError(t, err)

	testCases := []struct {
		name   string
		filter filter.ComponentFilter
		want   int
	}{
		{
			"exactly alpha",
			filter.Exact(Alpha{}),
			10,
		},
		{
			"contains alpha",
			filter.Contains(Alpha{}),
			40,
		},
		{
			"beta or gamma",
			filter.Or(
				filter.Exact(Beta{}),
				filter.Exact(Gamma{}),
			),
			20,
		},
		{
			"not alpha",
			filter.Not(filter.Exact(Alpha{})),
			60,
		},
		{
			"alpha and beta",
			filter.And(filter.Contains(Alpha{}),
				filter.Contains(Beta{}),
			), 20,
		},
		{
			"all",
			filter.All(),
			70,
		},
	}
	for _, tc := range testCases {
		msg := "problem with " + tc.name
		var count int
		count, err = worldCtx.NewSearch(tc.filter).Count(worldCtx)
		assert.NilError(t, err, msg)
		assert.Equal(t, tc.want, count, msg)
	}
}
