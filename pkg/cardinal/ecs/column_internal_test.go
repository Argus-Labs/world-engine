package ecs

import (
	"math/rand"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/ecs/internal/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestColumn_New(t *testing.T) {
	t.Parallel()

	col := newColumn[testutils.Health]()

	assert.Equal(t, 0, col.len())
	assert.Equal(t, "Health", col.name())
	assert.Equal(t, 16, cap(col.components))
}

func TestColumn_Set(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		setup       func(*column[testutils.Position])
		row         int
		component   testutils.Position
		expectedLen int
		validate    func(*testing.T, *column[testutils.Position])
	}{
		{
			name: "update existing component",
			setup: func(col *column[testutils.Position]) {
				col.extend()
				col.set(0, testutils.Position{X: 1, Y: 2})
			},
			row:         0,
			component:   testutils.Position{X: 10, Y: 20},
			expectedLen: 1,
			validate: func(t *testing.T, col *column[testutils.Position]) {
				assert.Equal(t, testutils.Position{X: 10, Y: 20}, col.get(0))
			},
		},
		{
			name: "set component in pre-allocated slot",
			setup: func(col *column[testutils.Position]) {
				col.extend()
				col.set(0, testutils.Position{X: 1, Y: 2})
				col.extend() // Allocate second slot
			},
			row:         1,
			component:   testutils.Position{X: 3, Y: 4},
			expectedLen: 2,
			validate: func(t *testing.T, col *column[testutils.Position]) {
				assert.Equal(t, testutils.Position{X: 1, Y: 2}, col.get(0))
				assert.Equal(t, testutils.Position{X: 3, Y: 4}, col.get(1))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			col := newColumn[testutils.Position]()
			tc.setup(&col)

			col.set(tc.row, tc.component)

			assert.Equal(t, tc.expectedLen, col.len())
			tc.validate(t, &col)
		})
	}
}

func TestColumn_Get(t *testing.T) {
	t.Parallel()

	col := newColumn[testutils.Health]()
	health := testutils.Health{Value: 100}

	col.extend()
	col.set(0, health)
	retrieved := col.get(0)

	assert.Equal(t, health, retrieved)
}

func TestColumn_SetAbstract(t *testing.T) {
	t.Parallel()

	col := newColumn[testutils.Position]()
	pos := testutils.Position{X: 5, Y: 10}

	col.extend()
	col.setAbstract(0, pos)

	assert.Equal(t, 1, col.len())
	assert.Equal(t, pos, col.get(0))

	// Test getAbstract too.
	retrieved := col.getAbstract(0)
	assert.Equal(t, pos, retrieved)
}

func TestColumn_Remove(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		setup       func(*column[testutils.Position])
		removeIdx   int
		expectedLen int
		validate    func(*testing.T, *column[testutils.Position])
	}{
		{
			name: "remove middle element swaps with last",
			setup: func(col *column[testutils.Position]) {
				col.extend()
				col.set(0, testutils.Position{X: 1, Y: 1})
				col.extend()
				col.set(1, testutils.Position{X: 2, Y: 2})
				col.extend()
				col.set(2, testutils.Position{X: 3, Y: 3})
			},
			removeIdx:   1,
			expectedLen: 2,
			validate: func(t *testing.T, col *column[testutils.Position]) {
				assert.Equal(t, testutils.Position{X: 1, Y: 1}, col.get(0))
				assert.Equal(t, testutils.Position{X: 3, Y: 3}, col.get(1)) // Last element moved to index 1.
			},
		},
		{
			name: "remove last element",
			setup: func(col *column[testutils.Position]) {
				col.extend()
				col.set(0, testutils.Position{X: 1, Y: 1})
				col.extend()
				col.set(1, testutils.Position{X: 2, Y: 2})
			},
			removeIdx:   1,
			expectedLen: 1,
			validate: func(t *testing.T, col *column[testutils.Position]) {
				assert.Equal(t, testutils.Position{X: 1, Y: 1}, col.get(0))
			},
		},
		{
			name: "remove single element",
			setup: func(col *column[testutils.Position]) {
				col.extend()
				col.set(0, testutils.Position{X: 1, Y: 1})
			},
			removeIdx:   0,
			expectedLen: 0,
			validate:    func(t *testing.T, col *column[testutils.Position]) {},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			col := newColumn[testutils.Position]()
			tc.setup(&col)

			col.remove(tc.removeIdx)

			assert.Equal(t, tc.expectedLen, col.len())
			tc.validate(t, &col)
		})
	}
}

// Property-based test for column operations.
func TestColumn_Operations_Sequence(t *testing.T) {
	t.Parallel()

	const numIterations = 100
	const maxOps = 50

	for range numIterations {
		t.Run("iteration", func(t *testing.T) {
			t.Parallel()

			col := newColumn[testutils.Position]()
			reference := make([]testutils.Position, 0)

			numOps := rand.Intn(maxOps) + 1

			for range numOps {
				if len(reference) == 0 {
					// Can only add when empty.
					pos := testutils.Position{X: rand.Intn(1000), Y: rand.Intn(1000)}
					col.extend()
					col.set(0, pos)
					reference = append(reference, pos)
				} else {
					operation := rand.Intn(3) // 0=extend, 1=update, 2=remove

					switch operation {
					case 0: // Extend and add new component.
						pos := testutils.Position{X: rand.Intn(1000), Y: rand.Intn(1000)}
						col.extend()
						col.set(len(reference), pos)
						reference = append(reference, pos)

					case 1: // Update existing component.
						if len(reference) > 0 {
							idx := rand.Intn(len(reference))
							pos := testutils.Position{X: rand.Intn(1000), Y: rand.Intn(1000)}
							col.set(idx, pos)
							reference[idx] = pos
						}

					case 2: // Remove component.
						if len(reference) > 0 {
							idx := rand.Intn(len(reference))
							col.remove(idx)

							// Simulate swap-remove in reference.
							lastIdx := len(reference) - 1
							reference[idx] = reference[lastIdx]
							reference = reference[:lastIdx]
						}
					}
				}

				// Verify invariants.
				require.Equal(t, len(reference), col.len())

				// Verify all components match.
				for j := range reference {
					assert.Equal(t, reference[j], col.get(j))
				}
			}
		})
	}
}

