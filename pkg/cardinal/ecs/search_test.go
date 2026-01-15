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
			name: "empty component list (now allowed)",
			params: ecs.SearchParam{
				Find:  []string{},
				Match: ecs.MatchExact, // Match is ignored when Find is empty
			},
			wantErr: false, // Empty Find is now allowed
		},
		{
			name: "invalid limit (negative)",
			params: ecs.SearchParam{
				Find:  []string{"Position"},
				Match: ecs.MatchExact,
				Limit: -1,
			},
			wantErr: true,
		},
		{
			name: "invalid limit (too large)",
			params: ecs.SearchParam{
				Find:  []string{"Position"},
				Match: ecs.MatchExact,
				Limit: 10001, // Exceeds MaxQueryLimit
			},
			wantErr: true,
		},
		{
			name: "invalid offset (negative)",
			params: ecs.SearchParam{
				Find:   []string{"Position"},
				Match:  ecs.MatchExact,
				Offset: -1,
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

			w.Init()

			_, err := w.Tick(nil)
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

			w.Init()

			_, err := w.Tick(nil)
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

			w.Init()

			_, err := w.Tick(nil)
			require.NoError(t, err)

			results, err := w.NewSearch(tt.params)
			require.NoError(t, err)

			tt.validate(t, results)
		})
	}
}

// TestSearch_EmptyFind tests querying all entities with empty Find list.
func TestSearch_EmptyFind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		params   ecs.SearchParam
		setup    func(*initSystemState) error
		validate func(*testing.T, []map[string]any)
	}{
		{
			name: "empty find queries all entities",
			params: ecs.SearchParam{
				Find: []string{}, // Empty = all entities
			},
			setup: func(state *initSystemState) error {
				_, position := state.Position.Create()
				position.Set(Position{X: 1, Y: 2})

				_, health := state.Health.Create()
				health.Set(Health{Value: 100})

				_, positionHealth := state.PositionHealth.Create()
				positionHealth.Position.Set(Position{X: 3, Y: 4})
				positionHealth.Health.Set(Health{Value: 200})

				return nil
			},
			validate: func(t *testing.T, results []map[string]any) {
				// Should return all 3 entities (default limit is 50, so all should be returned)
				assert.Len(t, results, 3)
			},
		},
		{
			name: "empty find with where filter",
			params: ecs.SearchParam{
				Find:  []string{},
				Where: "_id < 2", // Filter by entity ID
			},
			setup: func(state *initSystemState) error {
				_, position := state.Position.Create()
				position.Set(Position{X: 1, Y: 2})

				_, health := state.Health.Create()
				health.Set(Health{Value: 100})

				_, positionHealth := state.PositionHealth.Create()
				positionHealth.Position.Set(Position{X: 3, Y: 4})
				positionHealth.Health.Set(Health{Value: 200})

				return nil
			},
			validate: func(t *testing.T, results []map[string]any) {
				// Should return 2 entities with _id < 2
				assert.Len(t, results, 2)
			},
		},
		{
			name: "empty find ignores match",
			params: ecs.SearchParam{
				Find:  []string{},
				Match: "invalid", // Should be ignored when Find is empty
			},
			setup: func(state *initSystemState) error {
				_, position := state.Position.Create()
				position.Set(Position{X: 1, Y: 2})
				return nil
			},
			validate: func(t *testing.T, results []map[string]any) {
				// Should still work and return all entities
				assert.Len(t, results, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := ecs.NewWorld()
			ecs.RegisterSystem(w, tt.setup, ecs.WithHook(ecs.Init))

			w.Init()

			_, err := w.Tick(nil)
			require.NoError(t, err)

			results, err := w.NewSearch(tt.params)
			require.NoError(t, err)

			tt.validate(t, results)
		})
	}
}

