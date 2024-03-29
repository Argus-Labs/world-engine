package search_test

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types"
)

type Alpha struct {
	Name1 string
}
type Beta struct {
	Name1 string
}
type Gamma struct {
	Name1 string
}

func (Alpha) Name() string {
	return "alpha"
}

func (Beta) Name() string {
	return "beta"
}

func (Gamma) Name() string {
	return "gamma"
}

func TestSearchExample(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[Alpha](world))
	assert.NilError(t, cardinal.RegisterComponent[Beta](world))
	assert.NilError(t, cardinal.RegisterComponent[Gamma](world))
	tf.StartWorld()

	worldCtx := cardinal.NewWorldContext(world)
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
		count, err = cardinal.NewSearch(worldCtx, tc.filter).Count()
		assert.NilError(t, err, msg)
		assert.Equal(t, tc.want, count, msg)
	}
	amount, err := cardinal.NewSearch(worldCtx, filter.Exact(Alpha{}, Beta{})).Where(func(_ types.EntityID) bool {
		return false
	}).Count()
	assert.NilError(t, err)
	assert.Equal(t, amount, 0)

	counter := 0

	err =
		cardinal.NewSearch(worldCtx, filter.Exact(Alpha{})).
			Where(func(_ types.EntityID) bool { return true }).
			Where(func(_ types.EntityID) bool { return true }).
			Where(func(_ types.EntityID) bool { return true }).
			Where(func(_ types.EntityID) bool { return true }).
			Where(func(_ types.EntityID) bool { return true }).
			Each(func(id types.EntityID) bool {
				comp, err := cardinal.GetComponent[Alpha](worldCtx, id)
				assert.NilError(t, err)
				if counter%2 == 0 {
					comp.Name1 = "BLAH"
				}
				counter++
				err = cardinal.SetComponent[Alpha](worldCtx, id, comp)
				assert.NilError(t, err)
				return true
			})
	assert.NilError(t, err)
	amount, err = cardinal.NewSearch(worldCtx, filter.Exact(Alpha{})).
		Where(func(id types.EntityID) bool {
			comp, err := cardinal.GetComponent[Alpha](worldCtx, id)
			assert.NilError(t, err)
			return comp.Name1 == "BLAH"
		}).Count()
	assert.NilError(t, err)
	assert.Equal(t, amount, 5)
}
