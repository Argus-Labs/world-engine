package testsuite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/types"
)

// setupTestWorld creates a new test world and ensures it's properly initialized
func setupTestWorld(t *testing.T, opts ...cardinal.WorldOption) *cardinal.World {
	t.Helper()
	return NewTestWorld(t, opts...)
}

func TestLocationComponent(t *testing.T) {
	tests := []struct {
		name     string
		loc      LocationComponent
		wantX    uint64
		wantY    uint64
		wantName string
	}{
		{
			name:     "returns correct component name",
			loc:      LocationComponent{},
			wantX:    0,
			wantY:    0,
			wantName: "location",
		},
		{
			name: "stores positive coordinates",
			loc: LocationComponent{
				X: 10,
				Y: 20,
			},
			wantX:    10,
			wantY:    20,
			wantName: "location",
		},
		{
			name: "handles large coordinates",
			loc: LocationComponent{
				X: 999999,
				Y: 888888,
			},
			wantX:    999999,
			wantY:    888888,
			wantName: "location",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantName, tt.loc.Name(), "component name should match")
			assert.Equal(t, tt.wantX, tt.loc.X, "X coordinate should match")
			assert.Equal(t, tt.wantY, tt.loc.Y, "Y coordinate should match")
		})
	}
}

func TestValueComponent(t *testing.T) {
	tests := []struct {
		name      string
		component ValueComponent
		value     int64
		wantName  string
	}{
		{
			name:      "returns correct component name",
			component: ValueComponent{},
			wantName:  "value",
		},
		{
			name: "stores positive value",
			component: ValueComponent{
				Value: 100,
			},
			value:    100,
			wantName: "value",
		},
		{
			name: "stores negative value",
			component: ValueComponent{
				Value: -50,
			},
			value:    -50,
			wantName: "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantName, tt.component.Name(), "component name should match")
			assert.Equal(t, tt.value, tt.component.Value, "component value should match")
		})
	}
}

func TestPowerComponent(t *testing.T) {
	tests := []struct {
		name      string
		component PowerComponent
		power     int64
		wantName  string
	}{
		{
			name:      "returns correct component name",
			component: PowerComponent{},
			wantName:  "power",
		},
		{
			name: "stores positive power value",
			component: PowerComponent{
				Power: 1000,
			},
			power:    1000,
			wantName: "power",
		},
		{
			name: "stores negative power value",
			component: PowerComponent{
				Power: -500,
			},
			power:    -500,
			wantName: "power",
		},
		{
			name: "stores zero power value",
			component: PowerComponent{
				Power: 0,
			},
			power:    0,
			wantName: "power",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantName, tt.component.Name(), "component name should match")
			assert.Equal(t, tt.power, tt.component.Power, "component power should match")
		})
	}
}

func TestRegisterComponents(t *testing.T) {
	world := setupTestWorld(t)

	// Try to register components, ignoring "already registered" errors
	components := []types.ComponentMetadata{
		&LocationComponent{},
		&ValueComponent{},
		&PowerComponent{},
		&HealthComponent{},
		&SpeedComponent{},
		&TestComponent{},
		&TestTwoComponent{},
	}

	for _, comp := range components {
		err := world.RegisterComponent(comp)
		if err != nil && err.Error() != "message \""+comp.Name()+"\" is already registered" {
			t.Errorf("unexpected error registering component %s: %v", comp.Name(), err)
		}
	}

	// Verify each component exists in the world
	for _, comp := range components {
		_, err := world.GetComponentByName(comp.Name())
		require.NoError(t, err, "component %s should exist", comp.Name())
	}
}

// InvalidComponent is a component type that doesn't implement the Component interface correctly
type InvalidComponent struct{}

func (ic *InvalidComponent) Name() string { return "invalid" }

