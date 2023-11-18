package metadata_test

import (
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/cardinaltestutils"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/component/metadata"

	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

type ComponentDataA struct {
	Value string
}

func (ComponentDataA) Name() string { return "a" }

type ComponentDataB struct {
	Value string
}

func (ComponentDataB) Name() string { return "b" }

func getNameOfComponent(c metadata.Component) string {
	return c.Name()
}

func TestComponentInterfaceSignature(t *testing.T) {
	// The purpose of this test is to maintain api compatibility.
	// It is to prevent the interface signature of metadata.Component from changing.
	assert.Equal(t, getNameOfComponent(&ComponentDataA{}), "a")
}

func TestComponents(t *testing.T) {
	world := cardinaltestutils.NewTestWorld(t).Instance()
	ecs.MustRegisterComponent[ComponentDataA](world)
	ecs.MustRegisterComponent[ComponentDataB](world)

	ca, err := world.GetComponentByName("a")
	testutils.AssertNilErrorWithTrace(t, err)
	cb, err := world.GetComponentByName("b")
	testutils.AssertNilErrorWithTrace(t, err)

	tests := []*struct {
		comps    []metadata.ComponentMetadata
		archID   archetype.ID
		entityID entity.ID
		Value    string
	}{
		{
			[]metadata.ComponentMetadata{ca},
			0,
			0,
			"a",
		},
		{
			[]metadata.ComponentMetadata{ca, cb},
			1,
			0,
			"b",
		},
	}

	storeManager := world.StoreManager()
	for _, tt := range tests {
		entityID, err := storeManager.CreateEntity(tt.comps...)
		testutils.AssertNilErrorWithTrace(t, err)
		tt.entityID = entityID
		tt.archID, err = storeManager.GetArchIDForComponents(tt.comps)
		testutils.AssertNilErrorWithTrace(t, err)
	}

	for _, tt := range tests {
		componentsForArchID := storeManager.GetComponentTypesForArchID(tt.archID)
		for _, comp := range tt.comps {
			ok := filter.MatchComponentMetaData(componentsForArchID, comp)
			if !ok {
				t.Errorf("the archetype ID %d should contain the component %d", tt.archID, comp.ID())
			}
			iface, err := storeManager.GetComponentForEntity(comp, tt.entityID)
			testutils.AssertNilErrorWithTrace(t, err)

			switch component := iface.(type) {
			case ComponentDataA:
				component.Value = tt.Value
				testutils.AssertNilErrorWithTrace(t, storeManager.SetComponentForEntity(ca, tt.entityID, component))
			case ComponentDataB:
				component.Value = tt.Value
				testutils.AssertNilErrorWithTrace(t, storeManager.SetComponentForEntity(cb, tt.entityID, component))
			default:
				assert.Check(t, false, "unknown component type: %v", iface)
			}
		}
	}

	target := tests[0]

	srcArchIdx := target.archID
	var dstArchIdx archetype.ID = 1

	testutils.AssertNilErrorWithTrace(t, storeManager.AddComponentToEntity(cb, target.entityID))

	gotComponents, err := storeManager.GetComponentTypesForEntity(target.entityID)
	testutils.AssertNilErrorWithTrace(t, err)
	gotArchID, err := storeManager.GetArchIDForComponents(gotComponents)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Check(t, gotArchID != srcArchIdx, "the archetype ID should be different after adding a component")

	gotIDs, err := storeManager.GetEntitiesForArchID(srcArchIdx)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 0, len(gotIDs), "there should be no entities in the archetype ID %d", srcArchIdx)

	gotIDs, err = storeManager.GetEntitiesForArchID(dstArchIdx)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 2, len(gotIDs), "there should be 2 entities in the archetype ID %d", dstArchIdx)

	iface, err := storeManager.GetComponentForEntity(ca, target.entityID)
	testutils.AssertNilErrorWithTrace(t, err)

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
	world := cardinaltestutils.NewTestWorld(t).Instance()
	ecs.MustRegisterComponent[foundComp](world)
	ecs.MustRegisterComponent[notFoundComp](world)

	wCtx := ecs.NewWorldContext(world)
	id, err := component.Create(wCtx, foundComp{})
	testutils.AssertNilErrorWithTrace(t, err)
	_, err = component.GetComponent[notFoundComp](wCtx, id)
	testutils.AssertErrorIsWithTrace(t, err, storage.ErrComponentNotOnEntity)
}

type ValueComponent struct {
	Val int
}

func (ValueComponent) Name() string {
	return "ValueComponent"
}

func TestMultipleCallsToCreateSupported(t *testing.T) {
	world := cardinaltestutils.NewTestWorld(t).Instance()
	testutils.AssertNilErrorWithTrace(t, ecs.RegisterComponent[ValueComponent](world))

	wCtx := ecs.NewWorldContext(world)
	id, err := component.Create(wCtx, ValueComponent{})
	testutils.AssertNilErrorWithTrace(t, err)

	testutils.AssertNilErrorWithTrace(t, component.SetComponent[ValueComponent](wCtx, id, &ValueComponent{99}))

	val, err := component.GetComponent[ValueComponent](wCtx, id)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 99, val.Val)

	_, err = component.Create(wCtx, ValueComponent{})
	testutils.AssertNilErrorWithTrace(t, err)

	val, err = component.GetComponent[ValueComponent](wCtx, id)
	testutils.AssertNilErrorWithTrace(t, err)
	assert.Equal(t, 99, val.Val)
}
