package cardinal_test

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

func TestCanRemoveEntity(t *testing.T) {
	fixture := testutils.NewTestFixture(t, nil)
	world := fixture.World

	assert.NilError(t, cardinal.RegisterComponent[Alpha](world))
	assert.NilError(t, fixture.Engine.LoadGameState())

	testWorldCtx := testutils.WorldToEngineContext(world)
	ids, err := cardinal.CreateMany(testWorldCtx, 2, Alpha{})
	assert.NilError(t, err)

	removeID := ids[0]
	keepID := ids[1]

	assert.NilError(t, cardinal.Remove(testWorldCtx, removeID))

	count := 0
	assert.NilError(t, cardinal.NewSearch(testWorldCtx, cardinal.Exact(Alpha{})).Each(func(id cardinal.EntityID) bool {
		assert.Equal(t, id, keepID)
		count++
		return true
	}))
	assert.Equal(t, 1, count)

	// We should not be able to find the component for the removed ID
	_, err = cardinal.GetComponent[Alpha](testWorldCtx, removeID)
	assert.Check(t, err != nil)
}
