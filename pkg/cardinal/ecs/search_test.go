package ecs_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	. "github.com/argus-labs/world-engine/pkg/cardinal/ecs/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type initSystemState struct {
	Position       ecs.Exact[struct{ ecs.Ref[Position] }]
	Health         ecs.Exact[struct{ ecs.Ref[Health] }]
	Velocity       ecs.Exact[struct{ ecs.Ref[Velocity] }]
	PositionHealth ecs.Exact[struct {
		Position ecs.Ref[Position]
		Health   ecs.Ref[Health]
	}]
	PositionVelocity ecs.Exact[struct {
		Position ecs.Ref[Position]
		Velocity ecs.Ref[Velocity]
	}]
	HealthVelocity ecs.Exact[struct {
		Health   ecs.Ref[Health]
		Velocity ecs.Ref[Velocity]
	}]
}

func TestSearch_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		params  ecs.SearchParam
		wantErr bool
	}{
		{
			name: "empty component list",
			params: ecs.SearchParam{
				Find:  []string{},
				Match: ecs.MatchExact,
			},
			wantErr: true,
		},
		{
			name: "invalid match type",
			params: ecs.SearchParam{
				Find:  []string{"Position"},
				Match: "invalid",
			},
			wantErr: true,
		},
		{
			name: "unregistered component",
			params: ecs.SearchParam{
				Find:  []string{"UnregisteredComponent"},
				Match: ecs.MatchExact,
			},
			wantErr: true,
		},
		{
			name: "invalid where clause syntax",
			params: ecs.SearchParam{
				Find:  []string{"Health"},
				Match: ecs.MatchExact,
				Where: "Health.Value >",
			},
			wantErr: true,
		},
		{
			name: "valid params",
			params: ecs.SearchParam{
				Find:  []string{"Position"},
				Match: ecs.MatchExact,
				Where: "Position.X > 0",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := ecs.NewWorld()
			ecs.RegisterSystem(w, func(state *initSystemState) error {
				return nil // Placeholder system to register components
			}, ecs.WithHook(ecs.Init))

			w.InitSchedulers()

			err := w.InitSystems()
			require.NoError(t, err)

			_, err = w.NewSearch(tt.params)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSearch_FindAndMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		params   ecs.SearchParam
		setup    func(*initSystemState) error
		validate func(*testing.T, []map[string]any)
	}{
		{
			name: "exact match single component",
			params: ecs.SearchParam{
				Find:  []string{"Position"},
				Match: ecs.MatchExact,
			},
			setup: func(state *initSystemState) error {
				_, position := state.Position.Create()
				position.Set(Position{X: 1, Y: 2})

				_, position = state.Position.Create()
				position.Set(Position{X: 3, Y: 4})

				return nil
			},
			validate: func(t *testing.T, results []map[string]any) {
				assert.Len(t, results, 2)
				assert.Contains(t, results,
					map[string]any{"Position": Position{X: 1, Y: 2}, "_id": uint32(0)})
				assert.Contains(t, results,
					map[string]any{"Position": Position{X: 3, Y: 4}, "_id": uint32(1)})
			},
		},
		{
			name: "contains match single component",
			params: ecs.SearchParam{
				Find:  []string{"Position"},
				Match: ecs.MatchContains,
			},
			setup: func(state *initSystemState) error {
				_, position := state.Position.Create()
				position.Set(Position{X: 1, Y: 2})

				_, positionHealth := state.PositionHealth.Create()
				positionHealth.Position.Set(Position{X: 3, Y: 4})
				positionHealth.Health.Set(Health{Value: 100})

				return nil
			},
			validate: func(t *testing.T, results []map[string]any) {
				assert.Len(t, results, 2)
				assert.Contains(t, results,
					map[string]any{"Position": Position{X: 1, Y: 2}, "_id": uint32(0)})
				assert.Contains(t, results,
					map[string]any{
						"Position": Position{X: 3, Y: 4},
						"Health":   Health{Value: 100},
						"_id":      uint32(1),
					})
			},
		},
		{
			name: "empty result for no matching entities",
			params: ecs.SearchParam{
				Find:  []string{"Health"},
				Match: ecs.MatchExact,
			},
			setup: func(state *initSystemState) error {
				_, position := state.Position.Create()
				position.Set(Position{X: 1, Y: 2})
				return nil
			},
			validate: func(t *testing.T, results []map[string]any) {
				assert.Empty(t, results)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := ecs.NewWorld()
			ecs.RegisterSystem(w, tt.setup, ecs.WithHook(ecs.Init))

			w.InitSchedulers()

			err := w.InitSystems()
			require.NoError(t, err)

			results, err := w.NewSearch(tt.params)
			require.NoError(t, err)

			tt.validate(t, results)
		})
	}
}

// TestSearch_Where tests the Where clause filtering functionality.
func TestSearch_Where(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		params   ecs.SearchParam
		setup    func(*initSystemState) error
		validate func(*testing.T, []map[string]any)
	}{
		{
			name: "filter by health value",
			params: ecs.SearchParam{
				Find:  []string{"Health"},
				Match: ecs.MatchContains,
				Where: "Health.Value > 75",
			},
			setup: func(state *initSystemState) error {
				_, health := state.Health.Create()
				health.Set(Health{Value: 100})

				_, health = state.Health.Create()
				health.Set(Health{Value: 50})

				_, health = state.Health.Create()
				health.Set(Health{Value: 80})

				return nil
			},
			validate: func(t *testing.T, results []map[string]any) {
				assert.Len(t, results, 2)
				for _, entity := range results {
					health := entity["Health"].(Health)
					assert.Greater(t, health.Value, 75)
				}
			},
		},
		{
			name: "filter by position coordinates",
			params: ecs.SearchParam{
				Find:  []string{"Position"},
				Match: ecs.MatchContains,
				Where: "Position.X > 0 && Position.Y > 0",
			},
			setup: func(state *initSystemState) error {
				_, position := state.Position.Create()
				position.Set(Position{X: 1, Y: 2})

				_, position = state.Position.Create()
				position.Set(Position{X: -1, Y: 2})

				_, position = state.Position.Create()
				position.Set(Position{X: 3, Y: -4})

				_, position = state.Position.Create()
				position.Set(Position{X: 5, Y: 6})

				return nil
			},
			validate: func(t *testing.T, results []map[string]any) {
				assert.Len(t, results, 2)
				for _, entity := range results {
					pos := entity["Position"].(Position)
					assert.Positive(t, pos.X)
					assert.Positive(t, pos.Y)
				}
			},
		},
		{
			name: "complex filter with multiple components",
			params: ecs.SearchParam{
				Find:  []string{"Position", "Health"},
				Match: ecs.MatchContains,
				Where: "Position.X > 0 && Health.Value >= 100",
			},
			setup: func(state *initSystemState) error {
				_, positionHealth := state.PositionHealth.Create()
				positionHealth.Position.Set(Position{X: 1, Y: 2})
				positionHealth.Health.Set(Health{Value: 100})

				_, positionHealth = state.PositionHealth.Create()
				positionHealth.Position.Set(Position{X: -1, Y: 2})
				positionHealth.Health.Set(Health{Value: 100})

				_, positionHealth = state.PositionHealth.Create()
				positionHealth.Position.Set(Position{X: 3, Y: 4})
				positionHealth.Health.Set(Health{Value: 50})

				_, positionHealth = state.PositionHealth.Create()
				positionHealth.Position.Set(Position{X: 5, Y: 6})
				positionHealth.Health.Set(Health{Value: 150})

				return nil
			},
			validate: func(t *testing.T, results []map[string]any) {
				assert.Len(t, results, 2)
				for _, entity := range results {
					pos := entity["Position"].(Position)
					health := entity["Health"].(Health)
					assert.Positive(t, pos.X)
					assert.GreaterOrEqual(t, health.Value, 100)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := ecs.NewWorld()
			ecs.RegisterSystem(w, tt.setup, ecs.WithHook(ecs.Init))

			w.InitSchedulers()

			err := w.InitSystems()
			require.NoError(t, err)

			results, err := w.NewSearch(tt.params)
			require.NoError(t, err)

			tt.validate(t, results)
		})
	}
}
