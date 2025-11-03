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

// Core Functionality Tests

func TestWorldState_New(t *testing.T) {
	t.Parallel()

	ws := newWorldState()

	// Verify initialization.
	assert.Equal(t, EntityID(0), ws.nextID)
	assert.Empty(t, ws.free)
	assert.Len(t, ws.archetypes, 1)

	// Verify void archetype exists and is correct.
	voidArch := ws.archetypes[voidArchetypeID]
	assert.Equal(t, voidArchetypeID, voidArch.id)
	assert.Equal(t, 0, voidArch.components.Count())
	assert.Empty(t, voidArch.entities)
	assert.Empty(t, voidArch.columns)
}

func TestWorldState_NewEntity(t *testing.T) {
	t.Parallel()

	ws := newWorldState()

	// Register components for the test.
	_, _ = registerComponent[testutils.Health](ws)

	// Create entity.
	eid := ws.newEntity()

	// Verify entity creation.
	assert.Equal(t, EntityID(0), eid)
	assert.Equal(t, EntityID(1), ws.nextID)

	// Verify entity is in void archetype.
	archIndex, exists := ws.entityArch.get(eid)
	require.True(t, exists)
	assert.Equal(t, voidArchetypeID, archIndex)

	// Verify entity row in void archetype.
	voidArch := ws.archetypes[voidArchetypeID]
	entityRow, exists := voidArch.rows.get(eid)
	require.True(t, exists)
	assert.Equal(t, 0, entityRow)

	// Verify void archetype contains the entity.
	assert.Len(t, voidArch.entities, 1)
	assert.Equal(t, eid, voidArch.entities[0])

	// Create another entity.
	eid2 := ws.newEntity()
	assert.Equal(t, EntityID(1), eid2)
	assert.Equal(t, EntityID(2), ws.nextID)
	assert.Len(t, voidArch.entities, 2)
}

func TestWorldState_NewEntityWithArchetype(t *testing.T) {
	t.Parallel()

	ws := newWorldState()

	// Register components.
	_, _ = registerComponent[testutils.Health](ws)
	_, _ = registerComponent[testutils.Position](ws)

	// Create component bitmap.
	components := bitmap.Bitmap{}
	healthID, err := ws.components.getID("Health")
	require.NoError(t, err)
	components.Set(healthID)

	// Create entity with archetype.
	eid := ws.newEntityWithArchetype(components)

	// Verify entity exists and is in correct archetype.
	archIndex, exists := ws.entityArch.get(eid)
	require.True(t, exists)
	assert.NotEqual(t, voidArchetypeID, archIndex)

	archetype := ws.archetypes[archIndex]
	assert.Equal(t, 1, archetype.components.Count())
	assert.True(t, archetype.components.Contains(healthID))
}

func TestWorldState_RemoveEntity(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		setup          func(*worldState) []EntityID
		removeEntityID func([]EntityID) EntityID
		expectError    bool
		validate       func(*testing.T, *worldState, []EntityID, EntityID)
	}{
		{
			name: "remove entity and verify swap behavior",
			setup: func(ws *worldState) []EntityID {
				_, _ = registerComponent[testutils.Health](ws)
				eid1 := ws.newEntity()
				eid2 := ws.newEntity()
				return []EntityID{eid1, eid2}
			},
			removeEntityID: func(entities []EntityID) EntityID { return entities[0] },
			expectError:    false,
			validate: func(t *testing.T, ws *worldState, entities []EntityID, removedEID EntityID) {
				// Verify entity is removed.
				_, exists := ws.entityArch.get(removedEID)
				assert.False(t, exists)

				// Verify other entity still exists.
				archIndex, exists2 := ws.entityArch.get(entities[1])
				assert.True(t, exists2)

				// Verify entity row in archetype - should be moved to index 0.
				archetype := ws.archetypes[archIndex]
				entityRow, exists := archetype.rows.get(entities[1])
				require.True(t, exists)
				assert.Equal(t, 0, entityRow) // Should be moved to index 0.

				// Verify removed ID is added to free list.
				assert.Contains(t, ws.free, removedEID)
			},
		},
		{
			name: "remove non-existent entity returns false",
			setup: func(ws *worldState) []EntityID {
				return []EntityID{}
			},
			removeEntityID: func([]EntityID) EntityID { return EntityID(999) },
			expectError:    true,
			validate:       func(*testing.T, *worldState, []EntityID, EntityID) {},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ws := newWorldState()
			entities := tc.setup(ws)
			removeEID := tc.removeEntityID(entities)

			success := ws.removeEntity(removeEID)

			if tc.expectError {
				assert.False(t, success)
			} else {
				assert.True(t, success)
				tc.validate(t, ws, entities, removeEID)
			}
		})
	}
}

