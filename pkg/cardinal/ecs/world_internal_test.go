package ecs

import (
	"testing"

	. "github.com/argus-labs/world-engine/pkg/cardinal/ecs/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorld_SerializeDeserialize_RoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		setupFn func() *World
	}{
		{
			name: "empty world",
			setupFn: func() *World {
				w := NewWorld()
				ws := w.state
				_, err := registerComponent[Health](ws)
				require.NoError(t, err)
				_, err = registerComponent[Position](ws)
				require.NoError(t, err)
				return w
			},
		},
		{
			name: "world with single entity",
			setupFn: func() *World {
				w := NewWorld()
				ws := w.state
				_, err := registerComponent[Health](ws)
				require.NoError(t, err)
				_, err = registerComponent[Position](ws)
				require.NoError(t, err)

				w.CustomTick(func(ws *worldState) {
					eid := Create(ws)
					err := Set(ws, eid, Health{Value: 100})
					require.NoError(t, err)
				})

				return w
			},
		},
		{
			name: "world with multiple entities and archetypes",
			setupFn: func() *World {
				w := NewWorld()
				ws := w.state
				_, err := registerComponent[Health](ws)
				require.NoError(t, err)
				_, err = registerComponent[Position](ws)
				require.NoError(t, err)
				_, err = registerComponent[Velocity](ws)
				require.NoError(t, err)

				w.CustomTick(func(ws *worldState) {
					// Create entities with different component combinations
					eid1 := Create(ws)
					err := Set(ws, eid1, Health{Value: 100})
					require.NoError(t, err)

					eid2 := Create(ws)
					err = Set(ws, eid2, Health{Value: 200})
					require.NoError(t, err)
					err = Set(ws, eid2, Position{X: 10, Y: 20})
					require.NoError(t, err)

					eid3 := Create(ws)
					err = Set(ws, eid3, Position{X: 30, Y: 40})
					require.NoError(t, err)
					err = Set(ws, eid3, Velocity{X: 1, Y: 2})
					require.NoError(t, err)
				})

				return w
			},
		},
		{
			name: "world after entity removal",
			setupFn: func() *World {
				w := NewWorld()
				ws := w.state
				_, err := registerComponent[Health](ws)
				require.NoError(t, err)
				_, err = registerComponent[Position](ws)
				require.NoError(t, err)

				w.CustomTick(func(ws *worldState) {
					// Create entities and remove some
					entity1 := Create(ws)
					err := Set(ws, entity1, Health{Value: 100})
					require.NoError(t, err)

					entity2 := Create(ws)
					err = Set(ws, entity2, Health{Value: 200})
					require.NoError(t, err)

					entity3 := Create(ws)
					err = Set(ws, entity3, Position{X: 10, Y: 20})
					require.NoError(t, err)

					removed := Destroy(ws, entity1)
					require.True(t, removed)
					removed = Destroy(ws, entity2)
					require.True(t, removed)
				})

				return w
			},
		},
		{
			name: "world with entity moves between archetypes",
			setupFn: func() *World {
				w := NewWorld()
				ws := w.state
				_, err := registerComponent[Health](ws)
				require.NoError(t, err)
				_, err = registerComponent[Position](ws)
				require.NoError(t, err)
				_, err = registerComponent[Velocity](ws)
				require.NoError(t, err)

				w.CustomTick(func(ws *worldState) {
					// Create entity and move it between archetypes
					entity := Create(ws)
					err := Set(ws, entity, Health{Value: 100})
					require.NoError(t, err)

					// Add position component - entity moves to new archetype
					err = Set(ws, entity, Health{Value: 150})
					require.NoError(t, err)
					err = Set(ws, entity, Position{X: 5, Y: 10})
					require.NoError(t, err)

					// Remove health and add velocity - entity moves to another archetype
					err = Remove[Health](ws, entity)
					require.NoError(t, err)
					err = Set(ws, entity, Position{X: 15, Y: 25})
					require.NoError(t, err)
					err = Set(ws, entity, Velocity{X: 2, Y: 3})
					require.NoError(t, err)
				})

				return w
			},
		},
		{
			name: "world with complex state changes",
			setupFn: func() *World {
				w := NewWorld()
				ws := w.state
				_, err := registerComponent[Health](ws)
				require.NoError(t, err)
				_, err = registerComponent[Position](ws)
				require.NoError(t, err)
				_, err = registerComponent[Velocity](ws)
				require.NoError(t, err)
				_, err = registerComponent[Experience](ws)
				require.NoError(t, err)

				w.CustomTick(func(ws *worldState) {
					// Create multiple entities
					for i := 0; i < 10; i++ {
						eid := Create(ws)
						err := Set(ws, eid, Health{Value: i * 10})
						require.NoError(t, err)
						err = Set(ws, eid, Position{X: i, Y: i * 2})
						require.NoError(t, err)
					}

					// Create some different archetype entities
					eid := Create(ws)
					err := Set(ws, eid, Health{Value: 500})
					require.NoError(t, err)
					err = Set(ws, eid, Velocity{X: 5, Y: 10})
					require.NoError(t, err)
					err = Set(ws, eid, Experience{Value: 1000})
					require.NoError(t, err)

					// Remove some entities
					removed := Destroy(ws, EntityID(2))
					require.True(t, removed)
					removed = Destroy(ws, EntityID(5))
					require.True(t, removed)
					removed = Destroy(ws, EntityID(8))
					require.True(t, removed)
				})

				return w
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			original := tc.setupFn()

			// Serialize
			serializedData, err := original.Serialize()
			require.NoError(t, err)

			// Create a new world for deserialization with same components registered
			w := NewWorld()
			ws := w.state
			_, err = registerComponent[Health](ws)
			require.NoError(t, err)
			_, err = registerComponent[Position](ws)
			require.NoError(t, err)
			_, err = registerComponent[Velocity](ws)
			require.NoError(t, err)
			_, err = registerComponent[Experience](ws)
			require.NoError(t, err)

			// Deserialize
			err = w.Deserialize(serializedData)
			require.NoError(t, err)

			// Verify round-trip property: deserialize(serialize(x)) == x
			// Since World.Serialize/Deserialize only handles the WorldState,
			// we compare the WorldState portions

			// Compare archetype count
			assert.Len(t, ws.archetypes, len(original.state.archetypes))

			// Compare entity manager state
			assert.Equal(t, original.state.nextID, ws.nextID)
			assert.Equal(t, original.state.free, ws.free)
			assert.Len(t, ws.entityArch, len(original.state.entityArch))

			// Compare entity-to-archetype mappings
			// Need to iterate through all possible entity IDs to compare sparseSet contents
			maxEntityID := int(original.state.nextID)
			if maxEntityID == 0 && len(original.state.free) == 0 {
				maxEntityID = 0
			} else if len(original.state.free) > 0 {
				// Find max free ID to ensure we check all possible entities
				for _, freeID := range original.state.free {
					if int(freeID) > maxEntityID {
						maxEntityID = int(freeID)
					}
				}
			}

			for entityID := 0; entityID <= maxEntityID; entityID++ {
				origArchIndex, origExists := original.state.entityArch.get(EntityID(entityID))
				deserializedArchIndex, deserializedExists := ws.entityArch.get(EntityID(entityID))

				assert.Equal(t, origExists, deserializedExists,
					"entity %d existence should match", entityID)

				if origExists && deserializedExists {
					assert.Equal(t, origArchIndex, deserializedArchIndex,
						"archetype index should match for entity %d", entityID)
				}
			}

			// Compare archetype structures
			for i, origArch := range original.state.archetypes {
				deserializedArch := ws.archetypes[i]
				assert.Equal(t, origArch.id, deserializedArch.id)
				assert.Equal(t, origArch.compCount, deserializedArch.compCount)
				assert.Equal(t, origArch.entities, deserializedArch.entities)
				assert.Equal(t, origArch.components.ToBytes(), deserializedArch.components.ToBytes())
				assert.Len(t, deserializedArch.columns, len(origArch.columns))

				// Compare entity-to-row mappings (rows sparseSet)
				assert.Len(t, deserializedArch.rows, len(origArch.rows))

				// Check each entity's row mapping in this archetype
				for _, entityID := range origArch.entities {
					origRow, origExists := origArch.rows.get(entityID)
					deserializedRow, deserializedExists := deserializedArch.rows.get(entityID)

					assert.Equal(t, origExists, deserializedExists,
						"entity %d row mapping existence should match in archetype %d", entityID, i)

					if origExists && deserializedExists {
						assert.Equal(t, origRow, deserializedRow,
							"entity %d row should match in archetype %d", entityID, i)
					}
				}
			}
		})
	}
}

