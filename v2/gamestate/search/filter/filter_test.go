package filter_test

import (
	"fmt"
	"testing"

	"github.com/rs/zerolog"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/v2"
	"pkg.world.dev/world-engine/cardinal/v2/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/v2/world"
)

type Alpha struct{}

func (Alpha) Name() string { return "alpha" }

type Beta struct{}

func (Beta) Name() string { return "beta" }

type Gamma struct{}

func (Gamma) Name() string { return "gamma" }

func TestGetEverythingFilter(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)

	assert.NilError(t, world.RegisterComponent[Alpha](tf.World()))
	assert.NilError(t, world.RegisterComponent[Beta](tf.World()))
	assert.NilError(t, world.RegisterComponent[Gamma](tf.World()))

	subsetCount := 50
	assert.NilError(t, world.RegisterInitSystems(tf.World(), func(wCtx world.WorldContext) error {
		_, err := world.CreateMany(wCtx, subsetCount, Alpha{}, Beta{})
		assert.NilError(t, err)
		_, err = world.CreateMany(wCtx, 20, Alpha{}, Beta{}, Gamma{})
		assert.NilError(t, err)
		return nil
	}))

	tf.StartWorld()
	tf.DoTick()

	// Loop over every entity. There should
	// only be 50 + 20 entities.
	count, err := tf.Cardinal.World().Search(filter.All()).Count()
	assert.NilError(t, err)
	assert.Equal(t, count, subsetCount+20)
}

func TestCanFilterByArchetype(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)

	assert.NilError(t, world.RegisterComponent[Alpha](tf.World()))
	assert.NilError(t, world.RegisterComponent[Beta](tf.World()))
	assert.NilError(t, world.RegisterComponent[Gamma](tf.World()))

	subsetCount := 50
	assert.NilError(t, world.RegisterInitSystems(tf.World(), func(wCtx world.WorldContext) error {
		_, err := world.CreateMany(wCtx, subsetCount, Alpha{}, Beta{})
		assert.NilError(t, err)
		// Make some entities that have all 3 component.
		_, err = world.CreateMany(wCtx, 20, Alpha{}, Beta{}, Gamma{})
		assert.NilError(t, err)

		return nil
	}))

	tf.StartWorld()
	tf.DoTick()

	// Loop over every entity that has exactly the alpha and beta components. There should
	// only be subsetCount entities.
	count, err := tf.Cardinal.World().Search(filter.Exact(Alpha{}, Beta{})).Count()
	assert.NilError(t, err)
	assert.Equal(t, count, subsetCount)
}

func TestExactVsContains(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	assert.NilError(t, world.RegisterComponent[Alpha](tf.World()))
	assert.NilError(t, world.RegisterComponent[Beta](tf.World()))

	alphaCount := 75
	alphaBetaCount := 100
	assert.NilError(t, world.RegisterInitSystems(tf.World(), func(wCtx world.WorldContext) error {
		_, err := world.CreateMany(wCtx, alphaCount, Alpha{})
		assert.NilError(t, err)
		_, err = world.CreateMany(wCtx, alphaBetaCount, Alpha{}, Beta{})
		assert.NilError(t, err)
		return nil
	}))

	tf.StartWorld()
	tf.DoTick()

	// Contains(alpha) should return all entities
	count, err := tf.Cardinal.World().Search(filter.Contains(Alpha{})).Count()
	assert.NilError(t, err)
	assert.Equal(t, count, alphaCount+alphaBetaCount)

	// Exact(alpha, beta) should not return the entities that only have alpha
	count2, err := tf.Cardinal.World().Search(filter.Exact(Alpha{}, Beta{})).Count()
	assert.NilError(t, err)
	assert.Equal(t, count2, alphaBetaCount)

	// Contains(beta) should only return the entities that have both components
	count3, err := tf.Cardinal.World().Search(filter.Contains(Beta{})).Count()
	assert.NilError(t, err)
	assert.Equal(t, count3, alphaBetaCount)

	// Exact(alpha) should not return the entities that have both alpha and beta
	count4, err := tf.Cardinal.World().Search(filter.Exact(Alpha{})).Count()
	assert.NilError(t, err)
	assert.Equal(t, count4, alphaCount)
}

func TestCanGetArchetypeFromEntity(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	assert.NilError(t, world.RegisterComponent[Alpha](tf.World()))
	assert.NilError(t, world.RegisterComponent[Beta](tf.World()))

	alphaBetaCount := 50
	alphaCount := 20
	assert.NilError(t, world.RegisterInitSystems(tf.World(), func(wCtx world.WorldContext) error {
		_, err := world.CreateMany(wCtx, alphaBetaCount, Alpha{}, Beta{})
		assert.NilError(t, err)
		_, err = world.CreateMany(wCtx, alphaCount, Alpha{})
		assert.NilError(t, err)
		return nil
	}))

	tf.StartWorld()
	tf.DoTick()

	count, err := tf.Cardinal.World().Search(filter.Exact(Alpha{}, Beta{})).Count()
	assert.NilError(t, err)
	assert.Equal(t, count, alphaBetaCount)

	count2, err := tf.Cardinal.World().Search(filter.Exact(Alpha{})).Count()
	assert.NilError(t, err)
	assert.Equal(t, count2, alphaCount)
}

// BenchmarkFilterByArchetypeIsNotImpactedByTotalEntityCount verifies that the time it takes to filter
// by a specific archetype depends on the number of entities that have that archetype and NOT the
// total number of entities that have been world.Created.
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
	tf := cardinal.NewTestCardinal(b, nil)
	assert.NilError(b, world.RegisterComponent[Alpha](tf.World()))
	assert.NilError(b, world.RegisterComponent[Beta](tf.World()))

	assert.NilError(b, world.RegisterInitSystems(tf.World(), func(wCtx world.WorldContext) error {
		_, err := world.CreateMany(wCtx, relevantCount, Alpha{}, Beta{})
		assert.NilError(b, err)
		_, err = world.CreateMany(wCtx, ignoreCount, Alpha{})
		assert.NilError(b, err)
		return nil
	}))

	tf.StartWorld()
	tf.DoTick()

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		count, err := tf.Cardinal.World().Search(filter.Exact(Alpha{}, Beta{})).Count()
		assert.NilError(b, err)
		assert.Equal(b, count, relevantCount)
	}
}