// Component Operations Tests

func TestWorldState_SetComponent(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		setup          func(*worldState) EntityID
		entityID       func(EntityID) EntityID
		component      testutils.Health
		expectError    bool
		expectedErrMsg string
		validate       func(*testing.T, *worldState, EntityID, testutils.Health)
	}{
		{
			name: "add component to entity triggers archetype migration",
			setup: func(ws *worldState) EntityID {
				_, _ = registerComponent[testutils.Health](ws)
				return ws.newEntity()
			},
			entityID:    func(eid EntityID) EntityID { return eid },
			component:   testutils.Health{Value: 100},
			expectError: false,
			validate: func(t *testing.T, ws *worldState, eid EntityID, expectedHealth testutils.Health) {
				// Verify entity moved to new archetype.
				archIndex, exists := ws.entityArch.get(eid)
				require.True(t, exists)
				assert.NotEqual(t, voidArchetypeID, archIndex)

				archetype := ws.archetypes[archIndex]
				assert.Equal(t, 1, archetype.components.Count())
				healthID, err := ws.components.getID("Health")
				require.NoError(t, err)
				assert.True(t, archetype.components.Contains(healthID))

				// Verify component value.
				retrievedHealth, err := getComponent[testutils.Health](ws, eid)
				require.NoError(t, err)
				assert.Equal(t, expectedHealth, retrievedHealth)
			},
		},
		{
			name: "update existing component in same archetype",
			setup: func(ws *worldState) EntityID {
				_, _ = registerComponent[testutils.Health](ws)
				eid := ws.newEntity()
				health1 := testutils.Health{Value: 100}
				_ = setComponent(ws, eid, health1)
				return eid
			},
			entityID:    func(eid EntityID) EntityID { return eid },
			component:   testutils.Health{Value: 200},
			expectError: false,
			validate: func(t *testing.T, ws *worldState, eid EntityID, expectedHealth testutils.Health) {
				// Verify component value updated.
				retrievedHealth, err := getComponent[testutils.Health](ws, eid)
				require.NoError(t, err)
				assert.Equal(t, expectedHealth, retrievedHealth)
			},
		},
		{
			name: "set component on non-existent entity returns error",
			setup: func(ws *worldState) EntityID {
				_, _ = registerComponent[testutils.Health](ws)
				return ws.newEntity() // We'll use a different ID in entityID func.
			},
			entityID:       func(EntityID) EntityID { return EntityID(999) },
			component:      testutils.Health{Value: 100},
			expectError:    true,
			expectedErrMsg: "entity 999: entity does not exist",
			validate:       func(*testing.T, *worldState, EntityID, testutils.Health) {},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ws := newWorldState()
			eid := tc.setup(ws)
			targetEID := tc.entityID(eid)

			err := setComponent(ws, targetEID, tc.component)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
			} else {
				require.NoError(t, err)
				tc.validate(t, ws, targetEID, tc.component)
			}
		})
	}
}

func TestWorldState_GetComponent(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		setup          func(*worldState) EntityID
		entityID       func(EntityID) EntityID
		expectError    bool
		expectedErrMsg string
		expectedHealth *testutils.Health
	}{
		{
			name: "get existing component returns correct value",
			setup: func(ws *worldState) EntityID {
				_, _ = registerComponent[testutils.Health](ws)
				eid := ws.newEntity()
				health := testutils.Health{Value: 150}
				_ = setComponent(ws, eid, health)
				return eid
			},
			entityID:       func(eid EntityID) EntityID { return eid },
			expectError:    false,
			expectedHealth: &testutils.Health{Value: 150},
		},
		{
			name: "get non-existent component returns error",
			setup: func(ws *worldState) EntityID {
				_, _ = registerComponent[testutils.Health](ws)
				return ws.newEntity()
			},
			entityID:       func(eid EntityID) EntityID { return eid },
			expectError:    true,
			expectedErrMsg: "doesn't contain component Health",
		},
		{
			name: "get component from non-existent entity returns error",
			setup: func(ws *worldState) EntityID {
				_, _ = registerComponent[testutils.Health](ws)
				return ws.newEntity() // We'll use a different ID in entityID func.
			},
			entityID:       func(EntityID) EntityID { return EntityID(999) },
			expectError:    true,
			expectedErrMsg: "entity 999: entity does not exist",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ws := newWorldState()
			eid := tc.setup(ws)
			targetEID := tc.entityID(eid)

			retrievedHealth, err := getComponent[testutils.Health](ws, targetEID)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, *tc.expectedHealth, retrievedHealth)
			}
		})
	}
}

