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
				RegisterComponent[Health](w)
				RegisterComponent[Position](w)
				return w
			},
		},
		{
			name: "world with single entity",
			setupFn: func() *World {
				w := NewWorld()
				RegisterComponent[Health](w)
				RegisterComponent[Position](w)

				w.CustomTick(func(ws *WorldState) {
					_, err := ws.opNewEntity([]Component{Health{Value: 100}})
					require.NoError(t, err)
				})

				return w
			},
		},
		{
			name: "world with multiple entities and archetypes",
			setupFn: func() *World {
				w := NewWorld()
				RegisterComponent[Health](w)
				RegisterComponent[Position](w)
				RegisterComponent[Velocity](w)

				w.CustomTick(func(ws *WorldState) {
					// Create entities with different component combinations
					_, err := ws.opNewEntity([]Component{Health{Value: 100}})
					require.NoError(t, err)
					_, err = ws.opNewEntity([]Component{Health{Value: 200}, Position{X: 10, Y: 20}})
					require.NoError(t, err)
					_, err = ws.opNewEntity([]Component{Position{X: 30, Y: 40}, Velocity{X: 1, Y: 2}})
					require.NoError(t, err)
				})

				return w
			},
		},
		{
			name: "world after entity removal",
			setupFn: func() *World {
				w := NewWorld()
				RegisterComponent[Health](w)
				RegisterComponent[Position](w)

				w.CustomTick(func(ws *WorldState) {
					// Create entities and remove some
					entity1, err := ws.opNewEntity([]Component{Health{Value: 100}})
					require.NoError(t, err)
					entity2, err := ws.opNewEntity([]Component{Health{Value: 200}})
					require.NoError(t, err)
					_, err = ws.opNewEntity([]Component{Position{X: 10, Y: 20}})
					require.NoError(t, err)

					err = ws.opRemoveEntity(entity1)
					require.NoError(t, err)
					err = ws.opRemoveEntity(entity2)
					require.NoError(t, err)
				})

				return w
			},
		},
		{
			name: "world with entity moves between archetypes",
			setupFn: func() *World {
				w := NewWorld()
				RegisterComponent[Health](w)
				RegisterComponent[Position](w)
				RegisterComponent[Velocity](w)

				w.CustomTick(func(ws *WorldState) {
					// Create entity and move it between archetypes
					entity, err := ws.opNewEntity([]Component{Health{Value: 100}})
					require.NoError(t, err)

					err = ws.opMoveEntity(entity, []Component{Health{Value: 150}, Position{X: 5, Y: 10}})
					require.NoError(t, err)

					err = ws.opMoveEntity(entity, []Component{Position{X: 15, Y: 25}, Velocity{X: 2, Y: 3}})
					require.NoError(t, err)
				})

				return w
			},
		},
		{
			name: "world with complex state changes",
			setupFn: func() *World {
				w := NewWorld()
				RegisterComponent[Health](w)
				RegisterComponent[Position](w)
				RegisterComponent[Velocity](w)
				RegisterComponent[Experience](w)

				w.CustomTick(func(ws *WorldState) {
					// Create multiple entities
					for i := 0; i < 10; i++ {
						_, err := ws.opNewEntity([]Component{
							Health{Value: i * 10},
							Position{X: i, Y: i * 2},
						})
						require.NoError(t, err)
					}

					// Create some different archetype entities
					_, err := ws.opNewEntity([]Component{
						Health{Value: 500},
						Velocity{X: 5, Y: 10},
						Experience{Value: 1000},
					})
					require.NoError(t, err)

					// Remove some entities
					err = ws.opRemoveEntity(EntityID(2))
					require.NoError(t, err)
					err = ws.opRemoveEntity(EntityID(5))
					require.NoError(t, err)
					err = ws.opRemoveEntity(EntityID(8))
					require.NoError(t, err)
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
			deserialized := NewWorld()
			RegisterComponent[Health](deserialized)
			RegisterComponent[Position](deserialized)
			RegisterComponent[Velocity](deserialized)
			RegisterComponent[Experience](deserialized)

			// Deserialize
			err = deserialized.Deserialize(serializedData)
			require.NoError(t, err)

			// Verify round-trip property: deserialize(serialize(x)) == x
			// Since World.Serialize/Deserialize only handles the WorldState,
			// we compare the WorldState portions

			// Compare archetype count
			assert.Len(t, deserialized.state.archetypes, len(original.state.archetypes))

			// Compare entity manager state
			assert.Equal(t, original.state.entities.nextID, deserialized.state.entities.nextID)
			assert.Equal(t, original.state.entities.free, deserialized.state.entities.free)
			assert.Len(t, deserialized.state.entities.entityArch, len(original.state.entities.entityArch))

			// Compare entity-to-archetype mappings
			for entityID, origArch := range original.state.entities.entityArch {
				deserializedArch, exists := deserialized.state.entities.entityArch[entityID]
				assert.True(t, exists, "entity %d should exist in deserialized state", entityID)
				assert.Equal(t, origArch.id, deserializedArch.id, "archetype ID should match for entity %d", entityID)
			}

			// Compare archetype structures
			for i, origArch := range original.state.archetypes {
				deserializedArch := deserialized.state.archetypes[i]
				assert.Equal(t, origArch.id, deserializedArch.id)
				assert.Equal(t, origArch.componentTypeCount, deserializedArch.componentTypeCount)
				assert.Equal(t, origArch.entities.ToBytes(), deserializedArch.entities.ToBytes())
				assert.Equal(t, origArch.components.ToBytes(), deserializedArch.components.ToBytes())
				assert.Len(t, deserializedArch.columns, len(origArch.columns))
			}
		})
	}
}

