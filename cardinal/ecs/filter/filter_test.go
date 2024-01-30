package filter_test

import (
	"fmt"
	"testing"

	"pkg.world.dev/world-engine/cardinal/ecs/iterators"
	"pkg.world.dev/world-engine/cardinal/types/component"

	"github.com/rs/zerolog"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/cql"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

type gammaComponent struct{}

func (gammaComponent) Name() string {
	return "gamma"
}

func TestGetEverythingFilter(t *testing.T) {
	engine := testutils.NewTestFixture(t, nil).Engine

	assert.NilError(t, ecs.RegisterComponent[Alpha](engine))
	assert.NilError(t, ecs.RegisterComponent[Beta](engine))
	assert.NilError(t, ecs.RegisterComponent[Gamma](engine))

	assert.NilError(t, engine.LoadGameState())

	subsetCount := 50
	eCtx := ecs.NewEngineContext(engine)
	_, err := ecs.CreateMany(eCtx, subsetCount, Alpha{}, Beta{})
	assert.NilError(t, err)
	// Make some entities that have all 3 component.
	_, err = ecs.CreateMany(eCtx, 20, Alpha{}, Beta{}, Gamma{})
	assert.NilError(t, err)

	count := 0
	// Loop over every entity. There should
	// only be 50 + 20 entities.
	q := engine.NewSearch(filter.All())
	err = q.Each(
		func(id entity.ID) bool {
			count++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, subsetCount+20)
}

func TestCanFilterByArchetype(t *testing.T) {
	engine := testutils.NewTestFixture(t, nil).Engine

	assert.NilError(t, ecs.RegisterComponent[Alpha](engine))
	assert.NilError(t, ecs.RegisterComponent[Beta](engine))
	assert.NilError(t, ecs.RegisterComponent[Gamma](engine))

	assert.NilError(t, engine.LoadGameState())

	subsetCount := 50
	eCtx := ecs.NewEngineContext(engine)
	_, err := ecs.CreateMany(eCtx, subsetCount, Alpha{}, Beta{})
	assert.NilError(t, err)
	// Make some entities that have all 3 component.
	_, err = ecs.CreateMany(eCtx, 20, Alpha{}, Beta{}, Gamma{})
	assert.NilError(t, err)

	count := 0
	// Loop over every entity that has exactly the alpha and beta components. There should
	// only be subsetCount entities.
	q := engine.NewSearch(filter.Exact(Alpha{}, Beta{}))
	err = q.Each(
		func(id entity.ID) bool {
			count++
			// Make sure the gamma component is not on this entity
			_, err = ecs.GetComponent[gammaComponent](eCtx, id)
			assert.ErrorIs(t, err, iterators.ErrComponentNotOnEntity)
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, subsetCount)
}

type Alpha struct{}

func (Alpha) Name() string { return "alpha" }

type Beta struct{}

func (Beta) Name() string { return "beta" }

type Gamma struct{}

func (Gamma) Name() string { return "gamma" }

func TestExactVsContains(t *testing.T) {
	engine := testutils.NewTestFixture(t, nil).Engine
	assert.NilError(t, ecs.RegisterComponent[Alpha](engine))
	assert.NilError(t, ecs.RegisterComponent[Beta](engine))

	alphaCount := 75
	eCtx := ecs.NewEngineContext(engine)
	assert.NilError(t, engine.LoadGameState())
	_, err := ecs.CreateMany(eCtx, alphaCount, Alpha{})
	assert.NilError(t, err)
	bothCount := 100
	_, err = ecs.CreateMany(eCtx, bothCount, Alpha{}, Beta{})
	assert.NilError(t, err)
	count := 0
	// Contains(alpha) should return all entities
	q := engine.NewSearch(filter.Contains(Alpha{}))
	err = q.Each(
		func(id entity.ID) bool {
			count++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, alphaCount+bothCount)
	count2 := 0
	sameQuery, err := cql.Parse("CONTAINS(alpha)", engine.GetComponentByName)
	assert.NilError(t, err)
	err = engine.NewSearch(sameQuery).Each(
		func(id entity.ID) bool {
			count2++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count2, alphaCount+bothCount)

	count = 0
	// Contains(beta) should only return the entities that have both components
	q = engine.NewSearch(filter.Contains(Beta{}))
	err = q.Each(
		func(id entity.ID) bool {
			count++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, bothCount)

	count2 = 0
	sameQuery, err = cql.Parse("CONTAINS(beta)", engine.GetComponentByName)
	assert.NilError(t, err)
	err = engine.NewSearch(sameQuery).Each(
		func(id entity.ID) bool {
			count2++
			return true
		},
	)
	assert.NilError(t, err)

	count = 0
	// Exact(alpha) should not return the entities that have both alpha and beta
	q = engine.NewSearch(filter.Exact(Alpha{}))
	err = q.Each(
		func(id entity.ID) bool {
			count++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, alphaCount)

	count2 = 0
	sameQuery, err = cql.Parse("EXACT(alpha)", engine.GetComponentByName)
	assert.NilError(t, err)
	err = engine.NewSearch(sameQuery).Each(
		func(id entity.ID) bool {
			count2++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count2, alphaCount)

	count = 0
	// Exact(alpha, beta) should not return the entities that only have alpha
	q = engine.NewSearch(filter.Exact(Alpha{}, Beta{}))
	err = q.Each(
		func(id entity.ID) bool {
			count++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, bothCount)

	count2 = 0
	sameQuery, err = cql.Parse("EXACT(alpha, beta)", engine.GetComponentByName)
	assert.NilError(t, err)
	err = engine.NewSearch(sameQuery).Each(
		func(id entity.ID) bool {
			count2++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, bothCount)

	count = 0
	// Make sure the order of alpha/beta doesn't matter
	q = engine.NewSearch(filter.Exact(Beta{}, Alpha{}))
	err = q.Each(
		func(id entity.ID) bool {
			count++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, bothCount)

	count2 = 0
	sameQuery, err = cql.Parse("EXACT(beta, alpha)", engine.GetComponentByName)
	assert.NilError(t, err)
	err = engine.NewSearch(sameQuery).Each(
		func(id entity.ID) bool {
			count2++
			return true
		},
	)
	assert.NilError(t, err)
	assert.Equal(t, count, bothCount)
}

func TestCanGetArchetypeFromEntity(t *testing.T) {
	engine := testutils.NewTestFixture(t, nil).Engine
	assert.NilError(t, ecs.RegisterComponent[Alpha](engine))
	assert.NilError(t, ecs.RegisterComponent[Beta](engine))
	assert.NilError(t, engine.LoadGameState())

	wantCount := 50
	eCtx := ecs.NewEngineContext(engine)
	ids, err := ecs.CreateMany(eCtx, wantCount, Alpha{}, Beta{})
	assert.NilError(t, err)
	// Make some extra entities that will be ignored. Our query later
	// should NOT contain these entities
	_, err = ecs.CreateMany(eCtx, 20, Alpha{})
	assert.NilError(t, err)
	id := ids[0]
	comps, err := engine.GameStateManager().GetComponentTypesForEntity(id)
	assert.NilError(t, err)

	count := 0
	err = engine.NewSearch(filter.Exact(component.ConvertComponentMetadatasToComponents(comps)...)).Each(
		func(id entity.ID) bool {
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

	sameQuery, err := cql.Parse(queryString, engine.GetComponentByName)
	assert.NilError(t, err)
	err = engine.NewSearch(sameQuery).Each(
		func(id entity.ID) bool {
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
		engine := testutils.NewTestFixture(b, nil).Engine
		assert.NilError(b, ecs.RegisterComponent[Alpha](engine))
		assert.NilError(b, engine.LoadGameState())
		eCtx := ecs.NewEngineContext(engine)
		_, err := ecs.CreateMany(eCtx, 100000, Alpha{})
		assert.NilError(b, err)
	}
}

// BenchmarkFilterByArchetypeIsNotImpactedByTotalEntityCount verifies that the time it takes to filter
// by a specific archetype depends on the number of entities that have that archetype and NOT the
// total number of entities that have been created.
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
	engine := testutils.NewTestFixture(b, nil).Engine
	assert.NilError(b, ecs.RegisterComponent[Alpha](engine))
	assert.NilError(b, ecs.RegisterComponent[Beta](engine))
	assert.NilError(b, engine.LoadGameState())
	eCtx := ecs.NewEngineContext(engine)
	_, err := ecs.CreateMany(eCtx, relevantCount, Alpha{}, Beta{})
	assert.NilError(b, err)
	_, err = ecs.CreateMany(eCtx, ignoreCount, Alpha{})
	assert.NilError(b, err)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		q := engine.NewSearch(filter.Exact(Alpha{}, Beta{}))
		err = q.Each(
			func(id entity.ID) bool {
				count++
				return true
			},
		)
		assert.NilError(b, err)
		assert.Equal(b, count, relevantCount)
	}
}
