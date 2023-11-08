package filter_test

import (
	"fmt"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/cql"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

type gammaComponent struct{}

func (gammaComponent) Name() string {
	return "gamma"
}

func TestGetEverythingFilter(t *testing.T) {
	world := ecs.NewTestWorld(t)

	assert.NilError(t, ecs.RegisterComponent[Alpha](world))
	assert.NilError(t, ecs.RegisterComponent[Beta](world))
	assert.NilError(t, ecs.RegisterComponent[Gamma](world))

	assert.NilError(t, world.LoadGameState())

	subsetCount := 50
	wCtx := ecs.NewWorldContext(world)
	_, err := component.CreateMany(wCtx, subsetCount, Alpha{}, Beta{})
	assert.NilError(t, err)
	// Make some entities that have all 3 component.
	_, err = component.CreateMany(wCtx, 20, Alpha{}, Beta{}, Gamma{})
	assert.NilError(t, err)

	count := 0
	// Loop over every entity. There should
	// only be 50 + 20 entities.
	q, err := wCtx.NewSearch(ecs.All())
	assert.NilError(t, err)
	err = q.Each(wCtx, func(id entity.ID) bool {
		count++
		return true
	})
	assert.NilError(t, err)
	assert.Equal(t, count, subsetCount+20)
}

func TestCanFilterByArchetype(t *testing.T) {
	world := ecs.NewTestWorld(t)

	assert.NilError(t, ecs.RegisterComponent[Alpha](world))
	assert.NilError(t, ecs.RegisterComponent[Beta](world))
	assert.NilError(t, ecs.RegisterComponent[Gamma](world))

	assert.NilError(t, world.LoadGameState())

	subsetCount := 50
	wCtx := ecs.NewWorldContext(world)
	_, err := component.CreateMany(wCtx, subsetCount, Alpha{}, Beta{})
	assert.NilError(t, err)
	// Make some entities that have all 3 component.
	_, err = component.CreateMany(wCtx, 20, Alpha{}, Beta{}, Gamma{})
	assert.NilError(t, err)

	count := 0
	// Loop over every entity that has exactly the alpha and beta components. There should
	// only be subsetCount entities.
	q, err := wCtx.NewSearch(ecs.Exact(Alpha{}, Beta{}))
	assert.NilError(t, err)
	err = q.Each(wCtx, func(id entity.ID) bool {
		count++
		// Make sure the gamma component is not on this entity
		_, err = component.GetComponent[gammaComponent](wCtx, id)
		assert.ErrorIs(t, err, storage.ErrComponentNotOnEntity)
		return true
	})
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
	world := ecs.NewTestWorld(t)
	assert.NilError(t, ecs.RegisterComponent[Alpha](world))
	assert.NilError(t, ecs.RegisterComponent[Beta](world))

	alphaCount := 75
	wCtx := ecs.NewWorldContext(world)
	_, err := component.CreateMany(wCtx, alphaCount, Alpha{})
	assert.NilError(t, err)
	bothCount := 100
	_, err = component.CreateMany(wCtx, bothCount, Alpha{}, Beta{})
	assert.NilError(t, err)
	count := 0
	// Contains(alpha) should return all entities
	q, err := world.NewSearch(ecs.Contains(Alpha{}))
	assert.NilError(t, err)
	err = q.Each(wCtx, func(id entity.ID) bool {
		count++
		return true
	})
	assert.NilError(t, err)
	assert.Equal(t, count, alphaCount+bothCount)
	count2 := 0
	sameQuery, err := cql.Parse("CONTAINS(alpha)", world.GetComponentByName)
	assert.NilError(t, err)
	err = ecs.NewSearch(sameQuery).Each(wCtx, func(id entity.ID) bool {
		count2++
		return true
	})
	assert.NilError(t, err)
	assert.Equal(t, count2, alphaCount+bothCount)

	count = 0
	// Contains(beta) should only return the entities that have both components
	q, err = world.NewSearch(ecs.Contains(Beta{}))
	assert.NilError(t, err)
	err = q.Each(wCtx, func(id entity.ID) bool {
		count++
		return true
	})
	assert.NilError(t, err)
	assert.Equal(t, count, bothCount)

	count2 = 0
	sameQuery, err = cql.Parse("CONTAINS(beta)", world.GetComponentByName)
	assert.NilError(t, err)
	err = ecs.NewSearch(sameQuery).Each(wCtx, func(id entity.ID) bool {
		count2++
		return true
	})
	assert.NilError(t, err)

	count = 0
	// Exact(alpha) should not return the entities that have both alpha and beta
	q, err = world.NewSearch(ecs.Exact(Alpha{}))
	assert.NilError(t, err)
	err = q.Each(wCtx, func(id entity.ID) bool {
		count++
		return true
	})
	assert.NilError(t, err)
	assert.Equal(t, count, alphaCount)

	count2 = 0
	sameQuery, err = cql.Parse("EXACT(alpha)", world.GetComponentByName)
	assert.NilError(t, err)
	err = ecs.NewSearch(sameQuery).Each(wCtx, func(id entity.ID) bool {
		count2++
		return true
	})
	assert.NilError(t, err)
	assert.Equal(t, count2, alphaCount)

	count = 0
	// Exact(alpha, beta) should not return the entities that only have alpha
	q, err = world.NewSearch(ecs.Exact(Alpha{}, Beta{}))
	assert.NilError(t, err)
	err = q.Each(wCtx, func(id entity.ID) bool {
		count++
		return true
	})
	assert.NilError(t, err)
	assert.Equal(t, count, bothCount)

	count2 = 0
	sameQuery, err = cql.Parse("EXACT(alpha, beta)", world.GetComponentByName)
	assert.NilError(t, err)
	err = ecs.NewSearch(sameQuery).Each(wCtx, func(id entity.ID) bool {
		count2++
		return true
	})
	assert.NilError(t, err)
	assert.Equal(t, count, bothCount)

	count = 0
	// Make sure the order of alpha/beta doesn't matter
	q, err = world.NewSearch(ecs.Exact(Beta{}, Alpha{}))
	assert.NilError(t, err)
	err = q.Each(wCtx, func(id entity.ID) bool {
		count++
		return true
	})
	assert.NilError(t, err)
	assert.Equal(t, count, bothCount)

	count2 = 0
	sameQuery, err = cql.Parse("EXACT(beta, alpha)", world.GetComponentByName)
	assert.NilError(t, err)
	err = ecs.NewSearch(sameQuery).Each(wCtx, func(id entity.ID) bool {
		count2++
		return true
	})
	assert.NilError(t, err)
	assert.Equal(t, count, bothCount)
}

func TestCanGetArchetypeFromEntity(t *testing.T) {
	world := ecs.NewTestWorld(t)
	assert.NilError(t, ecs.RegisterComponent[Alpha](world))
	assert.NilError(t, ecs.RegisterComponent[Beta](world))
	assert.NilError(t, world.LoadGameState())

	wantCount := 50
	wCtx := ecs.NewWorldContext(world)
	ids, err := component.CreateMany(wCtx, wantCount, Alpha{}, Beta{})
	assert.NilError(t, err)
	// Make some extra entities that will be ignored. Our query later
	// should NOT contain these entities
	_, err = component.CreateMany(wCtx, 20, Alpha{})
	assert.NilError(t, err)
	id := ids[0]
	comps, err := world.StoreManager().GetComponentTypesForEntity(id)
	assert.NilError(t, err)

	count := 0
	err = ecs.NewSearch(filter.Exact(comps...)).Each(wCtx, func(id entity.ID) bool {
		count++
		return true
	})
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

	sameQuery, err := cql.Parse(queryString, world.GetComponentByName)
	assert.NilError(t, err)
	err = ecs.NewSearch(sameQuery).Each(wCtx, func(id entity.ID) bool {
		count2++
		return true
	})
	assert.NilError(t, err)
	assert.Equal(t, count2, wantCount)
}

func BenchmarkEntityCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		world := ecs.NewTestWorld(b)
		assert.NilError(b, ecs.RegisterComponent[Alpha](world))
		assert.NilError(b, world.LoadGameState())
		wCtx := ecs.NewWorldContext(world)
		_, err := component.CreateMany(wCtx, 100000, Alpha{})
		assert.NilError(b, err)
	}
}

