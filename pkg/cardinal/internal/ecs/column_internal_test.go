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
// This test verifies the archetype implementation correctness by applying random sequences of
// operations and comparing it against a regular Go slice as the model.
// -------------------------------------------------------------------------------------------------

func TestColumn_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		opsMax   = 1 << 15 // 32_768 iterations
		opExtend = "extend"
		opSet    = "set"
		opGet    = "get"
		opRemove = "remove"
	)

	// Randomize operation weights.
	operations := []string{opExtend, opSet, opGet, opRemove}
	weights := testutils.RandOpWeights(prng, operations)

	impl := newColumn[testutils.SimpleComponent]()
	model := make([]testutils.SimpleComponent, 0, columnCapacity)

	for range opsMax {
		op := testutils.RandWeightedOp(prng, weights)
		switch op {
		case opExtend:
			impl.extend()
			model = append(model, testutils.SimpleComponent{})

			// Property: length increases by 1.
			assert.Equal(t, len(model), impl.len(), "extend length mismatch")

		case opSet:
			if len(model) == 0 {
				continue
			}

			row := prng.IntN(len(model))

			value := testutils.SimpleComponent{Value: prng.Int()}
			impl.set(row, value)
			model[row] = value

			// Property: get(k) after set(k) returns same value.
			assert.Equal(t, value, impl.get(row), "set(%d) then get value mismatch", row)

		case opGet:
			if len(model) == 0 {
				continue
			}
			row := prng.IntN(len(model))

			implValue := impl.get(row)
			modelValue := model[row]

			// Property: get(k) returns same value as model.
			assert.Equal(t, modelValue, implValue, "get(%d) value mismatch", row)

		case opRemove:
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

// -------------------------------------------------------------------------------------------------
// Deserialization edge cases
// -------------------------------------------------------------------------------------------------
// Examples of some edge cases of fromProto we care about.
// -------------------------------------------------------------------------------------------------

func TestColumn_FromProto(t *testing.T) {
	t.Parallel()

	t.Run("rejects nil", func(t *testing.T) {
		t.Parallel()
		col := newColumn[testutils.SimpleComponent]()
		err := col.fromProto(nil)
		assert.Error(t, err)
	})

	t.Run("rejects component name mismatch", func(t *testing.T) {
		t.Parallel()

		colA := newColumn[testutils.ComponentA]()
		colA.extend()
		colA.set(0, testutils.ComponentA{X: 1, Y: 2, Z: 3})

		pb, err := colA.toProto()
		require.NoError(t, err)

		colB := newColumn[testutils.ComponentB]()
		err = colB.fromProto(pb)
		assert.Error(t, err)
	})
}
