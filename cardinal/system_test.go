package cardinal_test

import (
	"errors"
	"testing"

	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/test_utils"
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
			h.Value += 1
			return h
		}))
		return true
	}))
	if err := errors.Join(errs...); err != nil {
		return err
	}
	return err
}

func TestSystemExample(t *testing.T) {
	world, doTick := test_utils.MakeWorldAndTicker(t)
	assert.NilError(t, cardinal.RegisterComponent[Health](world))
	cardinal.RegisterSystems(world, HealthSystem)

	worldCtx := test_utils.WorldToWorldContext(world)
	ids, err := cardinal.CreateMany(worldCtx, 100, Health{})
	assert.NilError(t, err)

	// Make sure we have 100 entities all with a health of 0
	for _, id := range ids {
		health, err := cardinal.GetComponent[Health](worldCtx, id)
		assert.NilError(t, err)
		assert.Equal(t, 0, health.Value)
	}

	// do 5 ticks
	for i := 0; i < 5; i++ {
		doTick()
	}

	// Health should be 5 for everyone
	for _, id := range ids {
		health, err := cardinal.GetComponent[Health](worldCtx, id)
		assert.NilError(t, err)
		assert.Equal(t, 5, health.Value)
	}
}
