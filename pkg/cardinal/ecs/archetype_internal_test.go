package ecs

import (
	"math/rand"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/ecs/internal/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/kelindar/bitmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchetype_New(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		id             archetypeID
		setupBitmap    func() bitmap.Bitmap
		setupColumns   func() []abstractColumn
		expectedID     archetypeID
		expectedColLen int
	}{
		{
			name: "empty archetype",
			id:   0,
			setupBitmap: func() bitmap.Bitmap {
				return bitmap.Bitmap{}
			},
			setupColumns: func() []abstractColumn {
				return []abstractColumn{}
			},
			expectedID:     0,
			expectedColLen: 0,
		},
		{
			name: "archetype with multiple components",
			id:   42,
			setupBitmap: func() bitmap.Bitmap {
				components := bitmap.Bitmap{}
				components.Set(0) // Health component.
				components.Set(1) // Position component.
				return components
			},
			setupColumns: func() []abstractColumn {
				healthCol := newColumn[testutils.Health]()
				posCol := newColumn[testutils.Position]()
				return []abstractColumn{&healthCol, &posCol}
			},
			expectedID:     42,
			expectedColLen: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			components := tc.setupBitmap()
			columns := tc.setupColumns()
			arch := newArchetype(tc.id, components, columns)

			assert.Equal(t, tc.expectedID, arch.id)
			assert.Equal(t, components, arch.components)
			assert.Empty(t, arch.entities)
			assert.Len(t, arch.columns, tc.expectedColLen)
		})
	}
}

func TestArchetype_NewEntity(t *testing.T) {
	t.Parallel()

	t.Run("allocates rows and returns correct indices", func(t *testing.T) {
		t.Parallel()
		components := bitmap.Bitmap{}
		components.Set(0)
		healthCol := newColumn[testutils.Health]()
		columns := []abstractColumn{&healthCol}
		arch := newArchetype(0, components, columns)

		// Verify initial state
		assert.Equal(t, 0, healthCol.len())

		// Add entities
		arch.newEntity(EntityID(100))
		arch.newEntity(EntityID(200))
		arch.newEntity(EntityID(300))
		assert.Len(t, arch.entities, 3)
		assert.Equal(t, []EntityID{EntityID(100), EntityID(200), EntityID(300)}, arch.entities)

		// Verify that columns were extended to match entity count
		assert.Equal(t, 3, healthCol.len())
		// Verify columns have zero values allocated
		assert.Equal(t, testutils.Health{}, healthCol.get(0))
		assert.Equal(t, testutils.Health{}, healthCol.get(1))
		assert.Equal(t, testutils.Health{}, healthCol.get(2))
	})

	t.Run("allocates space in all columns for multi-component archetype", func(t *testing.T) {
		t.Parallel()
		components := bitmap.Bitmap{}
		components.Set(0) // Health
		components.Set(1) // Position
		healthCol := newColumn[testutils.Health]()
		posCol := newColumn[testutils.Position]()
		columns := []abstractColumn{&healthCol, &posCol}
		arch := newArchetype(1, components, columns)

		// Verify initial state
		assert.Equal(t, 0, healthCol.len())
		assert.Equal(t, 0, posCol.len())

		// Add an entity
		arch.newEntity(EntityID(100))
		assert.Len(t, arch.entities, 1)
		assert.Equal(t, EntityID(100), arch.entities[0])

		// Verify both columns were extended
		assert.Equal(t, 1, healthCol.len())
		assert.Equal(t, 1, posCol.len())
		// Verify columns have zero values allocated
		assert.Equal(t, testutils.Health{}, healthCol.get(0))
		assert.Equal(t, testutils.Position{}, posCol.get(0))
	})
}

