package ecs

import (
	"math/rand/v2"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing component registration
// -------------------------------------------------------------------------------------------------
// This test verifies the archetype implementation correctness by applying random sequences of
// operations and comparing it against a regular Go map of name->id as the model. We also verify
// structural invariants: name-id bijection and component id uniqueness.
// -------------------------------------------------------------------------------------------------

func TestComponent_RegisterModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		opsMax     = 1 << 15 // 32_768 iterations
		opRegister = "register"
		opGetID    = "getID"
	)

	// Randomize operation weights.
	operations := []string{opRegister, opGetID}
	weights := testutils.RandOpWeights(prng, operations)

	impl := newComponentManager()
	model := make(map[string]ComponentID) // name -> cid

	for range opsMax {
		op := testutils.RandWeightedOp(prng, weights)
		switch op {
		case opRegister:
			name := randValidComponentName(prng)

			implID, implErr := impl.register[testutils.SimpleComponent](name)
			modelID, modelExists := model[name]

			if modelExists {
				// Property: re-registering returns same ID.
				require.NoError(t, implErr)
				assert.Equal(t, modelID, implID, "re-register(%s) ID mismatch", name)
			} else {
				// Property: new registration succeeds and assigns next ID.
				require.NoError(t, implErr)
				model[name] = implID
			}

		case opGetID:
			// Bias toward registered names (80%) to test retrieval path.
			var name string
			if len(model) > 0 && prng.Float64() < 0.8 {
				name = testutils.RandMapKey(prng, model)
			} else {
				name = randValidComponentName(prng)
			}

			implID, implErr := impl.getID(name)
			modelID, modelExists := model[name]

			// Property: getID returns same existence and value as model.
			assert.Equal(t, modelExists, implErr == nil, "getID(%s) existence mismatch", name)
			if modelExists {
				assert.Equal(t, modelID, implID, "getID(%s) ID mismatch", name)
			}

		default:
			panic("unreachable")
		}
	}

	// Property: bijection holds between names and IDs.
	// Bijection means there's a 1-1 mapping of name->ID. Every name maps to a unique ID, and
	// every ID comes from a unique name.
	seenIDs := make(map[ComponentID]string)
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

	// Final state check: every model entry exists in impl with matching ID.
	assert.Len(t, impl.catalog, len(model), "catalog length mismatch")
	for name, modelID := range model {
		implID, err := impl.getID(name)
		require.NoError(t, err, "component %q in model but not in impl", name)
		assert.Equal(t, modelID, implID, "component %q ID mismatch", name)
	}

	// Simple test to confirm that registering the same name repeatedly is a no-op.
	t.Run("registration idempotence", func(t *testing.T) {
		t.Parallel()

		cm := newComponentManager()

		id1, err := cm.register[testutils.SimpleComponent]("hello")
		require.NoError(t, err)

		id2, err := cm.register[testutils.SimpleComponent]("hello")
		require.NoError(t, err)

		assert.Equal(t, id1, id2)

		id3, err := cm.register[testutils.SimpleComponent]("a_different_name")
		require.NoError(t, err)

		assert.Equal(t, id1+1, id3)
	})
}

// -------------------------------------------------------------------------------------------------
// Component name validation fuzz
// -------------------------------------------------------------------------------------------------
// This test verifies validateComponentName correctly implements the expr identifier specification:
// identifiers must start with [a-zA-Z_] and contain only [a-zA-Z0-9_].
// -------------------------------------------------------------------------------------------------

func TestComponent_NameValidationFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const opsMax = 1 << 15 // 32_768 iterations

	valid, invalid := 0, 0

	for range opsMax {
		// Generate valid names + invalid names (by corrupting valid names).
		// We're using a custom generator instead of Go's builtin testing/quick because the latter
		// generates purely random string which doesn't exercise the validation logic much.
		b := []byte(randValidComponentName(prng))

		if prng.Float64() < 0.95 && len(b) > 0 {
			numFlips := prng.IntN(5) + 1
			for range numFlips {
				idx := prng.IntN(len(b))
				bit := uint8(1 << prng.IntN(8))
				b[idx] ^= bit
			}
		}

		name := string(b)
		expected := assertNameProperties(name)
		actual := validateComponentName(name) == nil
		assert.Equal(t, expected, actual, "mismatch for name: %q", name)

		if actual {
			valid++
		} else {
			invalid++
		}
	}
	assert.Equal(t, opsMax, valid+invalid)
}

