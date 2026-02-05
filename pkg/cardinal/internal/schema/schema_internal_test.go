package schema_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSerialize_Uint64Precision verifies that serialization preserves uint64 precision for values
// above 2^53-1, which would be corrupted by JSON's float64 representation.
func TestSerialize_Uint64Precision(t *testing.T) {
	t.Parallel()

	testCases := []testutils.CommandUint64{
		{
			Amount:    18446744073709551615, // uint64 max
			EntityID:  9007199254740993,     // 2^53 + 1, loses precision in JSON
			Timestamp: 9223372036854775807,  // int64 max
		},
		{
			Amount:    10000000000000000000, // 10^19
			EntityID:  9007199254740992,     // 2^53, first value that loses precision in JSON
			Timestamp: -9223372036854775808, // int64 min
		},
	}

	for _, expected := range testCases {
		data, err := schema.Serialize(expected)
		require.NoError(t, err)

		var actual testutils.CommandUint64
		err = schema.Deserialize(data, &actual)
		require.NoError(t, err)

		assert.Equal(t, expected.Amount, actual.Amount, "Amount mismatch")
		assert.Equal(t, expected.EntityID, actual.EntityID, "EntityID mismatch")
		assert.Equal(t, expected.Timestamp, actual.Timestamp, "Timestamp mismatch")
	}
}

// TestSerialize_TypeCoverage verifies that serialization correctly handles all common Go types,
// with special attention to uint64 values above 2^53-1 which would lose precision in JSON.
func TestSerialize_TypeCoverage(t *testing.T) {
	t.Parallel()

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

	for i, expected := range testCases {
		data, err := schema.Serialize(expected)
		require.NoError(t, err)

		var actual testutils.ComponentMixed
		err = schema.Deserialize(data, &actual)
		require.NoError(t, err)

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
