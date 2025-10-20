package ecs

import (
	"testing"

	. "github.com/argus-labs/world-engine/pkg/cardinal/ecs/internal/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/cardinal/v1"
	"github.com/kelindar/bitmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchetype_Matches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		archComps []Component
		testComps []Component
		want      bool
	}{
		{
			name:      "exact match",
			archComps: []Component{Health{}, Position{}},
			testComps: []Component{Health{}, Position{}},
			want:      true,
		},
		{
			name:      "different order same components",
			archComps: []Component{Position{}, Health{}},
			testComps: []Component{Health{}, Position{}},
			want:      true,
		},
		{
			name:      "subset does not match",
			archComps: []Component{Health{}, Position{}},
			testComps: []Component{Health{}},
			want:      false,
		},
		{
			name:      "superset does not match",
			archComps: []Component{Health{}},
			testComps: []Component{Health{}, Position{}},
			want:      false,
		},
		{
			name:      "completely different components",
			archComps: []Component{Health{}},
			testComps: []Component{Position{}},
			want:      false,
		},
		{
			name:      "empty components",
			archComps: []Component{},
			testComps: []Component{},
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create bitmaps and columns for the archetype
			archBitmap := bitmap.Bitmap{}
			columns := make([]any, len(tt.archComps))
			for i, comp := range tt.archComps {
				archBitmap.Set(uint32(i))
				columns[i] = newColumnForType(comp)
			}
			assert.Equal(t, len(columns), archBitmap.Count())

			arch := newArchetype(0, archBitmap, columns)

			// Create bitmap for test components
			testBitmap := bitmap.Bitmap{}
			for _, comp := range tt.testComps {
				// Set bit based on component type
				switch comp.(type) {
				case Health:
					testBitmap.Set(0)
				case Position:
					testBitmap.Set(1)
				case Velocity:
					testBitmap.Set(2)
				}
			}

			got := arch.matches(testBitmap)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestArchetype_CollectComponents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		components    []Component
		entity        EntityID
		exclude       []string
		expectedCount int
		expectedNames []string
		setupEntity   bool
		entityPresent bool
	}{
		{
			name:          "collect all components",
			components:    []Component{Health{Value: 100}, Position{X: 1, Y: 2}},
			entity:        1,
			exclude:       nil,
			expectedCount: 2,
			expectedNames: []string{"Health", "Position"},
			setupEntity:   true,
			entityPresent: true,
		},
		{
			name:          "exclude one component",
			components:    []Component{Health{Value: 100}, Position{X: 1, Y: 2}},
			entity:        1,
			exclude:       []string{"Health"},
			expectedCount: 1,
			expectedNames: []string{"Position"},
			setupEntity:   true,
			entityPresent: true,
		},
		{
			name:          "non-existent entity",
			components:    []Component{Health{Value: 100}},
			entity:        999,
			exclude:       nil,
			expectedCount: 0,
			expectedNames: nil,
			setupEntity:   false,
			entityPresent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup archetype
			compBitmap := bitmap.Bitmap{}
			columns := make([]any, len(tt.components))
			for i, comp := range tt.components {
				compBitmap.Set(uint32(i))
				columns[i] = newColumnForType(comp)
			}
			assert.Equal(t, len(columns), compBitmap.Count())

			arch := newArchetype(0, compBitmap, columns)

			// Setup entity if required
			if tt.setupEntity {
				err := arch.newEntity(tt.entity, tt.components)
				require.NoError(t, err)
			}

			// Collect components
			got := arch.collectComponents(tt.entity, tt.exclude...)

			assert.Len(t, got, tt.expectedCount)
			if tt.expectedNames != nil {
				var gotNames []string
				for _, comp := range got {
					gotNames = append(gotNames, comp.Name())
				}
				assert.ElementsMatch(t, tt.expectedNames, gotNames)
			}
		})
	}
}

// Helper function to create a column for a component type.
func newColumnForType(comp Component) any {
	switch comp.(type) {
	case Health:
		return newColumn[Health]()
	case Position:
		return newColumn[Position]()
	case Velocity:
		return newColumn[Velocity]()
	default:
		panic("unsupported component type")
	}
}

