package ecs

import (
	"errors"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
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
// MessagePack codec smoke test
// -------------------------------------------------------------------------------------------------
// We don't extensively test the row codecs because:
// 1. They are thin wrappers around MessagePack.
// 2. The loop logic is trivial with no complex branching.
// 3. Heavy property-based testing would mostly exercise the component codec, not our code.
// -------------------------------------------------------------------------------------------------

func TestColumn_WireRoundTrip(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const lengthMax = 1000

	col1 := newColumn[testutils.SimpleComponent]()
	length := 1 + prng.IntN(lengthMax)
	for i := range length {
		col1.extend()
		col1.set(i, testutils.SimpleComponent{Value: i})
	}

	// Encode every row, decode into a fresh column.
	col2 := newColumn[testutils.SimpleComponent]()
	for i := range length {
		payload, err := col1.encode(i)
		require.NoError(t, err)

		col2.extend()
		require.NoError(t, col2.decode(i, payload))
	}

	// Property: decode(encode(x)) == x, row by row.
	for i := range length {
		assert.Equal(t, col1.get(i), col2.get(i), "row %d mismatch", i)
	}
}

func TestColumn_DecodeRejectsGarbage(t *testing.T) {
	t.Parallel()

	col := newColumn[testutils.SimpleComponent]()
	col.extend()
	col.set(0, testutils.SimpleComponent{Value: 42})
	err := col.decode(0, []byte{0xc1}) // reserved MessagePack marker
	require.Error(t, err)
	assert.Equal(t, testutils.SimpleComponent{Value: 42}, col.get(0))
}

// unencodablePayload passes component validation but always fails to encode, standing in for any
// encoding failure at snapshot time.
type unencodablePayload struct {
	Value int
}

func (unencodablePayload) Name() string { return "unencodable_payload" }

func (unencodablePayload) EncodeMsgpack(*msgpack.Encoder) error { return errors.New("cannot encode") }

func TestColumn_EncodeFailure(t *testing.T) {
	t.Parallel()
	col := newColumn[unencodablePayload]()
	col.extend()
	value := unencodablePayload{Value: 1}
	col.set(0, value)
	payload, err := col.encode(0)
	require.Error(t, err)
	assert.Nil(t, payload)
	assert.Equal(t, value, col.get(0))
}
