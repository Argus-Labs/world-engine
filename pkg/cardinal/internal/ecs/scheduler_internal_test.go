package ecs

import (
	"math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/kelindar/bitmap"
	"github.com/stretchr/testify/assert"
)

// -------------------------------------------------------------------------------------------------
// Concurrent systems fuzz
// -------------------------------------------------------------------------------------------------
// This test verifies that the scheduler's Run method maintains correct concurrent execution
// behavior. We generate random system configurations with random component dependencies, instrument
// each system with a logical clock to track execution ordering, and verify that all systems execute
// exactly once and respect their dependency ordering across multiple ticks.
// -------------------------------------------------------------------------------------------------

func TestScheduler_RunFuzzConcurrent(t *testing.T) {
	t.Parallel()

	const (
		opsMax     = 1 << 10 // 1024 test cases
		systemsMax = 100
		ticksMax   = 10
	)

	synctest.Test(t, func(t *testing.T) {
		prng := testutils.NewRand(t)

		for range opsMax {
			// Generate random systems with random dependencies.
			numSystems := prng.IntN(systemsMax) + 1
			systems := make([]systemMetadata, numSystems)
			for i := range numSystems {
				systems[i] = randSystem(prng)
			}

			// We use a logical clock (atomic counter) to track execution ordering, where each system
			// records its start/end time by incrementing the clock. We compare the start/end times of the
			// systems to check that they follow the correct schedule created from the dependency graph.
			// For example, if A depends on B, then A's start time must be after B's end time.
			var clock atomic.Int64
			var mu sync.Mutex
			events := make([]struct{ start, end int64 }, numSystems)

			scheduler := newSystemScheduler()
			for i, sys := range systems {
				systemID := i
				scheduler.register(sys.name, sys.deps, func() {
					start := clock.Add(2)
					// We add a sleep here to simulate goroutine interleaving for a more realistic test
					// scenario. In synctest.Test, the time package uses a fake clock, so this sleep doesn't
					// actually run for 2 seconds, it returns immediately.
					time.Sleep(2 * time.Second)
					end := clock.Add(1)

					mu.Lock()
					// Assert system hasn't run yet this tick (exactly once check).
					assert.Zero(t, events[systemID], "system %d executed more than once", systemID)
					events[systemID] = struct{ start, end int64 }{start: start, end: end}
					mu.Unlock()
				})
			}
			scheduler.createSchedule()

			// Run the scheduler multiple times to test double-buffer logic.
			for range ticksMax {
				// Reset tracking state for this run.
				clock.Store(0)
				for i := range events {
					events[i] = struct{ start, end int64 }{}
				}

				scheduler.Run()

				// Property: All systems execute exactly once.
				for i, ev := range events {
					assert.NotZero(t, ev.start, "system %d did not execute", i)
					assert.NotZero(t, ev.end, "system %d did not record end", i)
					assert.Less(t, ev.start, ev.end, "system %d has invalid timing", i)
				}

				// Property: systems follow the dependency ordering.
				for a, dependents := range scheduler.graph {
					for _, b := range dependents {
						assert.Less(t, events[a].end, events[b].start,
							"dependency violated: system %d (end=%d) should complete before system %d (start=%d)",
							a, events[a].end, b, events[b].start)
					}
				}
			}
		}
	})
}

// -------------------------------------------------------------------------------------------------
// Schedule graph fuzz
// -------------------------------------------------------------------------------------------------
// This test verifies the buildDependencyGraph function by generating random system configurations
// and checking that key invariants hold. Rather than comparing against a reference model, we
// verify structural properties that any correct dependency graph must satisfy.
// -------------------------------------------------------------------------------------------------