// TestSearch_Pagination tests limit and offset functionality.
func TestSearch_Pagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		params   ecs.SearchParam
		setup    func(*initSystemState) error
		validate func(*testing.T, []map[string]any)
	}{
		{
			name: "limit default (50)",
			params: ecs.SearchParam{
				Find:  []string{"Position"},
				Match: ecs.MatchExact,
				Limit: 0, // 0 should use default
			},
			setup: func(state *initSystemState) error {
				// Create 100 entities
				for i := 0; i < 100; i++ {
					_, position := state.Position.Create()
					position.Set(Position{X: i, Y: i})
				}
				return nil
			},
			validate: func(t *testing.T, results []map[string]any) {
				// Should return default limit (50)
				assert.Len(t, results, 50)
			},
		},
		{
			name: "limit specified",
			params: ecs.SearchParam{
				Find:  []string{"Position"},
				Match: ecs.MatchExact,
				Limit: 10,
			},
			setup: func(state *initSystemState) error {
				// Create 20 entities
				for i := 0; i < 20; i++ {
					_, position := state.Position.Create()
					position.Set(Position{X: i, Y: i})
				}
				return nil
			},
			validate: func(t *testing.T, results []map[string]any) {
				// Should return exactly 10
				assert.Len(t, results, 10)
			},
		},
		{
			name: "offset skips entities",
			params: ecs.SearchParam{
				Find:   []string{"Position"},
				Match:  ecs.MatchExact,
				Limit:  5,
				Offset: 3,
			},
			setup: func(state *initSystemState) error {
				// Create 10 entities
				for i := 0; i < 10; i++ {
					_, position := state.Position.Create()
					position.Set(Position{X: i, Y: i})
				}
				return nil
			},
			validate: func(t *testing.T, results []map[string]any) {
				// Should return 5 entities, skipping first 3
				assert.Len(t, results, 5)
			},
		},
		{
			name: "offset beyond total results",
			params: ecs.SearchParam{
				Find:   []string{"Position"},
				Match:  ecs.MatchExact,
				Limit:  10,
				Offset: 100, // More than total entities
			},
			setup: func(state *initSystemState) error {
				// Create only 5 entities
				for i := 0; i < 5; i++ {
					_, position := state.Position.Create()
					position.Set(Position{X: i, Y: i})
				}
				return nil
			},
			validate: func(t *testing.T, results []map[string]any) {
				// Should return empty (offset > total)
				assert.Empty(t, results)
			},
		},
		{
			name: "pagination with where filter",
			params: ecs.SearchParam{
				Find:   []string{"Health"},
				Match:  ecs.MatchContains,
				Where:  "Health.Value > 50",
				Limit:  2,
				Offset: 1,
			},
			setup: func(state *initSystemState) error {
				// Create entities with different health values
				_, health1 := state.Health.Create()
				health1.Set(Health{Value: 100}) // Matches filter

				_, health2 := state.Health.Create()
				health2.Set(Health{Value: 30}) // Doesn't match

				_, health3 := state.Health.Create()
				health3.Set(Health{Value: 80}) // Matches filter

				_, health4 := state.Health.Create()
				health4.Set(Health{Value: 90}) // Matches filter

				return nil
			},
			validate: func(t *testing.T, results []map[string]any) {
				// Should return 2 entities (after filtering and offset)
				// Total matching: 3 (100, 80, 90)
				// After offset 1: 2 entities (80, 90)
				assert.Len(t, results, 2)
				for _, entity := range results {
					health := entity["Health"].(Health)
					assert.Greater(t, health.Value, 50)
				}
			},
		},
		{
			name: "pagination across multiple archetypes",
			params: ecs.SearchParam{
				Find:   []string{"Position"},
				Match:  ecs.MatchContains, // Will match Position-only and Position+Health
				Limit:  3,
				Offset: 1,
			},
			setup: func(state *initSystemState) error {
				// Create entities in different archetypes
				_, position1 := state.Position.Create()
				position1.Set(Position{X: 1, Y: 1})

				_, position2 := state.Position.Create()
				position2.Set(Position{X: 2, Y: 2})

				_, positionHealth := state.PositionHealth.Create()
				positionHealth.Position.Set(Position{X: 3, Y: 3})
				positionHealth.Health.Set(Health{Value: 100})

				_, position3 := state.Position.Create()
				position3.Set(Position{X: 4, Y: 4})

				return nil
			},
			validate: func(t *testing.T, results []map[string]any) {
				// Should return 3 entities, skipping first 1
				// Total: 4 entities with Position
				// After offset 1: 3 entities
				assert.Len(t, results, 3)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := ecs.NewWorld()
			ecs.RegisterSystem(w, tt.setup, ecs.WithHook(ecs.Init))

			w.Init()

			_, err := w.Tick(nil)
			require.NoError(t, err)

			results, err := w.NewSearch(tt.params)
			require.NoError(t, err)

			tt.validate(t, results)
		})
	}
}
