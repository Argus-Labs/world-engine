package cardinal_test

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types"
)

var _ types.Component

type AlphaTest struct {
	Name1 string
}
type BetaTest struct {
	Name1 string
}
type GammaTest struct {
	Name1 string
}

func (AlphaTest) Name() string {
	return "alpha"
}

func (BetaTest) Name() string {
	return "beta"
}

func (GammaTest) Name() string {
	return "gamma"
}

func TestSearchExample(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[AlphaTest](world))
	assert.NilError(t, cardinal.RegisterComponent[BetaTest](world))
	assert.NilError(t, cardinal.RegisterComponent[GammaTest](world))
	tf.StartWorld()

	worldCtx := cardinal.NewWorldContext(world)
	_, err := cardinal.CreateMany(worldCtx, 10, AlphaTest{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, BetaTest{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, GammaTest{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, AlphaTest{}, BetaTest{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, AlphaTest{}, GammaTest{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, BetaTest{}, GammaTest{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, AlphaTest{}, BetaTest{}, GammaTest{})
	assert.NilError(t, err)

	testCases := []struct {
		name   string
		search *cardinal.Search
		want   int
	}{
		{
			"exactly alpha",
			cardinal.NewSearch(worldCtx).Exact(cardinal.SearchComponent[AlphaTest]()),
			10,
		},
		{
			"contains alpha",
			cardinal.NewSearch(worldCtx).Contains(cardinal.SearchComponent[AlphaTest]()),
			40,
		},
		{
			"beta or gamma",
			cardinal.NewSearch(worldCtx).Exact(cardinal.SearchComponent[BetaTest]()).
				Or(cardinal.NewSearch(worldCtx).Exact(cardinal.SearchComponent[GammaTest]())),
			20,
		},
		{
			"not alpha",
			cardinal.NewSearch(worldCtx).Exact(cardinal.SearchComponent[AlphaTest]()).Not(),
			60,
		},
		{
			"alpha and beta",
			cardinal.NewSearch(worldCtx).Contains(cardinal.SearchComponent[AlphaTest]()).And(
				cardinal.NewSearch(worldCtx).Contains(cardinal.SearchComponent[BetaTest]()),
			),
			20,
		},
		{
			"all",
			cardinal.NewSearch(worldCtx).All(),
			70,
		},
	}
	for _, tc := range testCases {
		msg := "problem with " + tc.name
		var count int
		count, err = tc.search.Count()
		assert.NilError(t, err, msg)
		assert.Equal(t, tc.want, count, msg)
	}
	amount, err := cardinal.NewSearch(worldCtx).
		Exact(cardinal.SearchComponent[AlphaTest](), cardinal.SearchComponent[BetaTest]()).
		Where(cardinal.FilterFunction[AlphaTest](func(_ AlphaTest) bool {
			return false
		})).Count()
	assert.NilError(t, err)
	assert.Equal(t, amount, 0)

	counter := 0

	err =
		cardinal.NewSearch(worldCtx).Exact(cardinal.SearchComponent[AlphaTest]()).
			Where(cardinal.FilterFunction[AlphaTest](func(_ AlphaTest) bool { return true })).
			Each(func(id types.EntityID) bool {
				comp, err := cardinal.GetComponent[AlphaTest](worldCtx, id)
				assert.NilError(t, err)
				if counter%2 == 0 {
					comp.Name1 = "BLAH"
				}
				counter++
				err = cardinal.SetComponent[AlphaTest](worldCtx, id, comp)
				assert.NilError(t, err)
				return true
			})
	assert.NilError(t, err)
	amount, err = cardinal.NewSearch(worldCtx).Exact(cardinal.SearchComponent[AlphaTest]()).
		Where(cardinal.FilterFunction[AlphaTest](func(comp AlphaTest) bool {
			return comp.Name1 == "BLAH"
		})).Count()
	assert.NilError(t, err)
	assert.Equal(t, amount, 5)
}
