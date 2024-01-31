package cardinal_test

import (
	filter2 "pkg.world.dev/world-engine/cardinal/filter"
	"testing"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"pkg.world.dev/world-engine/cardinal"
)

func TestSearchExample(t *testing.T) {
	fixture := testutils.NewTestFixture(t, nil)
	world := fixture.World
	assert.NilError(t, cardinal.RegisterComponent[Alpha](world))
	assert.NilError(t, cardinal.RegisterComponent[Beta](world))
	assert.NilError(t, cardinal.RegisterComponent[Gamma](world))
	assert.NilError(t, world.LoadGameState())

	worldCtx := testutils.WorldToEngineContext(world)
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
		filter filter2.ComponentFilter
		want   int
	}{
		{
			"exactly alpha",
			filter2.Exact(Alpha{}),
			10,
		},
		{
			"contains alpha",
			filter2.Contains(Alpha{}),
			40,
		},
		{
			"beta or gamma",
			filter2.Or(
				filter2.Exact(Beta{}),
				filter2.Exact(Gamma{}),
			),
			20,
		},
		{
			"not alpha",
			filter2.Not(filter2.Exact(Alpha{})),
			60,
		},
		{
			"alpha and beta",
			filter2.And(filter2.Contains(Alpha{}),
				filter2.Contains(Beta{}),
			), 20,
		},
		{
			"all",
			filter2.All(),
			70,
		},
	}
	for _, tc := range testCases {
		msg := "problem with " + tc.name
		var count int
		count, err = cardinal.NewSearch(worldCtx, tc.filter).Count()
		assert.NilError(t, err, msg)
		assert.Equal(t, tc.want, count, msg)
	}
}