func TestWorldState_RemoveComponent(t *testing.T) {
	t.Parallel()

	ws := newWorldState()
	_, _ = registerComponent[testutils.Health](ws)
	_, _ = registerComponent[testutils.Position](ws)

	eid := ws.newEntity()

	// Set components.
	health := testutils.Health{Value: 100}
	position := testutils.Position{X: 10, Y: 20}
	err := setComponent(ws, eid, health)
	require.NoError(t, err)
	err = setComponent(ws, eid, position)
	require.NoError(t, err)

	// Verify entity has both components.
	archIndex, exists := ws.entityArch.get(eid)
	require.True(t, exists)
	archetype := ws.archetypes[archIndex]
	assert.Equal(t, 2, archetype.components.Count())

	// Remove health component.
	err = removeComponent[testutils.Health](ws, eid)
	require.NoError(t, err)

	// Verify component removed and entity moved to different archetype.
	archIndex, exists = ws.entityArch.get(eid)
	require.True(t, exists)
	newArchetype := ws.archetypes[archIndex]
	assert.Equal(t, 1, newArchetype.components.Count())
	positionID, err := ws.components.getID("Position")
	require.NoError(t, err)
	assert.True(t, newArchetype.components.Contains(positionID))

	// Verify health component no longer accessible.
	_, err = getComponent[testutils.Health](ws, eid)
	require.Error(t, err)

	// Verify position component still accessible.
	retrievedPos, err := getComponent[testutils.Position](ws, eid)
	require.NoError(t, err)
	assert.Equal(t, position, retrievedPos)
}

// Archetype Management Tests

func TestWorldState_FindOrCreateArchetype(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		setup          func(*worldState) (bitmap.Bitmap, int)
		componentNames []string
		validate       func(*testing.T, *worldState, archetypeID, bitmap.Bitmap, int)
	}{
		{
			name: "find existing archetype returns same instance",
			setup: func(ws *worldState) (bitmap.Bitmap, int) {
				_, _ = registerComponent[testutils.Health](ws)
				components := bitmap.Bitmap{}
				healthID, err := ws.components.getID("Health")
				require.NoError(t, err)
				components.Set(healthID)
				_ = ws.findOrCreateArchetype(components)
				return components, len(ws.archetypes)
			},
			componentNames: []string{"Health"},
			validate: func(t *testing.T, ws *worldState, archetypeID archetypeID, components bitmap.Bitmap,
				initialCount int) {
				// Find existing archetype.
				archetype2ID := ws.findOrCreateArchetype(components)

				// Verify same archetype returned and no new archetype created.
				assert.Equal(t, archetypeID, archetype2ID)
				assert.Len(t, ws.archetypes, initialCount)
			},
		},
		{
			name: "create new archetype when none exists",
			setup: func(ws *worldState) (bitmap.Bitmap, int) {
				_, _ = registerComponent[testutils.Health](ws)
				_, _ = registerComponent[testutils.Position](ws)
				components := bitmap.Bitmap{}
				healthID, err := ws.components.getID("Health")
				require.NoError(t, err)
				components.Set(healthID)
				positionID, err := ws.components.getID("Position")
				require.NoError(t, err)
				components.Set(positionID)
				return components, len(ws.archetypes)
			},
			componentNames: []string{"Health", "Position"},
			validate: func(t *testing.T, ws *worldState, archetypeID archetypeID, components bitmap.Bitmap,
				initialCount int) {
				// Verify new archetype created.
				assert.Len(t, ws.archetypes, initialCount+1)

				// Get the actual archetype from the ID
				archetype := ws.archetypes[archetypeID]
				assert.Equal(t, 2, archetype.components.Count())
				healthID, err := ws.components.getID("Health")
				require.NoError(t, err)
				assert.True(t, archetype.components.Contains(healthID))
				positionID, err := ws.components.getID("Position")
				require.NoError(t, err)
				assert.True(t, archetype.components.Contains(positionID))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ws := newWorldState()
			components, initialCount := tc.setup(ws)

			aid := ws.findOrCreateArchetype(components)

			tc.validate(t, ws, aid, components, initialCount)
		})
	}
}

