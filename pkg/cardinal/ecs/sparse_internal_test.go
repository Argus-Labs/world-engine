package ecs

import (
	"math/rand/v2"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

// -------------------------------------------------------------------------------------------------
// Model-Based Fuzzing
//
// This test verifies the sparseSet implementation correctness using model-based testing. It
// compares our implementation against a Go's map as the model by applying random sequences of
// set/get/remove operations to both and asserting equivalence.
// Operations are weighted (set=55%, remove=35%, get=10%) to prioritize state mutations.
// -------------------------------------------------------------------------------------------------

func TestSparseSet_ModelBasedFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand()

	impl := newSparseSet()
	model := make(map[EntityID]int, sparseCapacity)

	const (
		opsMax = 1 << 15 // 32_768 iterations
		maxKey = 10_000
	)

	// Check the impl against the model by running the same operations on both.
	for range opsMax {
		key := EntityID(prng.IntN(maxKey))

		op := getRandomSparseSetOp(prng)
		switch op {
		case set:
			value := prng.Int()
			impl.set(key, value)
			model[key] = value

			// Property: get(k) after set(k) must exist and return the same value.
			got, ok := impl.get(key)
			assert.True(t, ok, "set(%d) then get should exist", key)
			assert.Equal(t, value, got, "set(%d) then get value mismatch", key)

		case get:
			// Bias toward existing keys (80%) to test value retrieval path.
			if len(model) > 0 && prng.Float64() < 0.8 {
				key = testutils.RandMapKey(prng, model)
			}
			gotImpl, okImpl := impl.get(key)
			gotModel, okModel := model[key]

			// Property: get(k) returns same existence and value as model.
			assert.Equal(t, okModel, okImpl, "get(%d) existence mismatch", key)
			if okImpl {
				assert.Equal(t, gotModel, gotImpl, "get(%d) value mismatch", key)
			}

			// Property: if key doesn't exist but is within bounds, internal value must be tombstone.
			if !okImpl && int(key) < len(impl) {
				assert.Equal(t, sparseTombstone, impl[key], "get(%d) non-existent key should be tombstone", key)
			}

		case remove:
			okImpl := impl.remove(key)
			_, okModel := model[key]
			delete(model, key)

			// Property: remove(k) returns same existence as model.
			assert.Equal(t, okModel, okImpl, "remove(%d) existence mismatch", key)

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
	for key, expectedVal := range model {
		gotVal, ok := impl.get(key)
		assert.True(t, ok, "key %d should exist in impl", key)
		assert.Equal(t, expectedVal, gotVal, "key %d value mismatch", key)
	}
}

type sparseSetOp uint8

const (
	set    sparseSetOp = 55
	remove sparseSetOp = 35
	get    sparseSetOp = 10
)

var sparseSetOps = []sparseSetOp{set, remove, get}

func getRandomSparseSetOp(r *rand.Rand) sparseSetOp {
	var total int
	for _, op := range sparseSetOps {
		total += int(op)
	}

	pick := r.IntN(total)
	for _, op := range sparseSetOps {
		weight := int(op)
		if pick < weight {
			return op
		}
		pick -= weight
	}
	panic("unreachable")
}
