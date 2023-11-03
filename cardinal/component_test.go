package cardinal_test

import (
	"testing"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal"
)

type Height struct {
	Inches int
}

type Number struct {
	num int
}

func (Number) Name() string {
	return "number"
}

func (Height) Name() string { return "height" }

type Weight struct {
	Pounds int
}

func (Weight) Name() string { return "weight" }

type Age struct {
	Years int
}

func (Age) Name() string { return "age" }

func TestComponentExample(t *testing.T) {
	world, _ := testutils.MakeWorldAndTicker(t)

	assert.NilError(t, cardinal.RegisterComponent[Height](world))
	assert.NilError(t, cardinal.RegisterComponent[Weight](world))
	assert.NilError(t, cardinal.RegisterComponent[Age](world))
	assert.NilError(t, cardinal.RegisterComponent[Number](world))

	testWorldCtx := testutils.WorldToWorldContext(world)
	startHeight := 72
	startWeight := 200
	startAge := 30
	numberID, err := cardinal.Create(testWorldCtx, &Number{})
	assert.NilError(t, err)
	err = cardinal.SetComponent[Number](testWorldCtx, numberID, &Number{num: 42})
	assert.NilError(t, err)
	newNum, err := cardinal.GetComponent[Number](testWorldCtx, numberID)
	assert.NilError(t, err)
	assert.Equal(t, newNum.num, 42)
	err = cardinal.Remove(testWorldCtx, numberID)
	assert.NilError(t, err)
	shouldBeNil, err := cardinal.GetComponent[Number](testWorldCtx, numberID)
	assert.Assert(t, err != nil)
	assert.Assert(t, shouldBeNil == nil)

	peopleIDs, err := cardinal.CreateMany(testWorldCtx, 10, Height{startHeight}, Weight{startWeight}, Age{startAge})
	assert.NilError(t, err)

	targetID := peopleIDs[4]
	height, err := cardinal.GetComponent[Height](testWorldCtx, targetID)
	assert.NilError(t, err)
	assert.Equal(t, startHeight, height.Inches)

	assert.NilError(t, cardinal.RemoveComponentFrom[Age](testWorldCtx, targetID))

	// Age was removed form exactly 1 entity.
	search, err := testWorldCtx.NewSearch(cardinal.Exact(Height{}, Weight{}))
	assert.NilError(t, err)
	count, err := search.Count(testWorldCtx)
	assert.NilError(t, err)
	assert.Equal(t, 1, count)

	// The rest of the entities still have the Age field.
	search, err = testWorldCtx.NewSearch(cardinal.Contains(Age{}))
	assert.NilError(t, err)
	count, err = search.Count(testWorldCtx)
	assert.NilError(t, err)
	assert.Equal(t, len(peopleIDs)-1, count)

	// Age does not exist on the target ID, so this should result in an error
	err = cardinal.UpdateComponent[Age](testWorldCtx, targetID, func(a *Age) *Age {
		return a
	})
	assert.Check(t, err != nil)

	heavyWeight := 999
	err = cardinal.UpdateComponent[Weight](testWorldCtx, targetID, func(w *Weight) *Weight {
		w.Pounds = heavyWeight
		return w
	})
	assert.NilError(t, err)

	// Adding the Age component to the targetID should not change the weight component
	assert.NilError(t, cardinal.AddComponentTo[Age](testWorldCtx, targetID))

	for _, id := range peopleIDs {
		var weight *Weight
		weight, err = cardinal.GetComponent[Weight](testWorldCtx, id)
		assert.NilError(t, err)
		if id == targetID {
			assert.Equal(t, heavyWeight, weight.Pounds)
		} else {
			assert.Equal(t, startWeight, weight.Pounds)
		}
	}
}