func TestWorldState_GetArchetypeExact(t *testing.T) {
	t.Parallel()

	ws := newWorldState()
	_, _ = registerComponent[testutils.Health](ws)

	// Create archetype.
	components := bitmap.Bitmap{}
	healthID, err := ws.components.getID("Health")
	require.NoError(t, err)
	components.Set(healthID)
	_ = ws.findOrCreateArchetype(components)

	// Find exact archetype.
	index, found := ws.archExact(components)
	assert.True(t, found)
	assert.NotEqual(t, voidArchetypeID, index)

	foundArchetype := ws.archetypes[index]
	assert.Equal(t, components, foundArchetype.components)
}

func TestWorldState_GetArchetypesContains(t *testing.T) {
	t.Parallel()

	ws := newWorldState()
	_, _ = registerComponent[testutils.Health](ws)
	_, _ = registerComponent[testutils.Position](ws)

	// Create multiple archetypes.
	healthOnly := bitmap.Bitmap{}
	healthID, err := ws.components.getID("Health")
	require.NoError(t, err)
	healthOnly.Set(healthID)

	healthPos := bitmap.Bitmap{}
	healthPos.Set(healthID)
	positionID, err := ws.components.getID("Position")
	require.NoError(t, err)
	healthPos.Set(positionID)

	_ = ws.findOrCreateArchetype(healthOnly)
	_ = ws.findOrCreateArchetype(healthPos)

	// Find archetypes containing Health.
	healthComponents := bitmap.Bitmap{}
	healthComponents.Set(healthID)

	indices := ws.archContains(healthComponents)

	// Should find both archetypes (excluding void).
	assert.Len(t, indices, 2)
	for _, index := range indices {
		archetype := ws.archetypes[index]
		assert.True(t, archetype.components.Contains(healthID))
	}
}

// Complex Logic Tests

func TestWorldState_EntityIDReuse(t *testing.T) {
	t.Parallel()

	ws := newWorldState()

	// Create entities.
	eid1 := ws.newEntity()
	eid2 := ws.newEntity()
	assert.Equal(t, EntityID(0), eid1)
	assert.Equal(t, EntityID(1), eid2)

	// Remove first entity.
	success := ws.removeEntity(eid1)
	require.True(t, success)

	// Create new entity - should reuse ID (testing if removeEntity properly adds to free list).
	eid3 := ws.newEntity()
	assert.Equal(t, EntityID(0), eid3) // Should reuse removed ID.

	// Create another entity - should use next sequential.
	eid4 := ws.newEntity()
	assert.Equal(t, EntityID(2), eid4)
}

func TestWorldState_EntitySwapOnRemoval(t *testing.T) {
	t.Parallel()

	ws := newWorldState()
	_, _ = registerComponent[testutils.Health](ws)

	// Create entities and add them to same archetype
	eid1 := ws.newEntity()
	eid2 := ws.newEntity()
	eid3 := ws.newEntity()

	health := testutils.Health{Value: 100}
	err := setComponent(ws, eid1, health)
	require.NoError(t, err)
	err = setComponent(ws, eid2, health)
	require.NoError(t, err)
	err = setComponent(ws, eid3, health)
	require.NoError(t, err)

	// All entities should be in same non-void archetype
	archIndex1, exists1 := ws.entityArch.get(eid1)
	archIndex2, exists2 := ws.entityArch.get(eid2)
	archIndex3, exists3 := ws.entityArch.get(eid3)
	require.True(t, exists1)
	require.True(t, exists2)
	require.True(t, exists3)
	assert.Equal(t, archIndex1, archIndex2)
	assert.Equal(t, archIndex1, archIndex3)

	// Check entity rows in archetype
	archetype := ws.archetypes[archIndex1]
	row1, exists := archetype.rows.get(eid1)
	require.True(t, exists)
	assert.Equal(t, 0, row1)
	row2, exists := archetype.rows.get(eid2)
	require.True(t, exists)
	assert.Equal(t, 1, row2)
	row3, exists := archetype.rows.get(eid3)
	require.True(t, exists)
	assert.Equal(t, 2, row3)

	// Remove middle entity
	success := ws.removeEntity(eid2)
	require.True(t, success)

	// Verify eid3 was swapped to eid2's position
	row1After, exists := archetype.rows.get(eid1)
	require.True(t, exists)
	assert.Equal(t, 0, row1After) // Unchanged
	row3After, exists := archetype.rows.get(eid3)
	require.True(t, exists)
	assert.Equal(t, 1, row3After) // Moved to eid2's position

	// Verify eid2 no longer exists
	_, exists = ws.entityArch.get(eid2)
	assert.False(t, exists)
}

