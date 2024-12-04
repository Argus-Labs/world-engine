package world_test

import (
	"errors"
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/v2"
	"pkg.world.dev/world-engine/cardinal/v2/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/v2/types"
	"pkg.world.dev/world-engine/cardinal/v2/world"
)

func IncrementSystem(w world.WorldContext) error {
	var errs []error
	errs = append(errs, w.Search(filter.Exact(ScalarComponentStatic{})).Each(func(id types.EntityID) bool {
		errs = append(errs, world.UpdateComponent[ScalarComponentStatic](w, id,
			func(h *ScalarComponentStatic) *ScalarComponentStatic {
				h.Val++
				return h
			}))
		return true
	}))
	err := errors.Join(errs...)
	return err
}

func TestSystemExample(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	assert.NilError(t, world.RegisterComponent[ScalarComponentStatic](tf.World()))
	assert.NilError(t, world.RegisterSystems(tf.World(), IncrementSystem))

	var ids []types.EntityID
	assert.NilError(t, world.RegisterInitSystems(tf.World(), func(wCtx world.WorldContext) error {
		var err error
		ids, err = world.CreateMany(wCtx, 100, ScalarComponentStatic{})
		assert.NilError(t, err)
		return nil
	}))

	tf.DoTick()

	// Make sure we have 100 entities all with a health of 0
	err := tf.World().View(func(wCtx world.WorldContextReadOnly) error {
		for _, id := range ids {
			c, err := world.GetComponent[ScalarComponentStatic](wCtx, id)
			assert.NilError(t, err)
			assert.Equal(t, 1, c.Val)
		}
		return nil
	})
	assert.NilError(t, err)

	// do 5 ticks
	for i := 0; i < 5; i++ {
		tf.DoTick()
	}

	err = tf.World().View(func(wCtx world.WorldContextReadOnly) error {
		// Health should be 5 for everyone
		for _, id := range ids {
			var c *ScalarComponentStatic
			c, err := world.GetComponent[ScalarComponentStatic](wCtx, id)
			assert.NilError(t, err)
			assert.Equal(t, 6, c.Val)
		}
		return nil
	})
	assert.NilError(t, err)
}

func TestCanRegisterMultipleSystem(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	var firstSystemCalled bool
	var secondSystemCalled bool

	firstSystem := func(world.WorldContext) error {
		firstSystemCalled = true
		return nil
	}
	secondSystem := func(world.WorldContext) error {
		secondSystemCalled = true
		return nil
	}

	err := world.RegisterSystems(tf.World(), firstSystem, secondSystem)
	assert.NilError(t, err)

	tf.DoTick()

	assert.Check(t, firstSystemCalled)
	assert.Check(t, secondSystemCalled)
}

func TestInitSystemRunsOnce(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	count := 0
	count2 := 0
	err := world.RegisterInitSystems(tf.World(), func(_ world.WorldContext) error {
		count++
		return nil
	}, func(_ world.WorldContext) error {
		count2 += 2
		return nil
	})
	assert.NilError(t, err)
	tf.DoTick()
	tf.DoTick()

	assert.Equal(t, count, 1)
	assert.Equal(t, count2, 2)
}

func TestSystemExecutionOrder(t *testing.T) {
	tf := cardinal.NewTestCardinal(t, nil)
	order := make([]int, 0, 3)
	err := world.RegisterSystems(
		tf.World(),
		func(world.WorldContext) error {
			order = append(order, 1)
			return nil
		}, func(world.WorldContext) error {
			order = append(order, 2)
			return nil
		}, func(world.WorldContext) error {
			order = append(order, 3)
			return nil
		},
	)
	assert.NilError(t, err)
	tf.StartWorld()
	assert.NilError(t, err)
	tf.DoTick()
	expectedOrder := []int{1, 2, 3}
	for i, elem := range order {
		assert.Equal(t, elem, expectedOrder[i])
	}
}
