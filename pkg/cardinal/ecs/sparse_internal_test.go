package ecs

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing sparse set operations
//
// This test verifies the sparseSet implementation correctness using model-based testing. It
// compares our implementation against a Go's map as the model by applying random sequences of
// set/get/remove operations to both and asserting equivalence.
// -------------------------------------------------------------------------------------------------

func TestSparseSet_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		opsMax = 1 << 15 // 32_768 iterations
		eidMax = 10_000
	)

	impl := newSparseSet()
	model := make(map[EntityID]int, sparseCapacity)

	// Check the impl against the model by running the same operations on both.
	for range opsMax {
		key := EntityID(prng.IntN(eidMax))

		op := testutils.RandWeightedOp(prng, sparseSetOps)
		switch op {
		case s_set:
			value := prng.Int()
			impl.set(key, value)
			model[key] = value

			// Property: get(k) after set(k) must exist and return the same value.
			got, ok := impl.get(key)
			assert.True(t, ok, "set(%d) then get should exist", key)
			assert.Equal(t, value, got, "set(%d) then get value mismatch", key)

		case s_get:
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

		case s_remove:
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

type sparseSetOp uint8

const (
	s_set    sparseSetOp = 55
	s_remove sparseSetOp = 35
	s_get    sparseSetOp = 10
)

var sparseSetOps = []sparseSetOp{s_set, s_remove, s_get}

// -------------------------------------------------------------------------------------------------
// Serialization smoke test
//
// We don't extensively test toInt64Slice/fromInt64Slice because:
// 1. The implementation is a trivial type conversion loop (int -> int64 and back).
// 2. There's no complex branching or error handling.
// 3. Heavy property-based testing would mostly verify Go's type conversion, not our logic.
// -------------------------------------------------------------------------------------------------

func TestSparseSet_SerializationSmoke(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		opsMax = 100
		eidMax = 10_000
	)

	impl1 := newSparseSet()
	for range opsMax {
		key := EntityID(prng.IntN(eidMax))
		value := prng.Int()
		impl1.set(key, value)
	}

	data := impl1.toInt64Slice()

	impl2 := newSparseSet()
	impl2.fromInt64Slice(data)

	// Property: deserialize(serialize(x)) == x.
	assert.Len(t, impl2, len(impl1))
	for i := range impl1 {
		assert.Equal(t, impl1[i], impl2[i])
	}
}
