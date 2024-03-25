package cardinal_test

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

type ScalarComponentAlpha struct {
	Val int
}

type ScalarComponentBeta struct {
	Val int
}

func (ScalarComponentAlpha) Name() string {
	return "alpha"
}

func (ScalarComponentBeta) Name() string {
	return "beta"
}

func TestTickHappyPath(t *testing.T) {
	tf1 := testutils.NewTestFixture(t, nil)
	world1 := tf1.World
	assert.NilError(t, cardinal.RegisterComponent[EnergyComponent](world1))
	tf1.StartWorld()

	for i := 0; i < 10; i++ {
		_, err := tf1.DoTick()
		assert.NilError(t, err)
	}

	assert.Equal(t, uint64(10), world1.CurrentTick())

	tf2 := testutils.NewTestFixture(t, tf1.Redis)
	world2 := tf2.World
	assert.NilError(t, cardinal.RegisterComponent[EnergyComponent](world2))
	tf2.StartWorld()
	assert.Equal(t, uint64(10), world2.CurrentTick())
}

func TestCanModifyArchetypeAndGetEntity(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[ScalarComponentAlpha](world))
	assert.NilError(t, cardinal.RegisterComponent[ScalarComponentBeta](world))
	tf.StartWorld()

	wCtx := cardinal.NewWorldContext(world)
	wantID, err := cardinal.Create(wCtx, ScalarComponentAlpha{})
	assert.NilError(t, err)

	wantScalar := ScalarComponentAlpha{99}

	assert.NilError(t, cardinal.SetComponent[ScalarComponentAlpha](wCtx, wantID, &wantScalar))

	verifyCanFindEntity := func() {
		// Make sure we can find the entity
		q := cardinal.NewSearch(wCtx, filter.Contains(ScalarComponentAlpha{}))
		gotID, err := q.First()
		assert.NilError(t, err)
		assert.Equal(t, wantID, gotID)

		// Make sure the associated component is correct
		gotScalar, err := cardinal.GetComponent[ScalarComponentAlpha](wCtx, wantID)
		assert.NilError(t, err)
		assert.Equal(t, wantScalar, *gotScalar)
	}

	// Make sure we can find the one-and-only entity ID
	verifyCanFindEntity()

	// Add on the beta component
	assert.NilError(t, cardinal.AddComponentTo[Beta](wCtx, wantID))
	verifyCanFindEntity()

	// Remove the beta component
	assert.NilError(t, cardinal.RemoveComponentFrom[Beta](wCtx, wantID))
	verifyCanFindEntity()
}
