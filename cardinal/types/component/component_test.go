package component_test

import (
	"testing"

	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/types/entity"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/types/archetype"
	"pkg.world.dev/world-engine/cardinal/types/component"

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
	engine := testutils.NewTestWorld(t).Engine()
	ecs.MustRegisterComponent[ComponentDataA](engine)
	ecs.MustRegisterComponent[ComponentDataB](engine)

	ca, err := engine.GetComponentByName("a")
	assert.NilError(t, err)
	cb, err := engine.GetComponentByName("b")
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

	storeManager := engine.StoreManager()
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
	engine := testutils.NewTestWorld(t).Engine()
	ecs.MustRegisterComponent[foundComp](engine)
	ecs.MustRegisterComponent[notFoundComp](engine)

	wCtx := ecs.NewEngineContext(engine)
	id, err := ecs.Create(wCtx, foundComp{})
	assert.NilError(t, err)
	_, err = ecs.GetComponent[notFoundComp](wCtx, id)
	assert.ErrorIs(t, err, storage.ErrComponentNotOnEntity)
}

type ValueComponent struct {
	Val int
}

func (ValueComponent) Name() string {
	return "ValueComponent"
}

func TestMultipleCallsToCreateSupported(t *testing.T) {
	engine := testutils.NewTestWorld(t).Engine()
	assert.NilError(t, ecs.RegisterComponent[ValueComponent](engine))

	eCtx := ecs.NewEngineContext(engine)
	id, err := ecs.Create(eCtx, ValueComponent{})
	assert.NilError(t, err)

	assert.NilError(t, ecs.SetComponent[ValueComponent](eCtx, id, &ValueComponent{99}))

	val, err := ecs.GetComponent[ValueComponent](eCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 99, val.Val)

	_, err = ecs.Create(eCtx, ValueComponent{})
	assert.NilError(t, err)

	val, err = ecs.GetComponent[ValueComponent](eCtx, id)
	assert.NilError(t, err)
	assert.Equal(t, 99, val.Val)
}