// Property-Based Integration Tests

func TestWorldState_EntityLifecycle_PropertyBased(t *testing.T) {
	t.Parallel()

	const numIterations = 50
	const maxOps = 20

	for i := 0; i < numIterations; i++ {
		t.Run("iteration", func(t *testing.T) {
			t.Parallel()

			ws := newWorldState()
			_, _ = registerComponent[testutils.Health](ws)
			_, _ = registerComponent[testutils.Position](ws)
			_, _ = registerComponent[testutils.Velocity](ws)

			var entities []EntityID
			numOps := rand.Intn(maxOps) + 1

			for j := 0; j < numOps; j++ {
				operation := rand.Intn(4) // 0=create, 1=remove, 2=add_component, 3=remove_component

				switch operation {
				case 0: // Create entity
					eid := ws.newEntity()
					entities = append(entities, eid)

				case 1: // Remove entity
					if len(entities) > 0 {
						idx := rand.Intn(len(entities))
						eid := entities[idx]
						success := ws.removeEntity(eid)
						require.True(t, success)
						// Remove from tracking
						entities = append(entities[:idx], entities[idx+1:]...)
					}

				case 2: // Add component
					if len(entities) > 0 {
						eid := entities[rand.Intn(len(entities))]
						componentType := rand.Intn(3)

						switch componentType {
						case 0:
							_ = setComponent(ws, eid, testutils.Health{Value: rand.Intn(100)})
						case 1:
							_ = setComponent(ws, eid, testutils.Position{X: rand.Intn(1000), Y: rand.Intn(1000)})
						case 2:
							_ = setComponent(ws, eid, testutils.Velocity{X: rand.Intn(100), Y: rand.Intn(100)})
						}
					}

				case 3: // Remove component
					if len(entities) > 0 {
						eid := entities[rand.Intn(len(entities))]
						componentType := rand.Intn(3)

						switch componentType {
						case 0:
							_ = removeComponent[testutils.Health](ws, eid)
						case 1:
							_ = removeComponent[testutils.Position](ws, eid)
						case 2:
							_ = removeComponent[testutils.Velocity](ws, eid)
						}
					}
				}

				// Verify invariants
				for _, eid := range entities {
					archIndex, exists := ws.entityArch.get(eid)
					require.True(t, exists, "entity %d should exist", eid)

					archetype := ws.archetypes[archIndex]
					entityRow, exists := archetype.rows.get(eid)
					require.True(t, exists, "entity %d should have row mapping", eid)
					require.Less(t, entityRow, len(archetype.entities),
						"entity row %d should be valid for archetype with %d entities", entityRow, len(archetype.entities))
					require.Equal(t, eid, archetype.entities[entityRow], "entity at row should match expected ID")
				}

				// Verify archetype column lengths match entity count
				for _, archetype := range ws.archetypes {
					for _, col := range archetype.columns {
						require.Equal(t, len(archetype.entities), col.(interface{ len() int }).len(),
							"column length should match entity count")
					}
				}
			}
		})
	}
}

