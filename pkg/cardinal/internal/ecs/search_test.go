package ecs

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/kelindar/bitmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// NewSearch public API tests
// -------------------------------------------------------------------------------------------------
// Verifies NewSearch through its public API. Covers match modes, where filters, pagination, empty
// results, and error conditions. We use JSON as the output format because it's more readable than
// Go's map[string]any. Results are sorted because map keys aren't ordered.
// -------------------------------------------------------------------------------------------------

var compA = testutils.ComponentA{}.Name()
var compB = testutils.ComponentB{}.Name()
var compC = testutils.ComponentC{}.Name()

func TestNewSearch(t *testing.T) {
	t.Parallel()

	// Fixture: 6 entities across 4 archetypes.
	//
	// | Entity | Archetype | ComponentA         | ComponentB        | ComponentC     |
	// |--------|-----------|--------------------|-------------------|----------------|
	// | E0     | {A}       | {X:10}             | —                 | —              |
	// | E1     | {A}       | {X:50}             | —                 | —              |
	// | E2     | {A,B}     | {X:100}            | {Enabled:true}    | —              |
	// | E3     | {A,B}     | {X:5}              | {Enabled:false}   | —              |
	// | E4     | {B,C}     | —                  | {Enabled:true}    | {Counter:1}    |
	// | E5     | {A,B,C}   | {X:200}            | {Enabled:true}    | {Counter:2}    |

	tests := []struct {
		name   string
		params SearchParam
		want   string // JSON expected output, or "ERROR" for expected errors
	}{
		// Happy paths: match modes
		{
			name: "exact match single component",
			params: SearchParam{
				Find:  []string{compA},
				Match: MatchExact,
			},
			want: `[
				{
					"_id": 0,
					"component_a": {"X": 10, "Y": 0, "Z": 0}
				},
				{
					"_id": 1,
					"component_a": {"X": 50, "Y": 0, "Z": 0}
				}
			]`,
		},
		{
			name: "exact match multi-component",
			params: SearchParam{
				Find:  []string{compA, compB},
				Match: MatchExact,
			},
			want: `[
				{
					"_id": 2,
					"component_a": {"X": 100, "Y": 0, "Z": 0},
					"component_b": {"ID": 0, "Label": "", "Enabled": true}
				},
				{
					"_id": 3,
					"component_a": {"X": 5, "Y": 0, "Z": 0},
					"component_b": {"ID": 0, "Label": "", "Enabled": false}
				}
			]`,
		},
		{
			name: "contains match single component",
			params: SearchParam{
				Find:  []string{compA},
				Match: MatchContains,
			},
			want: `[
				{
					"_id": 0,
					"component_a": {"X": 10, "Y": 0, "Z": 0}
				},
				{
					"_id": 1,
					"component_a": {"X": 50, "Y": 0, "Z": 0}
				},
				{
					"_id": 2,
					"component_a": {"X": 100, "Y": 0, "Z": 0},
					"component_b": {"ID": 0, "Label": "", "Enabled": true}
				},
				{
					"_id": 3,
					"component_a": {"X": 5, "Y": 0, "Z": 0},
					"component_b": {"ID": 0, "Label": "", "Enabled": false}
				},
				{
					"_id": 5,
					"component_a": {"X": 200, "Y": 0, "Z": 0},
					"component_b": {"ID": 0, "Label": "", "Enabled": true},
					"component_c": {"Values": [0,0,0,0,0,0,0,0], "Counter": 2}
				}
			]`,
		},
		{
			name: "contains match multi-component",
			params: SearchParam{
				Find:  []string{compA, compB},
				Match: MatchContains,
			},
			want: `[
				{
					"_id": 2,
					"component_a": {"X": 100, "Y": 0, "Z": 0},
					"component_b": {"ID": 0, "Label": "", "Enabled": true}
				},
				{
					"_id": 3,
					"component_a": {"X": 5, "Y": 0, "Z": 0},
					"component_b": {"ID": 0, "Label": "", "Enabled": false}
				},
				{
					"_id": 5,
					"component_a": {"X": 200, "Y": 0, "Z": 0},
					"component_b": {"ID": 0, "Label": "", "Enabled": true},
					"component_c": {"Values": [0,0,0,0,0,0,0,0], "Counter": 2}
				}
			]`,
		},
		{
			name: "match all",
			params: SearchParam{
				Match: MatchAll,
			},
			want: `[
				{
					"_id": 0,
					"component_a": {"X": 10, "Y": 0, "Z": 0}
				},
				{
					"_id": 1,
					"component_a": {"X": 50, "Y": 0, "Z": 0}
				},
				{
					"_id": 2,
					"component_a": {"X": 100, "Y": 0, "Z": 0},
					"component_b": {"ID": 0, "Label": "", "Enabled": true}
				},
				{
					"_id": 3,
					"component_a": {"X": 5, "Y": 0, "Z": 0},
					"component_b": {"ID": 0, "Label": "", "Enabled": false}
				},
				{
					"_id": 4,
					"component_b": {"ID": 0, "Label": "", "Enabled": true},
					"component_c": {"Values": [0,0,0,0,0,0,0,0], "Counter": 1}
				},
				{
					"_id": 5,
					"component_a": {"X": 200, "Y": 0, "Z": 0},
					"component_b": {"ID": 0, "Label": "", "Enabled": true},
					"component_c": {"Values": [0,0,0,0,0,0,0,0], "Counter": 2}
				}
			]`,
		},

		// Where filter
		{
			name: "where filters by component field",
			params: SearchParam{
				Find:  []string{compA},
				Match: MatchContains,
				Where: compA + ".X > 50",
			},
			want: `[
				{
					"_id": 2,
					"component_a": {"X": 100, "Y": 0, "Z": 0},
					"component_b": {"ID": 0, "Label": "", "Enabled": true}
				},
				{
					"_id": 5,
					"component_a": {"X": 200, "Y": 0, "Z": 0},
					"component_b": {"ID": 0, "Label": "", "Enabled": true},
					"component_c": {"Values": [0,0,0,0,0,0,0,0], "Counter": 2}
				}
			]`,
		},
		{
			name: "where matches nothing returns empty",
			params: SearchParam{
				Find:  []string{compA},
				Match: MatchContains,
				Where: compA + ".X > 999",
			},
			want: `[]`,
		},

		// Pagination
		{
			name: "limit truncates results",
			params: SearchParam{
				Find:  []string{compA},
				Match: MatchContains,
				Limit: 2,
			},
			want: `[
				{
					"_id": 0,
					"component_a": {"X": 10, "Y": 0, "Z": 0}
				},
				{
					"_id": 1,
					"component_a": {"X": 50, "Y": 0, "Z": 0}
				}
			]`,
		},
		{
			name: "offset skips results",
			params: SearchParam{
				Find:   []string{compA},
				Match:  MatchExact,
				Offset: 1,
			},
			want: `[
				{
					"_id": 1,
					"component_a": {"X": 50, "Y": 0, "Z": 0}
				}
			]`,
		},
		{
			name: "where plus offset plus limit compose correctly",
			params: SearchParam{
				Find:   []string{compA},
				Match:  MatchContains,
				Where:  compA + ".X >= 10",
				Offset: 1,
				Limit:  2,
			},
			want: `[
				{
					"_id": 1,
					"component_a": {"X": 50, "Y": 0, "Z": 0}
				},
				{
					"_id": 2,
					"component_a": {"X": 100, "Y": 0, "Z": 0},
					"component_b": {"ID": 0, "Label": "", "Enabled": true}
				}
			]`,
		},
		{
			name: "offset exceeds match count returns empty",
			params: SearchParam{
				Find:   []string{compA},
				Match:  MatchExact,
				Offset: 100,
			},
			want: `[]`,
		},

		// Empty results (non-error)
		{
			name: "no archetype matches returns empty",
			params: SearchParam{
				Find:  []string{compC},
				Match: MatchExact,
			},
			want: `[]`,
		},

		// Errors
		{
			name: "error: match all with non-empty find",
			params: SearchParam{
				Find:  []string{compA},
				Match: MatchAll,
			},
			want: "ERROR",
		},
		{
			name: "error: non-all match with empty find",
			params: SearchParam{
				Match: MatchExact,
			},
			want: "ERROR",
		},
		{
			name: "error: unregistered component",
			params: SearchParam{
				Find:  []string{"nonexistent"},
				Match: MatchExact,
			},
			want: "ERROR",
		},
		{
			name: "error: invalid where syntax",
			params: SearchParam{
				Find:  []string{compA},
				Match: MatchExact,
				Where: ">>=broken",
			},
			want: "ERROR",
		},
		{
			name: "error: where references field missing at runtime",
			params: SearchParam{
				Find:  []string{compA},
				Match: MatchContains,
				Where: compB + ".Enabled == true",
			},
			want: "ERROR",
		},
	}

	// Setup: create world and populate fixture.
	w := newSearchTestWorld(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			results, err := w.NewSearch(tt.params)
			if tt.want == "ERROR" {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			sort.Slice(results, func(i, j int) bool {
				return results[i]["_id"].(uint32) < results[j]["_id"].(uint32)
			})

			got, err := json.Marshal(results)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(got))
		})
	}
}

