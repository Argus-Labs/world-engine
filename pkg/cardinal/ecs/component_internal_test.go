package ecs

import (
	"testing"

	. "github.com/argus-labs/world-engine/pkg/cardinal/ecs/internal/testutils"
	"github.com/kelindar/bitmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentManager_Register(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupFn    func(*componentManager)
		registerFn func(*componentManager) error
		wantErr    bool
	}{
		{
			name: "register new component successfully",
			registerFn: func(cm *componentManager) error {
				return cm.register("Health", newColumnConstructor[Health]())
			},
		},
		{
			name: "register empty component name",
			registerFn: func(cm *componentManager) error {
				return cm.register("", newColumnConstructor[Health]())
			},
			wantErr: true,
		},
		{
			name: "register duplicate component",
			setupFn: func(cm *componentManager) {
				_ = cm.register("Health", newColumnConstructor[Health]())
			},
			registerFn: func(cm *componentManager) error {
				return cm.register("Health", newColumnConstructor[Health]())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cm := newComponentManager()

			if tt.name == "register duplicate component" {
				err := tt.registerFn(&cm)
				require.NoError(t, err)
			}

			err := tt.registerFn(&cm)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestComponentManager_GetComponentID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setupFn func(*componentManager)
		getID   map[Component]componentID
		wantErr bool
	}{
		{
			name: "get registered component type ID",
			setupFn: func(cm *componentManager) {
				_ = cm.register(Health{}.Name(), newColumnConstructor[Health]())
				_ = cm.register(Position{}.Name(), newColumnConstructor[Position]())
			},
			getID: map[Component]componentID{
				Health{}:   0,
				Position{}: 1,
			},
			wantErr: false,
		},
		{
			name: "error on unregistered component",
			getID: map[Component]componentID{
				Health{}: 0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cm := newComponentManager()
			if tt.setupFn != nil {
				tt.setupFn(&cm)
			}

			for component, expectedID := range tt.getID {
				id, err := cm.getComponentID(component)
				if tt.wantErr {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
				assert.Equal(t, expectedID, id)
			}
		})
	}
}

func TestComponentManager_ToComponentBitmap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupFn    func(*componentManager)
		components []Component
		wantErr    bool
		validate   func(*testing.T, bitmap.Bitmap)
	}{
		{
			name: "get bitmap for single component",
			setupFn: func(cm *componentManager) {
				_ = cm.register(Health{}.Name(), newColumnConstructor[Health]())
			},
			components: []Component{Health{}},
			wantErr:    false,
			validate: func(t *testing.T, b bitmap.Bitmap) {
				var expected bitmap.Bitmap
				expected.Set(0)

				assert.Equal(t, 1, b.Count())
				assert.Equal(t, expected, b, "bitmap should have exactly bit 0 set")
			},
		},
		{
			name: "get bitmap for multiple components",
			setupFn: func(cm *componentManager) {
				_ = cm.register(Health{}.Name(), newColumnConstructor[Health]())
				_ = cm.register(Position{}.Name(), newColumnConstructor[Position]())
			},
			components: []Component{Health{}, Position{}},
			wantErr:    false,
			validate: func(t *testing.T, b bitmap.Bitmap) {
				var expected bitmap.Bitmap
				expected.Set(0)
				expected.Set(1)

				assert.Equal(t, 2, b.Count())
				assert.Equal(t, expected, b, "bitmap should have exactly bits 0 and 1 set")
			},
		},
		{
			name:       "error on unregistered component",
			components: []Component{Health{}},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cm := newComponentManager()
			if tt.setupFn != nil {
				tt.setupFn(&cm)
			}

			bm, err := cm.toComponentBitmap(tt.components)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			tt.validate(t, bm)
		})
	}
}

func TestComponentManager_CreateArchetype(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setupFn  func(*componentManager)
		bitmap   func() bitmap.Bitmap
		validate func(*testing.T, *archetype)
	}{
		{
			name: "create archetype with single component",
			setupFn: func(cm *componentManager) {
				_ = cm.register((&Health{}).Name(), newColumnConstructor[Health]())
			},
			bitmap: func() bitmap.Bitmap {
				var b bitmap.Bitmap
				b.Set(0)
				return b
			},
			validate: func(t *testing.T, a *archetype) {
				assert.Equal(t, 1, a.componentTypeCount)
				assert.Equal(t, len(a.columns), a.componentTypeCount)
			},
		},
		{
			name: "create archetype with multiple components",
			setupFn: func(cm *componentManager) {
				_ = cm.register((&Health{}).Name(), newColumnConstructor[Health]())
				_ = cm.register((&Position{}).Name(), newColumnConstructor[Position]())
			},
			bitmap: func() bitmap.Bitmap {
				var b bitmap.Bitmap
				b.Set(0)
				b.Set(1)
				return b
			},
			validate: func(t *testing.T, a *archetype) {
				assert.Equal(t, 2, a.componentTypeCount)
				assert.Equal(t, len(a.columns), a.componentTypeCount)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cm := newComponentManager()
			if tt.setupFn != nil {
				tt.setupFn(&cm)
			}

			arch := cm.createArchetype(0, tt.bitmap())
			tt.validate(t, &arch)
		})
	}
}
