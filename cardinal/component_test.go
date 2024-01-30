package cardinal_test

import (
	"pkg.world.dev/world-engine/cardinal/events"
	"testing"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

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
	fixture := testutils.NewTestFixture(t, nil)
	world := fixture.World
	engine := fixture.Engine

	assert.NilError(t, cardinal.RegisterComponent[Height](world))
	assert.NilError(t, cardinal.RegisterComponent[Weight](world))
	assert.NilError(t, cardinal.RegisterComponent[Age](world))
	assert.NilError(t, cardinal.RegisterComponent[Number](world))
	assert.NilError(t, engine.LoadGameState())
	eCtx := testutils.WorldToEngineContext(world)
	assert.Equal(t, eCtx.CurrentTick(), uint64(0))
	eCtx.Logger().Info().Msg("test")               // Check for compile errors.
	eCtx.EmitEvent(&events.Event{Message: "test"}) // test for compiler errors, a check for this lives in e2e tests.
	startHeight := 72
	startWeight := 200
	startAge := 30
	numberID, err := cardinal.Create(eCtx, &Number{})
	assert.NilError(t, err)
	err = cardinal.SetComponent[Number](eCtx, numberID, &Number{num: 42})
	assert.NilError(t, err)
	newNum, err := cardinal.GetComponent[Number](eCtx, numberID)
	assert.NilError(t, err)
	assert.Equal(t, newNum.num, 42)
	err = cardinal.Remove(eCtx, numberID)
	assert.NilError(t, err)
	shouldBeNil, err := cardinal.GetComponent[Number](eCtx, numberID)
	assert.Assert(t, err != nil)
	assert.Assert(t, shouldBeNil == nil)

	peopleIDs, err := cardinal.CreateMany(eCtx, 10, Height{startHeight}, Weight{startWeight}, Age{startAge})
	assert.NilError(t, err)

	targetID := peopleIDs[4]
	height, err := cardinal.GetComponent[Height](eCtx, targetID)
	assert.NilError(t, err)
	assert.Equal(t, startHeight, height.Inches)

	assert.NilError(t, cardinal.RemoveComponentFrom[Age](eCtx, targetID))

	// Age was removed form exactly 1 entity.
	count, err := cardinal.NewSearch(eCtx, cardinal.Exact(Height{}, Weight{})).Count()
	assert.NilError(t, err)
	assert.Equal(t, 1, count)

	// The rest of the entities still have the Age field.
	count, err = cardinal.NewSearch(eCtx, cardinal.Contains(Age{})).Count()
	assert.NilError(t, err)
	assert.Equal(t, len(peopleIDs)-1, count)
	first, err := cardinal.NewSearch(eCtx, cardinal.Contains(Age{})).First()
	assert.NilError(t, err)
	assert.Equal(t, first, cardinal.EntityID(1))

	// Age does not exist on the target ID, so this should result in an error
	err = cardinal.UpdateComponent[Age](eCtx, targetID, func(a *Age) *Age {
		return a
	})
	assert.Check(t, err != nil)

	heavyWeight := 999
	err = cardinal.UpdateComponent[Weight](eCtx, targetID, func(w *Weight) *Weight {
		w.Pounds = heavyWeight
		return w
	})
	assert.NilError(t, err)

	// Adding the Age component to the targetID should not change the weight component
	assert.NilError(t, cardinal.AddComponentTo[Age](eCtx, targetID))

	for _, id := range peopleIDs {
		var weight *Weight
		weight, err = cardinal.GetComponent[Weight](eCtx, id)
		assert.NilError(t, err)
		if id == targetID {
			assert.Equal(t, heavyWeight, weight.Pounds)
		} else {
			assert.Equal(t, startWeight, weight.Pounds)
		}
	}
}