// newSearchTestWorld creates a World with the standard search test fixture:
// 6 entities across 4 archetypes (see table in TestNewSearch).
func newSearchTestWorld(t *testing.T) *World {
	t.Helper()

	w := NewWorld()
	w.OnComponentRegister(func(Component) error { return nil })

	_, err := registerComponent[testutils.ComponentA](w)
	require.NoError(t, err)
	_, err = registerComponent[testutils.ComponentB](w)
	require.NoError(t, err)
	_, err = registerComponent[testutils.ComponentC](w)
	require.NoError(t, err)

	ws := w.state

	// Helper to build a bitmap from component names and create an entity in that archetype.
	createEntity := func(names ...string) EntityID {
		t.Helper()
		var bm bitmap.Bitmap
		for _, name := range names {
			id, ok := ws.components.catalog[name]
			require.True(t, ok, "component %s not registered", name)
			bm.Set(id)
		}
		return ws.newEntityWithArchetype(bm)
	}

	// E0: {A} with X=10
	e0 := createEntity(compA)
	require.NoError(t, setComponent(ws, e0, testutils.ComponentA{X: 10}))

	// E1: {A} with X=50
	e1 := createEntity(compA)
	require.NoError(t, setComponent(ws, e1, testutils.ComponentA{X: 50}))

	// E2: {A,B} with X=100, Enabled=true
	e2 := createEntity(compA, compB)
	require.NoError(t, setComponent(ws, e2, testutils.ComponentA{X: 100}))
	require.NoError(t, setComponent(ws, e2, testutils.ComponentB{Enabled: true}))

	// E3: {A,B} with X=5, Enabled=false
	e3 := createEntity(compA, compB)
	require.NoError(t, setComponent(ws, e3, testutils.ComponentA{X: 5}))
	require.NoError(t, setComponent(ws, e3, testutils.ComponentB{Enabled: false}))

	// E4: {B,C} with Enabled=true, Counter=1
	e4 := createEntity(compB, compC)
	require.NoError(t, setComponent(ws, e4, testutils.ComponentB{Enabled: true}))
	require.NoError(t, setComponent(ws, e4, testutils.ComponentC{Counter: 1}))

	// E5: {A,B,C} with X=200, Enabled=true, Counter=2
	e5 := createEntity(compA, compB, compC)
	require.NoError(t, setComponent(ws, e5, testutils.ComponentA{X: 200}))
	require.NoError(t, setComponent(ws, e5, testutils.ComponentB{Enabled: true}))
	require.NoError(t, setComponent(ws, e5, testutils.ComponentC{Counter: 2}))

	return w
}
