package cardinal_test

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/search"
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

	q1 := cardinal.NewSearch(worldCtx).
		Contains(search.Component[Vampire](), search.Component[HP]())

	q2 := cardinal.NewSearch(worldCtx).
		Exact(search.Component[Player](), search.Component[HP]())

	testCases := []struct {
		name   string
		search *cardinal.Search
		want   int
	}{
		{
			"vampire and hp",
			q1,
			1,
		},
		{
			"player and hp",
			q2,
			0,
		},
		{
			"vampire or player where hp is 0 and player name is vampire guy",
			q1.Or(q2).Where(cardinal.FilterFunction[HP](func(comp HP) bool {
				return comp.amount == 0
			})).Where(cardinal.FilterFunction[Player](func(comp Player) bool {
				return comp.player == "VampireGuy"
			})),
			0,
		},
		{
			"does not have alpha, where gamma true",
			cardinal.NewSearch(worldCtx).
				Contains(search.Component[AlphaTest]()).Not().
				Where(cardinal.FilterFunction[GammaTest](func(_ GammaTest) bool {
					return true
				})),
			20,
		},
		{
			"exactly alpha",
			cardinal.NewSearch(worldCtx).Exact(search.Component[AlphaTest]()),
			10,
		},
		{
			"contains alpha",
			cardinal.NewSearch(worldCtx).Contains(search.Component[AlphaTest]()),
			40,
		},
		{
			"beta or gamma",
			cardinal.NewSearch(worldCtx).Exact(search.Component[BetaTest]()).
				Or(cardinal.NewSearch(worldCtx).Exact(search.Component[GammaTest]())),
			20,
		},
		{
			"not alpha",
			cardinal.NewSearch(worldCtx).Exact(search.Component[AlphaTest]()).Not(),
			61,
		},
		{
			"alpha and beta",
			cardinal.NewSearch(worldCtx).Contains(search.Component[AlphaTest]()).And(
				cardinal.NewSearch(worldCtx).Contains(search.Component[BetaTest]()),
			),
			20,
		},
		{
			"all",
			cardinal.NewSearch(worldCtx).All(),
			71,
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
		Exact(search.Component[AlphaTest](), search.Component[BetaTest]()).
		Where(cardinal.FilterFunction[AlphaTest](func(_ AlphaTest) bool {
			return false
		})).Count()
	assert.NilError(t, err)
	assert.Equal(t, amount, 0)

	counter := 0

	err =
		cardinal.NewSearch(worldCtx).Exact(search.Component[AlphaTest]()).
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
	amount, err = cardinal.NewSearch(worldCtx).Exact(search.Component[AlphaTest]()).
		Where(cardinal.FilterFunction[AlphaTest](func(comp AlphaTest) bool {
			return comp.Name1 == "BLAH"
		})).Count()
	assert.NilError(t, err)
	assert.Equal(t, amount, 5)
}
