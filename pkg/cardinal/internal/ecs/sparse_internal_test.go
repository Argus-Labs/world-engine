package ecs

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing sparse set operations
// -------------------------------------------------------------------------------------------------
// This test verifies the queue implementation correctness by applying random sequences of
// operations and comparing it against a regular Go map as the model.
// -------------------------------------------------------------------------------------------------

func TestSparseSet_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		opsMax   = 1 << 15 // 32_768 iterations
		eidMax   = 10_000
		opSet    = "set"
		opGet    = "get"
		opRemove = "remove"
		opClear  = "clear"
	)

	// Randomize operation weights.
	operations := []string{opSet, opGet, opRemove, opClear}
	weights := testutils.RandOpWeights(prng, operations)

	impl := newSparseSet()
	model := make(map[EntityID]int, sparseCapacity)

	// Check the impl against the model by running the same operations on both.
	for range opsMax {
		key := EntityID(prng.IntN(eidMax))

		op := testutils.RandWeightedOp(prng, weights)
		switch op {
		case opSet:
			value := prng.Int()
			impl.set(key, value)
			model[key] = value

			// Property: get(k) after set(k) must exist and return the same value.
			got, ok := impl.get(key)
			assert.True(t, ok, "set(%d) then get should exist", key)
			assert.Equal(t, value, got, "set(%d) then get value mismatch", key)

		case opGet:
			// Bias toward existing keys (80%) to test value retrieval path.
			if len(model) > 0 && prng.Float64() < 0.8 {
				key = testutils.RandMapKey(prng, model)
			}
			implValue, implOk := impl.get(key)
			modelValue, modelOk := model[key]

			// Property: get(k) returns same existence and value as model.
			assert.Equal(t, modelOk, implOk, "get(%d) existence mismatch", key)
			if implOk {
				assert.Equal(t, modelValue, implValue, "get(%d) value mismatch", key)
			}

			// Property: if key doesn't exist but is within bounds, internal value must be tombstone.
			if !implOk && int(key) < len(impl) {
				assert.Equal(t, sparseTombstone, impl[key], "get(%d) non-existent key should be tombstone", key)
			}

		case opRemove:
			implOk := impl.remove(key)
			_, modelOk := model[key]
			delete(model, key)

			// Property: remove(k) returns same existence as model.
			assert.Equal(t, modelOk, implOk, "remove(%d) existence mismatch", key)

			// Property: get(k) after remove(k) must not exist (value becomes tombstone).
			_, ok := impl.get(key)
			assert.False(t, ok, "remove(%d) then get should not exist", key)
			if int(key) < len(impl) {
				assert.Equal(t, sparseTombstone, impl[key], "remove(%d) internal value should be tombstone", key)
			}

		case opClear:
			impl.clear()
			clear(model)

			// Property: after clear, length of backing slice is unchanged but all values are tombstones.
			for i := range len(impl) {
				assert.Equal(t, sparseTombstone, impl[i], "clear: index %d should be tombstone", i)
			}

		default:
			panic("unreachable")
		}
	}

	// Final state check: verify all keys in model exist in impl with correct values.
	for key, modelValue := range model {
		implValue, ok := impl.get(key)
		assert.True(t, ok, "key %d should exist in impl", key)
		assert.Equal(t, modelValue, implValue, "key %d value mismatch", key)
	}
}
