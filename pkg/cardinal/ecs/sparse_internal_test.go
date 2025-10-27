package ecs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSparseSet(t *testing.T) {
	t.Parallel()

	s := newSparseSet()

	assert.Len(t, s, 128)
	for i := range s {
		assert.Equal(t, tombstone, s[i], "index %d should be tombstone", i)
	}
}

func TestSparseSet_Get(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		setup         func() sparseSet
		key           EntityID
		expectedValue int
		expectedFound bool
	}{
		{
			name: "existing value",
			setup: func() sparseSet {
				s := newSparseSet()
				s.set(EntityID(10), 42)
				return s
			},
			key:           EntityID(10),
			expectedValue: 42,
			expectedFound: true,
		},
		{
			name:          "tombstone value",
			setup:         newSparseSet,
			key:           EntityID(10),
			expectedValue: 0,
			expectedFound: false,
		},
		{
			name:          "out of bounds",
			setup:         newSparseSet,
			key:           EntityID(200), // > 128
			expectedValue: 0,
			expectedFound: false,
		},
		{
			name: "boundary index",
			setup: func() sparseSet {
				s := newSparseSet()
				s.set(EntityID(127), 99)
				return s
			},
			key:           EntityID(127),
			expectedValue: 99,
			expectedFound: true,
		},
		{
			name: "index zero",
			setup: func() sparseSet {
				s := newSparseSet()
				s.set(EntityID(0), 123)
				return s
			},
			key:           EntityID(0),
			expectedValue: 123,
			expectedFound: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := tc.setup()
			value, found := s.get(tc.key)

			assert.Equal(t, tc.expectedValue, value)
			assert.Equal(t, tc.expectedFound, found)
		})
	}
}

func TestSparseSet_Set(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		key            EntityID
		value          int
		expectedLength int
		validateGrowth func(*testing.T, sparseSet)
	}{
		{
			name:           "within bounds",
			key:            EntityID(10),
			value:          42,
			expectedLength: 128,
		},
		{
			name:           "at index zero",
			key:            EntityID(0),
			value:          999,
			expectedLength: 128,
		},
		{
			name:           "growth by doubling",
			key:            EntityID(150), // > 128, < 256
			value:          77,
			expectedLength: 256,
			validateGrowth: func(t *testing.T, s sparseSet) {
				for i := 128; i < 256; i++ {
					if i != 150 {
						assert.Equal(t, tombstone, s[i])
					}
				}
			},
		},
		{
			name:           "growth beyond doubling",
			key:            EntityID(300), // > 256
			value:          88,
			expectedLength: 301, // key+1
			validateGrowth: func(t *testing.T, s sparseSet) {
				for i := 128; i < 301; i++ {
					if i != 300 {
						assert.Equal(t, tombstone, s[i])
					}
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := newSparseSet()
			s.set(tc.key, tc.value)

			assert.Len(t, s, tc.expectedLength)
			value, found := s.get(tc.key)
			assert.True(t, found)
			assert.Equal(t, tc.value, value)

			if tc.validateGrowth != nil {
				tc.validateGrowth(t, s)
			}
		})
	}

	t.Run("overwrite value", func(t *testing.T) {
		t.Parallel()

		s := newSparseSet()
		s.set(EntityID(10), 42)
		s.set(EntityID(10), 84) // Overwrite

		value, found := s.get(EntityID(10))
		assert.True(t, found)
		assert.Equal(t, 84, value)
		assert.Len(t, s, 128) // Length unchanged
	})
}

func TestSparseSet_Remove(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		setup          func() sparseSet
		removeKey      EntityID
		expectedResult bool
		validateAfter  func(*testing.T, sparseSet, EntityID)
	}{
		{
			name: "existing value",
			setup: func() sparseSet {
				s := newSparseSet()
				s.set(EntityID(10), 42)
				return s
			},
			removeKey:      EntityID(10),
			expectedResult: true,
			validateAfter: func(t *testing.T, s sparseSet, key EntityID) {
				_, found := s.get(key)
				assert.False(t, found)
				assert.Equal(t, tombstone, s[key])
			},
		},
		{
			name:           "tombstone value",
			setup:          newSparseSet,
			removeKey:      EntityID(10),
			expectedResult: false,
			validateAfter: func(t *testing.T, s sparseSet, key EntityID) {
				_, found := s.get(key)
				assert.False(t, found)
			},
		},
		{
			name:           "out of bounds",
			setup:          newSparseSet,
			removeKey:      EntityID(200),
			expectedResult: false,
			validateAfter: func(t *testing.T, s sparseSet, key EntityID) {
				assert.Len(t, s, 128) // Length unchanged
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := tc.setup()
			result := s.remove(tc.removeKey)

			assert.Equal(t, tc.expectedResult, result)
			tc.validateAfter(t, s, tc.removeKey)
		})
	}
}

