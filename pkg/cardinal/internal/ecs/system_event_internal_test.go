package ecs

import (
	"math/rand/v2"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing system-event manager operations
// -------------------------------------------------------------------------------------------------
// This test verifies the systemEventManager implementation correctness using model-based testing.
// It compares our implementation against a map[string][]SystemEvent as the model by applying random
// sequences of enqueue/get/clear operations to both and asserting equivalence. System events are
// pre-registered since WithSystemEventEmitter/Receiver.init guarantees registration before use.
// -------------------------------------------------------------------------------------------------

func TestSystemEvent_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const opsMax = 1 << 15 // 32_768 iterations

	impl := newSystemEventManager()
	model := make(map[string][]SystemEvent) // name -> system-event buffer

	// Setup: pre-register a fixed set of system event names.
	for _, name := range allSystemEventNames {
		_, err := impl.register(name)
		require.NoError(t, err)
		model[name] = []SystemEvent{}
	}

	for range opsMax {
		op := testutils.RandWeightedOp(prng, systemEventOps)
		switch op {
		case sem_enqueue:
			name := testutils.RandMapKey(prng, model)
			event := randSystemEventByName(prng, name)

			impl.enqueue(name, event)
			model[name] = append(model[name], event)

		case sem_get:
			name := testutils.RandMapKey(prng, model)

			implSysEvents := impl.get(name)
			modelSysEvents := model[name]

			// Property: get returns system-events in same order as enqueued.
			assert.Len(t, implSysEvents, len(modelSysEvents), "get(%s) length mismatch", name)
			for i := range modelSysEvents {
				assert.Equal(t, modelSysEvents[i], implSysEvents[i], "get(%s)[%d] mismatch", name, i)
			}

		case sem_clear:
			impl.clear()
			for name := range model {
				model[name] = []SystemEvent{}
			}

			// Property: all buffers should be empty after clear.
			for name := range model {
				implSysEvents := impl.get(name)
				assert.Empty(t, implSysEvents, "clear() should empty buffer for %s", name)
			}

		default:
			panic("unreachable")
		}
	}

	// Final state check: verify all system-events match between impl and model.
	assert.Len(t, impl.registry, len(model), "registry length mismatch")
	for name, modelEvents := range model {
		implEvents := impl.get(name)
		assert.Len(t, implEvents, len(modelEvents), "final state: %s length mismatch", name)
		for i := range modelEvents {
			assert.Equal(t, modelEvents[i], implEvents[i], "final state: %s[%d] mismatch", name, i)
		}
	}
}

type systemEventOp uint8

const (
	sem_enqueue systemEventOp = 46
	sem_get     systemEventOp = 44
	sem_clear   systemEventOp = 10
)

var systemEventOps = []systemEventOp{sem_enqueue, sem_get, sem_clear}

var allSystemEventNames = []string{
	testutils.SystemEventA{}.Name(), testutils.SystemEventB{}.Name(), testutils.SystemEventC{}.Name(),
}

func randSystemEventByName(prng *rand.Rand, name string) SystemEvent {
	switch name {
	case testutils.SystemEventA{}.Name():
		return testutils.SystemEventA{X: prng.Float64(), Y: prng.Float64(), Z: prng.Float64()}
	case testutils.SystemEventB{}.Name():
		return testutils.SystemEventB{ID: prng.Uint64(), Label: "test", Enabled: prng.Float64() < 0.5}
	case testutils.SystemEventC{}.Name():
		return testutils.SystemEventC{Counter: uint16(prng.IntN(65536))}
	default:
		panic("unknown system event: " + name)
	}
}

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing system-event registration
// -------------------------------------------------------------------------------------------------
// This test verifies the systemEventManager registration correctness using model-based testing. It
// compares our implementation against a map[string]systemEventID as the model by applying random
// register operations and asserting equivalence. We also verify structural invariants:
// name-id bijection and ID uniqueness.
// -------------------------------------------------------------------------------------------------

func TestSystemEvent_RegisterModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const opsMax = 1 << 15 // 32_768 iterations

	impl := newSystemEventManager()
	model := make(map[string]systemEventID) // name -> ID

	for range opsMax {
		name := randValidEventName(prng) // Reuse the command name generator as they're identical
		implID, err := impl.register(name)
		require.NoError(t, err)

		if modelID, exists := model[name]; exists {
			assert.Equal(t, modelID, implID, "ID mismatch for re-registered %q", name)
		} else {
			model[name] = implID
		}
	}

	// Property: bijection holds between names and IDs.
	seenIDs := make(map[systemEventID]string)
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
		require.True(t, exists, "system event %q should be registered", name)
		assert.Equal(t, modelID, implID, "ID mismatch for %q", name)
	}

	// Simple test to confirm that registering the same name repeatedly is a no-op.
	t.Run("registration idempotence", func(t *testing.T) {
		t.Parallel()

		id1, err := impl.register("hello")
		require.NoError(t, err)

		id2, err := impl.register("hello")
		require.NoError(t, err)

		assert.Equal(t, id1, id2)

		id3, err := impl.register("a_different_name")
		require.NoError(t, err)

		assert.Equal(t, id1+1, id3)
	})
}

func randValidEventName(prng *rand.Rand) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"
	length := prng.IntN(50) + 1 // 1-50 characters
	b := make([]byte, length)
	for i := range b {
		b[i] = chars[prng.IntN(len(chars))]
	}
	return string(b)
}
