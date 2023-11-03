package cardinal_test

import (
	"testing"

	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

func TestCanRemoveEntity(t *testing.T) {
	world, _ := testutils.MakeWorldAndTicker(t)

	assert.NilError(t, cardinal.RegisterComponent[Alpha](world))

	testWorldCtx := testutils.WorldToWorldContext(world)
	ids, err := cardinal.CreateMany(testWorldCtx, 2, Alpha{})
	assert.NilError(t, err)

	removeID := ids[0]
	keepID := ids[1]

	assert.NilError(t, cardinal.Remove(testWorldCtx, removeID))

	search, err := testWorldCtx.NewSearch(cardinal.Exact(Alpha{}))
	assert.NilError(t, err)
	count := 0
	assert.NilError(t, search.Each(testWorldCtx, func(id cardinal.EntityID) bool {
		assert.Equal(t, id, keepID)
		count++
		return true
	}))
	assert.Equal(t, 1, count)

	// We should not be able to find the component for the removed ID
	_, err = cardinal.GetComponent[Alpha](testWorldCtx, removeID)
	assert.Check(t, err != nil)
}
