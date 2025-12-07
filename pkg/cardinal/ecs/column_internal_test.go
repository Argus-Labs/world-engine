package ecs

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing column operations
// -------------------------------------------------------------------------------------------------
// This test verifies the column implementation correctness using model-based testing. It compares
// our implementation against Go's slice with swap-remove semantics as the model by applying random
// sequences of extend/set/get/remove operations to both and asserting equivalence.
// -------------------------------------------------------------------------------------------------

func TestColumn_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const opsMax = 1 << 15 // 32_768 iterations

	impl := newColumn[testutils.SimpleComponent]()
	model := make([]testutils.SimpleComponent, 0, columnCapacity)

	for range opsMax {
		op := testutils.RandWeightedOp(prng, columnOps)
		switch op {
		case c_extend:
			impl.extend()
			model = append(model, testutils.SimpleComponent{})

			// Property: length increases by 1.
			assert.Equal(t, len(model), impl.len(), "extend length mismatch")

		case c_set:
			if len(model) == 0 {
				continue
			}

			row := prng.IntN(len(model))

			value := testutils.SimpleComponent{Value: prng.Int()}
			impl.set(row, value)
			model[row] = value

			// Property: get(k) after set(k) returns same value.
			assert.Equal(t, value, impl.get(row), "set(%d) then get value mismatch", row)

		case c_get:
			if len(model) == 0 {
				continue
			}
			row := prng.IntN(len(model))

			implValue := impl.get(row)
			modelValue := model[row]

			// Property: get(k) returns same value as model.
			assert.Equal(t, modelValue, implValue, "get(%d) value mismatch", row)

		case c_remove:
			if len(model) == 0 {
				continue
			}
			row := prng.IntN(len(model))

			impl.remove(row)
			// Reimplement the remove swap mechanism here.
			last := len(model) - 1
			model[row] = model[last]
			model = model[:last]

			// Property: length decreases by 1.
			assert.Equal(t, len(model), impl.len(), "remove length mismatch")

			// Property: if row still valid, it now contains what was the last element.
			if row < len(model) {
				assert.Equal(t, model[row], impl.get(row), "remove(%d) swap mismatch", row)
			}

		default:
			panic("unreachable")
		}
	}

	// Final state check: verify all elements match between impl and model.
	assert.Equal(t, len(model), impl.len(), "final length mismatch")
	for i, expected := range model {
		got := impl.get(i)
		assert.Equal(t, expected, got, "element %d mismatch", i)
	}
}

type columnOp uint8

const (
	c_extend columnOp = 20
	c_set    columnOp = 35
	c_remove columnOp = 30
	c_get    columnOp = 15
)

var columnOps = []columnOp{c_extend, c_set, c_remove, c_get}

// -------------------------------------------------------------------------------------------------
// Serialization smoke test
// -------------------------------------------------------------------------------------------------
// We don't extensively test toProto/fromProto because:
// 1. The implementation is a thin wrapper around json.Marshal/Unmarshal (well-tested stdlib).
// 2. The loop logic is trivial with no complex branching.
// 3. Heavy property-based testing would mostly exercise the json package, not our code.
// -------------------------------------------------------------------------------------------------

func TestColumn_SerializationSmoke(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const lengthMax = 1000

	col1 := newColumn[testutils.SimpleComponent]()
	for i := range prng.IntN(lengthMax) {
		col1.extend()
		col1.set(i, testutils.SimpleComponent{Value: i})
	}

	pb, err := col1.toProto()
	require.NoError(t, err)

	col2 := newColumn[testutils.SimpleComponent]()
	err = col2.fromProto(pb)
	require.NoError(t, err)

	// Property: deserialize(serialize(x)) == x.
	assert.Equal(t, col1, col2) // assert.Equal uses reflect.DeepEqual
}
