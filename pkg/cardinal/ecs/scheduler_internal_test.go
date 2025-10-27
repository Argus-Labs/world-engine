package ecs

import (
	"slices"
	"sync"
	"testing"

	. "github.com/argus-labs/world-engine/pkg/cardinal/ecs/internal/testutils"
	"github.com/kelindar/bitmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduler_RunExecutionOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setupFn func(w *World, mu *sync.Mutex, executionOrder *[]string)
		testFn  func(t *testing.T, executionOrder []string)
	}{
		{
			name:    "zero systems",
			setupFn: func(w *World, mu *sync.Mutex, executionOrder *[]string) {},
			testFn: func(t *testing.T, executionOrder []string) {
				assert.Empty(t, executionOrder)
			},
		},
		{
			name: "single system",
			setupFn: func(w *World, mu *sync.Mutex, executionOrder *[]string) {
				type systemStateA struct {
					Exact[struct {
						Health   Ref[Health]
						Position Ref[Position]
						Velocity Ref[Velocity]
					}]
				}
				RegisterSystem(w, func(state *systemStateA) error {
					appendSystem(mu, executionOrder, "A")
					return nil
				})
			},
			testFn: func(t *testing.T, executionOrder []string) {
				assert.Len(t, executionOrder, 1)
				assert.Equal(t, "A", executionOrder[0])
			},
		},
		{
			name: "multiple independent systems",
			setupFn: func(w *World, mu *sync.Mutex, executionOrder *[]string) {
				type systemStateA struct{ Exact[struct{ Ref[Health] }] }
				type systemStateB struct {
					Exact[struct{ Ref[Position] }]
				}
				type systemStateC struct {
					Exact[struct{ Ref[Velocity] }]
				}
				RegisterSystem(w, func(state *systemStateA) error {
					appendSystem(mu, executionOrder, "A")
					return nil
				})
				RegisterSystem(w, func(state *systemStateB) error {
					appendSystem(mu, executionOrder, "B")
					return nil
				})
				RegisterSystem(w, func(state *systemStateC) error {
					appendSystem(mu, executionOrder, "C")
					return nil
				})
			},
			testFn: func(t *testing.T, executionOrder []string) {
				assert.Len(t, executionOrder, 3)
				for _, s := range []string{"A", "B", "C"} {
					assert.True(t, slices.Contains(executionOrder, s))
				}
			},
		},
		{
			name: "two systems with shared dependency (A->B)",
			setupFn: func(w *World, mu *sync.Mutex, executionOrder *[]string) {
				type systemStateA struct{ Exact[struct{ Ref[Health] }] }
				type systemStateB struct {
					Exact[struct {
						Health   Ref[Health]
						Position Ref[Position]
					}]
				}
				RegisterSystem(w, func(state *systemStateA) error {
					appendSystem(mu, executionOrder, "A")
					return nil
				})
				RegisterSystem(w, func(state *systemStateB) error {
					appendSystem(mu, executionOrder, "B")
					return nil
				})
			},
			testFn: func(t *testing.T, executionOrder []string) {
				assert.Len(t, executionOrder, 2)
				assert.Equal(t, []string{"A", "B"}, executionOrder)
			},
		},
		{
			name: "three systems with chain dependency (A->B->C)",
			setupFn: func(w *World, mu *sync.Mutex, executionOrder *[]string) {
				type systemStateA struct{ Exact[struct{ Ref[Health] }] }
				type systemStateB struct {
					Exact[struct {
						Health   Ref[Health]
						Position Ref[Position]
					}]
				}
				type systemStateC struct {
					Exact[struct {
						Health   Ref[Health]
						Position Ref[Position]
					}]
				}
				RegisterSystem(w, func(state *systemStateA) error {
					appendSystem(mu, executionOrder, "A")
					return nil
				})
				RegisterSystem(w, func(state *systemStateB) error {
					appendSystem(mu, executionOrder, "B")
					return nil
				})
				RegisterSystem(w, func(state *systemStateC) error {
					appendSystem(mu, executionOrder, "C")
					return nil
				})
			},
			testFn: func(t *testing.T, executionOrder []string) {
				assert.Len(t, executionOrder, 3)
				assert.Equal(t, []string{"A", "B", "C"}, executionOrder)
			},
		},
		{
			name: "diamond dependencies (A->B | A->C | B->D | C->D)",
			setupFn: func(w *World, mu *sync.Mutex, executionOrder *[]string) {
				type systemStateA struct {
					Exact[struct {
						Health   Ref[Health]
						Position Ref[Position]
					}]
				}
				type systemStateB struct {
					Exact[struct {
						Health   Ref[Health]
						Velocity Ref[Velocity]
					}]
				}
				type systemStateC struct {
					Exact[struct {
						Position   Ref[Position]
						Experience Ref[Experience]
					}]
				}
				type systemStateD struct {
					Exact[struct {
						Velocity   Ref[Velocity]
						Experience Ref[Experience]
					}]
				}
				RegisterSystem(w, func(state *systemStateA) error {
					appendSystem(mu, executionOrder, "A")
					return nil
				})
				RegisterSystem(w, func(state *systemStateB) error {
					appendSystem(mu, executionOrder, "B")
					return nil
				})
				RegisterSystem(w, func(state *systemStateC) error {
					appendSystem(mu, executionOrder, "C")
					return nil
				})
				RegisterSystem(w, func(state *systemStateD) error {
					appendSystem(mu, executionOrder, "D")
					return nil
				})
			},
			testFn: func(t *testing.T, executionOrder []string) {
				assert.Len(t, executionOrder, 4)
				assert.Equal(t, "A", executionOrder[0])
				assert.Equal(t, "D", executionOrder[3])
				assert.True(t, slices.Contains(executionOrder, "B"))
				assert.True(t, slices.Contains(executionOrder, "C"))
			},
		},
		{
			name: "two separate dependency chains (A->B->C | D->E)",
			setupFn: func(w *World, mu *sync.Mutex, executionOrder *[]string) {
				type systemStateA struct{ Exact[struct{ Ref[Health] }] }
				type systemStateB struct{ Exact[struct{ Ref[Health] }] }
				type systemStateC struct {
					Exact[struct {
						Health   Ref[Health]
						Position Ref[Position]
					}]
				}
				type systemStateD struct {
					Exact[struct {
						Velocity   Ref[Velocity]
						Experience Ref[Experience]
					}]
				}
				type systemStateE struct {
					Exact[struct{ Ref[Experience] }]
				}
				RegisterSystem(w, func(state *systemStateA) error {
					appendSystem(mu, executionOrder, "A")
					return nil
				})
				RegisterSystem(w, func(state *systemStateB) error {
					appendSystem(mu, executionOrder, "B")
					return nil
				})
				RegisterSystem(w, func(state *systemStateC) error {
					appendSystem(mu, executionOrder, "C")
					return nil
				})
				RegisterSystem(w, func(state *systemStateD) error {
					appendSystem(mu, executionOrder, "D")
					return nil
				})
				RegisterSystem(w, func(state *systemStateE) error {
					appendSystem(mu, executionOrder, "E")
					return nil
				})
			},
			testFn: func(t *testing.T, executionOrder []string) {
				assert.Len(t, executionOrder, 5)
				for _, s := range []string{"A", "B", "C", "D", "E"} {
					assert.True(t, slices.Contains(executionOrder, s))
				}
				assert.Less(t, slices.Index(executionOrder, "A"), slices.Index(executionOrder, "B"))
				assert.Less(t, slices.Index(executionOrder, "B"), slices.Index(executionOrder, "C"))
				assert.Less(t, slices.Index(executionOrder, "D"), slices.Index(executionOrder, "E"))
			},
		},
		{
			name: "merged separate dependency chains (A->B->C->F | D->E->F)",
			setupFn: func(w *World, mu *sync.Mutex, executionOrder *[]string) {
				type systemStateA struct{ Exact[struct{ Ref[Health] }] }
				type systemStateB struct{ Exact[struct{ Ref[Health] }] }
				type systemStateC struct {
					Exact[struct {
						Health   Ref[Health]
						Position Ref[Position]
					}]
				}
				type systemStateD struct {
					Exact[struct {
						Velocity   Ref[Velocity]
						Experience Ref[Experience]
					}]
				}
				type systemStateE struct {
					Exact[struct{ Ref[Experience] }]
				}
				type systemStateF struct {
					Exact[struct {
						Experience Ref[Experience]
						Position   Ref[Position]
					}]
				}
				RegisterSystem(w, func(state *systemStateA) error {
					appendSystem(mu, executionOrder, "A")
					return nil
				})
				RegisterSystem(w, func(state *systemStateB) error {
					appendSystem(mu, executionOrder, "B")
					return nil
				})
				RegisterSystem(w, func(state *systemStateC) error {
					appendSystem(mu, executionOrder, "C")
					return nil
				})
				RegisterSystem(w, func(state *systemStateD) error {
					appendSystem(mu, executionOrder, "D")
					return nil
				})
				RegisterSystem(w, func(state *systemStateE) error {
					appendSystem(mu, executionOrder, "E")
					return nil
				})
				RegisterSystem(w, func(state *systemStateF) error {
					appendSystem(mu, executionOrder, "F")
					return nil
				})
			},
			testFn: func(t *testing.T, executionOrder []string) {
				assert.Len(t, executionOrder, 6)
				for _, s := range []string{"A", "B", "C", "D", "E", "F"} {
					assert.True(t, slices.Contains(executionOrder, s))
				}
				assert.Less(t, slices.Index(executionOrder, "A"), slices.Index(executionOrder, "B"))
				assert.Less(t, slices.Index(executionOrder, "B"), slices.Index(executionOrder, "C"))
				assert.Less(t, slices.Index(executionOrder, "D"), slices.Index(executionOrder, "E"))
				assert.Equal(t, "F", executionOrder[5])
			},
		},
		{
			name: "system with multiple dependencies (A->D | B->D | C->D)",
			setupFn: func(w *World, mu *sync.Mutex, executionOrder *[]string) {
				type systemStateA struct{ Exact[struct{ Ref[Health] }] }
				type systemStateB struct {
					Exact[struct{ Ref[Position] }]
				}
				type systemStateC struct {
					Exact[struct{ Ref[Velocity] }]
				}
				type systemStateD struct {
					Exact[struct {
						Health   Ref[Health]
						Position Ref[Position]
						Velocity Ref[Velocity]
					}]
				}
				RegisterSystem(w, func(state *systemStateA) error {
					appendSystem(mu, executionOrder, "A")
					return nil
				})
				RegisterSystem(w, func(state *systemStateB) error {
					appendSystem(mu, executionOrder, "B")
					return nil
				})
				RegisterSystem(w, func(state *systemStateC) error {
					appendSystem(mu, executionOrder, "C")
					return nil
				})
				RegisterSystem(w, func(state *systemStateD) error {
					appendSystem(mu, executionOrder, "D")
					return nil
				})
			},
			testFn: func(t *testing.T, executionOrder []string) {
				assert.Len(t, executionOrder, 4)
				for _, s := range []string{"A", "B", "C", "D"} {
					assert.True(t, slices.Contains(executionOrder, s))
				}
				assert.Equal(t, "D", executionOrder[3])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := NewWorld()
			ws := w.state
			_, err := registerComponent[Position](ws)
			require.NoError(t, err)
			_, err = registerComponent[Velocity](ws)
			require.NoError(t, err)
			_, err = registerComponent[Health](ws)
			require.NoError(t, err)
			_, err = registerComponent[PlayerTag](ws)
			require.NoError(t, err)
			_, err = registerComponent[Experience](ws)
			require.NoError(t, err)
			_, err = registerComponent[Level](ws)
			require.NoError(t, err)

			var mu sync.Mutex
			var executionOrder []string

			tt.setupFn(w, &mu, &executionOrder)
			w.scheduler[Update].createSchedule()

			// Make sure all properties hold after multiple ticks.
			for range 100 {
				executionOrder = executionOrder[:0]
				w.CustomTick(func(_ *worldState) {
					err := w.scheduler[Update].Run()
					require.NoError(t, err)
					tt.testFn(t, executionOrder)
				})
			}
		})
	}
}