func TestScheduler_GraphFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		opsMax     = 1 << 12 // 4096 iterations
		systemsMax = 100     // Maximum number of systems per test
	)

	for range opsMax {
		// Generate random systems.
		numSystems := prng.IntN(systemsMax) + 1
		systems := make([]systemMetadata, numSystems)
		for i := range numSystems {
			systems[i] = randSystem(prng)
		}

		graph, indegree := buildDependencyGraph(systems)

		// Property: Idempotence - running buildDependencyGraph again produces the same result.
		// The function should be deterministic for the same input.
		graph2, indegree2 := buildDependencyGraph(systems)
		assert.Equal(t, graph, graph2, "graph not idempotent")
		assert.Equal(t, indegree, indegree2, "indegree not idempotent")

		// -------------------------------------------------------------------------
		// Graph properties
		// -------------------------------------------------------------------------

		// Property: Topological ordering - for all edges (u -> v), u < v.
		for u, neighbors := range graph {
			for _, v := range neighbors {
				assert.Less(t, u, v, "edge %d -> %d violates topological order", u, v)
			}
		}

		// Property: Graph is a permutation of the input nodes.
		// This checks all nodes in the graph is a valid node from the input.
		seenNodes := make(map[int]bool)
		for u, neighbors := range graph {
			assert.GreaterOrEqual(t, u, 0, "source node %d is negative", u)
			assert.Less(t, u, len(systems), "source node %d out of bounds", u)
			seenNodes[u] = true
			for _, v := range neighbors {
				assert.GreaterOrEqual(t, v, 0, "target node %d is negative", v)
				assert.Less(t, v, len(systems), "target node %d out of bounds", v)
				seenNodes[v] = true
			}
		}
		// This checks all nodes 0..len(systems)-1 are present in the graph.
		for i := range len(systems) {
			assert.True(t, seenNodes[i], "node %d missing from graph", i)
		}

		// Property: No duplicate edges. Each edge (u -> v) should appear exactly once in graph[u].
		for u, neighbors := range graph {
			seen := make(map[int]bool)
			for _, v := range neighbors {
				assert.False(t, seen[v], "duplicate edge %d -> %d", u, v)
				seen[v] = true
			}
		}

		// Property: The graph contains no cycles (acyclicity).
		// This is implied by the topological ordering property (u < v), but we check it explicitly
		// as extra safety in case we change the sorting algorithm in the future. Here, we use Kahn's
		// algorithm: if we can process all nodes, the graph is acyclic.
		tempIndegree := make(map[int]int)
		for k, v := range indegree {
			tempIndegree[k] = v
		}
		var queue []int
		for i := range systems {
			if tempIndegree[i] == 0 {
				queue = append(queue, i)
			}
		}
		processed := 0
		for len(queue) > 0 {
			node := queue[0]
			queue = queue[1:]
			processed++
			for _, neighbor := range graph[node] {
				tempIndegree[neighbor]--
				if tempIndegree[neighbor] == 0 {
					queue = append(queue, neighbor)
				}
			}
		}
		assert.Equal(t, len(systems), processed, "graph contains a cycle")

		// -------------------------------------------------------------------------
		// Indegree properties
		// -------------------------------------------------------------------------

		// Property: Indegree consistency. indegree[v] equals the number of incoming edges to v.
		expectedIndegree := make(map[int]int)
		for _, neighbors := range graph {
			for _, v := range neighbors {
				expectedIndegree[v]++
			}
		}
		assert.Equal(t, expectedIndegree, indegree, "indegree mismatch")

		// Property: Sum of indegrees equals total edge count.
		// Each edge contributes exactly 1 to one node's indegree.
		totalEdges := 0
		for _, neighbors := range graph {
			totalEdges += len(neighbors)
		}
		sumIndegrees := 0
		for _, count := range indegree {
			sumIndegrees += count
		}
		assert.Equal(t, totalEdges, sumIndegrees, "sum of indegrees != total edges")

		// -------------------------------------------------------------------------
		// First tier properties
		// -------------------------------------------------------------------------

		firstTier := getFirstTier(systems, indegree)

		// Property: Non-empty. A DAG always has at least one node with zero indegree.
		assert.NotEmpty(t, firstTier, "first tier is empty for non-empty systems")

		// Property: Completeness. The first tier contains exactly nodes with indegree 0.
		var expectedFirstTier []int
		for i := range systems {
			if indegree[i] == 0 {
				expectedFirstTier = append(expectedFirstTier, i)
			}
		}
		slices.Sort(expectedFirstTier)
		slices.Sort(firstTier)
		assert.Equal(t, expectedFirstTier, firstTier, "first tier mismatch")
	}
}

// randSystem creates a single systemMetadata with random component dependencies.
// The name and fn fields are left empty as they are not used by buildDependencyGraph.
func randSystem(prng *rand.Rand) systemMetadata {
	const (
		maxComponentDeps = 10  // Maximum component dependencies per system
		maxComponentID   = 100 // Maximum component ID value
	)
	numDeps := prng.IntN(maxComponentDeps)
	deps := bitmap.Bitmap{}
	for range numDeps {
		deps.Set(uint32(prng.IntN(maxComponentID)))
	}
	return systemMetadata{deps: deps}
}

// -------------------------------------------------------------------------------------------------
// Schedule graph examples test
// -------------------------------------------------------------------------------------------------
// This test complements the fuzz test above with explicit, readable examples. These serve more as
// documentation and as regression tests or known bugs.
// -------------------------------------------------------------------------------------------------

