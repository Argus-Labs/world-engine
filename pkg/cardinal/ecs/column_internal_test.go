package ecs

import (
	"encoding/json"
	"testing"

	. "github.com/argus-labs/world-engine/pkg/cardinal/ecs/internal/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/cardinal/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestColumn_NewColumn(t *testing.T) {
	t.Parallel()

	col := newColumn[Health]()
	assert.NotNil(t, col)
}

func TestColumn_InsertAndGet(t *testing.T) {
	t.Parallel()

	col := newColumn[Health]()
	testCases := []struct {
		name        string
		entity      EntityID
		value       Health
		expected    Health
		shouldExist bool
		wantErr     bool
	}{
		{
			name:        "insert new component",
			entity:      1,
			value:       Health{Value: 42},
			expected:    Health{Value: 42},
			shouldExist: true,
		},
		{
			name:        "update existing component",
			entity:      1,
			value:       Health{Value: 100},
			expected:    Health{Value: 100},
			shouldExist: true,
		},
		{
			name:        "zero entity ID",
			entity:      0,
			value:       Health{Value: 50},
			expected:    Health{Value: 50},
			shouldExist: true,
		},
		{
			name:        "get non-existent entity",
			entity:      999,
			shouldExist: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.shouldExist {
				err := col.set(tc.entity, tc.value)
				require.NoError(t, err)
			}

			value, exists := col.get(tc.entity)
			assert.Equal(t, tc.shouldExist, exists, "existence check failed")
			if tc.shouldExist && !tc.wantErr {
				assert.Equal(t, tc.expected, value, "value mismatch")
			}
		})
	}
}

func TestColumn_Remove(t *testing.T) {
	t.Parallel()

	t.Run("existing entity", func(t *testing.T) {
		t.Parallel()

		col := newColumn[Health]()
		entity := EntityID(1)
		comp := Health{Value: 42}

		err := col.set(entity, comp)
		require.NoError(t, err)
		assert.True(t, col.contains(entity))

		col.remove(entity)
		assert.False(t, col.contains(entity))
		_, exists := col.get(entity)
		assert.False(t, exists)
	})

	t.Run("non-existing entity", func(t *testing.T) {
		t.Parallel()
		col := newColumn[Health]()
		entity := EntityID(1)
		col.remove(entity)
		assert.Equal(t, 0, col.len()) // Shouldn't change from no-op
	})

	t.Run("entity ID too large", func(t *testing.T) {
		t.Parallel()
		col := newColumn[Health]()
		entity := EntityID(1)
		comp := Health{Value: 42}

		err := col.set(entity, comp)
		require.NoError(t, err)
		assert.True(t, col.contains(entity))

		col.remove(EntityID(1<<30 + 1))
		assert.Equal(t, 1, col.len()) // Shouldn't change from no-op
	})
}

func TestColumn_Clear(t *testing.T) {
	t.Parallel()
	col := newColumn[Health]()

	entities := []EntityID{1, 2, 3}
	for _, entity := range entities {
		err := col.set(entity, Health{Value: 1})
		require.NoError(t, err)
	}

	assert.Equal(t, len(entities), col.len())

	col.clear()
	assert.Zero(t, col.len())

	// Test operations after clear
	err := col.set(1, Health{Value: 42})
	require.NoError(t, err)
	assert.Equal(t, 1, col.len())
	assert.True(t, col.contains(1))
}

func TestColumn_Len(t *testing.T) {
	t.Parallel()
	col := newColumn[Health]()
	assert.Zero(t, col.len(), "new column should be empty")

	err := col.set(1, Health{Value: 42})
	require.NoError(t, err)
	assert.Equal(t, 1, col.len(), "length should be 1 after insert")

	err = col.set(2, Health{Value: 43})
	require.NoError(t, err)
	assert.Equal(t, 2, col.len(), "length should be 2 after second insert")

	err = col.set(1, Health{Value: 44}) // Update existing
	require.NoError(t, err)
	assert.Equal(t, 2, col.len(), "length should not change after update")

	col.remove(1)
	assert.Equal(t, 1, col.len(), "length should be 1 after remove")

	col.remove(999) // Remove non-existent
	assert.Equal(t, 1, col.len(), "length should not change after removing non-existent")

	col.clear()
	assert.Zero(t, col.len(), "length should be 0 after clear")
}

func BenchmarkColumn_Set(b *testing.B) {
	col := newColumn[Health]()

	for i := 0; b.Loop(); i++ {
		_ = col.set(1, Health{Value: i})
	}
}

func BenchmarkColumn_Get(b *testing.B) {
	col := newColumn[Health]()
	_ = col.set(1, Health{Value: 1})

	for b.Loop() {
		_, _ = col.get(1)
	}
}