func TestArchetype_HasComponent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		components    []Component
		testComponent componentID
		want          bool
	}{
		{
			name:          "has component",
			components:    []Component{Health{}},
			testComponent: 0,
			want:          true,
		},
		{
			name:          "does not have component",
			components:    []Component{Health{}},
			testComponent: 1,
			want:          false,
		},
		{
			name:          "empty archetype",
			components:    []Component{},
			testComponent: 0,
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			compBitmap := bitmap.Bitmap{}
			columns := make([]any, len(tt.components))
			for i, comp := range tt.components {
				compBitmap.Set(uint32(i))
				columns[i] = newColumnForType(comp)
			}
			assert.Equal(t, len(columns), compBitmap.Count())

			arch := newArchetype(0, compBitmap, columns)

			got := arch.hasComponent(tt.testComponent)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestArchetype_ComponentTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		components []Component
		wantBits   []uint32
	}{
		{
			name:       "multiple components",
			components: []Component{Health{}, Position{}},
			wantBits:   []uint32{0, 1},
		},
		{
			name:       "single component",
			components: []Component{Health{}},
			wantBits:   []uint32{0},
		},
		{
			name:       "no components",
			components: []Component{},
			wantBits:   []uint32{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			compBitmap := bitmap.Bitmap{}
			columns := make([]any, len(tt.components))
			for i, comp := range tt.components {
				compBitmap.Set(uint32(i))
				columns[i] = newColumnForType(comp)
			}
			assert.Equal(t, len(columns), compBitmap.Count())

			arch := newArchetype(0, compBitmap, columns)

			got := arch.componentBitmap()
			for _, bit := range tt.wantBits {
				assert.True(t, got.Contains(bit))
			}
			assert.Equal(t, len(tt.wantBits), got.Count())
		})
	}
}