func TestScheduler_GraphExamples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		systems   []systemMetadata
		wantGraph map[string][]string
		wantTier0 []string
	}{
		{
			name:      "zero systems",
			systems:   []systemMetadata{},
			wantGraph: map[string][]string{},
			wantTier0: []string{},
		},
		{
			name: "single system",
			systems: []systemMetadata{
				{name: "A", deps: deps(1, 2, 3)},
			},
			wantGraph: map[string][]string{
				"A": {},
			},
			wantTier0: []string{"A"},
		},
		{
			name: "multiple independent systems",
			systems: []systemMetadata{
				{name: "A", deps: deps(1)},
				{name: "B", deps: deps(2)},
				{name: "C", deps: deps(3)},
			},
			wantGraph: map[string][]string{
				"A": {},
				"B": {},
				"C": {},
			},
			wantTier0: []string{"A", "B", "C"},
		},
		{
			name: "two systems with shared dependency (A->B)",
			systems: []systemMetadata{
				{name: "A", deps: deps(1, 2)},
				{name: "B", deps: deps(2, 3)},
			},
			wantGraph: map[string][]string{
				"A": {"B"},
				"B": {},
			},
			wantTier0: []string{"A"},
		},
		{
			name: "three systems with chain dependency (A->B->C)",
			systems: []systemMetadata{
				{name: "A", deps: deps(1, 2)},
				{name: "B", deps: deps(2, 3)},
				{name: "C", deps: deps(3, 4)},
			},
			wantGraph: map[string][]string{
				"A": {"B"},
				"B": {"C"},
				"C": {},
			},
			wantTier0: []string{"A"},
		},
		{
			name: "diamond dependencies (A->B | A->C | B->D | C->D)",
			systems: []systemMetadata{
				{name: "A", deps: deps(1, 2)},
				{name: "B", deps: deps(1, 3)},
				{name: "C", deps: deps(2, 4)},
				{name: "D", deps: deps(3, 4)},
			},
			wantGraph: map[string][]string{
				"A": {"B", "C"},
				"B": {"D"},
				"C": {"D"},
				"D": {},
			},
			wantTier0: []string{"A"},
		},
		{
			name: "two separate dependency chains (A->B->C | D->E)",
			systems: []systemMetadata{
				{name: "A", deps: deps(1, 2)},
				{name: "B", deps: deps(2, 3)},
				{name: "C", deps: deps(3, 4)},
				{name: "D", deps: deps(5, 6)},
				{name: "E", deps: deps(6, 7)},
			},
			wantGraph: map[string][]string{
				"A": {"B"},
				"B": {"C"},
				"C": {},
				"D": {"E"},
				"E": {},
			},
			wantTier0: []string{"A", "D"},
		},
		{
			name: "merged separate dependency chains (A->B->C->F | D->E->F)",
			systems: []systemMetadata{
				{name: "A", deps: deps(1, 2)},
				{name: "B", deps: deps(2, 3)},
				{name: "C", deps: deps(3, 4)},
				{name: "D", deps: deps(5, 6)},
				{name: "E", deps: deps(6, 7)},
				{name: "F", deps: deps(4, 7)},
			},
			wantGraph: map[string][]string{
				"A": {"B"},
				"B": {"C"},
				"C": {"F"},
				"D": {"E"},
				"E": {"F"},
				"F": {},
			},
			wantTier0: []string{"A", "D"},
		},
		{
			name: "system with multiple dependencies (A->D | B->D | C->D)",
			systems: []systemMetadata{
				{name: "A", deps: deps(1)},
				{name: "B", deps: deps(2)},
				{name: "C", deps: deps(3)},
				{name: "D", deps: deps(1, 2, 3)},
			},
			wantGraph: map[string][]string{
				"A": {"D"},
				"B": {"D"},
				"C": {"D"},
				"D": {},
			},
			wantTier0: []string{"A", "B", "C"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			graphActual, indegree := buildDependencyGraph(tt.systems)

			// Build name-to-index mapping. All systems must have names set.
			nameToIndex := make(map[string]int)
			for i, sys := range tt.systems {
				if sys.name == "" {
					t.Fatalf("system at index %d has no name", i)
				}
				nameToIndex[sys.name] = i
			}

			// Convert string-keyed graph to int-keyed graph for comparison with buildDependencyGraph output.
			graphExpected := make(map[int][]int)
			for src, neighbors := range tt.wantGraph {
				srcIdx := nameToIndex[src]
				if len(neighbors) == 0 {
					graphExpected[srcIdx] = nil
				} else {
					neighborIdxs := make([]int, len(neighbors))
					for i, dst := range neighbors {
						neighborIdxs[i] = nameToIndex[dst]
					}
					graphExpected[srcIdx] = neighborIdxs
				}
			}
			assert.Equal(t, graphExpected, graphActual)

			// Convert string tier0 slice to int slice.
			var tier0Expected []int
			if len(tt.wantTier0) > 0 {
				tier0Expected = make([]int, len(tt.wantTier0))
				for i, name := range tt.wantTier0 {
					tier0Expected[i] = nameToIndex[name]
				}
			}
			tier0Actual := getFirstTier(tt.systems, indegree)
			assert.Equal(t, tier0Expected, tier0Actual)
		})
	}
}

func deps(componentIDs ...uint32) bitmap.Bitmap {
	deps := bitmap.Bitmap{}
	for _, id := range componentIDs {
		deps.Set(id)
	}
	return deps
}
