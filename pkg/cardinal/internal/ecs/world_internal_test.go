package ecs

import (
	"math"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

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
//  1. Init systems run exactly once in World.Init(), in registration order.
//  2. On ticks, phases are strictly ordered: PreUpdate < Update < PostUpdate.
//  4. System events are cleared after each tick.
//  5. After Reset(), calling World.Init() re-runs init systems.
// -------------------------------------------------------------------------------------------------

func TestWorld_TickFuzz(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const (
			ticksMax        = 1 << 12 // 4096 ticks
			initSystemsMax  = 25
			phaseSystemsMax = 10 // per phase
		)

		prng := testutils.NewRand(t)
		setup := setupWorldTickFuzz(t, prng, initSystemsMax, phaseSystemsMax)

		// Step 5: Call World.Init() to create schedules for all 3 phases.
		setup.world.Init()

		// Step 6: Init systems run exactly once during Init, in registration order.
		setup.verifyInitSystems(t)

		// Step 7: Randomly interleave Tick and Reset operations.
		const (
			opTick  = "tick"
			opReset = "reset"
		)
		operations := []string{opTick, opReset}
		weights := testutils.RandOpWeights(prng, operations)

		for range ticksMax {
			op := testutils.RandWeightedOp(prng, weights)
			switch op {
			case opTick:
				// Reset trace structures for this tick.
				setup.clearTickTrace()

				setup.world.Tick()

				// Uncomment to see the system run order.
				// t.Logf("phaseEvents (%d total):", len(setup.phaseEvents))
				// for i, ev := range setup.phaseEvents {
				// 	t.Logf("  [%d] %d.%d start=%d end=%d",
				// 		i, ev.hook, ev.id, ev.start, ev.end)
				// }

				setup.verifyPhaseOrder(t)

				// Property: system events are cleared after tick.
				assert.Empty(t, setup.world.systemEvents.get(setup.systemEventName),
					"system event %q should be cleared after tick", setup.systemEventName)

			case opReset:
				setup.world.Reset()

				// Clear trace structures so the next Init call can be verified fresh.
				setup.clearResetTrace()

				setup.world.Init()
				setup.verifyInitSystems(t)
			}
		}
	})
}

type phaseEvent struct {
	hook, id, start, end int64
}

type tickFuzzSetup struct {
	world           *World
	systemEventName string
	numInitSystems  int
	initOrder       []int
	phaseEvents     []phaseEvent
	clock           atomic.Int64
}

func (s *tickFuzzSetup) verifyInitSystems(t *testing.T) {
	t.Helper()
	assert.Len(t, s.initOrder, s.numInitSystems, "init system(s) ran more than once")
	for i := range s.numInitSystems {
		assert.Equal(t, i, s.initOrder[i], "init system %d ran out of order", i)
	}
}

func (s *tickFuzzSetup) verifyPhaseOrder(t *testing.T) {
	t.Helper()

	// Property: PreUpdate < Update < PostUpdate order is respected.
	var maxEnd [3]int64
	var minStart [3]int64
	for i := range minStart {
		minStart[i] = math.MaxInt64
	}
	for _, ev := range s.phaseEvents {
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

func (s *tickFuzzSetup) clearTickTrace() {
	s.clock.Store(0)
	s.phaseEvents = s.phaseEvents[:0]
}

func (s *tickFuzzSetup) clearResetTrace() {
	s.initOrder = s.initOrder[:0]
	s.clearTickTrace()
}

func setupWorldTickFuzz(t *testing.T, prng *rand.Rand, initSystemsMax, phaseSystemsMax int) *tickFuzzSetup {
	t.Helper()

	// Logical clock and trace structures (same approach as scheduler tests).
	var mu sync.Mutex

	// Step 1: Create a new World.
	setup := &tickFuzzSetup{world: NewWorld(), systemEventName: "test-event"}

	// Step 2: Register a single system event on the world's systemEventManager.
	// We only need one to verify that events are cleared after each tick.
	_, err := setup.world.systemEvents.register(setup.systemEventName)
	require.NoError(t, err)
	// emitSystemEvent is a helper that enqueues a system event.
	// Both prng and systemEvents.enqueue are not concurrent-safe, so we guard with mu since
	// systems within the same phase run concurrently via the scheduler.
	emitSystemEvent := func() {
		mu.Lock()
		setup.world.systemEvents.enqueue(setup.systemEventName, testutils.SimpleSystemEvent{Value: prng.Int()})
		mu.Unlock()
	}

	// Step 3: Register random init systems.
	// Each init system records its execution order. We track run counts to assert exactly-once and
	// registration-order execution.
	setup.numInitSystems = prng.IntN(initSystemsMax) + 1
	setup.initOrder = make([]int, 0, setup.numInitSystems)
	for i := range setup.numInitSystems {
		systemID := i
		setup.world.initSystems = append(setup.world.initSystems, initSystem{
			name: testutils.RandString(prng, 8),
			fn: func() {
				setup.initOrder = append(setup.initOrder, systemID)
			},
		})
	}

	// Step 4: Register random scheduled systems across all 3 phases.
	// Each system records (hook, startTime, endTime) via the logical clock and emits a random system
	// event. We use empty deps (no shared components) because scheduler correctness is tested elsewhere.
	setup.phaseEvents = make([]phaseEvent, 0)
	for hook := range 3 {
		numSystems := prng.IntN(phaseSystemsMax) + 1
		for i := range numSystems {
			setup.world.scheduler[hook].register(
				testutils.RandString(prng, 8),
				bitmap.Bitmap{},
				func() {
					start := setup.clock.Add(1)
					time.Sleep(2 * time.Second)
					emitSystemEvent()
					end := setup.clock.Add(1)
					mu.Lock()
					setup.phaseEvents = append(setup.phaseEvents, phaseEvent{
						hook: int64(hook), id: int64(i), start: start, end: end,
					})
					mu.Unlock()
				},
			)
		}
	}

	return setup
}
