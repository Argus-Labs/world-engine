package ecs

import (
	"strconv"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing system-event manager operations
// -------------------------------------------------------------------------------------------------
// This test verifies the queue implementation correctness by applying random sequences of
// operations and comparing it against a regular Go map of name->[]SystemEvent as the model.
// System events are pre-registered since WithSystemEventEmitter/Receiver.init guarantees
// registration before use.
// -------------------------------------------------------------------------------------------------

func TestSystemEvent_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		opsMax            = 1 << 15 // 32_768 iterations
		opEnqueue         = "enqueue"
		opGet             = "get"
		opClear           = "clear"
		nSystemEventTypes = 128
	)

	// Randomize operation weights.
	operations := []string{opEnqueue, opGet, opClear}
	weights := testutils.RandOpWeights(prng, operations)

	impl := newSystemEventManager()
	model := make(map[string][]SystemEvent) // name -> system-event buffer

	// Setup: pre-register many system event names with boxed queues.
	boxedFactory := newSystemEventQueueFactory[SystemEvent]()
	for id := range nSystemEventTypes {
		name := seidToString(systemEventID(id))
		_, err := impl.register(name, boxedFactory)
		require.NoError(t, err)
		model[name] = []SystemEvent{}
	}

	for range opsMax {
		op := testutils.RandWeightedOp(prng, weights)
		switch op {
		case opEnqueue:
			name := testutils.RandMapKey(prng, model)
			// Create a random system event with the correct name.
			systemEvent := modelFuzzSystemEvent{
				EventName: name,
				Counter:   prng.Uint64(),
				Enabled:   prng.Float64() < 0.5,
			}

			err := impl.enqueueAbstract(systemEvent)
			require.NoError(t, err)
			model[name] = append(model[name], systemEvent)

		case opGet:
			name := testutils.RandMapKey(prng, model)

			implSysEvents, err := impl.getAbstract(name)
			require.NoError(t, err)
			modelSysEvents := model[name]

			// Property: get returns system-events in same order as enqueued.
			assert.Len(t, implSysEvents, len(modelSysEvents), "get(%s) length mismatch", name)
			for i := range modelSysEvents {
				assert.Equal(t, modelSysEvents[i], implSysEvents[i], "get(%s)[%d] mismatch", name, i)
			}

		case opClear:
			impl.clear()
			for name := range model {
				model[name] = []SystemEvent{}
			}

			// Property: all buffers should be empty after clear.
			for name := range model {
				implSysEvents, err := impl.getAbstract(name)
				require.NoError(t, err)
				assert.Empty(t, implSysEvents, "clear() should empty buffer for %s", name)
			}

		default:
			panic("unreachable")
		}
	}

	// Final state check: verify all system-events match between impl and model.
	assert.Len(t, impl.catalog, len(model), "catalog length mismatch")
	for name, modelEvents := range model {
		implEvents, err := impl.getAbstract(name)
		require.NoError(t, err)
		assert.Len(t, implEvents, len(modelEvents), "final state: %s length mismatch", name)
		for i := range modelEvents {
			assert.Equal(t, modelEvents[i], implEvents[i], "final state: %s[%d] mismatch", name, i)
		}
	}
}

// These are used over the default testutils system event because we want variable Name().
type modelFuzzSystemEvent struct {
	EventName string
	Counter   uint64
	Enabled   bool
}

func (s modelFuzzSystemEvent) Name() string {
	return s.EventName
}

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing system-event registration
// -------------------------------------------------------------------------------------------------
// This test verifies the system event manager registration correctness by applying random sequences
// of operations and comparing against a Go map as the model.
// -------------------------------------------------------------------------------------------------

func TestSystemEvent_RegisterModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const opsMax = 1 << 15 // 32_768 iterations

	impl := newSystemEventManager()
	model := make(map[string]systemEventID) // name -> ID
	boxedFactory := newSystemEventQueueFactory[SystemEvent]()

	for range opsMax {
		nameID := systemEventID(prng.IntN(opsMax / 4))
		name := seidToString(nameID)
		implID, err := impl.register(name, boxedFactory)
		require.NoError(t, err)

		if modelID, exists := model[name]; exists {
			assert.Equal(t, modelID, implID, "ID mismatch for re-registered %q", name)
		} else {
			model[name] = implID
		}
	}

	// Property: bijection holds between names and IDs.
	seenIDs := make(map[systemEventID]string)
	for name, id := range impl.catalog {
		if prevName, seen := seenIDs[id]; seen {
			t.Errorf("ID %d is mapped by both %q and %q", id, prevName, name)
		}
		seenIDs[id] = name
	}

	// Property: all IDs in catalog are in range [0, nextID).
	for name, id := range impl.catalog {
		assert.Less(t, id, impl.nextID, "ID for %q is out of range", name)
	}

	// Final state check: catalog matches model.
	assert.Len(t, impl.catalog, len(model), "catalog length mismatch")
	for name, modelID := range model {
		implID, exists := impl.catalog[name]
		require.True(t, exists, "system event %q should be registered", name)
		assert.Equal(t, modelID, implID, "ID mismatch for %q", name)
	}

	// Simple test to confirm that registering the same name repeatedly is a no-op.
	t.Run("registration idempotence", func(t *testing.T) {
		t.Parallel()
		impl := newSystemEventManager()
		name1 := seidToString(123)
		name2 := seidToString(124)

		id1, err := impl.register(name1, boxedFactory)
		require.NoError(t, err)

		id2, err := impl.register(name1, boxedFactory)
		require.NoError(t, err)

		assert.Equal(t, id1, id2)

		id3, err := impl.register(name2, boxedFactory)
		require.NoError(t, err)

		assert.Equal(t, id1+1, id3)
	})
}

func seidToString(id systemEventID) string {
	return strconv.FormatUint(uint64(id), 10)
}