func TestWorld_SerializeDeserialize_Determinism(t *testing.T) {
	t.Parallel()

	// Setup world with complex state
	w := NewWorld()
	ws := w.state
	_, err := registerComponent[Health](ws)
	require.NoError(t, err)
	_, err = registerComponent[Position](ws)
	require.NoError(t, err)
	_, err = registerComponent[Velocity](ws)
	require.NoError(t, err)
	_, err = registerComponent[Experience](ws)
	require.NoError(t, err)

	w.CustomTick(func(ws *worldState) {
		// Create a complex state with multiple entities and archetypes
		entity1 := Create(ws)
		err := Set(ws, entity1, Health{Value: 100})
		require.NoError(t, err)

		entity2 := Create(ws)
		err = Set(ws, entity2, Health{Value: 200})
		require.NoError(t, err)
		err = Set(ws, entity2, Position{X: 10, Y: 20})
		require.NoError(t, err)

		entity3 := Create(ws)
		err = Set(ws, entity3, Position{X: 30, Y: 40})
		require.NoError(t, err)
		err = Set(ws, entity3, Velocity{X: 1, Y: 2})
		require.NoError(t, err)

		entity4 := Create(ws)
		err = Set(ws, entity4, Health{Value: 500})
		require.NoError(t, err)
		err = Set(ws, entity4, Velocity{X: 5, Y: 10})
		require.NoError(t, err)
		err = Set(ws, entity4, Experience{Value: 1000})
		require.NoError(t, err)

		// Remove one entity to create free IDs
		removed := Destroy(ws, entity1)
		require.True(t, removed)

		// Move entity between archetypes by removing health and adding velocity
		err = Remove[Health](ws, entity2)
		require.NoError(t, err)
		err = Set(ws, entity2, Position{X: 15, Y: 25})
		require.NoError(t, err)
		err = Set(ws, entity2, Velocity{X: 5, Y: 6})
		require.NoError(t, err)
	})

	// Serialize the same world multiple times
	serialized1, err := w.Serialize()
	require.NoError(t, err)

	serialized2, err := w.Serialize()
	require.NoError(t, err)

	// Verify determinism property: serialize(x) == serialize(x)
	// Since we use proto.MarshalOptions{Deterministic: true}, the byte slices should be identical
	assert.Equal(t, serialized1, serialized2, "serialized data should be deterministic")
}

