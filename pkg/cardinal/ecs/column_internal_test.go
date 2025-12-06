package ecs

import (
	"math/rand/v2"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing
//
// This test verifies the column implementation correctness using model-based testing. It compares
// our implementation against Go's slice with swap-remove semantics as the model by applying random
// sequences of extend/set/get/remove operations to both and asserting equivalence.
// Operations are weighted (extend=20%, set=35%, remove=30%, get=15%) to prioritize state mutations.
// -------------------------------------------------------------------------------------------------

func TestColumn_ModelBasedFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand()

	impl := newColumn[testutils.SimpleComponent]()
	model := make([]testutils.SimpleComponent, 0, columnCapacity)

	const opsMax = 1 << 15 // 32_768 iterations

	for range opsMax {
		op := getRandomColumnOp(prng)
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

type columnOp uint8

const (
	opExtend columnOp = 20
	opSet    columnOp = 35
	opRemove columnOp = 30
	opGet    columnOp = 15
)

var columnOps = []columnOp{opExtend, opSet, opRemove, opGet}

func getRandomColumnOp(r *rand.Rand) columnOp {
	var total int
	for _, op := range columnOps {
		total += int(op)
	}

	pick := r.IntN(total)
	for _, op := range columnOps {
		weight := int(op)
		if pick < weight {
			return op
		}
		pick -= weight
	}
	panic("unreachable")
}

// -------------------------------------------------------------------------------------------------
// Serialization tests
//
// We don't extensively test serialize/deserialize because:
// 1. The implementation is a thin wrapper around json.Marshal/Unmarshal (well-tested stdlib).
// 2. The loop logic is trivial with no complex branching.
// 3. Heavy property-based testing would mostly exercise the json package, not our code.
// -------------------------------------------------------------------------------------------------

func TestColumn_Serialization(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand()

	const lengthMax = 1000

	col1 := newColumn[testutils.SimpleComponent]()
	for i := range prng.IntN(lengthMax) {
		col1.extend()
		col1.set(i, testutils.SimpleComponent{Value: i})
	}

	pb, err := col1.toProto()
	assert.NoError(t, err)

	col2 := newColumn[testutils.SimpleComponent]()
	err = col2.fromProto(pb)
	assert.NoError(t, err)

	// Property: deserialize(serialize(x)) == x.
	assert.Equal(t, col1.len(), col2.len())
	for i := range col1.len() {
		assert.Equal(t, col1.get(i), col2.get(i))
	}
}