func TestColumn_SerializeDeserialize_RoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		setupFn func() *column[Health]
	}{
		{
			name: "empty column",
			setupFn: func() *column[Health] {
				return newColumn[Health]()
			},
		},
		{
			name: "single entity",
			setupFn: func() *column[Health] {
				col := newColumn[Health]()
				_ = col.set(1, Health{Value: 42})
				return col
			},
		},
		{
			name: "multiple entities",
			setupFn: func() *column[Health] {
				col := newColumn[Health]()
				_ = col.set(1, Health{Value: 100})
				_ = col.set(5, Health{Value: 200})
				_ = col.set(10, Health{Value: 300})
				return col
			},
		},
		{
			name: "sparse column with gaps",
			setupFn: func() *column[Health] {
				col := newColumn[Health]()
				_ = col.set(1, Health{Value: 10})
				_ = col.set(100, Health{Value: 20})
				_ = col.set(1000, Health{Value: 30})
				return col
			},
		},
		{
			name: "column after removals",
			setupFn: func() *column[Health] {
				col := newColumn[Health]()
				_ = col.set(1, Health{Value: 10})
				_ = col.set(2, Health{Value: 20})
				_ = col.set(3, Health{Value: 30})
				col.remove(2) // Create gap
				return col
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			original := tc.setupFn()

			// Serialize
			serialized, err := original.serialize()
			require.NoError(t, err)

			// Deserialize into new column
			deserialized := newColumn[Health]()
			err = deserialized.deserialize(serialized)
			require.NoError(t, err)

			// Verify round-trip property: deserialize(serialize(x)) == x
			assert.Equal(t, original.compName, deserialized.compName)
			assert.Equal(t, original.sparse, deserialized.sparse)
			assert.Equal(t, original.denseEntityID, deserialized.denseEntityID)
			assert.Equal(t, original.denseComponent, deserialized.denseComponent)
		})
	}
}

func TestColumn_SerializeDeserialize_Determinism(t *testing.T) {
	t.Parallel()

	col := newColumn[Health]()
	_ = col.set(1, Health{Value: 100})
	_ = col.set(5, Health{Value: 200})
	_ = col.set(10, Health{Value: 300})

	// Serialize the same column multiple times
	serialized1, err := col.serialize()
	require.NoError(t, err)

	serialized2, err := col.serialize()
	require.NoError(t, err)

	// Verify determinism property: serialize(x) == serialize(x)
	assert.Equal(t, serialized1.GetComponentName(), serialized2.GetComponentName())
	assert.Equal(t, serialized1.GetSparse(), serialized2.GetSparse())
	assert.Equal(t, serialized1.GetDenseEntityIds(), serialized2.GetDenseEntityIds())
	assert.Equal(t, serialized1.GetDenseComponentData(), serialized2.GetDenseComponentData())
}

func TestColumn_SerializeDeserialize_ErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("component name mismatch", func(t *testing.T) {
		t.Parallel()

		col := newColumn[Health]()

		// Create protobuf with wrong component name
		invalidPb := &cardinalv1.Column{
			ComponentName:      "WrongComponent",
			Sparse:             []int64{},
			DenseEntityIds:     []uint32{},
			DenseComponentData: [][]byte{},
		}

		err := col.deserialize(invalidPb)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "component name mismatch")
	})

	t.Run("invalid JSON in component data", func(t *testing.T) {
		t.Parallel()

		col := newColumn[Health]()

		// Create protobuf with invalid JSON
		invalidPb := &cardinalv1.Column{
			ComponentName:      "Health",
			Sparse:             []int64{0},
			DenseEntityIds:     []uint32{1},
			DenseComponentData: [][]byte{[]byte("invalid json")},
		}

		err := col.deserialize(invalidPb)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to deserialize component")
	})

	t.Run("mismatched array lengths", func(t *testing.T) {
		t.Parallel()

		col := newColumn[Health]()

		// Create protobuf with mismatched dense array lengths
		validJSON, _ := json.Marshal(Health{Value: 42})
		invalidPb := &cardinalv1.Column{
			ComponentName:      "Health",
			Sparse:             []int64{0},
			DenseEntityIds:     []uint32{1},
			DenseComponentData: [][]byte{validJSON, validJSON}, // Extra component data
		}

		err := col.deserialize(invalidPb)
		// This should not panic and the column should handle gracefully
		// Note: Current implementation doesn't validate array length consistency
		// but it shouldn't crash
		if err == nil {
			// If no error, verify the column is in a valid state
			assert.Len(t, col.denseEntityID, 1)
			assert.Len(t, col.denseComponent, 2) // Takes what's provided
		}
	})

	t.Run("nil protobuf", func(t *testing.T) {
		t.Parallel()

		col := newColumn[Health]()

		// This should not panic
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("deserialize should not panic on nil input: %v", r)
				}
			}()

			err := col.deserialize(nil)
			// Should get an error when accessing nil protobuf fields
			require.Error(t, err)
		}()
	})
}