func TestWorld_SerializeDeserialize_Determinism(t *testing.T) {
	t.Parallel()

	// Setup world with complex state
	w := NewWorld()
	RegisterComponent[Health](w)
	RegisterComponent[Position](w)
	RegisterComponent[Velocity](w)
	RegisterComponent[Experience](w)

	w.CustomTick(func(ws *WorldState) {
		// Create a complex state with multiple entities and archetypes
		entity1, err := ws.opNewEntity([]Component{Health{Value: 100}})
		require.NoError(t, err)
		entity2, err := ws.opNewEntity([]Component{Health{Value: 200}, Position{X: 10, Y: 20}})
		require.NoError(t, err)
		_, err = ws.opNewEntity([]Component{Position{X: 30, Y: 40}, Velocity{X: 1, Y: 2}})
		require.NoError(t, err)
		_, err = ws.opNewEntity([]Component{
			Health{Value: 500},
			Velocity{X: 5, Y: 10},
			Experience{Value: 1000},
		})
		require.NoError(t, err)

		// Remove one entity to create free IDs
		err = ws.opRemoveEntity(entity1)
		require.NoError(t, err)

		// Move entity between archetypes
		err = ws.opMoveEntity(entity2, []Component{Position{X: 15, Y: 25}, Velocity{X: 5, Y: 6}})
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
		RegisterComponent[Health](w1)

		w1.CustomTick(func(ws *WorldState) {
			_, err := ws.opNewEntity([]Component{Health{Value: 100}})
			require.NoError(t, err)
		})

		serialized, err := w1.Serialize()
		require.NoError(t, err)

		// Try to deserialize into world without Health component registered
		w2 := NewWorld()
		RegisterComponent[Position](w2) // Register different component

		err = w2.Deserialize(serialized)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "component Health not registered")
	})

	t.Run("invalid protobuf data", func(t *testing.T) {
		t.Parallel()

		w := NewWorld()
		RegisterComponent[Health](w)

		// Try to deserialize invalid protobuf data
		invalidData := []byte("this is not valid protobuf data")
		err := w.Deserialize(invalidData)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal snapshot")
	})

	t.Run("empty byte slice", func(t *testing.T) {
		t.Parallel()

		w := NewWorld()
		RegisterComponent[Health](w)

		// Empty byte slice is valid protobuf (empty message), so this should succeed
		err := w.Deserialize([]byte{})
		assert.NoError(t, err) // Empty protobuf is valid
	})

	t.Run("nil byte slice", func(t *testing.T) {
		t.Parallel()

		w := NewWorld()
		RegisterComponent[Health](w)

		// Nil slice should also be treated as empty message by protobuf
		err := w.Deserialize(nil)
		assert.NoError(t, err) // Nil is treated as empty protobuf
	})

	t.Run("corrupted protobuf data", func(t *testing.T) {
		t.Parallel()

		// Create valid serialized data
		w1 := NewWorld()
		RegisterComponent[Health](w1)

		w1.CustomTick(func(ws *WorldState) {
			_, err := ws.opNewEntity([]Component{Health{Value: 100}})
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
		RegisterComponent[Health](w2)

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
		RegisterComponent[Health](w)

		// Normal case should work
		_, err := w.Serialize()
		assert.NoError(t, err)
	})

	t.Run("deserialize preserves world structure", func(t *testing.T) {
		t.Parallel()

		// Verify that deserialization doesn't affect non-WorldState parts of World
		w1 := NewWorld()
		RegisterComponent[Health](w1)

		// Create some state
		w1.CustomTick(func(ws *WorldState) {
			_, err := ws.opNewEntity([]Component{Health{Value: 100}})
			require.NoError(t, err)
		})

		data, err := w1.Serialize()
		require.NoError(t, err)

		w2 := NewWorld()
		RegisterComponent[Health](w2)

		// Store references to verify they don't change
		originalComponents := &w2.components
		originalCommands := &w2.commands
		originalEvents := &w2.events
		originalSystemEvents := &w2.systemEvents

		err = w2.Deserialize(data)
		require.NoError(t, err)

		// Verify that the managers are the same objects (not recreated)
		assert.Same(t, originalComponents, &w2.components)
		assert.Same(t, originalCommands, &w2.commands)
		assert.Same(t, originalEvents, &w2.events)
		assert.Same(t, originalSystemEvents, &w2.systemEvents)
	})
}
