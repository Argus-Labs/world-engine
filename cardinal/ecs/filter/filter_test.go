package filter_test

import (
	"fmt"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/cql"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

type alphaComponent struct{}
type betaComponent struct{}
type gammaComponent struct{}

func (alphaComponent) Name() string {
	return "alpha"
}

func (betaComponent) Name() string {
	return "beta"
}

func (gammaComponent) Name() string {
	return "gamma"
}

func TestCanFilterByArchetype(t *testing.T) {
	world := ecs.NewTestWorld(t)

	assert.NilError(t, ecs.RegisterComponent[Alpha](world))
	assert.NilError(t, ecs.RegisterComponent[Beta](world))
	assert.NilError(t, ecs.RegisterComponent[Gamma](world))

	assert.NilError(t, world.LoadGameState())

	subsetCount := 50
	_, err := ecs.CreateMany(world, subsetCount, Alpha{}, Beta{})
	assert.NilError(t, err)
	// Make some entities that have all 3 component.
	_, err = ecs.CreateMany(world, 20, Alpha{}, Beta{}, Gamma{})
	assert.NilError(t, err)

	count := 0
	// Loop over every entity that has exactly the alpha and beta components. There should
	// only be subsetCount entities.
	q, err := world.NewSearch(ecs.Exact(Alpha{}, Beta{}))
	assert.NilError(t, err)
	q.Each(world, func(id entity.ID) bool {
		count++
		// Make sure the gamma component is not on this entity
		_, err := ecs.GetComponent[gammaComponent](world, id)
		//_, err := gamma.Get(world, id)
		assert.ErrorIs(t, err, storage.ErrorComponentNotOnEntity)
		return true
	})

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
	_, err := ecs.CreateMany(world, alphaCount, Alpha{})
	assert.NilError(t, err)
	bothCount := 100
	_, err = ecs.CreateMany(world, bothCount, Alpha{}, Beta{})
	assert.NilError(t, err)
	count := 0
	// Contains(alpha) should return all entities
	q, err := world.NewSearch(ecs.Contains(Alpha{}))
	assert.NilError(t, err)
	q.Each(world, func(id entity.ID) bool {
		count++
		return true
	})
	assert.Equal(t, count, alphaCount+bothCount)
	count2 := 0
	sameQuery, err := cql.CQLParse("CONTAINS(alpha)", world.GetComponentByName)
	assert.NilError(t, err)
	ecs.NewSearch(sameQuery).Each(world, func(id entity.ID) bool {
		count2++
		return true
	})
	assert.Equal(t, count2, alphaCount+bothCount)

	count = 0
	// Contains(beta) should only return the entities that have both components
	q, err = world.NewSearch(ecs.Contains(Beta{}))
	assert.NilError(t, err)
	q.Each(world, func(id entity.ID) bool {
		count++
		return true
	})
	assert.Equal(t, count, bothCount)

	count2 = 0
	sameQuery, err = cql.CQLParse("CONTAINS(beta)", world.GetComponentByName)
	assert.NilError(t, err)
	ecs.NewSearch(sameQuery).Each(world, func(id entity.ID) bool {
		count2++
		return true
	})

	count = 0
	// Exact(alpha) should not return the entities that have both alpha and beta
	q, err = world.NewSearch(ecs.Exact(Alpha{}))
	assert.NilError(t, err)
	q.Each(world, func(id entity.ID) bool {
		count++
		return true
	})
	assert.Equal(t, count, alphaCount)

	count2 = 0
	sameQuery, err = cql.CQLParse("EXACT(alpha)", world.GetComponentByName)
	assert.NilError(t, err)
	ecs.NewSearch(sameQuery).Each(world, func(id entity.ID) bool {
		count2++
		return true
	})
	assert.Equal(t, count2, alphaCount)

	count = 0
	// Exact(alpha, beta) should not return the entities that only have alpha
	q, err = world.NewSearch(ecs.Exact(Alpha{}, Beta{}))
	assert.NilError(t, err)
	q.Each(world, func(id entity.ID) bool {
		count++
		return true
	})
	assert.Equal(t, count, bothCount)

	count2 = 0
	sameQuery, err = cql.CQLParse("EXACT(alpha, beta)", world.GetComponentByName)
	assert.NilError(t, err)
	ecs.NewSearch(sameQuery).Each(world, func(id entity.ID) bool {
		count2++
		return true
	})
	assert.Equal(t, count, bothCount)

	count = 0
	// Make sure the order of alpha/beta doesn't matter
	q, err = world.NewSearch(ecs.Exact(Beta{}, Alpha{}))
	assert.NilError(t, err)
	q.Each(world, func(id entity.ID) bool {
		count++
		return true
	})
	assert.Equal(t, count, bothCount)

	count2 = 0
	sameQuery, err = cql.CQLParse("EXACT(beta, alpha)", world.GetComponentByName)
	assert.NilError(t, err)
	ecs.NewSearch(sameQuery).Each(world, func(id entity.ID) bool {
		count2++
		return true
	})
	assert.Equal(t, count, bothCount)
}

func TestCanGetArchetypeFromEntity(t *testing.T) {
	world := ecs.NewTestWorld(t)
	assert.NilError(t, ecs.RegisterComponent[Alpha](world))
	assert.NilError(t, ecs.RegisterComponent[Beta](world))
	assert.NilError(t, world.LoadGameState())

	wantCount := 50
	ids, err := ecs.CreateMany(world, wantCount, Alpha{}, Beta{})
	assert.NilError(t, err)
	// Make some extra entities that will be ignored. Our query later
	// should NOT contain these entities
	_, err = ecs.CreateMany(world, 20, Alpha{})
	assert.NilError(t, err)
	id := ids[0]
	comps, err := world.StoreManager().GetComponentTypesForEntity(id)
	assert.NilError(t, err)

	count := 0
	ecs.NewSearch(filter.Exact(comps...)).Each(world, func(id entity.ID) bool {
		count++
		return true
	})
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

	sameQuery, err := cql.CQLParse(queryString, world.GetComponentByName)
	assert.NilError(t, err)
	ecs.NewSearch(sameQuery).Each(world, func(id entity.ID) bool {
		count2++
		return true
	})
	assert.Equal(t, count2, wantCount)

}

func BenchmarkEntityCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		world := ecs.NewTestWorld(b)
		assert.NilError(b, ecs.RegisterComponent[Alpha](world))
		assert.NilError(b, world.LoadGameState())
		_, err := ecs.CreateMany(world, 100000, Alpha{})
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
	_, err := ecs.CreateMany(world, relevantCount, Alpha{}, Beta{})
	assert.NilError(b, err)
	_, err = ecs.CreateMany(world, ignoreCount, Alpha{})
	assert.NilError(b, err)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		q, err := world.NewSearch(ecs.Exact(Alpha{}, Beta{}))
		assert.NilError(b, err)
		q.Each(world, func(id entity.ID) bool {
			count++
			return true
		})
		assert.Equal(b, count, relevantCount)
	}
}