func assertNameProperties(name string) bool {
	// Property: name cannot be empty.
	if name == "" {
		return false
	}
	// Property: name must start with a letter or underscore.
	first := name[0]
	if (first < 'a' || first > 'z') && (first < 'A' || first > 'Z') && first != '_' {
		return false
	}
	// Property: name can only contain alphanumeric characters and underscore.
	for i := 1; i < len(name); i++ {
		c := name[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

func randValidComponentName(prng *rand.Rand) string {
	const firstChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_"
	const restChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"
	length := prng.IntN(100) + 1 // 1-100 characters
	b := make([]byte, length)
	b[0] = firstChars[prng.IntN(len(firstChars))]
	for i := 1; i < length; i++ {
		b[i] = restChars[prng.IntN(len(restChars))]
	}
	return string(b)
}

type conflictingComponent struct {
	testutils.SimpleComponent
}

func TestWorld_HasComponent(t *testing.T) {
	t.Parallel()
	w := NewWorld()
	eid := w.Create()
	assert.False(t, w.Has[testutils.ComponentA](eid), "unregistered component")

	_, err := w.RegisterComponent[testutils.ComponentA]()
	require.NoError(t, err)
	assert.False(t, w.Has[testutils.ComponentA](eid), "registered but absent component")

	require.NoError(t, w.Set(eid, testutils.ComponentA{X: 42}))
	assert.True(t, w.Has[testutils.ComponentA](eid), "present component")

	require.NoError(t, w.Remove[testutils.ComponentA](eid))
	assert.False(t, w.Has[testutils.ComponentA](eid), "removed component")

	require.NoError(t, w.Set(eid, testutils.ComponentA{X: 42}))
	require.True(t, w.Destroy(eid))
	assert.False(t, w.Has[testutils.ComponentA](eid), "destroyed entity")
}

func TestWorld_RegisterComponentRejectsNameCollision(t *testing.T) {
	t.Parallel()
	w := NewWorld()
	id, err := w.RegisterComponent[testutils.SimpleComponent]()
	require.NoError(t, err)
	require.Equal(t, ComponentID(0), id)
	eid := w.Create()
	require.NoError(t, w.Set(eid, testutils.SimpleComponent{Value: 42}))

	id, err = w.RegisterComponent[testutils.SimpleComponent]()
	require.NoError(t, err)
	require.Equal(t, ComponentID(0), id)
	_, err = w.RegisterComponent[conflictingComponent]()
	require.ErrorContains(t, err, "component simple_component already registered with a different type")

	value, err := w.Get[testutils.SimpleComponent](eid)
	require.NoError(t, err)
	require.Equal(t, testutils.SimpleComponent{Value: 42}, value)
	require.NoError(t, w.Set(eid, testutils.SimpleComponent{Value: 7}))
	value, err = w.Get[testutils.SimpleComponent](eid)
	require.NoError(t, err)
	require.Equal(t, testutils.SimpleComponent{Value: 7}, value)
}

func TestWorld_ConflictingComponentAccessPreservesData(t *testing.T) {
	t.Parallel()
	w := NewWorld()
	_, err := w.RegisterComponent[testutils.SimpleComponent]()
	require.NoError(t, err)
	eid := w.Create()
	require.NoError(t, w.Set(eid, testutils.SimpleComponent{Value: 42}))

	require.ErrorIs(t, w.Remove[conflictingComponent](eid), ErrComponentNotFound)
	assert.False(t, w.Has[conflictingComponent](eid))
	_, err = w.Get[conflictingComponent](eid)
	require.ErrorIs(t, err, ErrComponentNotFound)
	value, err := w.Get[testutils.SimpleComponent](eid)
	require.NoError(t, err)
	assert.Equal(t, testutils.SimpleComponent{Value: 42}, value)
}