func appendSystem(mu *sync.Mutex, executionOrder *[]string, system string) {
	mu.Lock()
	*executionOrder = append(*executionOrder, system)
	mu.Unlock()
}

func TestScheduler_BuildDependencyGraph(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		systems      []systemMetadata
		wantGraph    map[int][]int
		wantIndegree map[int]int
	}{
		{
			name:         "zero systems",
			systems:      []systemMetadata{},
			wantGraph:    map[int][]int{},
			wantIndegree: map[int]int{},
		},
		{
			name: "single system",
			systems: []systemMetadata{
				{deps: createSystemDeps(1, 2, 3)},
			},
			wantGraph:    map[int][]int{},
			wantIndegree: map[int]int{},
		},
		{
			name: "multiple independent systems",
			systems: []systemMetadata{
				{name: "A", deps: createSystemDeps(1)},
				{name: "B", deps: createSystemDeps(2)},
				{name: "C", deps: createSystemDeps(3)},
			},
			wantGraph:    map[int][]int{},
			wantIndegree: map[int]int{},
		},
		{
			name: "two systems with shared dependency (A->B)",
			systems: []systemMetadata{
				{name: "A", deps: createSystemDeps(1, 2)},
				{name: "B", deps: createSystemDeps(2, 3)},
			},
			wantGraph:    map[int][]int{0: {1}},
			wantIndegree: map[int]int{1: 1},
		},
		{
			name: "three systems with chain dependency (A->B->C)",
			systems: []systemMetadata{
				{name: "A", deps: createSystemDeps(1, 2)},
				{name: "B", deps: createSystemDeps(2, 3)},
				{name: "C", deps: createSystemDeps(3, 4)},
			},
			wantGraph: map[int][]int{
				0: {1},
				1: {2},
			},
			wantIndegree: map[int]int{
				1: 1,
				2: 1,
			},
		},
		{
			name: "diamond dependencies (A->B | A->C | B->D | C->D)",
			systems: []systemMetadata{
				{deps: createSystemDeps(1, 2)},

				{deps: createSystemDeps(1, 3)},
				{deps: createSystemDeps(2, 4)},

				{deps: createSystemDeps(3, 4)},
			},
			wantGraph: map[int][]int{
				0: {1, 2},
				1: {3},
				2: {3},
			},
			wantIndegree: map[int]int{
				1: 1,
				2: 1,
				3: 2,
			},
		},
		{
			name: "two separate dependency chains (A->B->C | D->E)",
			systems: []systemMetadata{
				{deps: createSystemDeps(1, 2)},
				{deps: createSystemDeps(2, 3)},
				{deps: createSystemDeps(3, 4)},

				{deps: createSystemDeps(5, 6)},
				{deps: createSystemDeps(6, 7)},
			},
			wantGraph: map[int][]int{
				0: {1},
				1: {2},
				3: {4},
			},
			wantIndegree: map[int]int{
				1: 1,
				2: 1,
				4: 1,
			},
		},
		{
			name: "merged separate dependency chains (A->B->C->F | D->E->F)",
			systems: []systemMetadata{
				{deps: createSystemDeps(1, 2)},
				{deps: createSystemDeps(2, 3)},
				{deps: createSystemDeps(3, 4)},

				{deps: createSystemDeps(5, 6)},
				{deps: createSystemDeps(6, 7)},

				{deps: createSystemDeps(4, 7)},
			},
			wantGraph: map[int][]int{
				0: {1},
				1: {2},
				2: {5},
				3: {4},
				4: {5},
			},
			wantIndegree: map[int]int{
				1: 1,
				2: 1,
				4: 1,
				5: 2,
			},
		},
		{
			name: "system with multiple dependencies (A->D | B->D | C->D)",
			systems: []systemMetadata{
				{deps: createSystemDeps(1)},
				{deps: createSystemDeps(2)},
				{deps: createSystemDeps(3)},

				{deps: createSystemDeps(1, 2, 3)},
			},
			wantGraph: map[int][]int{
				0: {3},
				1: {3},
				2: {3},
			},
			wantIndegree: map[int]int{
				3: 3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			graph, indegree := buildDependencyGraph(tt.systems)
			assert.Equal(t, tt.wantGraph, graph)
			assert.Equal(t, tt.wantIndegree, indegree)
		})
	}
}

