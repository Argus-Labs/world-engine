package cardinal_test

import (
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/cardinaltestutils"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

type Health struct {
	Value int
}

func (Health) Name() string { return "health" }

func HealthSystem(worldCtx cardinal.WorldContext) error {
	q, err := worldCtx.NewSearch(cardinal.Exact(Health{}))
	if err != nil {
		return err
	}
	var errs []error
	errs = append(errs, q.Each(worldCtx, func(id cardinal.EntityID) bool {
		errs = append(errs, cardinal.UpdateComponent[Health](worldCtx, id, func(h *Health) *Health {
			h.Value++
			return h
		}))
		return true
	}))
	if err = errors.Join(errs...); err != nil {
		return err
	}
	return err
}

func TestSystemExample(t *testing.T) {
	world, doTick := cardinaltestutils.MakeWorldAndTicker(t)
	testutils.AssertNilErrorWithTrace(t, cardinal.RegisterComponent[Health](world))
	err := cardinal.RegisterSystems(world, HealthSystem)
	testutils.AssertNilErrorWithTrace(t, err)

	worldCtx := cardinaltestutils.WorldToWorldContext(world)
	doTick()
	ids, err := cardinal.CreateMany(worldCtx, 100, Health{})
	testutils.AssertNilErrorWithTrace(t, err)

	// Make sure we have 100 entities all with a health of 0
	for _, id := range ids {
		var health *Health
		health, err = cardinal.GetComponent[Health](worldCtx, id)
		testutils.AssertNilErrorWithTrace(t, err)
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
		testutils.AssertNilErrorWithTrace(t, err)
		assert.Equal(t, 5, health.Value)
	}
}

func TestCanRegisterMultipleSystem(t *testing.T) {
	world, doTick := cardinaltestutils.MakeWorldAndTicker(t)
	var firstSystemCalled bool
	var secondSystemCalled bool

	err := cardinal.RegisterSystems(world, func(context cardinal.WorldContext) error {
		firstSystemCalled = true
		return nil
	}, func(context cardinal.WorldContext) error {
		secondSystemCalled = true
		return nil
	})
	testutils.AssertNilErrorWithTrace(t, err)

	doTick()

	assert.Check(t, firstSystemCalled)
	assert.Check(t, secondSystemCalled)
}
