package cardinal_test

import (
	"testing"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal"
)

type Alpha struct{}

func (Alpha) Name() string { return "alpha" }

type Beta struct{}

func (Beta) Name() string { return "beta" }

type Gamma struct{}

func (Gamma) Name() string { return "gamma" }

func TestSearchExample(t *testing.T) {
	world, _ := testutils.MakeWorldAndTicker(t)
	assert.NilError(t, cardinal.RegisterComponent[Alpha](world))
	assert.NilError(t, cardinal.RegisterComponent[Beta](world))
	assert.NilError(t, cardinal.RegisterComponent[Gamma](world))

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
		filter cardinal.Filter
		want   int
	}{
		{
			"exactly alpha",
			cardinal.Exact(Alpha{}),
			10,
		},
		{
			"contains alpha",
			cardinal.Contains(Alpha{}),
			40,
		},
		{
			"beta or gamma",
			cardinal.Or(
				cardinal.Exact(Beta{}),
				cardinal.Exact(Gamma{}),
			),
			20,
		},
		{
			"not alpha",
			cardinal.Not(cardinal.Exact(Alpha{})),
			60,
		},
		{
			"alpha and beta",
			cardinal.And(cardinal.Contains(Alpha{}),
				cardinal.Contains(Beta{}),
			), 20,
		},
		{
			"all",
			cardinal.All(),
			70,
		},
	}
	for _, tc := range testCases {
		msg := "problem with " + tc.name
		var q *cardinal.Search
		q, err = worldCtx.NewSearch(tc.filter)
		assert.NilError(t, err, msg)
		var count int
		count, err = q.Count(worldCtx)
		assert.NilError(t, err, msg)
		assert.Equal(t, tc.want, count, msg)
	}
}
