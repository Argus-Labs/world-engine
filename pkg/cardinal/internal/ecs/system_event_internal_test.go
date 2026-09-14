package ecs

import (
	"bytes"
	"encoding/gob"
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
	model := make(map[string][]modelFuzzSystemEvent) // name -> system-event buffer

	// Setup: pre-register many system event names with concrete queues.
	for id := range nSystemEventTypes {
		name := seidToString(SystemEventID(id))
		_, err := impl.register[modelFuzzSystemEvent](name)
		require.NoError(t, err)
		model[name] = []modelFuzzSystemEvent{}
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

			err := impl.enqueue(systemEvent)
			require.NoError(t, err)
			model[name] = append(model[name], systemEvent)

		case opGet:
			name := testutils.RandMapKey(prng, model)

			implSysEvents, err := impl.get[modelFuzzSystemEvent](name)
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
				model[name] = []modelFuzzSystemEvent{}
			}

			// Property: all buffers should be empty after clear.
			for name := range model {
				implSysEvents, err := impl.get[modelFuzzSystemEvent](name)
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
		implEvents, err := impl.get[modelFuzzSystemEvent](name)
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

func (c modelFuzzSystemEvent) SizeWire() int              { return len(c.MarshalWire()) }
func (c modelFuzzSystemEvent) AppendWire(b []byte) []byte { return append(b, c.MarshalWire()...) }

func (s modelFuzzSystemEvent) MarshalWire() []byte {
	var b bytes.Buffer
	// A test double that cannot encode itself is a broken fixture, not a runtime condition.
	if err := gob.NewEncoder(&b).Encode(s); err != nil {
		panic(err)
	}
	return b.Bytes()
}

func (modelFuzzSystemEvent) UnmarshalWire(b []byte) (any, error) {
	var v modelFuzzSystemEvent
	err := gob.NewDecoder(bytes.NewReader(b)).Decode(&v)
	return v, err
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
	model := make(map[string]SystemEventID) // name -> ID

	for range opsMax {
		nameID := SystemEventID(prng.IntN(opsMax / 4))
		name := seidToString(nameID)
		implID, err := impl.register[modelFuzzSystemEvent](name)
		require.NoError(t, err)

		if modelID, exists := model[name]; exists {
			assert.Equal(t, modelID, implID, "ID mismatch for re-registered %q", name)
		} else {
			model[name] = implID
		}
	}

	// Property: bijection holds between names and IDs.
	seenIDs := make(map[SystemEventID]string)
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
		sem := newSystemEventManager()
		name1 := seidToString(123)
		name2 := seidToString(124)

		id1, err := sem.register[modelFuzzSystemEvent](name1)
		require.NoError(t, err)

		id2, err := sem.register[modelFuzzSystemEvent](name1)
		require.NoError(t, err)

		assert.Equal(t, id1, id2)

		id3, err := sem.register[modelFuzzSystemEvent](name2)
		require.NoError(t, err)

		assert.Equal(t, id1+1, id3)
	})
}

func seidToString(id SystemEventID) string {
	return strconv.FormatUint(uint64(id), 10)
}

func TestSystemEvent_TypedRegistrationPreservesQueue(t *testing.T) {
	t.Parallel()
	s := newSystemEventManager()

	_, err := s.register[modelFuzzSystemEvent]("")
	require.ErrorContains(t, err, "system event name cannot be empty")
	_, err = s.get[modelFuzzSystemEvent]("missing")
	require.ErrorIs(t, err, ErrSystemEventNotFound)
	require.ErrorIs(t, s.enqueue(modelFuzzSystemEvent{EventName: "missing"}), ErrSystemEventNotFound)

	id, err := s.register[modelFuzzSystemEvent]("first")
	require.NoError(t, err)
	require.Equal(t, SystemEventID(0), id)
	require.NoError(t, s.enqueue(modelFuzzSystemEvent{EventName: "first", Counter: 42, Enabled: true}))

	duplicateID, err := s.register[modelFuzzSystemEvent]("first")
	require.NoError(t, err)
	require.Equal(t, SystemEventID(0), duplicateID)
	secondID, err := s.register[modelFuzzSystemEvent]("second")
	require.NoError(t, err)
	require.Equal(t, SystemEventID(1), secondID)

	events, err := s.get[modelFuzzSystemEvent]("first")
	require.NoError(t, err)
	assert.Equal(t, []modelFuzzSystemEvent{{EventName: "first", Counter: 42, Enabled: true}}, events)
	otherEvents, err := s.get[modelFuzzSystemEvent]("second")
	require.NoError(t, err)
	assert.Empty(t, otherEvents)

	s.clear()
	events, err = s.get[modelFuzzSystemEvent]("first")
	require.NoError(t, err)
	assert.Empty(t, events)
	require.NoError(t, s.enqueue(modelFuzzSystemEvent{EventName: "first", Counter: 7}))
	events, err = s.get[modelFuzzSystemEvent]("first")
	require.NoError(t, err)
	assert.Equal(t, []modelFuzzSystemEvent{{EventName: "first", Counter: 7}}, events)
}

type conflictingSystemEvent struct {
	testutils.SimpleSystemEvent
}

func TestWorld_RegisterSystemEventRejectsNameCollision(t *testing.T) {
	t.Parallel()
	w := NewWorld()
	id, err := w.RegisterSystemEvent[testutils.SimpleSystemEvent]()
	require.NoError(t, err)
	require.Equal(t, SystemEventID(0), id)
	require.NoError(t, w.EmitSystemEvent(testutils.SimpleSystemEvent{Value: 42}))

	id, err = w.RegisterSystemEvent[testutils.SimpleSystemEvent]()
	require.NoError(t, err)
	require.Equal(t, SystemEventID(0), id)
	_, err = w.RegisterSystemEvent[conflictingSystemEvent]()
	require.ErrorContains(t, err, "system event simple_system_event already registered with a different type")

	events, err := w.GetSystemEvents[testutils.SimpleSystemEvent]()
	require.NoError(t, err)
	require.Equal(t, []testutils.SimpleSystemEvent{{Value: 42}}, events)
	require.NoError(t, w.EmitSystemEvent(testutils.SimpleSystemEvent{Value: 7}))
	events, err = w.GetSystemEvents[testutils.SimpleSystemEvent]()
	require.NoError(t, err)
	require.Equal(t, []testutils.SimpleSystemEvent{{Value: 42}, {Value: 7}}, events)
}
