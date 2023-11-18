package cardinal_test

import (
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/cardinaltestutils"
	"pkg.world.dev/world-engine/cardinal/testutils"
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
	world, _ := cardinaltestutils.MakeWorldAndTicker(t)

	testutils.AssertNilErrorWithTrace(t, cardinal.RegisterComponent[Height](world))
	testutils.AssertNilErrorWithTrace(t, cardinal.RegisterComponent[Weight](world))
	testutils.AssertNilErrorWithTrace(t, cardinal.RegisterComponent[Age](world))
	testutils.AssertNilErrorWithTrace(t, cardinal.RegisterComponent[Number](world))

	testWorldCtx := cardinaltestutils.WorldToWorldContext(world)
	assert.Equal(t, testWorldCtx.CurrentTick(), uint64(0))
	testWorldCtx.Logger().Info().Msg("test") // Check for compile errors.
	testWorldCtx.EmitEvent("test")           // test for compiler errors, a check for this lives in e2e tests.
	startHeight := 72
	startWeight := 200
	startAge := 30
	numberID, err := cardinal.Create(testWorldCtx, &Number{})
	testutils.AssertNilErrorWithTrace(t, err)
	err = cardinal.SetComponent[Number](testWorldCtx, numberID, &Number{num: 42})
	testutils.AssertNilErrorWithTrace(t, err)
	newNum, err := cardinal.GetComponent[Number](testWorldCtx, numberID)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, newNum.num, 42)
	err = cardinal.Remove(testWorldCtx, numberID)
	testutils.AssertNilErrorWithTrace(t, err)
	shouldBeNil, err := cardinal.GetComponent[Number](testWorldCtx, numberID)
	assert.Assert(t, err != nil)
	assert.Assert(t, shouldBeNil == nil)

	peopleIDs, err := cardinal.CreateMany(testWorldCtx, 10, Height{startHeight}, Weight{startWeight}, Age{startAge})
	testutils.AssertNilErrorWithTrace(t, err)

	targetID := peopleIDs[4]
	height, err := cardinal.GetComponent[Height](testWorldCtx, targetID)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, startHeight, height.Inches)

	testutils.AssertNilErrorWithTrace(t, cardinal.RemoveComponentFrom[Age](testWorldCtx, targetID))

	// Age was removed form exactly 1 entity.
	search, err := testWorldCtx.NewSearch(cardinal.Exact(Height{}, Weight{}))
	testutils.AssertNilErrorWithTrace(t, err)
	count, err := search.Count(testWorldCtx)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 1, count)

	// The rest of the entities still have the Age field.
	search, err = testWorldCtx.NewSearch(cardinal.Contains(Age{}))
	testutils.AssertNilErrorWithTrace(t, err)
	count, err = search.Count(testWorldCtx)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, len(peopleIDs)-1, count)
	first, err := search.First(testWorldCtx)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, first, cardinal.EntityID(1))

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
	testutils.AssertNilErrorWithTrace(t, err)

	// Adding the Age component to the targetID should not change the weight component
	testutils.AssertNilErrorWithTrace(t, cardinal.AddComponentTo[Age](testWorldCtx, targetID))

	for _, id := range peopleIDs {
		var weight *Weight
		weight, err = cardinal.GetComponent[Weight](testWorldCtx, id)
		testutils.AssertNilErrorWithTrace(t, err)
		if id == targetID {
			assert.Equal(t, heavyWeight, weight.Pounds)
		} else {
			assert.Equal(t, startWeight, weight.Pounds)
		}
	}
}
