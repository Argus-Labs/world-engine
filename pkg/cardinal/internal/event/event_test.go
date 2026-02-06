package event_test

import (
	"sync"
	"testing"
	"testing/synctest"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing event manager operations
// -------------------------------------------------------------------------------------------------
// This test verifies the event manager implementation correctness by applying random sequences of
// operations and comparing it against a Go slice as the model.
// -------------------------------------------------------------------------------------------------

func TestEvent_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		opsMax     = 1 << 15 // 32_768 iterations
		opEnqueue  = "enqueue"
		opDispatch = "dispatch"
	)

	impl := event.NewManager(1024)
	model := make([]event.Event, 0) // Queue of pending events

	// Slice to capture events dispatched by handlers.
	var dispatched []event.Event
	var mu sync.Mutex

	// Register handlers for kinds 0 to N-1.
	numKinds := prng.IntN(256) + 1 // 1-256 kinds
	for i := range numKinds {
		impl.RegisterHandler(event.Kind(i), func(e event.Event) error {
			mu.Lock()
			dispatched = append(dispatched, e)
			mu.Unlock()
			return nil
		})
	}

	// Randomize operation weights.
	operations := []string{opEnqueue, opDispatch}
	weights := testutils.RandOpWeights(prng, operations)

	// Run opsMax iterations.
	for range opsMax {
		op := testutils.RandWeightedOp(prng, weights)
		switch op {
		case opEnqueue:
			// Pick a random registered event kind and create event with random payload.
			kind := event.Kind(prng.IntN(numKinds))
			e := event.Event{Kind: kind, Payload: prng.Int()}

			impl.Enqueue(e)
			model = append(model, e)

		case opDispatch:
			err := impl.Dispatch()
			require.NoError(t, err)

			// Property: dispatched events must match model (pending queue).
			assert.ElementsMatch(t, model, dispatched, "dispatched events mismatch")

			// Clear model and dispatched slice.
			model = model[:0]
			dispatched = dispatched[:0]

		default:
			panic("unreachable")
		}
	}

	// Final state check.
	err := impl.Dispatch()
	require.NoError(t, err)

	// Property: all enqueued events must be dispatched.
	assert.ElementsMatch(t, model, dispatched, "final dispatched events mismatch")
}

// -------------------------------------------------------------------------------------------------
// Channel overflow regression test
// -------------------------------------------------------------------------------------------------
// This test verifies that enqueue does not block when the channel is full. Before the fix,
// enqueue would block indefinitely when the channel capacity (1024) was exceeded, causing
// a deadlock. After the fix, enqueue should flush the channel to the buffer when full.
// -------------------------------------------------------------------------------------------------

func TestEvent_EnqueueChannelFull(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const channelCapacity = 16
		const totalEvents = channelCapacity * 3 // Well beyond channel capacity

		impl := event.NewManager(channelCapacity)

		// Register a handler for the default kind.
		var dispatched []event.Event
		impl.RegisterHandler(event.KindDefault, func(e event.Event) error {
			dispatched = append(dispatched, e)
			return nil
		})

		// Enqueue more events than channel capacity.
		// Before fix: this blocks forever after 16 events, causing deadlock.
		// After fix: this completes without blocking.
		done := false
		go func() {
			for i := range totalEvents {
				impl.Enqueue(event.Event{Kind: event.KindDefault, Payload: i})
			}
			done = true
		}()

		// Wait for all goroutines to complete or durably block.
		// If enqueue blocks, synctest.Test will detect deadlock and fail.
		synctest.Wait()

		if !done {
			t.Fatal("enqueue blocked: channel overflow not handled")
		}

		// Verify all events are captured.
		err := impl.Dispatch()
		require.NoError(t, err)

		assert.Len(t, dispatched, totalEvents, "expected all %d events to be captured", totalEvents)

		// Verify data integrity.
		for i, evt := range dispatched {
			assert.Equal(t, event.KindDefault, evt.Kind, "event kind mismatch at index %d", i)
			assert.Equal(t, i, evt.Payload, "payload mismatch at index %d", i)
		}
	})
}
