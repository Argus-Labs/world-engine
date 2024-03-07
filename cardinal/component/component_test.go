package component_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/types"

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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World

	assert.NilError(t, cardinal.RegisterComponent[Height](world))
	assert.NilError(t, cardinal.RegisterComponent[Weight](world))
	assert.NilError(t, cardinal.RegisterComponent[Age](world))
	assert.NilError(t, cardinal.RegisterComponent[Number](world))

	tf.StartWorld()

	wCtx := cardinal.NewWorldContext(world)
	assert.Equal(t, wCtx.CurrentTick(), uint64(0))
	wCtx.Logger().Info().Msg("test") // Check for compile errors.
	assert.NilError(t, wCtx.EmitEvent(map[string]any{"message": "test"}))
	// test for compiler errors, a check for this lives in e2e tests.
	startHeight := 72
	startWeight := 200
	startAge := 30
	numberID, err := cardinal.Create(wCtx, &Number{})
	assert.NilError(t, err)
	err = cardinal.SetComponent[Number](wCtx, numberID, &Number{num: 42})
	assert.NilError(t, err)
	newNum, err := cardinal.GetComponent[Number](wCtx, numberID)
	assert.NilError(t, err)
	assert.Equal(t, newNum.num, 42)
	err = cardinal.Remove(wCtx, numberID)
	assert.NilError(t, err)
	shouldBeNil, err := cardinal.GetComponent[Number](wCtx, numberID)
	assert.Assert(t, err != nil)
	assert.Assert(t, shouldBeNil == nil)

	peopleIDs, err := cardinal.CreateMany(wCtx, 10, Height{startHeight}, Weight{startWeight}, Age{startAge})
	assert.NilError(t, err)

	targetID := peopleIDs[4]
	height, err := cardinal.GetComponent[Height](wCtx, targetID)
	assert.NilError(t, err)
	assert.Equal(t, startHeight, height.Inches)

	assert.NilError(t, cardinal.RemoveComponentFrom[Age](wCtx, targetID))

	// Age was removed form exactly 1 entity.
	count, err := cardinal.NewSearch(wCtx, filter.Exact(Height{}, Weight{})).Count()
	assert.NilError(t, err)
	assert.Equal(t, 1, count)

	// The rest of the entities still have the Age field.
	count, err = cardinal.NewSearch(wCtx, filter.Contains(Age{})).Count()
	assert.NilError(t, err)
	assert.Equal(t, len(peopleIDs)-1, count)
	first, err := cardinal.NewSearch(wCtx, filter.Contains(Age{})).First()
	assert.NilError(t, err)
	assert.Equal(t, first, types.EntityID(1))

	// Age does not exist on the target EntityID, so this should result in an error
	err = cardinal.UpdateComponent[Age](wCtx, targetID, func(a *Age) *Age {
		return a
	})
	assert.Check(t, err != nil)

	heavyWeight := 999
	err = cardinal.UpdateComponent[Weight](wCtx, targetID, func(w *Weight) *Weight {
		w.Pounds = heavyWeight
		return w
	})
	assert.NilError(t, err)

	// Adding the Age component to the targetID should not change the weight component
	assert.NilError(t, cardinal.AddComponentTo[Age](wCtx, targetID))

	for _, id := range peopleIDs {
		var weight *Weight
		weight, err = cardinal.GetComponent[Weight](wCtx, id)
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

func getNameOfComponent(c types.Component) string {
	return c.Name()
}

func TestComponentSchemaValidation(t *testing.T) {
	componentASchemaBytes, err := types.SerializeComponentSchema(ComponentDataA{Value: "test"})
	assert.NilError(t, err)
	valid, err := types.IsComponentValid(ComponentDataA{Value: "anything"}, componentASchemaBytes)
	assert.NilError(t, err)
	assert.Assert(t, valid)
	valid, err = types.IsComponentValid(ComponentDataB{Value: "blah"}, componentASchemaBytes)
	assert.NilError(t, err)
	assert.Assert(t, !valid)
}

func TestComponentInterfaceSignature(t *testing.T) {
	// The purpose of this test is to maintain api compatibility.
	// It is to prevent the interface signature of metadata.Component from changing.
	assert.Equal(t, getNameOfComponent(&ComponentDataA{}), "a")
}

func TestComponents(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	cardinal.MustRegisterComponent[ComponentDataA](world)
	cardinal.MustRegisterComponent[ComponentDataB](world)

	ca, err := world.GetComponentByName("a")
	assert.NilError(t, err)
	cb, err := world.GetComponentByName("b")
	assert.NilError(t, err)

	tests := []*struct {
		comps    []types.ComponentMetadata
		archID   types.ArchetypeID
		entityID types.EntityID
		Value    string
	}{
		{
			[]types.ComponentMetadata{ca},
			0,
			0,
			"a",
		},
		{
			[]types.ComponentMetadata{ca, cb},
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
		matchComponent := filter.CreateComponentMatcher(
			types.ConvertComponentMetadatasToComponents(componentsForArchID))
		for _, comp := range tt.comps {
			ok := matchComponent(comp)
			if !ok {
				t.Errorf("the archetype EntityID %d should contain the component %d", tt.archID, comp.ID())
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
	var dstArchIdx types.ArchetypeID = 1

	assert.NilError(t, storeManager.AddComponentToEntity(cb, target.entityID))

	gotComponents, err := storeManager.GetComponentTypesForEntity(target.entityID)
	assert.NilError(t, err)
	gotArchID, err := storeManager.GetArchIDForComponents(gotComponents)
	assert.NilError(t, err)
	assert.Check(t, gotArchID != srcArchIdx, "the archetype EntityID should be different after adding a component")

	gotIDs, err := storeManager.GetEntitiesForArchID(srcArchIdx)
	assert.NilError(t, err)
	assert.Equal(t, 0, len(gotIDs), "there should be no entities in the archetype EntityID %d", srcArchIdx)

	gotIDs, err = storeManager.GetEntitiesForArchID(dstArchIdx)
	assert.NilError(t, err)
	assert.Equal(t, 2, len(gotIDs), "there should be 2 entities in the archetype EntityID %d", dstArchIdx)

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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	cardinal.MustRegisterComponent[foundComp](world)
	cardinal.MustRegisterComponent[notFoundComp](world)

	wCtx := cardinal.NewWorldContext(world)

	tf.StartWorld()

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
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[ValueComponent](world))

	wCtx := cardinal.NewWorldContext(world)

	tf.StartWorld()

	id, err := cardinal.Create(wCtx, ValueComponent{})
	assert.NilError(t, err)

	assert.NilError(t, cardinal.SetComponent[ValueComponent](wCtx, id, &ValueComponent{99}))

	val, err := cardinal.GetComponent[ValueComponent](wCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 99, val.Val)

	_, err = cardinal.Create(wCtx, ValueComponent{})
	assert.NilError(t, err)

	val, err = cardinal.GetComponent[ValueComponent](wCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 99, val.Val)
}

func TestRegisterComponent_ErrorOnDuplicateComponentName(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	assert.NilError(t, cardinal.RegisterComponent[ValueComponent](world))
	assert.ErrorContains(t, cardinal.RegisterComponent[ValueComponent](world), "is already registered")
}

type OldComponent struct {
	Val int
}

func (OldComponent) Name() string {
	return "OldComponent"
}

type NewComponent struct {
	Val                     int
	NewFieldToScrewUpSchema int
}

func (NewComponent) Name() string {
	return "OldComponent"
}

func TestRegisterComponent_ErrorOnSchemaMismatch(t *testing.T) {
	// We create a miniredis instance to reuse across the two world instance
	redis := miniredis.RunT(t)

	// Create first world, this should work normally
	tf1 := testutils.NewTestFixture(t, redis)
	world := tf1.World
	assert.NilError(t, cardinal.RegisterComponent[OldComponent](world))

	// Create second world, this should fail because the schema of the new component does not match the old component
	tf2 := testutils.NewTestFixture(t, redis)
	world = tf2.World
	assert.ErrorContains(t, cardinal.RegisterComponent[NewComponent](world),
		"component schema does not match target schema")
}

func TestGetRegisteredComponents(t *testing.T) {
	tf1 := testutils.NewTestFixture(t, nil)
	world := tf1.World

	// Register some components
	assert.NilError(t, cardinal.RegisterComponent[Height](world))
	assert.NilError(t, cardinal.RegisterComponent[Weight](world))

	// Get the registered components
	components := world.GetRegisteredComponents()

	// Check that the components are in the list
	compNames := make([]string, 0, len(components))
	for _, comp := range components {
		compNames = append(compNames, comp.Name())
	}
	assert.Contains(t, compNames, "height")
	assert.Contains(t, compNames, "weight")
}

func TestGetMessageByName(t *testing.T) {
	tf1 := testutils.NewTestFixture(t, nil)
	world := tf1.World

	// Register some components
	assert.NilError(t, cardinal.RegisterComponent[Height](world))
	assert.NilError(t, cardinal.RegisterComponent[Weight](world))

	// Check that we are able to obtain the registered components by name
	heightComp, err := world.GetComponentByName("height")
	assert.NilError(t, err)
	assert.Equal(t, heightComp.Name(), "height")

	weightComp, err := world.GetComponentByName("weight")
	assert.NilError(t, err)
	assert.Equal(t, weightComp.Name(), "weight")
}