func TestSparseSet_Serialize(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		setup    func() sparseSet
		expected []int64
	}{
		{
			name:  "empty sparse set",
			setup: newSparseSet,
			expected: func() []int64 {
				result := make([]int64, 128)
				for i := range result {
					result[i] = int64(tombstone)
				}
				return result
			}(),
		},
		{
			name: "with values",
			setup: func() sparseSet {
				s := newSparseSet()
				s.set(EntityID(5), 55)
				s.set(EntityID(10), 100)
				return s
			},
			expected: func() []int64 {
				result := make([]int64, 128)
				for i := range result {
					result[i] = int64(tombstone)
				}
				result[5] = 55
				result[10] = 100
				return result
			}(),
		},
		{
			name: "after growth",
			setup: func() sparseSet {
				s := newSparseSet()
				s.set(EntityID(200), 2000) // Triggers growth
				return s
			},
			expected: func() []int64 {
				result := make([]int64, 256) // Doubled size
				for i := range result {
					result[i] = int64(tombstone)
				}
				result[200] = 2000
				return result
			}(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := tc.setup()
			result := s.serialize()

			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestSparseSet_Deserialize(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    []int64
		validate func(*testing.T, sparseSet)
	}{
		{
			name:  "empty slice",
			input: []int64{},
			validate: func(t *testing.T, s sparseSet) {
				assert.Empty(t, s)
			},
		},
		{
			name: "with values",
			input: func() []int64 {
				result := make([]int64, 128)
				for i := range result {
					result[i] = int64(tombstone)
				}
				result[5] = 55
				result[10] = 100
				return result
			}(),
			validate: func(t *testing.T, s sparseSet) {
				assert.Len(t, s, 128)

				value, found := s.get(EntityID(5))
				assert.True(t, found)
				assert.Equal(t, 55, value)

				value, found = s.get(EntityID(10))
				assert.True(t, found)
				assert.Equal(t, 100, value)

				// Check tombstone value
				_, found = s.get(EntityID(0))
				assert.False(t, found)
			},
		},
		{
			name:  "replaces existing content",
			input: []int64{10, 20, 30},
			validate: func(t *testing.T, s sparseSet) {
				assert.Len(t, s, 3)
				for i, expected := range []int{10, 20, 30} {
					value, found := s.get(EntityID(i))
					assert.True(t, found)
					assert.Equal(t, expected, value)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Start with pre-populated sparse set to verify replacement
			s := newSparseSet()
			s.set(EntityID(99), 999)

			s.deserialize(tc.input)
			tc.validate(t, s)
		})
	}
}

func TestSparseSet_SerializeDeserialize_RoundTrip(t *testing.T) {
	t.Parallel()

	s := newSparseSet()
	s.set(EntityID(1), 11)
	s.set(EntityID(200), 2000) // triggers growth
	s.set(EntityID(50), 500)
	s.remove(EntityID(1)) // create tombstone

	// Serialize and deserialize
	serialized := s.serialize()
	var restored sparseSet
	restored.deserialize(serialized)

	// Verify equivalence
	require.Len(t, restored, len(s))
	for i := range s {
		origValue, origFound := s.get(EntityID(i))
		restoredValue, restoredFound := restored.get(EntityID(i))

		assert.Equal(t, origFound, restoredFound, "existence mismatch at index %d", i)
		if origFound {
			assert.Equal(t, origValue, restoredValue, "value mismatch at index %d", i)
		}
	}
}

func TestSparseSet_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("zero value handling", func(t *testing.T) {
		t.Parallel()

		s := newSparseSet()
		s.set(EntityID(10), 0) // Set zero (not tombstone)

		value, found := s.get(EntityID(10))
		assert.True(t, found, "should find zero value")
		assert.Equal(t, 0, value)

		// Remove it
		removed := s.remove(EntityID(10))
		assert.True(t, removed)

		// Should not be found now
		_, found = s.get(EntityID(10))
		assert.False(t, found)
	})

	t.Run("complex operation sequence", func(t *testing.T) {
		t.Parallel()

		s := newSparseSet()

		// Set, remove, re-set same key
		s.set(EntityID(15), 150)
		removed := s.remove(EntityID(15))
		assert.True(t, removed)

		s.set(EntityID(15), 300)
		value, found := s.get(EntityID(15))
		assert.True(t, found)
		assert.Equal(t, 300, value)

		// Trigger growth and verify old values persist
		s.set(EntityID(200), 2000)
		assert.Len(t, s, 256)

		value, found = s.get(EntityID(15))
		assert.True(t, found)
		assert.Equal(t, 300, value)
	})
}
