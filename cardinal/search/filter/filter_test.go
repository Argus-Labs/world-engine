package filter_test

import (
	"fmt"
	"testing"

	"github.com/rs/zerolog"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/server/handler/cql"
	"pkg.world.dev/world-engine/cardinal/types"
)

type Alpha struct{}

func (Alpha) Name() string { return "alpha" }

type Beta struct{}

func (Beta) Name() string { return "beta" }

type Gamma struct{}

func (Gamma) Name() string { return "gamma" }

func TestGetEverythingFilter(t *testing.T) {
	tf := cardinal.NewTestFixture(t, nil)
	world := tf.World

	assert.NilError(t, cardinal.RegisterComponent[Alpha](world))
	assert.NilError(t, cardinal.RegisterComponent[Beta](world))
	assert.NilError(t, cardinal.RegisterComponent[Gamma](world))

	tf.StartWorld()

	subsetCount := 50
	wCtx := cardinal.NewWorldContext(world)
	_, err := cardinal.CreateMany(wCtx, subsetCount, Alpha{}, Beta{})
	assert.NilError(t, err)
	// Make some entities that have all 3 component.
	_, err = cardinal.CreateMany(wCtx, 20, Alpha{}, Beta{}, Gamma{})
	assert.NilError(t, err)

	count := 0
	// Loop over every entity. There should
	// only be 50 + 20 entities.
	q := cardinal.NewSearch().Entity(filter.All())
	err = q.Each(wCtx,
		func(types.EntityID) bool {
			count++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, subsetCount+20)
}

func TestCanFilterByArchetype(t *testing.T) {
	tf := cardinal.NewTestFixture(t, nil)
	world := tf.World

	assert.NilError(t, cardinal.RegisterComponent[Alpha](world))
	assert.NilError(t, cardinal.RegisterComponent[Beta](world))
	assert.NilError(t, cardinal.RegisterComponent[Gamma](world))

	tf.StartWorld()

	subsetCount := 50
	wCtx := cardinal.NewWorldContext(world)
	_, err := cardinal.CreateMany(wCtx, subsetCount, Alpha{}, Beta{})
	assert.NilError(t, err)
	// Make some entities that have all 3 component.
	_, err = cardinal.CreateMany(wCtx, 20, Alpha{}, Beta{}, Gamma{})
	assert.NilError(t, err)

	count := 0
	// Loop over every entity that has exactly the alpha and beta components. There should
	// only be subsetCount entities.
	q := cardinal.NewSearch().Entity(filter.Exact(
		filter.Component[Alpha](),
		filter.Component[Beta]()))
	err = q.Each(wCtx,
		func(id types.EntityID) bool {
			count++
			// Make sure the gamma component is not on this entity
			_, err = cardinal.GetComponent[Gamma](wCtx, id)
			assert.ErrorIs(t, err, iterators.ErrComponentNotOnEntity)
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, subsetCount)
}

func TestExactVsContains(t *testing.T) {
	tf := cardinal.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[Alpha](world))
	assert.NilError(t, cardinal.RegisterComponent[Beta](world))

	alphaCount := 75
	wCtx := cardinal.NewWorldContext(world)
	tf.StartWorld()
	_, err := cardinal.CreateMany(wCtx, alphaCount, Alpha{})
	assert.NilError(t, err)
	bothCount := 100
	_, err = cardinal.CreateMany(wCtx, bothCount, Alpha{}, Beta{})
	assert.NilError(t, err)
	count := 0
	// Contains(alpha) should return all entities
	q := cardinal.NewSearch().Entity(filter.Contains(filter.Component[Alpha]()))
	err = q.Each(wCtx,
		func(types.EntityID) bool {
			count++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, alphaCount+bothCount)
	count2 := 0

	getComponentByName := func(name string) (types.Component, error) {
		comp, err := world.GetComponentByName(name)
		if err != nil {
			return nil, err
		}
		return comp, nil
	}

	sameQuery, err := cql.Parse("CONTAINS(alpha)", getComponentByName)
	assert.NilError(t, err)
	err = cardinal.NewLegacySearch(sameQuery).Each(wCtx,
		func(types.EntityID) bool {
			count2++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count2, alphaCount+bothCount)

	count = 0
	// Contains(beta) should only return the entities that have both components
	q = cardinal.NewSearch().Entity(filter.Contains(filter.Component[Beta]()))
	err = q.Each(wCtx,
		func(types.EntityID) bool {
			count++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, bothCount)

	count2 = 0
	sameQuery, err = cql.Parse("CONTAINS(beta)", getComponentByName)
	assert.NilError(t, err)
	err = cardinal.NewLegacySearch(sameQuery).Each(wCtx,
		func(types.EntityID) bool {
			count2++
			return true
		},
	)
	assert.NilError(t, err)

	count = 0
	// Exact(alpha) should not return the entities that have both alpha and beta
	q = cardinal.NewSearch().Entity(filter.Exact(filter.Component[Alpha]()))
	err = q.Each(wCtx,
		func(types.EntityID) bool {
			count++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, alphaCount)

	count2 = 0
	sameQuery, err = cql.Parse("EXACT(alpha)", getComponentByName)
	assert.NilError(t, err)
	err = cardinal.NewLegacySearch(sameQuery).Each(wCtx,
		func(types.EntityID) bool {
			count2++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count2, alphaCount)

	count = 0
	// Exact(alpha, beta) should not return the entities that only have alpha
	q = cardinal.NewLegacySearch(filter.Exact(filter.Component[Alpha](), filter.Component[Beta]()))
	err = q.Each(wCtx,
		func(types.EntityID) bool {
			count++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, bothCount)

	count2 = 0
	sameQuery, err = cql.Parse("EXACT(alpha, beta)", getComponentByName)
	assert.NilError(t, err)
	err = cardinal.NewLegacySearch(sameQuery).Each(wCtx,
		func(types.EntityID) bool {
			count2++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, bothCount)

	count = 0
	// Make sure the order of alpha/beta doesn't matter
	q = cardinal.NewSearch().Entity(filter.Exact(
		filter.Component[Beta](),
		filter.Component[Alpha]()))
	err = q.Each(wCtx,
		func(types.EntityID) bool {
			count++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, bothCount)

	count2 = 0
	sameQuery, err = cql.Parse("EXACT(beta, alpha)", getComponentByName)
	assert.NilError(t, err)
	err = cardinal.NewLegacySearch(sameQuery).Each(wCtx,
		func(types.EntityID) bool {
			count2++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, bothCount)
}

func TestCanGetArchetypeFromEntity(t *testing.T) {
	tf := cardinal.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[Alpha](world))
	assert.NilError(t, cardinal.RegisterComponent[Beta](world))
	tf.StartWorld()

	wantCount := 50
	wCtx := cardinal.NewWorldContext(world)
	ids, err := cardinal.CreateMany(wCtx, wantCount, Alpha{}, Beta{})
	assert.NilError(t, err)
	// Make some extra entities that will be ignored. Our query later
	// should NOT contain these entities
	_, err = cardinal.CreateMany(wCtx, 20, Alpha{})
	assert.NilError(t, err)
	id := ids[0]
	comps, err := world.GameStateManager().GetComponentTypesForEntity(id)
	assert.NilError(t, err)

	count := 0
	err = cardinal.NewLegacySearch(
		filter.Exact(filter.ConvertComponentMetadatasToComponentWrappers(comps)...)).Each(wCtx,
		func(types.EntityID) bool {
			count++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, wantCount)

	count2 := 0

	queryString := "EXACT("
	for i, c := range comps {
		queryString += c.Name()
		if i < len(comps)-1 {
			queryString += ", "
		}
	}
	queryString += ")"

	getComponentByName := func(name string) (types.Component, error) {
		comp, err := world.GetComponentByName(name)
		if err != nil {
			return nil, err
		}
		return comp, nil
	}

	sameQuery, err := cql.Parse(queryString, getComponentByName)
	assert.NilError(t, err)
	err = cardinal.NewLegacySearch(sameQuery).Each(wCtx,
		func(types.EntityID) bool {
			count2++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count2, wantCount)
}

func BenchmarkEntityCreation(b *testing.B) {
	zerolog.SetGlobalLevel(zerolog.Disabled)
	for i := 0; i < b.N; i++ {
		tf := cardinal.NewTestFixture(b, nil)
		world := tf.World
		assert.NilError(b, cardinal.RegisterComponent[Alpha](world))
		tf.StartWorld()
		wCtx := cardinal.NewWorldContext(world)
		_, err := cardinal.CreateMany(wCtx, 100000, Alpha{})
		assert.NilError(b, err)
	}
}

// BenchmarkFilterByArchetypeIsNotImpactedByTotalEntityCount verifies that the time it takes to filter
// by a specific archetype depends on the number of entities that have that archetype and NOT the
// total number of entities that have been cardinal.Created.
func BenchmarkFilterByArchetypeIsNotImpactedByTotalEntityCount(b *testing.B) {
	relevantCount := 100
	zerolog.SetGlobalLevel(zerolog.Disabled)
	for i := 10; i <= 10000; i *= 10 {
		ignoreCount := i
		b.Run(
			fmt.Sprintf("IgnoreCount:%d", ignoreCount), func(b *testing.B) {
				helperArchetypeFilter(b, relevantCount, ignoreCount)
			},
		)
	}
}

func helperArchetypeFilter(b *testing.B, relevantCount, ignoreCount int) {
	b.StopTimer()
	tf := cardinal.NewTestFixture(b, nil)
	world := tf.World
	assert.NilError(b, cardinal.RegisterComponent[Alpha](world))
	assert.NilError(b, cardinal.RegisterComponent[Beta](world))
	tf.StartWorld()
	wCtx := cardinal.NewWorldContext(world)
	_, err := cardinal.CreateMany(wCtx, relevantCount, Alpha{}, Beta{})
	assert.NilError(b, err)
	_, err = cardinal.CreateMany(wCtx, ignoreCount, Alpha{})
	assert.NilError(b, err)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		q := cardinal.NewSearch().Entity(filter.Exact(
			filter.Component[Alpha](),
			filter.Component[Beta]()))
		err = q.Each(wCtx,
			func(types.EntityID) bool {
				count++
				return true
			},
		)
		assert.NilError(b, err)
		assert.Equal(b, count, relevantCount)
	}
}
