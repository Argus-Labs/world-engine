package system_test

import (
	"errors"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"testing"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"
)

type Health struct {
	Value int
}

func (Health) Name() string { return "health" }

func HealthSystem(eCtx engine.Context) error {
	var errs []error
	errs = append(errs, cardinal.NewSearch(eCtx, filter.Exact(Health{})).Each(func(id types.EntityID) bool {
		errs = append(errs, cardinal.UpdateComponent[Health](eCtx, id, func(h *Health) *Health {
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

	firstSystem := func(context engine.Context) error {
		firstSystemCalled = true
		return nil
	}
	secondSystem := func(context engine.Context) error {
		secondSystemCalled = true
		return nil
	}

	err := cardinal.RegisterSystems(world, firstSystem, secondSystem)
	assert.NilError(t, err)

	doTick()

	assert.Check(t, firstSystemCalled)
	assert.Check(t, secondSystemCalled)
}
