package search_test

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/gamestate/search"
	"pkg.world.dev/world-engine/cardinal/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/world"
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
	Player string
}

type Vampire struct {
	Vampire bool
}

type HP struct {
	Amount int
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
	tf := cardinal.NewTestCardinal(t, nil)
	assert.NilError(t, world.RegisterComponent[AlphaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[BetaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[GammaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[HP](tf.World()))

	assert.NilError(t, world.RegisterInitSystems(tf.World(), func(ctx world.WorldContext) error {
		_, err := world.CreateMany(ctx, 10, AlphaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, BetaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, AlphaTest{}, GammaTest{})
		assert.NilError(t, err)

		hpids, err := world.CreateMany(ctx, 10, HP{})
		assert.NilError(t, err)
		for i, id := range hpids {
			err = world.SetComponent[HP](ctx, id, &HP{Amount: i})
			assert.NilError(t, err)
		}
		return nil
	}))

	tf.StartWorld()
	tf.DoTick()

	err := tf.Cardinal.World().View(func(wCtx world.WorldContextReadOnly) error {
		amt, err := wCtx.Search(filter.Not(filter.Or(
			filter.Contains(filter.Component[AlphaTest]()),
			filter.Contains(filter.Component[BetaTest]()),
			filter.Contains(filter.Component[GammaTest]())),
		)).Where(func(_ types.EntityID) (bool, error) {
			return true, nil
		}).Count()
		assert.NilError(t, err)
		assert.Equal(t, amt, 10)

		q := wCtx.Search(filter.Not(filter.Or(
			filter.Contains(filter.Component[AlphaTest]()),
			filter.Contains(filter.Component[BetaTest]()),
			filter.Contains(filter.Component[GammaTest]())),
		)).Where(func(id types.EntityID) (bool, error) {
			c, err := world.GetComponent[HP](wCtx, id)
			if err != nil {
				return false, err
			}
			if c.Amount < 3 {
				return true, nil
			}
			return false, nil
		})

		amt, err = q.Count()
		assert.NilError(t, err)
		assert.Equal(t, amt, 3)

		ids, err := q.Collect()
		assert.NilError(t, err)
		assert.True(t, areIDsSorted(ids))
		return nil
	})
	assert.NilError(t, err)
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

func TestSearch_Integration(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	assert.NilError(t, world.RegisterComponent[AlphaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[BetaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[GammaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[Player](tf.World()))
	assert.NilError(t, world.RegisterComponent[Vampire](tf.World()))
	assert.NilError(t, world.RegisterComponent[HP](tf.World()))
	assert.NilError(t, world.RegisterInitSystems(tf.World(), func(ctx world.WorldContext) error {
		_, err := world.CreateMany(ctx, 10, AlphaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, BetaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, AlphaTest{}, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, BetaTest{}, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, AlphaTest{}, BetaTest{}, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 1, &Player{Player: "AnyName"}, &Vampire{Vampire: true}, &HP{Amount: 0})
		return nil
	}))

	tf.StartWorld()
	tf.DoTick()

	tf.Cardinal.World().View(func(wCtx world.WorldContextReadOnly) error {
		testCases := []struct {
			name   string
			search *search.Search
			want   int
		}{
			{
				"does not have alpha",
				wCtx.Search(filter.Not(filter.Contains(AlphaTest{}))).
					Where(func(_ types.EntityID) (bool, error) {
						return true, nil
					}),
				31,
			},
			{
				"exactly alpha",
				wCtx.Search(filter.Exact(AlphaTest{})),
				10,
			},
			{
				"contains alpha",
				wCtx.Search(filter.Contains(AlphaTest{})),
				30,
			},
			{
				"all",
				wCtx.Search(filter.All()),
				61,
			},
		}
		for _, tc := range testCases {
			msg := "problem with " + tc.name
			var count int
			count, err := tc.search.Count()
			assert.NilError(t, err, msg)
			assert.Equal(t, tc.want, count, msg)
		}

		amount, err := wCtx.Search(filter.Exact(
			filter.Component[AlphaTest](),
			filter.Component[BetaTest]())).
			Where(func(_ types.EntityID) (bool, error) {
				return false, nil
			}).Count()
		assert.NilError(t, err)
		assert.Equal(t, amount, 0)
		return nil
	})
}

func TestSearch_Exact_ReturnsExactComponentMatch(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	assert.NilError(t, world.RegisterComponent[AlphaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[BetaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[GammaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[Player](tf.World()))
	assert.NilError(t, world.RegisterComponent[Vampire](tf.World()))
	assert.NilError(t, world.RegisterComponent[HP](tf.World()))
	assert.NilError(t, world.RegisterInitSystems(tf.World(), func(ctx world.WorldContext) error {
		_, err := world.CreateMany(ctx, 10, AlphaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 12, BetaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, AlphaTest{}, BetaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, AlphaTest{}, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, BetaTest{}, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, AlphaTest{}, BetaTest{}, GammaTest{})
		assert.NilError(t, err)
		return nil
	}))

	tf.StartWorld()
	tf.DoTick()

	tf.Cardinal.World().View(func(wCtx world.WorldContextReadOnly) error {
		amt, err := wCtx.Search(filter.Exact(filter.Component[BetaTest]())).Count()
		assert.NilError(t, err)
		assert.Equal(t, amt, 12)
		return nil
	})
}

func TestSearch_Contains_ReturnsEntityThatContainsComponents(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	assert.NilError(t, world.RegisterComponent[AlphaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[BetaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[GammaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[Player](tf.World()))
	assert.NilError(t, world.RegisterComponent[Vampire](tf.World()))
	assert.NilError(t, world.RegisterComponent[HP](tf.World()))
	assert.NilError(t, world.RegisterInitSystems(tf.World(), func(ctx world.WorldContext) error {
		_, err := world.CreateMany(ctx, 10, AlphaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 12, BetaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, AlphaTest{}, BetaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, AlphaTest{}, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, BetaTest{}, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, AlphaTest{}, BetaTest{}, GammaTest{})
		assert.NilError(t, err)
		return nil
	}))

	tf.StartWorld()
	tf.DoTick()

	tf.Cardinal.World().View(func(wCtx world.WorldContextReadOnly) error {
		amt, err := wCtx.Search(filter.Contains(filter.Component[BetaTest]())).Count()
		assert.NilError(t, err)
		assert.Equal(t, amt, 42)
		return nil
	})
}

func TestSearch_ComponentNotRegistered_ReturnsZeroEntityWithNoError(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	assert.NilError(t, world.RegisterComponent[AlphaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[BetaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[GammaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[Player](tf.World()))
	assert.NilError(t, world.RegisterComponent[Vampire](tf.World()))
	assert.NilError(t, world.RegisterInitSystems(tf.World(), func(ctx world.WorldContext) error {
		_, err := world.CreateMany(ctx, 10, AlphaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 12, BetaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, AlphaTest{}, BetaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, AlphaTest{}, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, BetaTest{}, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, AlphaTest{}, BetaTest{}, GammaTest{})
		assert.NilError(t, err)
		return nil
	}))

	tf.StartWorld()
	tf.DoTick()

	tf.Cardinal.World().View(func(wCtx world.WorldContextReadOnly) error {
		amt, err := wCtx.Search(filter.Contains(HP{})).Count()
		assert.NilError(t, err)
		assert.Equal(t, amt, 0)
		return nil
	})
}

func TestWhereClauseOnSearch(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	assert.NilError(t, world.RegisterComponent[AlphaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[BetaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[GammaTest](tf.World()))
	assert.NilError(t, world.RegisterComponent[Player](tf.World()))
	assert.NilError(t, world.RegisterComponent[Vampire](tf.World()))
	assert.NilError(t, world.RegisterInitSystems(tf.World(), func(ctx world.WorldContext) error {
		_, err := world.CreateMany(ctx, 10, AlphaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 12, BetaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, AlphaTest{}, BetaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, AlphaTest{}, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, BetaTest{}, GammaTest{})
		assert.NilError(t, err)
		_, err = world.CreateMany(ctx, 10, AlphaTest{}, BetaTest{}, GammaTest{})
		assert.NilError(t, err)
		return nil
	}))

	tf.StartWorld()
	tf.DoTick()

	err := tf.Cardinal.World().View(func(wCtx world.WorldContextReadOnly) error {
		amt, err := wCtx.Search(filter.All()).Where(func(id types.EntityID) (bool, error) {
			_, err := world.GetComponent[AlphaTest](wCtx, id)
			if err != nil {
				return false, err
			}
			return true, nil
		}).Count()
		assert.NilError(t, err)
		assert.Equal(t, amt, 40)
		return nil
	})
	assert.NilError(t, err)
}
