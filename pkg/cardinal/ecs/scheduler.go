package ecs

import (
	"sync/atomic"

	"slices"

	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
	"golang.org/x/sync/errgroup"
)

// systemMetadata contains the metadata for a system.
type systemMetadata struct {
	name string        // The name of the system
	deps bitmap.Bitmap // Bitmap of system dependencies (components + system events)
	fn   func() error  // Function that wraps a System
}

// systemScheduler manages the execution of systems in a dependency-aware concurrent manner.
// It orders systems based on their component and system event dependencies and is optimized to
// maximize parallelism while maintaining correct order.
type systemScheduler struct {
	systems        []systemMetadata // The systems to run
	tier0          []int            // The first execution tier
	graph          map[int][]int    // Mapping of systems -> systems that depend on it
	activeIndegree uint8            // Determines which indegree is currently active (0 or 1)
	// indegree0 and indegree1 are double-buffered counters tracking remaining dependencies
	// for each system. They alternate between runs to avoid reinitialization.
	indegree0 []atomic.Int32
	indegree1 []atomic.Int32
}

// newSystemScheduler creates a new system scheduler.
func newSystemScheduler() systemScheduler {
	return systemScheduler{
		systems:        make([]systemMetadata, 0),
		tier0:          make([]int, 0),
		graph:          make(map[int][]int),
		activeIndegree: 0,
	}
}

// register registers a system with the scheduler.
func (s *systemScheduler) register(name string, systemDep bitmap.Bitmap, systemFn func() error) {
	s.systems = append(s.systems, systemMetadata{name: name, deps: systemDep, fn: systemFn})
}

// Run executes the systems in the order of their dependencies. It returns an error if any system
// returns an error. If multiple systems fail, all errors are wrapped in a single error.
func (s *systemScheduler) Run() error {
	// Fast path: no systems in hook.
	if len(s.systems) == 0 {
		return nil
	}

	executionQueue := make(chan int, len(s.systems))
	defer close(executionQueue)

	currentIndegree, nextIndegree := s.getCurrentAndNextIndegrees()
	g := new(errgroup.Group)

	// Schedule all tier 0 systems
	for _, systemID := range s.tier0 {
		executionQueue <- systemID
	}

	// Launch goroutines to execute systems
	for range s.systems {
		systemID := <-executionQueue
		g.Go(func() error {
			// Do not return the system error early here so that the dependent systems can be scheduled to
			// run first. If we return early then some systems might not run. We do this so that we can
			// guarantee all of the systems are executed (`for range s.systems`) instead of being
			// optimistic about it.
			var err error
			if err = s.systems[systemID].fn(); err != nil { // The error assignment is intended here
				err = eris.Wrapf(err, "system %s failed", s.systems[systemID].name)
			}

			// Process all systems that depend on this one.
			for _, dependent := range s.graph[systemID] {
				remainingDeps := currentIndegree[dependent].Add(-1)
				nextIndegree[dependent].Add(1)

				// If this was the last dependency, schedule it for execution.
				if remainingDeps == 0 {
					executionQueue <- dependent
				}
			}

			return err
		})
	}

	if err := g.Wait(); err != nil {
		return eris.Wrap(err, "system returned an error")
	}
	return nil
}

// getCurrentAndNextIndegrees returns the current and next indegrees. It also switches the active
// indegree buffer with the next one.
func (s *systemScheduler) getCurrentAndNextIndegrees() ([]atomic.Int32, []atomic.Int32) {
	isFirstBuffer := s.activeIndegree == 0  // Capture current state before toggle
	s.activeIndegree = 1 - s.activeIndegree // Toggle between 0 and 1

	if isFirstBuffer {
		return s.indegree0, s.indegree1
	}

	return s.indegree1, s.indegree0
}

// createSchedule initializes the dependency graph and execution schedule for the systems.
// Must be called after all systems are registered and before the first Run.
func (s *systemScheduler) createSchedule() {
	graph, indegree := buildDependencyGraph(s.systems)
	s.graph = graph

	// Initialize double-buffered atomic counters for tracking dependencies. These are used to avoid
	// reallocation during system execution.
	s.indegree0 = make([]atomic.Int32, len(s.systems))
	s.indegree1 = make([]atomic.Int32, len(s.systems))

	// Initialize the first buffer with the initial dependency counts.
	for k, v := range indegree {
		s.indegree0[k].Store(int32(v)) //nolint:gosec // Won't overflow
	}

	s.tier0 = getFirstTier(s.systems, indegree)
}

// buildDependencyGraph creates a directed acyclic graph (DAG) of system dependencies
// based on their shared component access patterns. It returns the graph as an adjacency
// list and a map of each system's dependency count.
func buildDependencyGraph(systems []systemMetadata) (map[int][]int, map[int]int) {
	graph := make(map[int][]int, len(systems))
	indegree := make(map[int]int, len(systems))

	for systemA := range len(systems) - 1 {
		for systemB := systemA + 1; systemB < len(systems); systemB++ {
			depsA := systems[systemA].deps
			depsB := systems[systemB].deps

			var deps []uint32
			depsA.Range(func(x uint32) {
				deps = append(deps, x)
			})

			// Check if systemB depends on systemA.
			if slices.ContainsFunc(deps, depsB.Contains) {
				graph[systemA] = append(graph[systemA], systemB)
				indegree[systemB]++
			}
		}
	}

	return graph, indegree
}

// getFirstTier returns the list of systems without any dependencies. These will be the first
// systems to be run.
func getFirstTier(systems []systemMetadata, indegree map[int]int) []int {
	var currentTier []int
	for systemID := range systems {
		if indegree[systemID] == 0 {
			currentTier = append(currentTier, systemID)
		}
	}
	return currentTier
}