func TestComponentValidation(t *testing.T) {
	tests := []struct {
		name string
		comp types.Component
		want bool
	}{
		{
			name: "valid location component",
			comp: &LocationComponent{X: 1, Y: 2},
			want: true,
		},
		{
			name: "valid value component",
			comp: &ValueComponent{Value: 100},
			want: true,
		},
		{
			name: "valid power component",
			comp: &PowerComponent{Power: 50},
			want: true,
		},
		{
			name: "nil component",
			comp: nil,
			want: false,
		},
		{
			name: "invalid component type",
			comp: &InvalidComponent{},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			// Use setupTestWorld to handle miniredis setup consistently
			world := setupTestWorld(t)

			// Test component registration
			var regErr error
			switch tt.comp.(type) {
			case nil:
				regErr = cardinal.RegisterComponent[*InvalidComponent](world)
				require.Error(t, regErr, "expected error for nil component")
			case *LocationComponent, *ValueComponent, *PowerComponent:
				// These components are already registered by setupTestWorld
				_, err := world.GetComponentByName(tt.comp.Name())
				require.NoError(t, err, "component should already be registered")
				regErr = nil
			case *InvalidComponent:
				regErr = cardinal.RegisterComponent[*InvalidComponent](world)
			default:
				t.Fatalf("unexpected component type: %T", tt.comp)
			}

			// Check if error matches expectation
			if tt.want {
				_, err := world.GetComponentByName(tt.comp.Name())
				if regErr != nil && err != nil {
					t.Errorf("unexpected error: %v", regErr)
				}
			} else {
				require.Error(t, regErr, "expected invalid component")
			}
		})
	}
}