func TestWorldState_MultipleComponentOperations_PropertyBased(t *testing.T) {
	t.Parallel()

	const numIterations = 30
	const maxEntities = 10
	const maxOpsPerEntity = 8

	for range numIterations {
		t.Run("iteration", func(t *testing.T) {
			t.Parallel()

			ws := newWorldState()
			_, _ = registerComponent[testutils.Health](ws)
			_, _ = registerComponent[testutils.Position](ws)
			_, _ = registerComponent[testutils.Velocity](ws)
			_, _ = registerComponent[testutils.Experience](ws)

			numEntities := rand.Intn(maxEntities) + 1
			entities := make([]EntityID, numEntities)

			// Create entities
			for j := range numEntities {
				entities[j] = ws.newEntity()
			}

			// Perform random component operations on each entity
			for _, eid := range entities {
				numOps := rand.Intn(maxOpsPerEntity) + 1

				for range numOps {
					operation := rand.Intn(2) // 0=add, 1=remove
					componentType := rand.Intn(4)

					if operation == 0 { // Add component
						switch componentType {
						case 0:
							_ = setComponent(ws, eid, testutils.Health{Value: rand.Intn(100)})
						case 1:
							_ = setComponent(ws, eid, testutils.Position{X: rand.Intn(1000), Y: rand.Intn(1000)})
						case 2:
							_ = setComponent(ws, eid, testutils.Velocity{X: rand.Intn(100), Y: rand.Intn(100)})
						case 3:
							_ = setComponent(ws, eid, testutils.Experience{Value: rand.Intn(1000)})
						}
					} else { // Remove component
						switch componentType {
						case 0:
							_ = removeComponent[testutils.Health](ws, eid)
						case 1:
							_ = removeComponent[testutils.Position](ws, eid)
						case 2:
							_ = removeComponent[testutils.Velocity](ws, eid)
						case 3:
							_ = removeComponent[testutils.Experience](ws, eid)
						}
					}
				}

				// Verify entity consistency after operations
				archIndex, exists := ws.entityArch.get(eid)
				require.True(t, exists, "entity should exist after operations")

				archetype := ws.archetypes[archIndex]
				entityRow, exists := archetype.rows.get(eid)
				require.True(t, exists, "entity should have row mapping")
				require.Equal(t, eid, archetype.entities[entityRow], "entity should be at correct row")

				// Verify we can read any components that should exist
				healthID, err := ws.components.getID("Health")
				require.NoError(t, err)
				positionID, err := ws.components.getID("Position")
				require.NoError(t, err)
				velocityID, err := ws.components.getID("Velocity")
				require.NoError(t, err)
				experienceID, err := ws.components.getID("Experience")
				require.NoError(t, err)

				for j := uint32(0); j < 32; j++ {
					if archetype.components.Contains(j) {
						switch j {
						case healthID:
							_, err := getComponent[testutils.Health](ws, eid)
							require.NoError(t, err, "should be able to read Health component")
						case positionID:
							_, err := getComponent[testutils.Position](ws, eid)
							require.NoError(t, err, "should be able to read Position component")
						case velocityID:
							_, err := getComponent[testutils.Velocity](ws, eid)
							require.NoError(t, err, "should be able to read Velocity component")
						case experienceID:
							_, err := getComponent[testutils.Experience](ws, eid)
							require.NoError(t, err, "should be able to read Experience component")
						}
					}
				}
			}

			// Final consistency check
			for _, archetype := range ws.archetypes {
				for _, col := range archetype.columns {
					require.Equal(t, len(archetype.entities), col.(interface{ len() int }).len(),
						"final column length should match entity count")
				}
			}
		})
	}
}