func TestScheduler_CreateExecutionTiers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		systems   []systemMetadata
		wantTier0 []int
	}{
		{
			name:      "zero systems",
			systems:   []systemMetadata{},
			wantTier0: []int(nil),
		},
		{
			name: "single system",
			systems: []systemMetadata{
				{deps: createSystemDeps(1, 2, 3)},
			},
			wantTier0: []int{0},
		},
		{
			name: "multiple independent systems",
			systems: []systemMetadata{
				{deps: createSystemDeps(1)},
				{deps: createSystemDeps(2)},
				{deps: createSystemDeps(3)},
			},
			wantTier0: []int{0, 1, 2},
		},
		{
			name: "two systems with shared dependency (A->B)",
			systems: []systemMetadata{
				{deps: createSystemDeps(1, 2)},
				{deps: createSystemDeps(2, 3)},
			},
			wantTier0: []int{0},
		},
		{
			name: "three systems with chain dependency (A->B->C)",
			systems: []systemMetadata{
				{deps: createSystemDeps(1, 2)},
				{deps: createSystemDeps(2, 3)},
				{deps: createSystemDeps(3, 4)},
			},
			wantTier0: []int{0},
		},
		{
			name: "diamond dependencies (A->B | A->C | B->D | C->D)",
			systems: []systemMetadata{
				{deps: createSystemDeps(1, 2)},

				{deps: createSystemDeps(1, 3)},
				{deps: createSystemDeps(2, 4)},

				{deps: createSystemDeps(3, 4)},
			},
			wantTier0: []int{0},
		},
		{
			name: "two separate dependency chains (A->B->C | D->E)",
			systems: []systemMetadata{
				{deps: createSystemDeps(1, 2)},
				{deps: createSystemDeps(2, 3)},
				{deps: createSystemDeps(3, 4)},

				{deps: createSystemDeps(5, 6)},
				{deps: createSystemDeps(6, 7)},
			},
			wantTier0: []int{0, 3},
		},
		{
			name: "merged separate dependency chains (A->B->C->F | D->E->F)",
			systems: []systemMetadata{
				{deps: createSystemDeps(1, 2)},
				{deps: createSystemDeps(2, 3)},
				{deps: createSystemDeps(3, 4)},

				{deps: createSystemDeps(5, 6)},
				{deps: createSystemDeps(6, 7)},

				{deps: createSystemDeps(4, 7)},
			},
			wantTier0: []int{0, 3},
		},
		{
			name: "system with multiple dependencies (A->D | B->D | C->D)",
			systems: []systemMetadata{
				{deps: createSystemDeps(1)},
				{deps: createSystemDeps(2)},
				{deps: createSystemDeps(3)},

				{deps: createSystemDeps(1, 2, 3)},
			},
			wantTier0: []int{0, 1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, indegree := buildDependencyGraph(tt.systems)
			tier0 := getFirstTier(tt.systems, indegree)
			assert.Equal(t, tt.wantTier0, tier0)
		})
	}
}

// Helper function to create the component dependencies bitmap.
func createSystemDeps(componentIDs ...uint32) bitmap.Bitmap {
	deps := bitmap.Bitmap{}
	for _, id := range componentIDs {
		deps.Set(id)
	}
	return deps
}