func TestComponentOperations(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T, world *cardinal.World, entityID types.EntityID)
	}{
		{
			name: "add and get location component",
			fn: func(t *testing.T, world *cardinal.World, entityID types.EntityID) {
				ctx := cardinal.NewWorldContext(world)
				require.NoError(t, cardinal.AddComponentTo[LocationComponent](ctx, entityID))
				loc := &LocationComponent{X: 1, Y: 2}
				require.NoError(t, cardinal.SetComponent(ctx, entityID, loc))
				readCtx := world.GetReadOnlyCtx()
				got, err := cardinal.GetComponent[LocationComponent](readCtx, entityID)
				require.NoError(t, err)
				assert.Equal(t, uint64(1), got.X)
				assert.Equal(t, uint64(2), got.Y)
			},
		},
		{
			name: "add and get value component",
			fn: func(t *testing.T, world *cardinal.World, entityID types.EntityID) {
				ctx := cardinal.NewWorldContext(world)
				require.NoError(t, cardinal.AddComponentTo[ValueComponent](ctx, entityID))
				val := &ValueComponent{Value: 100}
				require.NoError(t, cardinal.SetComponent(ctx, entityID, val))
				readCtx := world.GetReadOnlyCtx()
				got, err := cardinal.GetComponent[ValueComponent](readCtx, entityID)
				require.NoError(t, err)
				assert.Equal(t, int64(100), got.Value)
			},
		},
		{
			name: "add and get power component",
			fn: func(t *testing.T, world *cardinal.World, entityID types.EntityID) {
				ctx := cardinal.NewWorldContext(world)
				require.NoError(t, cardinal.AddComponentTo[PowerComponent](ctx, entityID))
				power := &PowerComponent{Power: 50}
				require.NoError(t, cardinal.SetComponent(ctx, entityID, power))
				readCtx := world.GetReadOnlyCtx()
				got, err := cardinal.GetComponent[PowerComponent](readCtx, entityID)
				require.NoError(t, err)
				assert.Equal(t, int64(50), got.Power)
			},
		},
		{
			name: "get non-existent component returns error",
			fn: func(t *testing.T, world *cardinal.World, entityID types.EntityID) {
				readCtx := world.GetReadOnlyCtx()
				_, err := cardinal.GetComponent[LocationComponent](readCtx, entityID)
				require.Error(t, err)
			},
		},
		{
			name: "add component to non-existent entity returns error",
			fn: func(t *testing.T, world *cardinal.World, entityID types.EntityID) {
				ctx := cardinal.NewWorldContext(world)
				err := cardinal.SetComponent(ctx, types.EntityID(999999), &LocationComponent{})
				require.Error(t, err)
			},
		},
		{
			name: "remove component",
			fn: func(t *testing.T, world *cardinal.World, entityID types.EntityID) {
				ctx := cardinal.NewWorldContext(world)
				// Add and verify component exists
				require.NoError(t, cardinal.AddComponentTo[LocationComponent](ctx, entityID))
				loc := &LocationComponent{X: 1, Y: 2}
				require.NoError(t, cardinal.SetComponent(ctx, entityID, loc))
				readCtx := world.GetReadOnlyCtx()
				_, err := cardinal.GetComponent[LocationComponent](readCtx, entityID)
				require.NoError(t, err)

				// Remove and verify it's gone
				require.NoError(t, cardinal.RemoveComponentFrom[LocationComponent](ctx, entityID))
				_, err = cardinal.GetComponent[LocationComponent](readCtx, entityID)
				require.Error(t, err)
			},
		},
		{
			name: "update component",
			fn: func(t *testing.T, world *cardinal.World, entityID types.EntityID) {
				ctx := cardinal.NewWorldContext(world)
				// Add initial component
				require.NoError(t, cardinal.AddComponentTo[ValueComponent](ctx, entityID))
				val := &ValueComponent{Value: 100}
				require.NoError(t, cardinal.SetComponent(ctx, entityID, val))
				// Update value
				newVal := &ValueComponent{Value: 200}
				require.NoError(t, cardinal.SetComponent(ctx, entityID, newVal))

				readCtx := world.GetReadOnlyCtx()
				// Verify updated value
				got, err := cardinal.GetComponent[ValueComponent](readCtx, entityID)
				require.NoError(t, err)
				assert.Equal(t, int64(200), got.Value)
			},
		},
		{
			name: "remove non-existent component returns error",
			fn: func(t *testing.T, world *cardinal.World, entityID types.EntityID) {
				ctx := cardinal.NewWorldContext(world)
				err := cardinal.RemoveComponentFrom[LocationComponent](ctx, entityID)
				require.Error(t, err)
			},
		},
		{
			name: "multiple components on single entity",
			fn: func(t *testing.T, world *cardinal.World, entityID types.EntityID) {
				ctx := cardinal.NewWorldContext(world)
				// Add multiple components
				require.NoError(t, cardinal.AddComponentTo[LocationComponent](ctx, entityID))
				require.NoError(t, cardinal.SetComponent(ctx, entityID, &LocationComponent{X: 1, Y: 2}))
				require.NoError(t, cardinal.AddComponentTo[ValueComponent](ctx, entityID))
				require.NoError(t, cardinal.SetComponent(ctx, entityID, &ValueComponent{Value: 100}))
				require.NoError(t, cardinal.AddComponentTo[PowerComponent](ctx, entityID))
				require.NoError(t, cardinal.SetComponent(ctx, entityID, &PowerComponent{Power: 50}))
				readCtx := world.GetReadOnlyCtx()
				// Verify all components
				loc, err := cardinal.GetComponent[LocationComponent](readCtx, entityID)
				require.NoError(t, err)
				assert.Equal(t, uint64(1), loc.X)
				assert.Equal(t, uint64(2), loc.Y)

				val, err := cardinal.GetComponent[ValueComponent](readCtx, entityID)
				require.NoError(t, err)
				assert.Equal(t, int64(100), val.Value)

				power, err := cardinal.GetComponent[PowerComponent](readCtx, entityID)
				require.NoError(t, err)
				assert.Equal(t, int64(50), power.Power)
			},
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			// Use setupTestWorld to handle miniredis setup consistently
			world := setupTestWorld(t)

			// Create an entity first, except for the non-existent entity test
			var entityID types.EntityID
			var err error
			if tt.name != "add component to non-existent entity returns error" {
				ctx := cardinal.NewWorldContext(world)
				// Create entity with initial LocationComponent since entities must have at least one component
				entityID, err = cardinal.Create(ctx, &LocationComponent{})
				require.NoError(t, err, "failed to create entity")
			}

			tt.fn(t, world, entityID)
		})
	}
}
