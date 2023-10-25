package cardinal_test

import (
	"testing"

	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/test_utils"
)

type Alpha struct{}

func (Alpha) Name() string { return "alpha" }

type Beta struct{}

func (Beta) Name() string { return "beta" }

type Gamma struct{}

func (Gamma) Name() string { return "gamma" }

func TestSearchExample(t *testing.T) {
	world, _ := test_utils.MakeWorldAndTicker(t)
	assert.NilError(t, cardinal.RegisterComponent[Alpha](world))
	assert.NilError(t, cardinal.RegisterComponent[Beta](world))
	assert.NilError(t, cardinal.RegisterComponent[Gamma](world))

	worldCtx := test_utils.WorldToWorldContext(world)
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
	}
	for _, tc := range testCases {
		msg := "problem with " + tc.name
		q, err := worldCtx.NewSearch(tc.filter)
		assert.NilError(t, err, msg)
		count, err := q.Count(worldCtx)
		assert.NilError(t, err, msg)
		assert.Equal(t, tc.want, count, msg)
	}
}