// BenchmarkFilterByArchetypeIsNotImpactedByTotalEntityCount verifies that the time it takes to filter
// by a specific archetype depends on the number of entities that have that archetype and NOT the
// total number of entities that have been created.
func BenchmarkFilterByArchetypeIsNotImpactedByTotalEntityCount(b *testing.B) {
	relevantCount := 100
	for i := 10; i <= 10000; i *= 10 {
		ignoreCount := i
		b.Run(fmt.Sprintf("IgnoreCount:%d", ignoreCount), func(b *testing.B) {
			helperArchetypeFilter(b, relevantCount, ignoreCount)
		})
	}
}

func helperArchetypeFilter(b *testing.B, relevantCount, ignoreCount int) {
	b.StopTimer()
	world := ecs.NewTestWorld(b)
	assert.NilError(b, ecs.RegisterComponent[Alpha](world))
	assert.NilError(b, ecs.RegisterComponent[Beta](world))
	assert.NilError(b, world.LoadGameState())
	wCtx := ecs.NewWorldContext(world)
	_, err := component.CreateMany(wCtx, relevantCount, Alpha{}, Beta{})
	assert.NilError(b, err)
	_, err = component.CreateMany(wCtx, ignoreCount, Alpha{})
	assert.NilError(b, err)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		var q *ecs.Search
		q, err = world.NewSearch(ecs.Exact(Alpha{}, Beta{}))
		assert.NilError(b, err)
		err = q.Each(wCtx, func(id entity.ID) bool {
			count++
			return true
		})
		assert.NilError(b, err)
		assert.Equal(b, count, relevantCount)
	}
}