func TestArchetype_HasEntity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		entityToCreate EntityID
		entityToCheck  EntityID
		want           bool
	}{
		{
			name:           "has entity",
			entityToCreate: 1,
			entityToCheck:  1,
			want:           true,
		},
		{
			name:           "does not have entity",
			entityToCreate: 1,
			entityToCheck:  2,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			arch := newArchetype(0, bitmap.Bitmap{}, []any{})
			err := arch.newEntity(tt.entityToCreate, []Component{})
			require.NoError(t, err)
			got := arch.hasEntity(tt.entityToCheck)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestArchetype_NewEntity_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entity     EntityID
		components []Component
		wantErr    bool
	}{
		{
			name:       "mismatched component count",
			entity:     1,
			components: []Component{Health{}, Position{}},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			compBitmap := bitmap.Bitmap{}
			compBitmap.Set(0)
			arch := newArchetype(0, compBitmap, []any{newColumn[Health]()})

			err := arch.newEntity(tt.entity, tt.components)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestArchetype_UpdateEntity_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupEntity func(a *archetype) EntityID
		components  []Component
		wantErr     bool
	}{
		{
			name: "mismatched component count",
			setupEntity: func(a *archetype) EntityID {
				entityID := EntityID(1)
				err := a.newEntity(entityID, []Component{Health{}})
				require.NoError(t, err)
				return entityID
			},
			components: []Component{Health{}, Position{}},
			wantErr:    true,
		},
		{
			name: "non-existing entity",
			setupEntity: func(a *archetype) EntityID {
				return EntityID(1<<30 + 1)
			},
			components: []Component{Health{}},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			compBitmap := bitmap.Bitmap{}
			compBitmap.Set(0)
			arch := newArchetype(0, compBitmap, []any{newColumn[Health]()})

			entity := tt.setupEntity(&arch)
			err := arch.updateEntity(entity, tt.components)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestArchetype_SerializeDeserialize_RoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		setupFn func() (*archetype, *componentManager)
	}{
		{
			name: "empty archetype",
			setupFn: func() (*archetype, *componentManager) {
				cm := newComponentManager()
				arch := newArchetype(1, bitmap.Bitmap{}, []any{})
				return &arch, &cm
			},
		},
		{
			name: "single component archetype",
			setupFn: func() (*archetype, *componentManager) {
				cm := newComponentManager()
				err := cm.register("Health", newColumnConstructor[Health]())
				require.NoError(t, err)

				compBitmap := bitmap.Bitmap{}
				compBitmap.Set(0)
				arch := newArchetype(1, compBitmap, []any{newColumn[Health]()})
				return &arch, &cm
			},
		},
		{
			name: "multiple component archetype",
			setupFn: func() (*archetype, *componentManager) {
				cm := newComponentManager()
				err := cm.register("Health", newColumnConstructor[Health]())
				require.NoError(t, err)
				err = cm.register("Position", newColumnConstructor[Position]())
				require.NoError(t, err)

				compBitmap := bitmap.Bitmap{}
				compBitmap.Set(0)
				compBitmap.Set(1)
				columns := []any{newColumn[Health](), newColumn[Position]()}
				arch := newArchetype(2, compBitmap, columns)
				return &arch, &cm
			},
		},
		{
			name: "archetype with entities",
			setupFn: func() (*archetype, *componentManager) {
				cm := newComponentManager()
				err := cm.register("Health", newColumnConstructor[Health]())
				require.NoError(t, err)
				err = cm.register("Position", newColumnConstructor[Position]())
				require.NoError(t, err)

				compBitmap := bitmap.Bitmap{}
				compBitmap.Set(0)
				compBitmap.Set(1)
				columns := []any{newColumn[Health](), newColumn[Position]()}
				arch := newArchetype(3, compBitmap, columns)

				// Add some entities
				err = arch.newEntity(1, []Component{Health{Value: 100}, Position{X: 10, Y: 20}})
				require.NoError(t, err)
				err = arch.newEntity(5, []Component{Health{Value: 200}, Position{X: 30, Y: 40}})
				require.NoError(t, err)

				return &arch, &cm
			},
		},
		{
			name: "archetype after entity removal",
			setupFn: func() (*archetype, *componentManager) {
				cm := newComponentManager()
				err := cm.register("Health", newColumnConstructor[Health]())
				require.NoError(t, err)

				compBitmap := bitmap.Bitmap{}
				compBitmap.Set(0)
				arch := newArchetype(4, compBitmap, []any{newColumn[Health]()})

				// Add and remove entity
				err = arch.newEntity(1, []Component{Health{Value: 100}})
				require.NoError(t, err)
				err = arch.newEntity(2, []Component{Health{Value: 200}})
				require.NoError(t, err)
				arch.removeEntity(1) // Remove first entity

				return &arch, &cm
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			original, cm := tc.setupFn()

			// Serialize
			serialized, err := original.serialize()
			require.NoError(t, err)

			// Deserialize into new archetype
			deserialized := &archetype{}
			err = deserialized.deserialize(serialized, cm)
			require.NoError(t, err)

			// Verify round-trip property: deserialize(serialize(x)) == x
			assert.Equal(t, original.id, deserialized.id)
			assert.Equal(t, original.componentTypeCount, deserialized.componentTypeCount)

			// Compare bitmaps by converting to bytes for deterministic comparison
			assert.Equal(t, original.entities.ToBytes(), deserialized.entities.ToBytes())
			assert.Equal(t, original.components.ToBytes(), deserialized.components.ToBytes())

			// Verify column count matches
			assert.Len(t, deserialized.columns, len(original.columns))

			// Verify each column is properly deserialized by checking component names
			for i, origCol := range original.columns {
				origAbstract := toAbstractColumn(origCol)
				deserializedAbstract := toAbstractColumn(deserialized.columns[i])
				assert.Equal(t, origAbstract.componentName(), deserializedAbstract.componentName())
			}
		})
	}
}

func TestArchetype_SerializeDeserialize_Determinism(t *testing.T) {
	t.Parallel()

	// Setup archetype with multiple components and entities
	cm := newComponentManager()
	err := cm.register("Health", newColumnConstructor[Health]())
	require.NoError(t, err)
	err = cm.register("Position", newColumnConstructor[Position]())
	require.NoError(t, err)

	compBitmap := bitmap.Bitmap{}
	compBitmap.Set(0)
	compBitmap.Set(1)
	columns := []any{newColumn[Health](), newColumn[Position]()}
	arch := newArchetype(5, compBitmap, columns)

	// Add entities
	err = arch.newEntity(1, []Component{Health{Value: 100}, Position{X: 10, Y: 20}})
	require.NoError(t, err)
	err = arch.newEntity(2, []Component{Health{Value: 200}, Position{X: 30, Y: 40}})
	require.NoError(t, err)

	// Serialize the same archetype multiple times
	serialized1, err := arch.serialize()
	require.NoError(t, err)

	serialized2, err := arch.serialize()
	require.NoError(t, err)

	// Verify determinism property: serialize(x) == serialize(x)
	assert.Equal(t, serialized1.GetId(), serialized2.GetId())
	assert.Equal(t, serialized1.GetEntitiesBitmap(), serialized2.GetEntitiesBitmap())
	assert.Equal(t, serialized1.GetComponentsBitmap(), serialized2.GetComponentsBitmap())
	assert.Len(t, serialized2.GetColumns(), len(serialized1.GetColumns()))

	// Compare each column
	for i, col1 := range serialized1.GetColumns() {
		col2 := serialized2.GetColumns()[i]
		assert.Equal(t, col1.GetComponentName(), col2.GetComponentName())
		assert.Equal(t, col1.GetSparse(), col2.GetSparse())
		assert.Equal(t, col1.GetDenseEntityIds(), col2.GetDenseEntityIds())
		assert.Equal(t, col1.GetDenseComponentData(), col2.GetDenseComponentData())
	}
}

func TestArchetype_SerializeDeserialize_ErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("component not registered in manager", func(t *testing.T) {
		t.Parallel()

		cm := newComponentManager()
		// Don't register Health component

		// Create protobuf with Health component
		invalidPb := &cardinalv1.Archetype{
			Id:               1,
			EntitiesBitmap:   []byte{},
			ComponentsBitmap: []byte{},
			Columns: []*cardinalv1.Column{
				{
					ComponentName:      "Health",
					Sparse:             []int64{},
					DenseEntityIds:     []uint32{},
					DenseComponentData: [][]byte{},
				},
			},
		}

		arch := &archetype{}
		err := arch.deserialize(invalidPb, &cm)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get column factory for component Health")
	})

	t.Run("column deserialization failure", func(t *testing.T) {
		t.Parallel()

		cm := newComponentManager()
		err := cm.register("Health", newColumnConstructor[Health]())
		require.NoError(t, err)

		// Create protobuf with invalid column data
		invalidPb := &cardinalv1.Archetype{
			Id:               1,
			EntitiesBitmap:   []byte{},
			ComponentsBitmap: []byte{},
			Columns: []*cardinalv1.Column{
				{
					ComponentName:      "Health",
					Sparse:             []int64{0},
					DenseEntityIds:     []uint32{1},
					DenseComponentData: [][]byte{[]byte("invalid json")},
				},
			},
		}

		arch := &archetype{}
		err = arch.deserialize(invalidPb, &cm)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to deserialize column")
	})

	t.Run("nil protobuf", func(t *testing.T) {
		t.Parallel()

		cm := newComponentManager()
		arch := &archetype{}

		// This should not panic - protobuf Get methods handle nil gracefully
		// but should result in empty archetype since nil protobuf has no columns
		err := arch.deserialize(nil, &cm)
		require.NoError(t, err) // No error for empty protobuf
		assert.Equal(t, uint64(0), arch.id)
		assert.Equal(t, 0, arch.componentTypeCount)
		assert.Empty(t, arch.columns)
	})

	t.Run("nil component manager with columns", func(t *testing.T) {
		t.Parallel()

		pb := &cardinalv1.Archetype{
			Id:               1,
			EntitiesBitmap:   []byte{},
			ComponentsBitmap: []byte{},
			Columns: []*cardinalv1.Column{
				{
					ComponentName:      "Health",
					Sparse:             []int64{},
					DenseEntityIds:     []uint32{},
					DenseComponentData: [][]byte{},
				},
			},
		}

		arch := &archetype{}

		// This should panic when trying to access nil component manager
		assert.Panics(t, func() {
			_ = arch.deserialize(pb, nil)
		})
	})
}
