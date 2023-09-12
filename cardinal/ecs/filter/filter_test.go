package filter_test

import (
	"fmt"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/cql"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/inmem"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

func TestCanFilterByArchetype(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)

	alpha := ecs.NewComponentType[string]("alpha")
	beta := ecs.NewComponentType[string]("beta")
	gamma := ecs.NewComponentType[string]("gamma")

	assert.NilError(t, world.RegisterComponents(alpha, beta, gamma))
	assert.NilError(t, world.LoadGameState())

	subsetCount := 50
	// Make some entities that only have the alpha and beta components
	_, err := world.CreateMany(subsetCount, alpha, beta)
	assert.NilError(t, err)
	// Make some entities that have all 3 component.
	_, err = world.CreateMany(20, alpha, beta, gamma)
	assert.NilError(t, err)

	count := 0
	// Loop over every entity that has exactly the alpha and beta components. There should
	// only be subsetCount entities.
	ecs.NewQuery(filter.Exact(alpha, beta)).Each(world, func(id storage.EntityID) bool {
		count++
		// Make sure the gamma component is not on this entity
		_, err := gamma.Get(world, id)
		assert.ErrorIs(t, err, storage.ErrorComponentNotOnEntity)
		return true
	})

	assert.Equal(t, count, subsetCount)
}

// TestExactVsContains ensures the Exact filter will return a subset of a Contains filter when called
// with the same parameters.
func TestExactVsContains(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	alpha := ecs.NewComponentType[string]("alpha")
	beta := ecs.NewComponentType[string]("beta")
	err := world.RegisterComponents(alpha, beta)
	assert.NilError(t, err)
	alphaCount := 75
	_, err = world.CreateMany(alphaCount, alpha)
	assert.NilError(t, err)
	bothCount := 100
	_, err = world.CreateMany(bothCount, alpha, beta)
	assert.NilError(t, err)
	count := 0
	// Contains(alpha) should return all entities
	ecs.NewQuery(filter.Contains(alpha)).Each(world, func(id storage.EntityID) bool {
		count++
		return true
	})
	assert.Equal(t, count, alphaCount+bothCount)
	count2 := 0
	sameQuery, err := cql.CQLParse("CONTAINS(alpha)", world.GetComponentByName)
	assert.NilError(t, err)
	ecs.NewQuery(sameQuery).Each(world, func(id storage.EntityID) bool {
		count2++
		return true
	})
	assert.Equal(t, count2, alphaCount+bothCount)

	count = 0
	// Contains(beta) should only return the entities that have both components
	ecs.NewQuery(filter.Contains(beta)).Each(world, func(id storage.EntityID) bool {
		count++
		return true
	})
	assert.Equal(t, count, bothCount)

	count2 = 0
	sameQuery, err = cql.CQLParse("CONTAINS(beta)", world.GetComponentByName)
	assert.NilError(t, err)
	ecs.NewQuery(sameQuery).Each(world, func(id storage.EntityID) bool {
		count2++
		return true
	})

	count = 0
	// Exact(alpha) should not return the entities that have both alpha and beta
	ecs.NewQuery(filter.Exact(alpha)).Each(world, func(id storage.EntityID) bool {
		count++
		return true
	})
	assert.Equal(t, count, alphaCount)

	count2 = 0
	sameQuery, err = cql.CQLParse("EXACT(alpha)", world.GetComponentByName)
	assert.NilError(t, err)
	ecs.NewQuery(sameQuery).Each(world, func(id storage.EntityID) bool {
		count2++
		return true
	})
	assert.Equal(t, count2, alphaCount)

	count = 0
	// Exact(alpha, beta) should not return the entities that only have alpha
	ecs.NewQuery(filter.Exact(alpha, beta)).Each(world, func(id storage.EntityID) bool {
		count++
		return true
	})
	assert.Equal(t, count, bothCount)

	count2 = 0
	sameQuery, err = cql.CQLParse("EXACT(alpha, beta)", world.GetComponentByName)
	assert.NilError(t, err)
	ecs.NewQuery(sameQuery).Each(world, func(id storage.EntityID) bool {
		count2++
		return true
	})
	assert.Equal(t, count, bothCount)

	count = 0
	// Make sure the order of alpha/beta doesn't matter
	ecs.NewQuery(filter.Exact(beta, alpha)).Each(world, func(id storage.EntityID) bool {
		count++
		return true
	})
	assert.Equal(t, count, bothCount)

	count2 = 0
	sameQuery, err = cql.CQLParse("EXACT(beta, alpha)", world.GetComponentByName)
	assert.NilError(t, err)
	ecs.NewQuery(sameQuery).Each(world, func(id storage.EntityID) bool {
		count2++
		return true
	})
	assert.Equal(t, count, bothCount)
}

func TestCanGetArchetypeFromEntity(t *testing.T) {
	world := inmem.NewECSWorldForTest(t)
	alpha := ecs.NewComponentType[string]("alpha")
	beta := ecs.NewComponentType[string]("beta")
	assert.NilError(t, world.RegisterComponents(alpha, beta))
	assert.NilError(t, world.LoadGameState())

	wantCount := 50
	ids, err := world.CreateMany(wantCount, alpha, beta)
	assert.NilError(t, err)
	// Make some extra entities that will be ignored. Our queryString later
	// should NOT contain these entities
	_, err = world.CreateMany(20, alpha)
	assert.NilError(t, err)
	id := ids[0]
	comps, err := world.GetComponentsOnEntity(id)
	assert.NilError(t, err)

	count := 0
	ecs.NewQuery(filter.Exact(comps...)).Each(world, func(id storage.EntityID) bool {
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
	ecs.NewQuery(sameQuery).Each(world, func(id storage.EntityID) bool {
		count2++
		return true
	})
	assert.Equal(t, count2, wantCount)

}

func BenchmarkEntityCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		world := inmem.NewECSWorldForTest(b)
		alpha := ecs.NewComponentType[string]("alpha")
		assert.NilError(b, world.RegisterComponents(alpha))
		assert.NilError(b, world.LoadGameState())
		_, err := world.CreateMany(100000, alpha)
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
	world := inmem.NewECSWorldForTest(b)
	alpha := ecs.NewComponentType[string]("alpha")
	beta := ecs.NewComponentType[string]("beta")
	assert.NilError(b, world.RegisterComponents(alpha, beta))
	assert.NilError(b, world.LoadGameState())
	_, err := world.CreateMany(relevantCount, alpha, beta)
	assert.NilError(b, err)
	_, err = world.CreateMany(ignoreCount, alpha)
	assert.NilError(b, err)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		ecs.NewQuery(filter.Exact(alpha, beta)).Each(world, func(id storage.EntityID) bool {
			count++
			return true
		})
		assert.Equal(b, count, relevantCount)
	}
}
