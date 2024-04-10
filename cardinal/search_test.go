package cardinal_test

import (
	"fmt"
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/search"
	"pkg.world.dev/world-engine/cardinal/search/filter"
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

type Player struct {
	player string
}

type Vampire struct {
	vampire bool
}

type HP struct {
	amount int
}

func (p Player) Name() string {
	return "Player"
}

func (a HP) Name() string {
	return "HP"
}

func (v Vampire) Name() string {
	return "Vampire"
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
	assert.NilError(t, cardinal.RegisterComponent[Player](world))
	assert.NilError(t, cardinal.RegisterComponent[Vampire](world))
	assert.NilError(t, cardinal.RegisterComponent[HP](world))

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
	_, err = cardinal.CreateMany(worldCtx, 1, &Player{
		player: "AnyName",
	}, &Vampire{vampire: true}, &HP{amount: 0})
	assert.NilError(t, err)

	testCases := []struct {
		name   string
		search search.Searchable
		want   int
	}{
		{
			"does not have alpha, where gamma true",
			cardinal.NewSearch(worldCtx).Entity(filter.Not(filter.
				Contains(filter.Component[AlphaTest]()))).Where(cardinal.FilterFunction[GammaTest](func(_ GammaTest) bool {
				return true
			})),
			20,
		},
		{
			"exactly alpha",
			cardinal.NewSearch(worldCtx).Entity(filter.Exact(filter.Component[AlphaTest]())),
			10,
		},
		{
			"contains alpha",
			cardinal.NewSearch(worldCtx).Entity(filter.Contains(filter.Component[AlphaTest]())),
			40,
		},
		{
			"beta or gamma",
			search.Or(cardinal.NewSearch(worldCtx).Entity(filter.Exact(filter.Component[BetaTest]())),
				cardinal.NewSearch(worldCtx).Entity(filter.Exact(filter.Component[GammaTest]()))),
			20,
		},
		{
			"not alpha",
			search.Not(cardinal.NewSearch(worldCtx).Entity(filter.Exact(filter.Component[AlphaTest]()))),
			61,
		},
		{
			"alpha and beta",
			cardinal.NewSearch(worldCtx).Entity(filter.And(
				filter.Contains(filter.Component[AlphaTest]()),
				filter.Contains(filter.Component[BetaTest]()),
			)),
			20,
		},
		{
			"all",
			cardinal.NewSearch(worldCtx).Entity(filter.All()),
			71,
		},
	}
	for i, tc := range testCases {
		msg := "problem with " + tc.name
		var count int
		count, err = tc.search.Count()
		assert.NilError(t, err, msg)
		fmt.Println(i)
		assert.Equal(t, tc.want, count, msg)
	}
	amount, err := cardinal.NewSearch(worldCtx).Entity(filter.Exact(filter.Component[AlphaTest](), filter.Component[BetaTest]())).
		Where(cardinal.FilterFunction[AlphaTest](func(_ AlphaTest) bool {
			return false
		})).Count()
	assert.NilError(t, err)
	assert.Equal(t, amount, 0)

	counter := 0

	err =
		cardinal.NewSearch(worldCtx).Entity(filter.Exact(filter.Component[AlphaTest]())).
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
	amount, err = cardinal.NewSearch(worldCtx).Entity(filter.Exact(filter.Component[AlphaTest]())).
		Where(cardinal.FilterFunction[AlphaTest](func(comp AlphaTest) bool {
			return comp.Name1 == "BLAH"
		})).Count()
	assert.NilError(t, err)
	assert.Equal(t, amount, 5)
}
