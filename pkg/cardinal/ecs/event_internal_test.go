package ecs

import (
	"reflect"
	"testing"
	"testing/synctest"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing event manager operations
// -------------------------------------------------------------------------------------------------
// This test verifies the eventManager implementation correctness using model-based testing. It
// compares our implementation against two slices (inFlight and buffer) as the model by applying
// random sequences of enqueue/getEvents/clear operations to both and asserting equivalence.
// The model tracks events in two stages: inFlight (channel) and buffer (drained events).
// -------------------------------------------------------------------------------------------------

func TestEvent_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const opsMax = 1 << 15 // 32_768 iterations

	impl := newEventManager()
	// Model: track in-flight (channel) and buffered events separately
	inFlight := make([]RawEvent, 0) // events enqueued but not yet drained
	buffer := make([]RawEvent, 0)   // events in the buffer after getEvents

	for range opsMax {
		op := testutils.RandWeightedOp(prng, eventOps)
		switch op {
		case em_enqueue:
			n := prng.IntN(10) + 1
			for range n {
				kind := EventKind(prng.IntN(int(CustomEventKindStart)) + 1)
				payload := prng.Int()
				event := RawEvent{Kind: kind, Payload: payload}

				impl.enqueue(kind, payload)
				inFlight = append(inFlight, event)
			}
		case em_get:
			implEvents := impl.getEvents()
			buffer = append(buffer, inFlight...)
			inFlight = inFlight[:0]
			assert.Equal(t, buffer, implEvents, "getEvents mismatch")
		case em_clear:
			impl.clear()
			buffer = buffer[:0]
		default:
			panic("unreachable")
		}
	}

	// Final state check: drain remaining and compare to model.
	implEvents := impl.getEvents()
	buffer = append(buffer, inFlight...)
	assert.Equal(t, buffer, implEvents, "final buffer mismatch")
}

type eventOp uint8

const (
	em_enqueue eventOp = 75
	em_get     eventOp = 20
	em_clear   eventOp = 5
)

var eventOps = []eventOp{em_enqueue, em_get, em_clear}

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

		impl := newEventManager(withChannelCapacity(channelCapacity))

		// Enqueue more events than channel capacity.
		// Before fix: this blocks forever after 16 events, causing deadlock.
		// After fix: this completes without blocking.
		done := false
		go func() {
			for i := range totalEvents {
				impl.enqueue(EventKindDefault, i)
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
		events := impl.getEvents()
		assert.Len(t, events, totalEvents, "expected all %d events to be captured", totalEvents)

		// Verify data integrity.
		for i, event := range events {
			assert.Equal(t, EventKindDefault, event.Kind, "event kind mismatch at index %d", i)
			assert.Equal(t, i, event.Payload, "payload mismatch at index %d", i)
		}
	})
}

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing event registration
// -------------------------------------------------------------------------------------------------
// This test verifies the eventManager registration correctness using model-based testing. It
// compares our implementation against a map[string]uint32 as the model by applying random
// register operations and asserting equivalence. We also verify structural invariants:
// name-id bijection and ID uniqueness.
// -------------------------------------------------------------------------------------------------

func TestEvent_RegisterModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const opsMax = 1 << 15 // 32_768 iterations

	impl := newEventManager()
	model := make(map[string]uint32) // name -> ID

	for range opsMax {
		name := randValidCommandName(prng)
		implID, err := impl.register(name, reflect.TypeOf(name))
		require.NoError(t, err)

		if modelID, exists := model[name]; exists {
			assert.Equal(t, modelID, implID, "ID mismatch for re-registered %q", name)
		} else {
			model[name] = implID
		}
	}

	// Property: bijection holds between names and IDs.
	seenIDs := make(map[uint32]string)
	for name, id := range impl.registry {
		if prevName, seen := seenIDs[id]; seen {
			t.Errorf("ID %d is mapped by both %q and %q", id, prevName, name)
		}
		seenIDs[id] = name
	}

	// Property: all IDs in registry are in range [0, nextID).
	for name, id := range impl.registry {
		assert.Less(t, id, impl.nextID, "ID for %q is out of range", name)
	}

	// Final state check: registry matches model.
	assert.Len(t, impl.registry, len(model), "registry length mismatch")
	for name, modelID := range model {
		implID, exists := impl.registry[name]
		require.True(t, exists, "event %q should be registered", name)
		assert.Equal(t, modelID, implID, "ID mismatch for %q", name)
	}

	// Simple test to confirm that registering the same name repeatedly is a no-op.
	t.Run("registration idempotence", func(t *testing.T) {
		t.Parallel()

		id1, err := impl.register("hello", reflect.TypeOf(nil))
		require.NoError(t, err)

		id2, err := impl.register("hello", reflect.TypeOf(nil))
		require.NoError(t, err)

		assert.Equal(t, id1, id2)

		id3, err := impl.register("a_different_name", reflect.TypeOf(nil))
		require.NoError(t, err)

		assert.Equal(t, id1+1, id3)
	})
}
