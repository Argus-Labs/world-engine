package ecs

import (
	"math"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/kelindar/bitmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Tick phase ordering fuzz
// -------------------------------------------------------------------------------------------------
// This test verifies that World.Tick correctly orchestrates its subsystems. The individual
// subsystems (scheduler, systemEventManager, worldState) are already thoroughly tested in their
// own test files, so here we focus on integration properties:
//
//  1. Init systems run exactly once on the first tick, in registration order.
//  2. Scheduled systems do NOT run on the init tick.
//  3. On non-init ticks, phases are strictly ordered: PreUpdate < Update < PostUpdate.
//  4. System events are cleared after each tick.
//  5. After Reset(), the next tick re-runs init systems.
// -------------------------------------------------------------------------------------------------

func TestWorld_TickFuzz(t *testing.T) {
	t.Parallel()

	const (
		ticksMax            = 500
		initSystemsMax      = 20
		phaseSystemsMax     = 5 // per phase
		systemEventNamesMax = 10
	)

	prng := testutils.NewRand(t)

	// Logical clock and trace structures (same approach as scheduler tests).
	var clock atomic.Int64
	var mu sync.Mutex

	// Step 1: Create a new World.
	world := NewWorld()

	// Step 2: Register random system events on the world's systemEventManager.
	// Must be registered before systems so that system closures can enqueue into them.
	numSystemEvents := prng.IntN(systemEventNamesMax) + 1
	systemEventNames := make([]string, numSystemEvents)
	for i := range numSystemEvents {
		name := testutils.RandString(prng, 8)
		_, err := world.systemEvents.register(name)
		require.NoError(t, err)
		systemEventNames[i] = name
	}

	// emitRandomSystemEvent is a helper that enqueues a random system event into a random buffer.
	// Both prng and systemEvents.enqueue are not concurrent-safe, so we guard with mu since
	// systems within the same phase run concurrently via the scheduler.
	emitRandomSystemEvent := func() {
		mu.Lock()
		name := systemEventNames[prng.IntN(len(systemEventNames))]
		world.systemEvents.enqueue(name, testutils.SimpleSystemEvent{Value: prng.Int()})
		mu.Unlock()
	}

	// Step 3: Register random init systems.
	// Each init system records its execution order via the logical clock, and emits a random system
	// event. We track run counts to assert exactly-once and registration-order execution.
	numInitSystems := prng.IntN(initSystemsMax) + 1
	initRunCounts := make([]int, numInitSystems)
	initOrder := make([]int, 0, numInitSystems)
	for i := range numInitSystems {
		systemID := i
		world.initSystems = append(world.initSystems, initSystem{
			name: testutils.RandString(prng, 8),
			fn: func() {
				mu.Lock()
				initRunCounts[systemID]++
				initOrder = append(initOrder, systemID)
				mu.Unlock()
				emitRandomSystemEvent()
			},
		})
	}

	// Step 4: Register random scheduled systems across all 3 phases.
	// Each system records (hook, startTime, endTime) via the logical clock, and emits a random
	// system event. We use empty deps (no shared components) to keep things simple as scheduling
	// correctness is already tested. Here we only care about cross-phase ordering.
	type phaseEvent struct{ hook, start, end int64 }
	phaseEvents := make([]phaseEvent, 0)

	for hook := range 3 {
		numSystems := prng.IntN(phaseSystemsMax) + 1
		for range numSystems {
			hookID := int64(hook)
			world.scheduler[hook].register(
				testutils.RandString(prng, 8),
				bitmap.Bitmap{},
				func() {
					start := clock.Add(1)
					emitRandomSystemEvent()
					end := clock.Add(1)
					mu.Lock()
					phaseEvents = append(phaseEvents, phaseEvent{hook: hookID, start: start, end: end})
					mu.Unlock()
				},
			)
		}
	}

	// Step 5: Call World.Init() to create schedules for all 3 phases.
	world.Init()

	// Step 6: Randomly interleave Tick and Reset operations.
	// We track a model variable `expectInit` to know whether the next Tick should be an init tick
	// or a regular tick.
	const (
		opTick  = "tick"
		opReset = "reset"
	)
	operations := []string{opTick, opReset}
	weights := testutils.RandOpWeights(prng, operations)
	expectInit := true

	for range ticksMax {
		op := testutils.RandWeightedOp(prng, weights)
		switch op {
		case opTick:
			if expectInit { //nolint:nestif // it's fine
				world.Tick()

				// Property: each init system ran exactly once.
				for i, count := range initRunCounts {
					assert.Equal(t, 1, count, "init system %d ran %d times, expected 1", i, count)
				}

				// Property: init systems ran in registration order.
				expectedOrder := make([]int, numInitSystems)
				for i := range expectedOrder {
					expectedOrder[i] = i
				}
				assert.Equal(t, expectedOrder, initOrder, "init systems ran out of order")

				// Property: no scheduled system ran.
				assert.Empty(t, phaseEvents, "scheduled systems should not run on init tick")

				expectInit = false
			} else {
				// Reset trace structures for this tick.
				clock.Store(0)
				phaseEvents = phaseEvents[:0]

				world.Tick()

				// Property: phase ordering — all PreUpdate systems finish before any Update system
				// starts, and all Update systems finish before any PostUpdate system starts.
				var maxEnd [3]int64
				var minStart [3]int64
				for i := range minStart {
					minStart[i] = math.MaxInt64
				}
				for _, ev := range phaseEvents {
					if ev.end > maxEnd[ev.hook] {
						maxEnd[ev.hook] = ev.end
					}
					if ev.start < minStart[ev.hook] {
						minStart[ev.hook] = ev.start
					}
				}
				if minStart[Update] < math.MaxInt64 {
					assert.Less(t, maxEnd[PreUpdate], minStart[Update],
						"PreUpdate must finish before Update starts")
				}
				if minStart[PostUpdate] < math.MaxInt64 {
					assert.Less(t, maxEnd[Update], minStart[PostUpdate],
						"Update must finish before PostUpdate starts")
				}
			}

			// Property: system events are cleared after tick.
			for _, name := range systemEventNames {
				assert.Empty(t, world.systemEvents.get(name),
					"system event %q should be cleared after tick", name)
			}

		case opReset:
			world.Reset()

			// Clear trace structures so the next init tick can be verified fresh.
			for i := range initRunCounts {
				initRunCounts[i] = 0
			}
			initOrder = initOrder[:0]
			phaseEvents = phaseEvents[:0]
			clock.Store(0)

			expectInit = true
		}
	}
}
