package cardinal_test

import (
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/cardinaltestutils"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

type Alpha struct{}

func (Alpha) Name() string { return "alpha" }

type Beta struct{}

func (Beta) Name() string { return "beta" }

type Gamma struct{}

func (Gamma) Name() string { return "gamma" }

func TestSearchExample(t *testing.T) {
	world, _ := cardinaltestutils.MakeWorldAndTicker(t)
	testutils.AssertNilErrorWithTrace(t, cardinal.RegisterComponent[Alpha](world))
	testutils.AssertNilErrorWithTrace(t, cardinal.RegisterComponent[Beta](world))
	testutils.AssertNilErrorWithTrace(t, cardinal.RegisterComponent[Gamma](world))

	worldCtx := cardinaltestutils.WorldToWorldContext(world)
	_, err := cardinal.CreateMany(worldCtx, 10, Alpha{})
	testutils.AssertNilErrorWithTrace(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Beta{})
	testutils.AssertNilErrorWithTrace(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Gamma{})
	testutils.AssertNilErrorWithTrace(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Alpha{}, Beta{})
	testutils.AssertNilErrorWithTrace(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Alpha{}, Gamma{})
	testutils.AssertNilErrorWithTrace(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Beta{}, Gamma{})
	testutils.AssertNilErrorWithTrace(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, Alpha{}, Beta{}, Gamma{})
	testutils.AssertNilErrorWithTrace(t, err)

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
		testutils.AssertNilErrorWithTrace(t, err, msg)
		var count int
		count, err = q.Count(worldCtx)
		testutils.AssertNilErrorWithTrace(t, err, msg)
		assert.Equal(t, tc.want, count, msg)
	}
}
