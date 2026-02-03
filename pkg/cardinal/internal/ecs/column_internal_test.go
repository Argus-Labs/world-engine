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
// Serialization precision and type coverage test
// -------------------------------------------------------------------------------------------------
// This test verifies that MessagePack serialization correctly handles all common Go types,
// with special attention to uint64 values above 2^53-1 which would lose precision in JSON.
// -------------------------------------------------------------------------------------------------

func TestColumn_MsgpackTypeCoverage(t *testing.T) {
	t.Parallel()

	// Comprehensive test cases covering all common types
	testCases := []testutils.ComponentMixed{
		{
			// Integer edge cases
			Int8Val:   -128,
			Int16Val:  -32768,
			Int32Val:  -2147483648,
			Int64Val:  -9223372036854775808, // int64 min
			Uint8Val:  255,
			Uint16Val: 65535,
			Uint32Val: 4294967295,
			Uint64Val: 18446744073709551615, // uint64 max - CRITICAL: loses precision in JSON

			// Float edge cases
			Float32Val: 3.4028235e+38, // float32 max
			Float64Val: 1.7976931348623157e+308,

			// String and bool
			StringVal: "hello world with unicode: 你好世界 🌍",
			BoolVal:   true,

			// Slices and arrays
			IntSlice:   []int{1, 2, 3, -1000, 9007199254740993}, // includes value > 2^53
			ByteSlice:  []byte{0x00, 0xFF, 0xAB, 0xCD},
			FloatArray: [3]float64{1.1, 2.2, 3.3},

			// Nested struct with large uint64
			Nested: testutils.NestedData{
				ID:    9007199254740993, // 2^53 + 1, loses precision in JSON
				Name:  "nested entity",
				Score: 99.99,
			},

			// Map
			Metadata: map[string]int{"count": 42, "level": 100},
		},
		{
			// Zero/empty values
			Int8Val:    0,
			Int16Val:   0,
			Int32Val:   0,
			Int64Val:   0,
			Uint8Val:   0,
			Uint16Val:  0,
			Uint32Val:  0,
			Uint64Val:  0,
			Float32Val: 0.0,
			Float64Val: 0.0,
			StringVal:  "",
			BoolVal:    false,
			IntSlice:   []int{},
			ByteSlice:  []byte{},
			FloatArray: [3]float64{0, 0, 0},
			Nested:     testutils.NestedData{},
			Metadata:   map[string]int{},
		},
		{
			// Values around JSON precision boundary (2^53 - 1 = 9007199254740991)
			Int64Val:   9223372036854775807, // int64 max
			Uint64Val:  9007199254740992,    // 2^53, first value that loses precision in JSON
			Float64Val: 9007199254740991.0,  // max safe integer as float

			StringVal:  "boundary test",
			BoolVal:    true,
			IntSlice:   []int{9007199254740991, 9007199254740992, 9007199254740993},
			ByteSlice:  []byte("test"),
			FloatArray: [3]float64{-1.0, 0.0, 1.0},

			Nested: testutils.NestedData{
				ID:    10000000000000000000, // 10^19, well above JSON safe range
				Name:  "large id entity",
				Score: -0.5,
			},

			Metadata: map[string]int{"a": 1, "b": 2, "c": 3},
		},
	}

	col := newColumn[testutils.ComponentMixed]()
	for i, tc := range testCases {
		col.extend()
		col.set(i, tc)
	}

	// Serialize to proto (uses MessagePack internally)
	pb, err := col.toProto()
	require.NoError(t, err)

	// Deserialize back
	col2 := newColumn[testutils.ComponentMixed]()
	err = col2.fromProto(pb)
	require.NoError(t, err)

	// Verify all values are preserved exactly
	require.Equal(t, col.len(), col2.len(), "length mismatch")
	for i, expected := range testCases {
		actual := col2.get(i)

		// Integer types
		assert.Equal(t, expected.Int8Val, actual.Int8Val, "Int8Val mismatch at %d", i)
		assert.Equal(t, expected.Int16Val, actual.Int16Val, "Int16Val mismatch at %d", i)
		assert.Equal(t, expected.Int32Val, actual.Int32Val, "Int32Val mismatch at %d", i)
		assert.Equal(t, expected.Int64Val, actual.Int64Val, "Int64Val mismatch at %d", i)
		assert.Equal(t, expected.Uint8Val, actual.Uint8Val, "Uint8Val mismatch at %d", i)
		assert.Equal(t, expected.Uint16Val, actual.Uint16Val, "Uint16Val mismatch at %d", i)
		assert.Equal(t, expected.Uint32Val, actual.Uint32Val, "Uint32Val mismatch at %d", i)
		// Critical: uint64 values > 2^53-1 would lose precision with JSON
		assert.Equal(t, expected.Uint64Val, actual.Uint64Val, "Uint64Val mismatch at %d", i)

		// Float types - use InDelta for floating point comparison
		assert.InDelta(t, expected.Float32Val, actual.Float32Val, 1e-6, "Float32Val mismatch at %d", i)
		assert.InDelta(t, expected.Float64Val, actual.Float64Val, 1e-10, "Float64Val mismatch at %d", i)

		// String and bool
		assert.Equal(t, expected.StringVal, actual.StringVal, "StringVal mismatch at %d", i)
		assert.Equal(t, expected.BoolVal, actual.BoolVal, "BoolVal mismatch at %d", i)

		// Slices and arrays
		assert.Equal(t, expected.IntSlice, actual.IntSlice, "IntSlice mismatch at %d", i)
		assert.Equal(t, expected.ByteSlice, actual.ByteSlice, "ByteSlice mismatch at %d", i)
		assert.InDeltaSlice(t, expected.FloatArray[:], actual.FloatArray[:], 1e-10, "FloatArray mismatch at %d", i)

		// Nested struct - uint64 ID would lose precision with JSON
		assert.Equal(t, expected.Nested.ID, actual.Nested.ID, "Nested.ID mismatch at %d", i)
		assert.Equal(t, expected.Nested.Name, actual.Nested.Name, "Nested.Name mismatch at %d", i)
		assert.InDelta(t, expected.Nested.Score, actual.Nested.Score, 1e-10, "Nested.Score mismatch at %d", i)

		// Map
		assert.Equal(t, expected.Metadata, actual.Metadata, "Metadata mismatch at %d", i)
	}
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
