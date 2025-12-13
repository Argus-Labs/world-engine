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
// This test verifies the componentManager registration correctness using model-based testing. It
// compares our implementation against a map[string]componentID as the model by applying random
// sequences of register/getID operations to both and asserting equivalence.
// We also verify structural invariants: name-id bijection and component id uniqueness.
// -------------------------------------------------------------------------------------------------

func TestComponent_RegisterModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const opsMax = 1 << 15 // 32_768 iterations

	impl := newComponentManager()
	model := make(map[string]componentID) // name -> cid

	for range opsMax {
		// 70% register, 30% getID
		if prng.Float64() < 0.7 { //nolint:nestif // it's not bad
			name := randValidComponentName(prng)

			implID, implErr := impl.register(name, nil) // we don't use the columnFactory so it's ok
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
		} else {
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
		}
	}

	// Property: bijection holds between names and IDs.
	// Bijection means there's a 1-1 mapping of name->ID. Every name maps to a unique ID, and
	// every ID comes from a unique name.
	seenIDs := make(map[componentID]string)
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

		id1, err := cm.register("hello", nil)
		require.NoError(t, err)

		id2, err := cm.register("hello", nil)
		require.NoError(t, err)

		assert.Equal(t, id1, id2)

		id3, err := cm.register("a_different_name", nil)
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