func TestWorldState_SerializeDeserialize_RoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		setupFn func() *worldState
	}{
		{
			name: "empty world state",
			setupFn: func() *worldState {
				ws := newWorldState()
				_, _ = registerComponent[testutils.Health](ws)
				_, _ = registerComponent[testutils.Position](ws)
				return ws
			},
		},
		{
			name: "world state with single entity",
			setupFn: func() *worldState {
				ws := newWorldState()
				_, _ = registerComponent[testutils.Health](ws)
				_, _ = registerComponent[testutils.Position](ws)

				eid := ws.newEntity()
				_ = setComponent(ws, eid, testutils.Health{Value: 100})

				return ws
			},
		},
		{
			name: "world state with multiple entities and archetypes",
			setupFn: func() *worldState {
				ws := newWorldState()
				_, _ = registerComponent[testutils.Health](ws)
				_, _ = registerComponent[testutils.Position](ws)
				_, _ = registerComponent[testutils.Velocity](ws)

				eid1 := ws.newEntity()
				_ = setComponent(ws, eid1, testutils.Health{Value: 100})

				eid2 := ws.newEntity()
				_ = setComponent(ws, eid2, testutils.Health{Value: 200})
				_ = setComponent(ws, eid2, testutils.Position{X: 10, Y: 20})

				eid3 := ws.newEntity()
				_ = setComponent(ws, eid3, testutils.Position{X: 30, Y: 40})
				_ = setComponent(ws, eid3, testutils.Velocity{X: 1, Y: 2})

				return ws
			},
		},
		{
			name: "world state after entity removal",
			setupFn: func() *worldState {
				ws := newWorldState()
				_, _ = registerComponent[testutils.Health](ws)
				_, _ = registerComponent[testutils.Position](ws)

				eid1 := ws.newEntity()
				_ = setComponent(ws, eid1, testutils.Health{Value: 100})

				eid2 := ws.newEntity()
				_ = setComponent(ws, eid2, testutils.Health{Value: 200})

				eid3 := ws.newEntity()
				_ = setComponent(ws, eid3, testutils.Position{X: 10, Y: 20})

				ws.removeEntity(eid1)
				ws.removeEntity(eid2)

				return ws
			},
		},
		{
			name: "world state with entity moves between archetypes",
			setupFn: func() *worldState {
				ws := newWorldState()
				_, _ = registerComponent[testutils.Health](ws)
				_, _ = registerComponent[testutils.Position](ws)
				_, _ = registerComponent[testutils.Velocity](ws)

				eid := ws.newEntity()
				_ = setComponent(ws, eid, testutils.Health{Value: 100})
				_ = setComponent(ws, eid, testutils.Position{X: 5, Y: 10})
				_ = removeComponent[testutils.Health](ws, eid)
				_ = setComponent(ws, eid, testutils.Velocity{X: 2, Y: 3})

				return ws
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

			// Create a new world state for deserialization.
			deserialized := newWorldState()
			_, _ = registerComponent[testutils.Health](deserialized)
			_, _ = registerComponent[testutils.Position](deserialized)
			_, _ = registerComponent[testutils.Velocity](deserialized)

			// Deserialize.
			err = deserialized.deserialize(serialized)
			require.NoError(t, err)

			// Verify round-trip property: deserialize(serialize(x)) == x.
			assert.Len(t, deserialized.archetypes, len(original.archetypes))
			assert.Equal(t, original.nextID, deserialized.nextID)
			assert.Equal(t, original.free, deserialized.free)

			// Compare archetype structures.
			for i, origArch := range original.archetypes {
				deserializedArch := deserialized.archetypes[i]
				assert.Equal(t, origArch.id, deserializedArch.id)
				assert.Equal(t, origArch.compCount, deserializedArch.compCount)
				assert.Equal(t, origArch.components.ToBytes(), deserializedArch.components.ToBytes())
				assert.Equal(t, origArch.entities, deserializedArch.entities)
			}
		})
	}
}

