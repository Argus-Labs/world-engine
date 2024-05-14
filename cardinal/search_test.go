package cardinal_test

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
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

func TestSearchUsingAllMethods(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[AlphaTest](world))
	assert.NilError(t, cardinal.RegisterComponent[BetaTest](world))
	assert.NilError(t, cardinal.RegisterComponent[GammaTest](world))
	assert.NilError(t, cardinal.RegisterComponent[HP](world))
	tf.StartWorld()

	worldCtx := cardinal.NewWorldContext(world)
	_, err := cardinal.CreateMany(worldCtx, 10, AlphaTest{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, BetaTest{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, GammaTest{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 10, AlphaTest{}, GammaTest{})
	assert.NilError(t, err)
	hpids, err := cardinal.CreateMany(worldCtx, 10, HP{})
	assert.NilError(t, err)
	for i, id := range hpids {
		err = cardinal.SetComponent[HP](worldCtx, id, &HP{amount: i})
		assert.NilError(t, err)
	}
	amt, err := cardinal.NewSearch().Entity(filter.Not(filter.Or(
		filter.Contains(filter.Component[AlphaTest]()),
		filter.Contains(filter.Component[BetaTest]()),
		filter.Contains(filter.Component[GammaTest]())),
	)).Where(func(_ cardinal.WorldContext, _ types.EntityID) (bool, error) {
		return true, nil
	}).Count(worldCtx)
	assert.NilError(t, err)
	assert.Equal(t, amt, 10)
	q := cardinal.NewSearch().Entity(filter.Not(filter.Or(
		filter.Contains(filter.Component[AlphaTest]()),
		filter.Contains(filter.Component[BetaTest]()),
		filter.Contains(filter.Component[GammaTest]())),
	)).Where(func(wCtx cardinal.WorldContext, id types.EntityID) (bool, error) {
		c, err := cardinal.GetComponent[HP](wCtx, id)
		if err != nil {
			return false, err
		}
		if c.amount < 3 {
			return true, nil
		}
		return false, nil
	})
	amt, err = q.Count(worldCtx)
	assert.NilError(t, err)
	assert.Equal(t, amt, 3)
	ids, err := q.Collect(worldCtx)
	assert.NilError(t, err)
	assert.True(t, areIDsSorted(ids))
}

func areIDsSorted(ids []types.EntityID) bool {
	for index, id := range ids {
		if index < len(ids)-1 {
			if id <= ids[index+1] {
				continue
			}
			return false
		}
	}
	return true
}

func TestSetOperationsOnSearch(t *testing.T) {
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
	_, err = cardinal.CreateMany(worldCtx, 10, AlphaTest{}, GammaTest{})
	assert.NilError(t, err)

	q1 := cardinal.NewSearch().Entity(filter.Exact(filter.Component[AlphaTest]()))
	q2 := cardinal.NewSearch().Entity(filter.Exact(filter.Component[BetaTest]()))
	q3 := cardinal.NewSearch().Entity(filter.Exact(filter.Component[GammaTest]()))
	q4 := cardinal.NewSearch().Entity(filter.Contains(
		filter.Component[AlphaTest]()))

	tests := []struct {
		search cardinal.Searchable
		count  int
	}{
		{
			search: cardinal.And(q1, q2),
			count:  0,
		}, {
			search: cardinal.Or(q1, q2),
			count:  20,
		}, {
			search: cardinal.Not(cardinal.Or(q1, q2, q3)),
			count:  10,
		}, {
			search: cardinal.Not(cardinal.And(q1, q2, q3)),
			count:  40,
		}, {
			search: cardinal.Not(q4),
			count:  20,
		},
	}
	for _, searchStruct := range tests {
		amt, err := searchStruct.search.Count(worldCtx)
		assert.NilError(t, err)
		assert.Equal(t, amt, searchStruct.count)
		ids, err := searchStruct.search.Collect(worldCtx)
		assert.NilError(t, err)
		assert.True(t, areIDsSorted(ids))
	}
}

func TestSearch_Integration(t *testing.T) {
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
		search cardinal.Searchable
		want   int
	}{
		{
			"does not have alpha, where gamma true",
			cardinal.NewSearch().Entity(filter.Not(filter.
				Contains(filter.Component[AlphaTest]()))).Where(cardinal.FilterFunction[GammaTest](func(_ GammaTest) bool {
				return true
			})),
			20,
		},
		{
			"exactly alpha",
			cardinal.NewSearch().Entity(filter.Exact(filter.Component[AlphaTest]())),
			10,
		},
		{
			"contains alpha",
			cardinal.NewSearch().Entity(filter.Contains(filter.Component[AlphaTest]())),
			40,
		},
		{
			"beta or gamma",
			cardinal.Or(cardinal.NewSearch().Entity(filter.Exact(filter.Component[BetaTest]())),
				cardinal.NewSearch().Entity(filter.Exact(filter.Component[GammaTest]()))),
			20,
		},
		{
			"not alpha",
			cardinal.Not(cardinal.NewSearch().Entity(filter.Exact(filter.Component[AlphaTest]()))),
			61,
		},
		{
			"alpha and beta",
			cardinal.NewSearch().Entity(filter.And(
				filter.Contains(filter.Component[AlphaTest]()),
				filter.Contains(filter.Component[BetaTest]()),
			)),
			20,
		},
		{
			"all",
			cardinal.NewSearch().Entity(filter.All()),
			71,
		},
	}
	for _, tc := range testCases {
		msg := "problem with " + tc.name
		var count int
		count, err = tc.search.Count(worldCtx)
		assert.NilError(t, err, msg)
		assert.Equal(t, tc.want, count, msg)
	}
	amount, err := cardinal.NewSearch().Entity(filter.Exact(
		filter.Component[AlphaTest](),
		filter.Component[BetaTest]())).
		Where(cardinal.FilterFunction[AlphaTest](func(_ AlphaTest) bool {
			return false
		})).Count(worldCtx)
	assert.NilError(t, err)
	assert.Equal(t, amount, 0)

	counter := 0

	err =
		cardinal.NewSearch().Entity(filter.Exact(filter.Component[AlphaTest]())).
			Where(cardinal.FilterFunction[AlphaTest](func(_ AlphaTest) bool { return true })).
			Each(worldCtx, func(id types.EntityID) bool {
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
	amount, err = cardinal.NewSearch().Entity(filter.Exact(filter.Component[AlphaTest]())).
		Where(cardinal.FilterFunction[AlphaTest](
			func(comp AlphaTest) bool {
				return comp.Name1 == "BLAH"
			})).Count(worldCtx)
	assert.NilError(t, err)
	assert.Equal(t, amount, 5)
}

func TestSearch_Exact_ReturnsExactComponentMatch(t *testing.T) {
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
	_, err = cardinal.CreateMany(worldCtx, 12, BetaTest{})
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

	amt, err := cardinal.NewSearch().Entity(filter.Exact(filter.Component[BetaTest]())).Count(worldCtx)
	assert.NilError(t, err)
	assert.Equal(t, amt, 12)
}

func TestSearch_Contains_ReturnsEntityThatContainsComponents(t *testing.T) {
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
	_, err = cardinal.CreateMany(worldCtx, 12, BetaTest{})
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

	amt, err := cardinal.NewSearch().Entity(filter.Contains(filter.Component[BetaTest]())).Count(worldCtx)
	assert.NilError(t, err)
	assert.Equal(t, amt, 42)
}

func TestSearch_ComponentNotRegistered_ReturnsZeroEntityWithNoError(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[AlphaTest](world))
	assert.NilError(t, cardinal.RegisterComponent[BetaTest](world))
	assert.NilError(t, cardinal.RegisterComponent[GammaTest](world))
	assert.NilError(t, cardinal.RegisterComponent[Player](world))
	assert.NilError(t, cardinal.RegisterComponent[Vampire](world))
	// assert.NilError(t, cardinal.RegisterComponent[HP](world)) // unregistered

	tf.StartWorld()

	worldCtx := cardinal.NewWorldContext(world)
	_, err := cardinal.CreateMany(worldCtx, 10, AlphaTest{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 12, BetaTest{})
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

	amt, err := cardinal.NewSearch().Entity(filter.Contains(filter.Component[HP]())).Count(worldCtx)
	assert.NilError(t, err)
	assert.Equal(t, amt, 0)
}

func TestUnregisteredComponentOnSetOperators(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[AlphaTest](world))
	assert.NilError(t, cardinal.RegisterComponent[BetaTest](world))
	assert.NilError(t, cardinal.RegisterComponent[GammaTest](world))
	assert.NilError(t, cardinal.RegisterComponent[Player](world))
	assert.NilError(t, cardinal.RegisterComponent[Vampire](world))
	// assert.NilError(t, cardinal.RegisterComponent[HP](world)) // unregistered

	tf.StartWorld()

	worldCtx := cardinal.NewWorldContext(world)
	_, err := cardinal.CreateMany(worldCtx, 10, AlphaTest{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 12, BetaTest{})
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

	q1 := cardinal.NewSearch().Entity(filter.Contains(filter.Component[HP]()))
	q2 := cardinal.NewSearch().Entity(filter.Contains(filter.Component[AlphaTest]()))

	tests := []struct {
		search cardinal.Searchable
		count  int
	}{
		{
			search: cardinal.And(q1, q2),
			count:  0,
		},
		{
			search: cardinal.Or(q1, q2),
			count:  40,
		},
		{
			search: cardinal.Not(q1),
			count:  72,
		},
	}
	for _, searchStruct := range tests {
		amt, err := searchStruct.search.Count(worldCtx)
		assert.NilError(t, err)
		assert.Equal(t, amt, searchStruct.count)
	}
}

func TestWhereClauseOnSearch(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[AlphaTest](world))
	assert.NilError(t, cardinal.RegisterComponent[BetaTest](world))
	assert.NilError(t, cardinal.RegisterComponent[GammaTest](world))
	assert.NilError(t, cardinal.RegisterComponent[Player](world))
	assert.NilError(t, cardinal.RegisterComponent[Vampire](world))

	tf.StartWorld()

	worldCtx := cardinal.NewWorldContext(world)
	_, err := cardinal.CreateMany(worldCtx, 10, AlphaTest{})
	assert.NilError(t, err)
	_, err = cardinal.CreateMany(worldCtx, 12, BetaTest{})
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

	q1 := cardinal.NewSearch().Entity(filter.All()).Where(func(
		wCtx cardinal.WorldContext, id types.EntityID) (bool, error) {
		_, err := cardinal.GetComponent[AlphaTest](wCtx, id)
		if err != nil {
			return false, err
		}
		return true, nil
	})

	amt, err := q1.Count(worldCtx)
	assert.NilError(t, err)
	assert.Equal(t, amt, 40)
}