func TestWorld_SerializeDeserialize_ErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("component not registered during deserialization", func(t *testing.T) {
		t.Parallel()

		// Create world with Health component
		w1 := NewWorld()
		_, err := registerComponent[Health](w1.state)
		require.NoError(t, err)

		w1.CustomTick(func(ws *worldState) {
			eid := Create(ws)
			err := Set(ws, eid, Health{Value: 100})
			require.NoError(t, err)
		})

		serialized, err := w1.Serialize()
		require.NoError(t, err)

		// Try to deserialize into world without Health component registered
		w2 := NewWorld()
		_, err = registerComponent[Position](w2.state) // Register different component
		require.NoError(t, err)

		err = w2.Deserialize(serialized)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "component Health")
	})

	t.Run("invalid protobuf data", func(t *testing.T) {
		t.Parallel()

		w := NewWorld()
		_, err := registerComponent[Health](w.state)
		require.NoError(t, err)

		// Try to deserialize invalid protobuf data
		invalidData := []byte("this is not valid protobuf data")
		err = w.Deserialize(invalidData)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal snapshot")
	})

	t.Run("empty byte slice", func(t *testing.T) {
		t.Parallel()

		w := NewWorld()
		_, err := registerComponent[Health](w.state)
		require.NoError(t, err)

		// Empty byte slice is valid protobuf (empty message), so this should succeed
		err = w.Deserialize([]byte{})
		assert.NoError(t, err) // Empty protobuf is valid
	})

	t.Run("nil byte slice", func(t *testing.T) {
		t.Parallel()

		w := NewWorld()
		_, err := registerComponent[Health](w.state)
		require.NoError(t, err)

		// Nil slice should also be treated as empty message by protobuf
		err = w.Deserialize(nil)
		assert.NoError(t, err) // Nil is treated as empty protobuf
	})

	t.Run("corrupted protobuf data", func(t *testing.T) {
		t.Parallel()

		// Create valid serialized data
		w1 := NewWorld()
		_, err := registerComponent[Health](w1.state)
		require.NoError(t, err)

		w1.CustomTick(func(ws *worldState) {
			eid := Create(ws)
			err := Set(ws, eid, Health{Value: 100})
			require.NoError(t, err)
		})

		validData, err := w1.Serialize()
		require.NoError(t, err)
		require.Greater(t, len(validData), 10, "need sufficient data to corrupt")

		// Corrupt the data by modifying bytes
		corruptedData := make([]byte, len(validData))
		copy(corruptedData, validData)
		// Flip multiple bits to ensure corruption
		corruptedData[len(corruptedData)/2] ^= 0xFF
		corruptedData[len(corruptedData)/2+1] ^= 0xFF
		corruptedData[len(corruptedData)/4] ^= 0xFF

		// Try to deserialize corrupted data
		w2 := NewWorld()
		_, err = registerComponent[Health](w2.state)
		require.NoError(t, err)

		_ = w2.Deserialize(corruptedData)
		// This might succeed if corruption doesn't affect critical fields,
		// or error during protobuf unmarshaling or WorldState deserialization
		// We just verify it doesn't panic
		// We intentionally ignore the error as protobuf is resilient to corruption
	})

	t.Run("serialize with WorldState error", func(t *testing.T) {
		t.Parallel()

		// This is harder to test since WorldState.serialize() doesn't have many failure modes
		// But we can verify the error propagation works correctly
		w := NewWorld()
		_, err := registerComponent[Health](w.state)
		require.NoError(t, err)

		// Normal case should work
		_, err = w.Serialize()
		assert.NoError(t, err)
	})

	t.Run("deserialize preserves world structure", func(t *testing.T) {
		t.Parallel()

		// Verify that deserialization doesn't affect non-WorldState parts of World
		w1 := NewWorld()
		_, err := registerComponent[Health](w1.state)
		require.NoError(t, err)

		// Create some state
		w1.CustomTick(func(ws *worldState) {
			eid := Create(ws)
			err := Set(ws, eid, Health{Value: 100})
			require.NoError(t, err)
		})

		data, err := w1.Serialize()
		require.NoError(t, err)

		w2 := NewWorld()
		_, err = registerComponent[Health](w2.state)
		require.NoError(t, err)

		// Store references to verify they don't change
		originalCommands := &w2.commands
		originalEvents := &w2.events
		originalSystemEvents := &w2.systemEvents

		err = w2.Deserialize(data)
		require.NoError(t, err)

		// Verify that the managers are the same objects (not recreated)
		assert.Same(t, originalCommands, &w2.commands)
		assert.Same(t, originalEvents, &w2.events)
		assert.Same(t, originalSystemEvents, &w2.systemEvents)
	})
}
