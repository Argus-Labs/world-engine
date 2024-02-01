package cardinal_test

import (
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/types/archetype"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
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

	assert.NilError(t, cardinal.RegisterComponent[Height](world))
	assert.NilError(t, cardinal.RegisterComponent[Weight](world))
	assert.NilError(t, cardinal.RegisterComponent[Age](world))
	assert.NilError(t, cardinal.RegisterComponent[Number](world))
	assert.NilError(t, world.LoadGameState())
	eCtx := testutils.WorldToEngineContext(world)
	assert.Equal(t, eCtx.CurrentTick(), uint64(0))
	eCtx.Logger().Info().Msg("test") // Check for compile errors.
	eCtx.EmitEvent("test")           // test for compiler errors, a check for this lives in e2e tests.
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
	count, err := cardinal.NewSearch(eCtx, filter.Exact(Height{}, Weight{})).Count()
	assert.NilError(t, err)
	assert.Equal(t, 1, count)

	// The rest of the entities still have the Age field.
	count, err = cardinal.NewSearch(eCtx, filter.Contains(Age{})).Count()
	assert.NilError(t, err)
	assert.Equal(t, len(peopleIDs)-1, count)
	first, err := cardinal.NewSearch(eCtx, filter.Contains(Age{})).First()
	assert.NilError(t, err)
	assert.Equal(t, first, entity.ID(1))

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

type ComponentDataA struct {
	Value string
}

func (ComponentDataA) Name() string { return "a" }

type ComponentDataB struct {
	Value string
}

func (ComponentDataB) Name() string { return "b" }

type ComponentDataC struct {
	Value int
}

func getNameOfComponent(c component.Component) string {
	return c.Name()
}

func TestComponentSchemaValidation(t *testing.T) {
	componentASchemaBytes, err := component.SerializeComponentSchema(ComponentDataA{Value: "test"})
	assert.NilError(t, err)
	valid, err := component.IsComponentValid(ComponentDataA{Value: "anything"}, componentASchemaBytes)
	assert.NilError(t, err)
	assert.Assert(t, valid)
	valid, err = component.IsComponentValid(ComponentDataB{Value: "blah"}, componentASchemaBytes)
	assert.NilError(t, err)
	assert.Assert(t, !valid)
}

func TestComponentInterfaceSignature(t *testing.T) {
	// The purpose of this test is to maintain api compatibility.
	// It is to prevent the interface signature of metadata.Component from changing.
	assert.Equal(t, getNameOfComponent(&ComponentDataA{}), "a")
}

func TestComponents(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	cardinal.MustRegisterComponent[ComponentDataA](world)
	cardinal.MustRegisterComponent[ComponentDataB](world)

	ca, err := world.GetComponentByName("a")
	assert.NilError(t, err)
	cb, err := world.GetComponentByName("b")
	assert.NilError(t, err)

	tests := []*struct {
		comps    []component.ComponentMetadata
		archID   archetype.ID
		entityID entity.ID
		Value    string
	}{
		{
			[]component.ComponentMetadata{ca},
			0,
			0,
			"a",
		},
		{
			[]component.ComponentMetadata{ca, cb},
			1,
			0,
			"b",
		},
	}

	storeManager := world.GameStateManager()
	for _, tt := range tests {
		entityID, err := storeManager.CreateEntity(tt.comps...)
		assert.NilError(t, err)
		tt.entityID = entityID
		tt.archID, err = storeManager.GetArchIDForComponents(tt.comps)
		assert.NilError(t, err)
	}

	for _, tt := range tests {
		componentsForArchID := storeManager.GetComponentTypesForArchID(tt.archID)
		for _, comp := range tt.comps {
			ok := filter.MatchComponent(component.ConvertComponentMetadatasToComponents(componentsForArchID), comp)
			if !ok {
				t.Errorf("the archetype ID %d should contain the component %d", tt.archID, comp.ID())
			}
			iface, err := storeManager.GetComponentForEntity(comp, tt.entityID)
			assert.NilError(t, err)

			switch comp := iface.(type) {
			case ComponentDataA:
				comp.Value = tt.Value
				assert.NilError(t, storeManager.SetComponentForEntity(ca, tt.entityID, comp))
			case ComponentDataB:
				comp.Value = tt.Value
				assert.NilError(t, storeManager.SetComponentForEntity(cb, tt.entityID, comp))
			default:
				assert.Check(t, false, "unknown component type: %v", iface)
			}
		}
	}

	target := tests[0]

	srcArchIdx := target.archID
	var dstArchIdx archetype.ID = 1

	assert.NilError(t, storeManager.AddComponentToEntity(cb, target.entityID))

	gotComponents, err := storeManager.GetComponentTypesForEntity(target.entityID)
	assert.NilError(t, err)
	gotArchID, err := storeManager.GetArchIDForComponents(gotComponents)
	assert.NilError(t, err)
	assert.Check(t, gotArchID != srcArchIdx, "the archetype ID should be different after adding a component")

	gotIDs, err := storeManager.GetEntitiesForArchID(srcArchIdx)
	assert.NilError(t, err)
	assert.Equal(t, 0, len(gotIDs), "there should be no entities in the archetype ID %d", srcArchIdx)

	gotIDs, err = storeManager.GetEntitiesForArchID(dstArchIdx)
	assert.NilError(t, err)
	assert.Equal(t, 2, len(gotIDs), "there should be 2 entities in the archetype ID %d", dstArchIdx)

	iface, err := storeManager.GetComponentForEntity(ca, target.entityID)
	assert.NilError(t, err)

	got, ok := iface.(ComponentDataA)
	assert.Check(t, ok, "component %v is of wrong type", iface)
	assert.Equal(t, got.Value, target.Value, "component should have value of %q got %q", target.Value, got.Value)
}

type foundComp struct{}
type notFoundComp struct{}

func (foundComp) Name() string {
	return "foundComp"
}

func (notFoundComp) Name() string {
	return "notFoundComp"
}

func TestErrorWhenAccessingComponentNotOnEntity(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	cardinal.MustRegisterComponent[foundComp](world)
	cardinal.MustRegisterComponent[notFoundComp](world)

	wCtx := cardinal.NewWorldContext(world)
	assert.NilError(t, world.LoadGameState())
	id, err := cardinal.Create(wCtx, foundComp{})
	assert.NilError(t, err)
	_, err = cardinal.GetComponent[notFoundComp](wCtx, id)
	assert.ErrorIs(t, err, iterators.ErrComponentNotOnEntity)
}

type ValueComponent struct {
	Val int
}

func (ValueComponent) Name() string {
	return "ValueComponent"
}

func TestMultipleCallsToCreateSupported(t *testing.T) {
	world := testutils.NewTestFixture(t, nil).World
	assert.NilError(t, cardinal.RegisterComponent[ValueComponent](world))

	eCtx := cardinal.NewWorldContext(world)
	assert.NilError(t, world.LoadGameState())
	id, err := cardinal.Create(eCtx, ValueComponent{})
	assert.NilError(t, err)

	assert.NilError(t, cardinal.SetComponent[ValueComponent](eCtx, id, &ValueComponent{99}))

	val, err := cardinal.GetComponent[ValueComponent](eCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 99, val.Val)

	_, err = cardinal.Create(eCtx, ValueComponent{})
	assert.NilError(t, err)

	val, err = cardinal.GetComponent[ValueComponent](eCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 99, val.Val)
}