func TestColumn_SerializeDeserialize_RoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		setupFn func() *column[testutils.Health]
	}{
		{
			name: "empty column",
			setupFn: func() *column[testutils.Health] {
				col := newColumn[testutils.Health]()
				return &col
			},
		},
		{
			name: "single entity",
			setupFn: func() *column[testutils.Health] {
				col := newColumn[testutils.Health]()
				col.extend()
				col.set(0, testutils.Health{Value: 42})
				return &col
			},
		},
		{
			name: "multiple entities",
			setupFn: func() *column[testutils.Health] {
				col := newColumn[testutils.Health]()
				col.extend()
				col.set(0, testutils.Health{Value: 100})
				col.extend()
				col.set(1, testutils.Health{Value: 200})
				col.extend()
				col.set(2, testutils.Health{Value: 300})
				return &col
			},
		},
		{
			name: "column after removals",
			setupFn: func() *column[testutils.Health] {
				col := newColumn[testutils.Health]()
				col.extend()
				col.set(0, testutils.Health{Value: 10})
				col.extend()
				col.set(1, testutils.Health{Value: 20})
				col.extend()
				col.set(2, testutils.Health{Value: 30})
				col.remove(1)
				return &col
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			original := tc.setupFn()

			// Serialize.
			serialized, err := original.serialize()
			require.NoError(t, err)

			// Deserialize into new column.
			deserialized := newColumn[testutils.Health]()
			err = deserialized.deserialize(serialized)
			require.NoError(t, err)

			// Verify round-trip property: deserialize(serialize(x)) == x.
			assert.Equal(t, original.compName, deserialized.compName)
			assert.Equal(t, original.components, deserialized.components)
		})
	}
}

func TestColumn_SerializeDeserialize_Determinism(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		setupFn func() abstractColumn
	}{
		{
			name: "simple health component",
			setupFn: func() abstractColumn {
				col := newColumn[testutils.Health]()
				col.extend()
				col.set(0, testutils.Health{Value: 100})
				col.extend()
				col.set(1, testutils.Health{Value: 200})
				col.extend()
				col.set(2, testutils.Health{Value: 300})
				return &col
			},
		},
		{
			name: "component with map fields",
			setupFn: func() abstractColumn {
				col := newColumn[testutils.MapComponent]()
				col.extend()
				col.set(0, testutils.MapComponent{
					Items: map[string]int{
						"sword":   1,
						"shield":  1,
						"potion":  5,
						"gold":    100,
						"key":     3,
						"torch":   2,
						"rope":    1,
						"compass": 1,
					},
				})
				return &col
			},
		},
		{
			name: "multiple entities with maps",
			setupFn: func() abstractColumn {
				col := newColumn[testutils.MapComponent]()
				col.extend()
				col.set(0, testutils.MapComponent{
					Items: map[string]int{
						"apple":  3,
						"banana": 7,
						"cherry": 2,
					},
				})
				col.extend()
				col.set(1, testutils.MapComponent{
					Items: map[string]int{
						"zinc":    10,
						"alpha":   5,
						"beta":    15,
						"gamma":   20,
						"delta":   25,
						"epsilon": 30,
					},
				})
				return &col
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			col := tc.setupFn()

			// Serialize the same column multiple times and verify determinism.
			const iterations = 10
			var prev *cardinalv1.Column

			for i := 0; i < iterations; i++ {
				current, err := col.serialize()
				require.NoError(t, err)

				if prev != nil {
					assert.Equal(t, prev.GetComponentName(), current.GetComponentName(),
						"iteration %d: component name differs", i)
					assert.Equal(t, prev.GetComponents(), current.GetComponents(),
						"iteration %d: components differ", i)
				}

				prev = current
			}
		})
	}
}

func TestColumn_SerializeDeserialize_ErrorHandling(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		setupColumn   func() abstractColumn
		setupProtobuf func() *cardinalv1.Column
		errorContains string
	}{
		{
			name: "component name mismatch",
			setupColumn: func() abstractColumn {
				col := newColumn[testutils.Health]()
				return &col
			},
			setupProtobuf: func() *cardinalv1.Column {
				return &cardinalv1.Column{
					ComponentName: "WrongComponent",
					Components:    [][]byte{},
				}
			},
			errorContains: "component name mismatch",
		},
		{
			name: "invalid JSON in component data",
			setupColumn: func() abstractColumn {
				col := newColumn[testutils.Health]()
				return &col
			},
			setupProtobuf: func() *cardinalv1.Column {
				return &cardinalv1.Column{
					ComponentName: "Health",
					Components:    [][]byte{[]byte("invalid json")},
				}
			},
			errorContains: "failed to deserialize component",
		},
		{
			name: "nil protobuf",
			setupColumn: func() abstractColumn {
				col := newColumn[testutils.Health]()
				return &col
			},
			setupProtobuf: func() *cardinalv1.Column {
				return nil
			},
			errorContains: "protobuf column is nil",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			col := tc.setupColumn()
			pb := tc.setupProtobuf()

			err := col.deserialize(pb)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errorContains)
		})
	}
}
