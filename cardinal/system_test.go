package cardinal_test

import (
	"errors"
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types"
)

func HealthSystem(wCtx cardinal.Context) error {
	var errs []error
	errs = append(errs, cardinal.NewSearch().Entity(filter.
		Exact(filter.Component[Health]())).
		Each(wCtx, func(id types.EntityID) bool {
			errs = append(errs, cardinal.UpdateComponent[Health](wCtx, id, func(h *Health) *Health {
				h.Value++
				return h
			}))
			return true
		}))
	err := errors.Join(errs...)
	return err
}

func TestSystemExample(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world, doTick := tf.World, tf.DoTick
	assert.NilError(t, cardinal.RegisterComponent[Health](world))
	err := cardinal.RegisterSystems(world, HealthSystem)
	assert.NilError(t, err)

	worldCtx := cardinal.NewWorldContext(world)
	doTick()
	ids, err := cardinal.CreateMany(worldCtx, 100, Health{})
	assert.NilError(t, err)

	// Make sure we have 100 entities all with a health of 0
	for _, id := range ids {
		var health *Health
		health, err = cardinal.GetComponent[Health](worldCtx, id)
		assert.NilError(t, err)
		assert.Equal(t, 0, health.Value)
	}

	// do 5 ticks
	for i := 0; i < 5; i++ {
		doTick()
	}

	// Health should be 5 for everyone
	for _, id := range ids {
		var health *Health
		health, err = cardinal.GetComponent[Health](worldCtx, id)
		assert.NilError(t, err)
		assert.Equal(t, 5, health.Value)
	}
}

func TestCanRegisterMultipleSystem(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world, doTick := tf.World, tf.DoTick
	var firstSystemCalled bool
	var secondSystemCalled bool

	firstSystem := func(cardinal.Context) error {
		firstSystemCalled = true
		return nil
	}
	secondSystem := func(cardinal.Context) error {
		secondSystemCalled = true
		return nil
	}

	err := cardinal.RegisterSystems(world, firstSystem, secondSystem)
	assert.NilError(t, err)

	doTick()

	assert.Check(t, firstSystemCalled)
	assert.Check(t, secondSystemCalled)
}

func TestInitSystemRunsOnce(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	w := tf.World
	count := 0
	count2 := 0
	err := cardinal.RegisterInitSystems(w, func(_ cardinal.Context) error {
		count++
		return nil
	}, func(_ cardinal.Context) error {
		count2 += 2
		return nil
	})
	assert.NilError(t, err)
	tf.DoTick()
	tf.DoTick()

	assert.Equal(t, count, 1)
	assert.Equal(t, count2, 2)
}
