package ecs

import (
	"testing"

	. "github.com/argus-labs/world-engine/pkg/cardinal/ecs/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponentManager_Register(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupFn    func(*componentManager)
		registerFn func(*componentManager) (componentID, error)
		wantID     componentID
		wantErr    bool
	}{
		{
			name: "register new component successfully",
			registerFn: func(cm *componentManager) (componentID, error) {
				return cm.register("Health", newColumnFactory[Health]())
			},
			wantID: 0,
		},
		{
			name: "register empty component name",
			registerFn: func(cm *componentManager) (componentID, error) {
				return cm.register("", newColumnFactory[Health]())
			},
			wantErr: true,
		},
		{
			name: "register duplicate component",
			setupFn: func(cm *componentManager) {
				_, _ = cm.register("Health", newColumnFactory[Health]())
			},
			registerFn: func(cm *componentManager) (componentID, error) {
				return cm.register("Health", newColumnFactory[Health]())
			},
			wantID: 0,
		},
		{
			name: "register second component gets ID 1",
			setupFn: func(cm *componentManager) {
				_, _ = cm.register("Health", newColumnFactory[Health]())
			},
			registerFn: func(cm *componentManager) (componentID, error) {
				return cm.register("Position", newColumnFactory[Position]())
			},
			wantID: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cm := newComponentManager()

			if tt.setupFn != nil {
				tt.setupFn(&cm)
			}

			id, err := tt.registerFn(&cm)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, id)
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
				_, _ = cm.register(Health{}.Name(), newColumnFactory[Health]())
				_, _ = cm.register(Position{}.Name(), newColumnFactory[Position]())
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
				id, err := cm.getID(component.Name())
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

func TestValidateComponentName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		compName  string
		wantError bool
	}{
		// Valid names
		{name: "simple name", compName: "Health", wantError: false},
		{name: "with underscore", compName: "player_data", wantError: false},
		{name: "starts with underscore", compName: "_internal", wantError: false},
		{name: "with numbers", compName: "Component123", wantError: false},

		// Invalid names
		{name: "empty string", compName: "", wantError: true},
		{name: "starts with number", compName: "123Component", wantError: true},
		{name: "contains hyphen", compName: "player-data", wantError: true},
		{name: "contains space", compName: "my component", wantError: true},
		{name: "contains dot", compName: "player.data", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateComponentName(tt.compName)

			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