func TestArchetype_RemoveEntity(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                string
		setup               func(*archetype, *column[testutils.Health])
		removeEntityID      EntityID
		expectedEntityCount int
		validate            func(*testing.T, *archetype, *column[testutils.Health])
	}{
		{
			name: "swap and pop middle entity",
			setup: func(arch *archetype, healthCol *column[testutils.Health]) {
				arch.newEntity(EntityID(100))
				healthCol.set(0, testutils.Health{Value: 10})
				arch.newEntity(EntityID(200))
				healthCol.set(1, testutils.Health{Value: 20})
				arch.newEntity(EntityID(300))
				healthCol.set(2, testutils.Health{Value: 30})
			},
			removeEntityID:      EntityID(200),
			expectedEntityCount: 2,
			validate: func(t *testing.T, arch *archetype, healthCol *column[testutils.Health]) {
				assert.Equal(t, EntityID(100), arch.entities[0])
				assert.Equal(t, EntityID(300), arch.entities[1]) // Last entity moved to index 1.
				assert.Equal(t, testutils.Health{Value: 10}, healthCol.get(0))
				assert.Equal(t, testutils.Health{Value: 30}, healthCol.get(1)) // Last component moved to index 1.
				assert.Equal(t, 2, healthCol.len())
			},
		},
		{
			name: "remove only entity",
			setup: func(arch *archetype, healthCol *column[testutils.Health]) {
				arch.newEntity(EntityID(100))
				healthCol.set(0, testutils.Health{Value: 50})
			},
			removeEntityID:      EntityID(100),
			expectedEntityCount: 0,
			validate: func(t *testing.T, arch *archetype, healthCol *column[testutils.Health]) {
				assert.Empty(t, arch.entities)
				assert.Equal(t, 0, healthCol.len())
			},
		},
		{
			name: "remove last entity",
			setup: func(arch *archetype, healthCol *column[testutils.Health]) {
				arch.newEntity(EntityID(100))
				healthCol.set(0, testutils.Health{Value: 10})
				arch.newEntity(EntityID(200))
				healthCol.set(1, testutils.Health{Value: 20})
			},
			removeEntityID:      EntityID(200),
			expectedEntityCount: 1,
			validate: func(t *testing.T, arch *archetype, healthCol *column[testutils.Health]) {
				assert.Equal(t, EntityID(100), arch.entities[0])
				assert.Equal(t, testutils.Health{Value: 10}, healthCol.get(0))
				assert.Equal(t, 1, healthCol.len())
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			components := bitmap.Bitmap{}
			components.Set(0)
			healthCol := newColumn[testutils.Health]()
			columns := []abstractColumn{&healthCol}
			arch := newArchetype(0, components, columns)

			tc.setup(&arch, &healthCol)

			arch.removeEntity(tc.removeEntityID)

			assert.Len(t, arch.entities, tc.expectedEntityCount)
			tc.validate(t, &arch, &healthCol)
		})
	}
}

func TestArchetype_MoveEntity(t *testing.T) {
	t.Parallel()

	t.Run("move to archetype with additional component type", func(t *testing.T) {
		t.Parallel()

		// Source archetype: Health only.
		srcComponents := bitmap.Bitmap{}
		srcComponents.Set(0) // Health.
		srcHealthCol := newColumn[testutils.Health]()
		srcColumns := []abstractColumn{&srcHealthCol}
		srcArch := newArchetype(0, srcComponents, srcColumns)

		// Destination archetype: Health + Position (additional component).
		dstComponents := bitmap.Bitmap{}
		dstComponents.Set(0) // Health (shared).
		dstComponents.Set(1) // Position (additional).
		dstHealthCol := newColumn[testutils.Health]()
		dstPosCol := newColumn[testutils.Position]()
		dstColumns := []abstractColumn{&dstHealthCol, &dstPosCol}
		dstArch := newArchetype(1, dstComponents, dstColumns)

		// Setup entity.
		srcArch.newEntity(EntityID(100))
		srcHealthCol.set(0, testutils.Health{Value: 50})

		// Move entity.
		srcArch.moveEntity(&dstArch, EntityID(100))

		// Verify entity ID is the same in destination.
		assert.Len(t, dstArch.entities, 1)
		assert.Equal(t, EntityID(100), dstArch.entities[0])

		// Verify shared component (Health) was transferred correctly.
		assert.Equal(t, testutils.Health{Value: 50}, dstHealthCol.get(0))

		// Verify source is completely empty.
		assert.Empty(t, srcArch.entities)
		assert.Equal(t, 0, srcHealthCol.len())
	})

	t.Run("move to archetype with one less component type", func(t *testing.T) {
		t.Parallel()

		// Source archetype: Health + Position.
		srcComponents := bitmap.Bitmap{}
		srcComponents.Set(0) // Health.
		srcComponents.Set(1) // Position.
		srcHealthCol := newColumn[testutils.Health]()
		srcPosCol := newColumn[testutils.Position]()
		srcColumns := []abstractColumn{&srcHealthCol, &srcPosCol}
		srcArch := newArchetype(0, srcComponents, srcColumns)

		// Destination archetype: Health only (one less component).
		dstComponents := bitmap.Bitmap{}
		dstComponents.Set(0) // Health (shared).
		dstHealthCol := newColumn[testutils.Health]()
		dstColumns := []abstractColumn{&dstHealthCol}
		dstArch := newArchetype(1, dstComponents, dstColumns)

		// Setup entity.
		srcArch.newEntity(EntityID(200))
		srcHealthCol.set(0, testutils.Health{Value: 75})
		srcPosCol.set(0, testutils.Position{X: 15, Y: 25})

		// Move entity.
		srcArch.moveEntity(&dstArch, EntityID(200))

		// Verify entity ID is the same in destination.
		assert.Len(t, dstArch.entities, 1)
		assert.Equal(t, EntityID(200), dstArch.entities[0])

		// Verify shared component (Health) was transferred correctly.
		assert.Equal(t, testutils.Health{Value: 75}, dstHealthCol.get(0))

		// Verify source is completely empty (both components and entity removed).
		assert.Empty(t, srcArch.entities)
		assert.Equal(t, 0, srcHealthCol.len())
		assert.Equal(t, 0, srcPosCol.len())
	})
}