func TestWorldState_SerializeDeserialize_Determinism(t *testing.T) {
	t.Parallel()

	// Setup world state with multiple entities and archetypes.
	ws := newWorldState()
	_, _ = registerComponent[testutils.Health](ws)
	_, _ = registerComponent[testutils.Position](ws)
	_, _ = registerComponent[testutils.Velocity](ws)
	_, _ = registerComponent[testutils.MapComponent](ws)

	// Create a complex state.
	eid1 := ws.newEntity()
	_ = setComponent(ws, eid1, testutils.Health{Value: 100})
	_ = setComponent(ws, eid1, testutils.MapComponent{
		Items: map[string]int{
			"sword":  1,
			"shield": 1,
			"potion": 5,
		},
	})

	eid2 := ws.newEntity()
	_ = setComponent(ws, eid2, testutils.Health{Value: 200})
	_ = setComponent(ws, eid2, testutils.Position{X: 10, Y: 20})

	eid3 := ws.newEntity()
	_ = setComponent(ws, eid3, testutils.Position{X: 30, Y: 40})
	_ = setComponent(ws, eid3, testutils.Velocity{X: 1, Y: 2})

	// Remove one entity to create free IDs.
	ws.removeEntity(eid1)

	// Move entity between archetypes.
	_ = removeComponent[testutils.Health](ws, eid2)
	_ = setComponent(ws, eid2, testutils.Velocity{X: 5, Y: 6})

	// Serialize the same world state multiple times and verify determinism.
	const iterations = 10
	var prev *cardinalv1.CardinalSnapshot

	for i := 0; i < iterations; i++ {
		current, err := ws.serialize()
		require.NoError(t, err)

		if prev != nil {
			assert.Equal(t, prev.GetNextId(), current.GetNextId(),
				"iteration %d: next ID differs", i)
			assert.Equal(t, prev.GetFreeIds(), current.GetFreeIds(),
				"iteration %d: free IDs differ", i)
			assert.Equal(t, prev.GetEntityArch(), current.GetEntityArch(),
				"iteration %d: entity archetypes differ", i)
			assert.Len(t, current.GetArchetypes(), len(prev.GetArchetypes()),
				"iteration %d: archetype count differs", i)

			// Compare each archetype.
			for j, prevArch := range prev.GetArchetypes() {
				currentArch := current.GetArchetypes()[j]
				assert.Equal(t, prevArch.GetId(), currentArch.GetId(),
					"iteration %d, archetype %d: ID differs", i, j)
				assert.Equal(t, prevArch.GetComponentsBitmap(), currentArch.GetComponentsBitmap(),
					"iteration %d, archetype %d: components bitmap differs", i, j)
				assert.Equal(t, prevArch.GetEntities(), currentArch.GetEntities(),
					"iteration %d, archetype %d: entities differ", i, j)
				assert.Len(t, currentArch.GetColumns(), len(prevArch.GetColumns()),
					"iteration %d, archetype %d: column count differs", i, j)

				// Compare each column in archetype.
				for k, prevCol := range prevArch.GetColumns() {
					currentCol := currentArch.GetColumns()[k]
					assert.Equal(t, prevCol.GetComponentName(), currentCol.GetComponentName(),
						"iteration %d, archetype %d, column %d: component name differs", i, j, k)
					assert.Equal(t, prevCol.GetComponents(), currentCol.GetComponents(),
						"iteration %d, archetype %d, column %d: components differ", i, j, k)
				}
			}
		}

		prev = current
	}
}

func TestWorldState_SerializeDeserialize_ErrorHandling(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		setupOriginal func() *worldState
		setupTarget   func() *worldState
		setupProtobuf func() *cardinalv1.CardinalSnapshot
		errorContains string
	}{
		{
			name: "component not registered during deserialization",
			setupOriginal: func() *worldState {
				ws := newWorldState()
				_, _ = registerComponent[testutils.Health](ws)
				eid := ws.newEntity()
				_ = setComponent(ws, eid, testutils.Health{Value: 100})
				return ws
			},
			setupTarget: func() *worldState {
				ws := newWorldState()
				_, _ = registerComponent[testutils.Position](ws) // Different component.
				return ws
			},
			setupProtobuf: nil,
			errorContains: "component is not registered",
		},
		{
			name:          "archetype deserialization failure",
			setupOriginal: nil,
			setupTarget: func() *worldState {
				ws := newWorldState()
				_, _ = registerComponent[testutils.Health](ws)
				return ws
			},
			setupProtobuf: func() *cardinalv1.CardinalSnapshot {
				return &cardinalv1.CardinalSnapshot{
					Archetypes: []*cardinalv1.Archetype{
						{
							Id:               0,
							ComponentsBitmap: []byte{},
							Entities:         []uint32{},
							Columns: []*cardinalv1.Column{
								{
									ComponentName: "Health",
									Components:    [][]byte{[]byte("invalid json")},
								},
							},
						},
					},
					NextId:     1,
					FreeIds:    []uint32{},
					EntityArch: []int64{},
				}
			},
			errorContains: "failed to deserialize archetype 0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var serialized *cardinalv1.CardinalSnapshot
			var err error

			if tc.setupProtobuf != nil {
				serialized = tc.setupProtobuf()
			} else {
				original := tc.setupOriginal()
				serialized, err = original.serialize()
				require.NoError(t, err)
			}

			target := tc.setupTarget()
			err = target.deserialize(serialized)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errorContains)
		})
	}
}
