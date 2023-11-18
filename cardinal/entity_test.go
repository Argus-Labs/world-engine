package cardinal_test

import (
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/cardinaltestutils"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

func TestCanRemoveEntity(t *testing.T) {
	world, _ := cardinaltestutils.MakeWorldAndTicker(t)

	testutils.AssertNilErrorWithTrace(t, cardinal.RegisterComponent[Alpha](world))

	testWorldCtx := cardinaltestutils.WorldToWorldContext(world)
	ids, err := cardinal.CreateMany(testWorldCtx, 2, Alpha{})
	testutils.AssertNilErrorWithTrace(t, err)

	removeID := ids[0]
	keepID := ids[1]

	testutils.AssertNilErrorWithTrace(t, cardinal.Remove(testWorldCtx, removeID))

	search, err := testWorldCtx.NewSearch(cardinal.Exact(Alpha{}))
	testutils.AssertNilErrorWithTrace(t, err)
	count := 0
	testutils.AssertNilErrorWithTrace(t, search.Each(testWorldCtx, func(id cardinal.EntityID) bool {
		assert.Equal(t, id, keepID)
		count++
		return true
	}))
	assert.Equal(t, 1, count)

	// We should not be able to find the component for the removed ID
	_, err = cardinal.GetComponent[Alpha](testWorldCtx, removeID)
	assert.Check(t, err != nil)
}
