package ecs

import (
	"fmt"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing world lifecycle operations
// -------------------------------------------------------------------------------------------------
// This test verifies World lifecycle correctness by applying random sequences of Tick and Reset
// operations and comparing the observed execution order against the fixed sequential model.
// -------------------------------------------------------------------------------------------------

func TestWorld_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	world := NewWorld()
	_, err := world.systemEvents.register(
		testutils.SimpleSystemEvent{}.Name(), newSystemEventQueueFactory[testutils.SimpleSystemEvent]())
	require.NoError(t, err)

	numInitSystems := prng.IntN(5) + 1
	initOrder := make([]int, 0, numInitSystems)
	expectedInitOrder := make([]int, numInitSystems)

	for systemID := range numInitSystems {
		expectedInitOrder[systemID] = systemID

		world.systems[Init] = append(world.systems[Init], systemMetadata{
			name: fmt.Sprintf("init-%d", systemID),
			fn: func() {
				initOrder = append(initOrder, systemID)
			},
		})
	}

	expectedTickOrder := make([]string, 0)
	tickOrder := make([]string, 0)

	for _, hook := range []SystemHook{PreUpdate, Update, PostUpdate} {
		numSystems := prng.IntN(5) + 1
		for systemID := range numSystems {
			name := fmt.Sprintf("%d-%d", hook, systemID)
			expectedTickOrder = append(expectedTickOrder, name)

			world.systems[hook] = append(world.systems[hook], systemMetadata{
				name: name,
				fn: func() {
					tickOrder = append(tickOrder, name)
					if hook == Update {
						world.systemEvents.enqueueAbstract(testutils.SimpleSystemEvent{Value: 1})
					}
				},
			})
		}
	}

	// Property: Init runs init systems exactly once and in registration order.
	world.Init()
	assert.Equal(t, expectedInitOrder, initOrder)

	const (
		opsMax  = 1 << 12 // 4096 operations
		opTick  = "tick"
		opReset = "reset"
	)

	// Randomize operation weights.
	operations := []string{opTick, opReset}
	weights := testutils.RandOpWeights(prng, operations)

	// Check the world against the sequential model by running the same lifecycle operations.
	for range opsMax {
		switch testutils.RandWeightedOp(prng, weights) {
		case opTick:
			tickOrder = tickOrder[:0]

			world.Tick()

			// Property: Tick runs systems in hook order and registration order.
			assert.Equal(t, expectedTickOrder, tickOrder)

			systemEvents, err := world.systemEvents.getAbstract(testutils.SimpleSystemEvent{}.Name())
			require.NoError(t, err)

			// Property: system events are cleared after each tick.
			assert.Empty(t, systemEvents, "system event should be cleared after tick")

		case opReset:
			initOrder = initOrder[:0]

			world.Reset()
			world.Init()

			// Property: Reset allows Init to re-run init systems in registration order.
			assert.Equal(t, expectedInitOrder, initOrder)

		default:
			panic("unreachable")
		}
	}
}
