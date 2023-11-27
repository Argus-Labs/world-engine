package metadata_test

import (
	"testing"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"

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

type ComponentDataC struct {
	Value int
}

func getNameOfComponent(c metadata.Component) string {
	return c.Name()
}

func TestComponentSchemaValidation(t *testing.T) {
	componentASchemaBytes, err := metadata.SerializeComponentSchema(ComponentDataA{Value: "test"})
	assert.NilError(t, err)
	valid, err := metadata.IsComponentValid(ComponentDataA{Value: "anything"}, componentASchemaBytes)
	assert.NilError(t, err)
	assert.Assert(t, valid)
	valid, err = metadata.IsComponentValid(ComponentDataB{Value: "blah"}, componentASchemaBytes)
	assert.NilError(t, err)
	assert.Assert(t, !valid)
}

func TestComponentInterfaceSignature(t *testing.T) {
	// The purpose of this test is to maintain api compatibility.
	// It is to prevent the interface signature of metadata.Component from changing.
	assert.Equal(t, getNameOfComponent(&ComponentDataA{}), "a")
}

func TestComponents(t *testing.T) {
	world := testutils.NewTestWorld(t).Instance()
	ecs.MustRegisterComponent[ComponentDataA](world)
	ecs.MustRegisterComponent[ComponentDataB](world)

	ca, err := world.GetComponentByName("a")
	assert.NilError(t, err)
	cb, err := world.GetComponentByName("b")
	assert.NilError(t, err)

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
		assert.NilError(t, err)
		tt.entityID = entityID
		tt.archID, err = storeManager.GetArchIDForComponents(tt.comps)
		assert.NilError(t, err)
	}

	for _, tt := range tests {
		componentsForArchID := storeManager.GetComponentTypesForArchID(tt.archID)
		for _, comp := range tt.comps {
			ok := filter.MatchComponentMetaData(componentsForArchID, comp)
			if !ok {
				t.Errorf("the archetype ID %d should contain the component %d", tt.archID, comp.ID())
			}
			iface, err := storeManager.GetComponentForEntity(comp, tt.entityID)
			assert.NilError(t, err)

			switch component := iface.(type) {
			case ComponentDataA:
				component.Value = tt.Value
				assert.NilError(t, storeManager.SetComponentForEntity(ca, tt.entityID, component))
			case ComponentDataB:
				component.Value = tt.Value
				assert.NilError(t, storeManager.SetComponentForEntity(cb, tt.entityID, component))
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
	world := testutils.NewTestWorld(t).Instance()
	ecs.MustRegisterComponent[foundComp](world)
	ecs.MustRegisterComponent[notFoundComp](world)

	wCtx := ecs.NewWorldContext(world)
	id, err := component.Create(wCtx, foundComp{})
	assert.NilError(t, err)
	_, err = component.GetComponent[notFoundComp](wCtx, id)
	assert.ErrorIs(t, err, storage.ErrComponentNotOnEntity)
}

type ValueComponent struct {
	Val int
}

func (ValueComponent) Name() string {
	return "ValueComponent"
}

func TestMultipleCallsToCreateSupported(t *testing.T) {
	world := testutils.NewTestWorld(t).Instance()
	assert.NilError(t, ecs.RegisterComponent[ValueComponent](world))

	wCtx := ecs.NewWorldContext(world)
	id, err := component.Create(wCtx, ValueComponent{})
	assert.NilError(t, err)

	assert.NilError(t, component.SetComponent[ValueComponent](wCtx, id, &ValueComponent{99}))

	val, err := component.GetComponent[ValueComponent](wCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 99, val.Val)

	_, err = component.Create(wCtx, ValueComponent{})
	assert.NilError(t, err)

	val, err = component.GetComponent[ValueComponent](wCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 99, val.Val)
}
