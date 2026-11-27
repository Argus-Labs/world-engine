package event_test

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"testing/synctest"
	"weak"

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
	var (
		dispatched []event.Event
		mu         sync.Mutex
	)

	// Register handlers for kinds 0 to N-1.
	numKinds := prng.IntN(256) + 1 // 1-256 kinds
	for i := range numKinds {
		impl.RegisterHandler(event.Kind(i), func(_ context.Context, e event.Event) error {
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
			err := impl.Dispatch(context.Background())
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
	err := impl.Dispatch(context.Background())
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

		impl.RegisterHandler(event.KindDefault, func(_ context.Context, e event.Event) error {
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
		err := impl.Dispatch(context.Background())
		require.NoError(t, err)

		assert.Len(t, dispatched, totalEvents, "expected all %d events to be captured", totalEvents)

		// Verify data integrity.
		for i, evt := range dispatched {
			assert.Equal(t, event.KindDefault, evt.Kind, "event kind mismatch at index %d", i)
			assert.Equal(t, i, evt.Payload, "payload mismatch at index %d", i)
		}
	})
}

// -------------------------------------------------------------------------------------------------
// Payload retention
// -------------------------------------------------------------------------------------------------
// The event buffer is reused every tick. These tests verify that reuse doesn't keep handled or
// discarded payloads reachable: once Dispatch or Clear retires an event, the GC must be able to
// free its payload.
// -------------------------------------------------------------------------------------------------

func TestEvent_RetiredPayloadsAreCollectable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		retire func(*testing.T, *event.Manager)
	}{
		{name: "dispatch", retire: func(t *testing.T, m *event.Manager) {
			require.NoError(t, m.Dispatch(t.Context()))
		}},
		{name: "clear", retire: func(_ *testing.T, m *event.Manager) { m.Clear() }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, refs := retireRetentionEvents(t, tc.retire)
			assert.Equal(t, 0, liveRefs(refs), "manager retains retired event payloads")
			runtime.KeepAlive(m)

			// Control: m is dead past KeepAlive, so with the manager gone nothing else may hold the
			// payloads. A failure above can then only come from the manager's buffers.
			assert.Equal(t, 0, liveRefs(refs), "payloads retained outside the manager")
		})
	}
}

// retireRetentionEvents enqueues more events than the channel holds, so they overflow into the
// buffer, records weak pointers to their payloads, then retires them. It isn't inlined so no
// payload stays live on the test stack.
//
//go:noinline
func retireRetentionEvents(
	t *testing.T, retire func(*testing.T, *event.Manager),
) (*event.Manager, []weak.Pointer[byte]) {
	t.Helper()

	const (
		channelCapacity = 4
		n               = channelCapacity * 4
	)

	m := event.NewManager(channelCapacity)
	m.RegisterHandler(event.KindDefault, func(context.Context, event.Event) error { return nil })

	refs := make([]weak.Pointer[byte], 0, n)
	for range n {
		payload := make([]byte, 4<<10)
		refs = append(refs, weak.Make(&payload[0]))
		m.Enqueue(event.Event{Kind: event.KindDefault, Payload: payload})
	}

	retire(t, &m)

	return &m, refs
}

// liveRefs runs a full GC and counts the weak pointers whose targets are still reachable.
func liveRefs(refs []weak.Pointer[byte]) int {
	runtime.GC()

	live := 0

	for _, ref := range refs {
		if ref.Value() != nil {
			live++
		}
	}

	return live
}