// Property-based test for archetype entity lifecycle operations.
func TestArchetype_EntityLifecycle_PropertyBased(t *testing.T) {
	t.Parallel()

	const numIterations = 20
	const maxOps = 15

	for range numIterations {
		t.Run("iteration", func(t *testing.T) {
			t.Parallel()

			// Create two simple archetypes with different component counts.
			// Archetype 1: Health only (1 component).
			arch1Components := bitmap.Bitmap{}
			arch1Components.Set(0)
			arch1HealthCol := newColumn[testutils.Health]()
			arch1 := newArchetype(0, arch1Components, []abstractColumn{&arch1HealthCol})

			// Archetype 2: Health + Position (2 components).
			arch2Components := bitmap.Bitmap{}
			arch2Components.Set(0)
			arch2Components.Set(1)
			arch2HealthCol := newColumn[testutils.Health]()
			arch2PosCol := newColumn[testutils.Position]()
			arch2 := newArchetype(1, arch2Components, []abstractColumn{&arch2HealthCol, &arch2PosCol})

			archetypes := []*archetype{&arch1, &arch2}
			entityCounter := EntityID(1000)

			numOps := rand.Intn(maxOps) + 1

			for range numOps {
				operation := rand.Intn(2) // 0=add, 1=remove (skip moves for simplicity).

				switch operation {
				case 0: // Add entity to random archetype.
					archIdx := rand.Intn(len(archetypes))
					arch := archetypes[archIdx]

					entityCounter++
					eid := entityCounter
					arch.newEntity(eid)

					// Add component data based on archetype.
					// Get the row index for the entity we just added
					row, exists := arch.rows.get(eid)
					require.True(t, exists, "entity should exist in archetype")

					if archIdx == 0 { // Health only.
						arch1HealthCol.set(row, testutils.Health{Value: rand.Intn(100)})
					} else { // Health + Position.
						arch2HealthCol.set(row, testutils.Health{Value: rand.Intn(100)})
						arch2PosCol.set(row, testutils.Position{X: rand.Intn(1000), Y: rand.Intn(1000)})
					}

				case 1: // Remove entity from random non-empty archetype.
					var candidateArchs []int
					for i, arch := range archetypes {
						if len(arch.entities) > 0 {
							candidateArchs = append(candidateArchs, i)
						}
					}

					if len(candidateArchs) > 0 {
						archIdx := candidateArchs[rand.Intn(len(candidateArchs))]
						arch := archetypes[archIdx]
						entityIdx := rand.Intn(len(arch.entities))
						entityID := arch.entities[entityIdx]

						arch.removeEntity(entityID)
					}
				}

				// Verify invariants after each operation.
				for i, arch := range archetypes {
					// Entities and columns should have matching lengths.
					if i == 0 { // Health only.
						require.Equal(t, len(arch.entities), arch1HealthCol.len(), "archetype %d entity/column length mismatch", i)
					} else { // Health + Position.
						require.Equal(t, len(arch.entities), arch2HealthCol.len(), "archetype %d health column length mismatch", i)
						require.Equal(t, len(arch.entities), arch2PosCol.len(), "archetype %d position column length mismatch", i)
					}
				}
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
				arch := newArchetype(1, bitmap.Bitmap{}, []abstractColumn{})
				return &arch, &cm
			},
		},
		{
			name: "single component archetype",
			setupFn: func() (*archetype, *componentManager) {
				cm := newComponentManager()
				healthCol := newColumn[testutils.Health]()
				_, err := cm.register("Health", newColumnFactory[testutils.Health]())
				require.NoError(t, err)

				compBitmap := bitmap.Bitmap{}
				compBitmap.Set(0)
				arch := newArchetype(1, compBitmap, []abstractColumn{&healthCol})
				return &arch, &cm
			},
		},
		{
			name: "multiple component archetype",
			setupFn: func() (*archetype, *componentManager) {
				cm := newComponentManager()
				healthCol := newColumn[testutils.Health]()
				posCol := newColumn[testutils.Position]()
				_, err := cm.register("Health", newColumnFactory[testutils.Health]())
				require.NoError(t, err)
				_, err = cm.register("Position", newColumnFactory[testutils.Position]())
				require.NoError(t, err)

				compBitmap := bitmap.Bitmap{}
				compBitmap.Set(0)
				compBitmap.Set(1)
				columns := []abstractColumn{&healthCol, &posCol}
				arch := newArchetype(2, compBitmap, columns)
				return &arch, &cm
			},
		},
		{
			name: "archetype with entities",
			setupFn: func() (*archetype, *componentManager) {
				cm := newComponentManager()
				healthCol := newColumn[testutils.Health]()
				posCol := newColumn[testutils.Position]()
				_, err := cm.register("Health", newColumnFactory[testutils.Health]())
				require.NoError(t, err)
				_, err = cm.register("Position", newColumnFactory[testutils.Position]())
				require.NoError(t, err)

				compBitmap := bitmap.Bitmap{}
				compBitmap.Set(0)
				compBitmap.Set(1)
				columns := []abstractColumn{&healthCol, &posCol}
				arch := newArchetype(3, compBitmap, columns)

				// Add some entities.
				arch.newEntity(EntityID(1))
				healthCol.set(0, testutils.Health{Value: 100})
				posCol.set(0, testutils.Position{X: 10, Y: 20})
				arch.newEntity(EntityID(5))
				healthCol.set(1, testutils.Health{Value: 200})
				posCol.set(1, testutils.Position{X: 30, Y: 40})

				return &arch, &cm
			},
		},
		{
			name: "archetype after entity removal",
			setupFn: func() (*archetype, *componentManager) {
				cm := newComponentManager()
				healthCol := newColumn[testutils.Health]()
				_, err := cm.register("Health", newColumnFactory[testutils.Health]())
				require.NoError(t, err)

				compBitmap := bitmap.Bitmap{}
				compBitmap.Set(0)
				arch := newArchetype(4, compBitmap, []abstractColumn{&healthCol})

				// Add and remove entity.
				arch.newEntity(EntityID(1))
				healthCol.set(0, testutils.Health{Value: 100})
				arch.newEntity(EntityID(2))
				healthCol.set(1, testutils.Health{Value: 200})
				arch.removeEntity(EntityID(1))

				return &arch, &cm
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			original, cm := tc.setupFn()

			// Serialize.
			serialized, err := original.serialize()
			require.NoError(t, err)

			// Deserialize into new archetype.
			deserialized := &archetype{}
			err = deserialized.deserialize(serialized, cm)
			require.NoError(t, err)

			// Verify round-trip property: deserialize(serialize(x)) == x.
			assert.Equal(t, original.id, deserialized.id)
			assert.Equal(t, original.compCount, deserialized.compCount)

			// Compare bitmaps by converting to bytes for deterministic comparison.
			assert.Equal(t, original.components.ToBytes(), deserialized.components.ToBytes())

			// Verify entity lists match.
			assert.Equal(t, original.entities, deserialized.entities)

			// Verify column count matches.
			assert.Len(t, deserialized.columns, len(original.columns))

			// Verify each column is properly deserialized by checking component names.
			for i, origCol := range original.columns {
				deserializedCol := deserialized.columns[i]
				assert.Equal(t, origCol.name(), deserializedCol.name())
			}
		})
	}
}

func TestArchetype_SerializeDeserialize_Determinism(t *testing.T) {
	t.Parallel()

	// Setup archetype with multiple components and entities.
	cm := newComponentManager()
	healthCol := newColumn[testutils.Health]()
	mapCol := newColumn[testutils.MapComponent]()
	_, err := cm.register("Health", newColumnFactory[testutils.Health]())
	require.NoError(t, err)
	_, err = cm.register("MapComponent", newColumnFactory[testutils.MapComponent]())
	require.NoError(t, err)

	compBitmap := bitmap.Bitmap{}
	compBitmap.Set(0)
	compBitmap.Set(1)
	columns := []abstractColumn{&healthCol, &mapCol}
	arch := newArchetype(5, compBitmap, columns)

	// Add entities with map components.
	arch.newEntity(EntityID(1))
	healthCol.set(0, testutils.Health{Value: 100})
	mapCol.set(0, testutils.MapComponent{
		Items: map[string]int{
			"sword":  1,
			"shield": 1,
			"potion": 5,
			"gold":   100,
			"key":    3,
		},
	})
	arch.newEntity(EntityID(2))
	healthCol.set(1, testutils.Health{Value: 200})
	mapCol.set(1, testutils.MapComponent{
		Items: map[string]int{
			"armor": 1,
			"bow":   1,
			"arrow": 50,
		},
	})

	// Serialize the same archetype multiple times and verify determinism.
	const iterations = 10
	var prev *cardinalv1.Archetype

	for i := range iterations {
		current, err := arch.serialize()
		require.NoError(t, err)

		if prev != nil {
			assert.Equal(t, prev.GetId(), current.GetId(),
				"iteration %d: archetype ID differs", i)
			assert.Equal(t, prev.GetComponentsBitmap(), current.GetComponentsBitmap(),
				"iteration %d: components bitmap differs", i)
			assert.Equal(t, prev.GetEntities(), current.GetEntities(),
				"iteration %d: entities differ", i)
			assert.Len(t, current.GetColumns(), len(prev.GetColumns()),
				"iteration %d: column count differs", i)

			// Compare each column.
			for j, prevCol := range prev.GetColumns() {
				currentCol := current.GetColumns()[j]
				assert.Equal(t, prevCol.GetComponentName(), currentCol.GetComponentName(),
					"iteration %d, column %d: component name differs", i, j)
				assert.Equal(t, prevCol.GetComponents(), currentCol.GetComponents(),
					"iteration %d, column %d: components differ", i, j)
			}
		}

		prev = current
	}
}

func TestArchetype_SerializeDeserialize_ErrorHandling(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		setupManager  func() *componentManager
		setupProtobuf func() *cardinalv1.Archetype
		errorContains string
		expectPanic   bool
		nilManager    bool
	}{
		{
			name: "component not registered in manager",
			setupManager: func() *componentManager {
				cm := newComponentManager()
				return &cm
			},
			setupProtobuf: func() *cardinalv1.Archetype {
				return &cardinalv1.Archetype{
					Id:               1,
					ComponentsBitmap: []byte{},
					Entities:         []uint32{},
					Columns: []*cardinalv1.Column{
						{
							ComponentName: "Health",
							Components:    [][]byte{},
						},
					},
				}
			},
			errorContains: "failed to get component id",
		},
		{
			name: "column deserialization failure",
			setupManager: func() *componentManager {
				cm := newComponentManager()
				_, _ = cm.register("Health", newColumnFactory[testutils.Health]())
				return &cm
			},
			setupProtobuf: func() *cardinalv1.Archetype {
				return &cardinalv1.Archetype{
					Id:               1,
					ComponentsBitmap: []byte{},
					Entities:         []uint32{},
					Columns: []*cardinalv1.Column{
						{
							ComponentName: "Health",
							Components:    [][]byte{[]byte("invalid json")},
						},
					},
				}
			},
			errorContains: "failed to deserialize column",
		},
		{
			name: "nil protobuf",
			setupManager: func() *componentManager {
				cm := newComponentManager()
				return &cm
			},
			setupProtobuf: func() *cardinalv1.Archetype {
				return nil
			},
			errorContains: "protobuf archetype is nil",
		},
		{
			name: "nil component manager",
			setupManager: func() *componentManager {
				return nil
			},
			setupProtobuf: func() *cardinalv1.Archetype {
				healthCol := newColumn[testutils.Health]()
				compBitmap := bitmap.Bitmap{}
				compBitmap.Set(0)
				arch := newArchetype(1, compBitmap, []abstractColumn{&healthCol})
				pb, _ := arch.serialize()
				return pb
			},
			expectPanic: true,
			nilManager:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cm := tc.setupManager()
			pb := tc.setupProtobuf()
			arch := &archetype{}

			if tc.expectPanic {
				assert.Panics(t, func() {
					_ = arch.deserialize(pb, cm)
				})
			} else {
				err := arch.deserialize(pb, cm)
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
			}
		})
	}
}
