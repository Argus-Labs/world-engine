package ecs

import (
	"testing"

	. "github.com/argus-labs/world-engine/pkg/cardinal/ecs/internal/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/cardinal/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorldState_SerializeDeserialize_RoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		setupFn func() *WorldState
	}{
		{
			name: "empty world state",
			setupFn: func() *WorldState {
				w := NewWorld()
				RegisterComponent[Health](w)
				RegisterComponent[Position](w)
				ws := newWorldState(w)
				return &ws
			},
		},
		{
			name: "world state with single entity",
			setupFn: func() *WorldState {
				w := NewWorld()
				RegisterComponent[Health](w)
				RegisterComponent[Position](w)
				ws := newWorldState(w)

				// Create a single entity
				_, err := ws.opNewEntity([]Component{Health{Value: 100}})
				require.NoError(t, err)

				return &ws
			},
		},
		{
			name: "world state with multiple entities and archetypes",
			setupFn: func() *WorldState {
				w := NewWorld()
				RegisterComponent[Health](w)
				RegisterComponent[Position](w)
				RegisterComponent[Velocity](w)
				ws := newWorldState(w)

				// Create entities with different component combinations
				_, err := ws.opNewEntity([]Component{Health{Value: 100}})
				require.NoError(t, err)
				_, err = ws.opNewEntity([]Component{Health{Value: 200}, Position{X: 10, Y: 20}})
				require.NoError(t, err)
				_, err = ws.opNewEntity([]Component{Position{X: 30, Y: 40}, Velocity{X: 1, Y: 2}})
				require.NoError(t, err)

				return &ws
			},
		},
		{
			name: "world state after entity removal",
			setupFn: func() *WorldState {
				w := NewWorld()
				RegisterComponent[Health](w)
				RegisterComponent[Position](w)
				ws := newWorldState(w)

				// Create entities and remove one
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

				return &ws
			},
		},
		{
			name: "world state with entity moves between archetypes",
			setupFn: func() *WorldState {
				w := NewWorld()
				RegisterComponent[Health](w)
				RegisterComponent[Position](w)
				RegisterComponent[Velocity](w)
				ws := newWorldState(w)

				// Create entity and move it between archetypes
				entity, err := ws.opNewEntity([]Component{Health{Value: 100}})
				require.NoError(t, err)

				err = ws.opMoveEntity(entity, []Component{Health{Value: 150}, Position{X: 5, Y: 10}})
				require.NoError(t, err)

				err = ws.opMoveEntity(entity, []Component{Position{X: 15, Y: 25}, Velocity{X: 2, Y: 3}})
				require.NoError(t, err)

				return &ws
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

			// Create a new world state for deserialization
			w := NewWorld()
			RegisterComponent[Health](w)
			RegisterComponent[Position](w)
			RegisterComponent[Velocity](w)
			deserialized := newWorldState(w)

			// Deserialize
			err = deserialized.deserialize(serialized)
			require.NoError(t, err)

			// Verify round-trip property: deserialize(serialize(x)) == x

			// Compare archetype count
			assert.Len(t, deserialized.archetypes, len(original.archetypes))

			// Compare entity manager state
			assert.Equal(t, original.entities.nextID, deserialized.entities.nextID)
			assert.Equal(t, original.entities.free, deserialized.entities.free)
			assert.Len(t, deserialized.entities.entityArch, len(original.entities.entityArch))

			// Compare entity-to-archetype mappings
			for entityID, origArch := range original.entities.entityArch {
				deserializedArch, exists := deserialized.entities.entityArch[entityID]
				assert.True(t, exists, "entity %d should exist in deserialized state", entityID)
				assert.Equal(t, origArch.id, deserializedArch.id, "archetype ID should match for entity %d", entityID)
			}

			// Compare archetype structures
			for i, origArch := range original.archetypes {
				deserializedArch := deserialized.archetypes[i]
				assert.Equal(t, origArch.id, deserializedArch.id)
				assert.Equal(t, origArch.componentTypeCount, deserializedArch.componentTypeCount)
				assert.Equal(t, origArch.entities.ToBytes(), deserializedArch.entities.ToBytes())
				assert.Equal(t, origArch.components.ToBytes(), deserializedArch.components.ToBytes())
			}
		})
	}
}

func TestWorldState_SerializeDeserialize_Determinism(t *testing.T) {
	t.Parallel()

	// Setup world state with multiple entities and archetypes
	w := NewWorld()
	RegisterComponent[Health](w)
	RegisterComponent[Position](w)
	RegisterComponent[Velocity](w)
	ws := newWorldState(w)

	// Create a complex state
	entity1, err := ws.opNewEntity([]Component{Health{Value: 100}})
	require.NoError(t, err)
	entity2, err := ws.opNewEntity([]Component{Health{Value: 200}, Position{X: 10, Y: 20}})
	require.NoError(t, err)
	_, err = ws.opNewEntity([]Component{Position{X: 30, Y: 40}, Velocity{X: 1, Y: 2}})
	require.NoError(t, err)

	// Remove one entity to create free IDs
	err = ws.opRemoveEntity(entity1)
	require.NoError(t, err)

	// Move entity between archetypes
	err = ws.opMoveEntity(entity2, []Component{Position{X: 15, Y: 25}, Velocity{X: 5, Y: 6}})
	require.NoError(t, err)

	// Serialize the same world state multiple times
	serialized1, err := ws.serialize()
	require.NoError(t, err)

	serialized2, err := ws.serialize()
	require.NoError(t, err)

	// Verify determinism property: serialize(x) == serialize(x)
	assert.Equal(t, serialized1.GetNextId(), serialized2.GetNextId())
	assert.Equal(t, serialized1.GetFreeIds(), serialized2.GetFreeIds())
	assert.Equal(t, serialized1.GetEntityArchetypes(), serialized2.GetEntityArchetypes())
	assert.Len(t, serialized2.GetArchetypes(), len(serialized1.GetArchetypes()))

	// Compare each archetype
	for i, arch1 := range serialized1.GetArchetypes() {
		arch2 := serialized2.GetArchetypes()[i]
		assert.Equal(t, arch1.GetId(), arch2.GetId())
		assert.Equal(t, arch1.GetEntitiesBitmap(), arch2.GetEntitiesBitmap())
		assert.Equal(t, arch1.GetComponentsBitmap(), arch2.GetComponentsBitmap())
		assert.Len(t, arch2.GetColumns(), len(arch1.GetColumns()))

		// Compare each column in archetype
		for j, col1 := range arch1.GetColumns() {
			col2 := arch2.GetColumns()[j]
			assert.Equal(t, col1.GetComponentName(), col2.GetComponentName())
			assert.Equal(t, col1.GetSparse(), col2.GetSparse())
			assert.Equal(t, col1.GetDenseEntityIds(), col2.GetDenseEntityIds())
			assert.Equal(t, col1.GetDenseComponentData(), col2.GetDenseComponentData())
		}
	}
}

func TestWorldState_SerializeDeserialize_ErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("component not registered during deserialization", func(t *testing.T) {
		t.Parallel()

		// Create world state with Health component
		w1 := NewWorld()
		RegisterComponent[Health](w1)
		ws1 := newWorldState(w1)

		_, err := ws1.opNewEntity([]Component{Health{Value: 100}})
		require.NoError(t, err)

		serialized, err := ws1.serialize()
		require.NoError(t, err)

		// Try to deserialize into world state without Health component registered
		w2 := NewWorld()
		RegisterComponent[Position](w2) // Register different component
		ws2 := newWorldState(w2)

		err = ws2.deserialize(serialized)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "component Health not registered")
	})

	t.Run("invalid archetype ID in entity mapping", func(t *testing.T) {
		t.Parallel()

		w := NewWorld()
		RegisterComponent[Health](w)
		ws := newWorldState(w)

		// Create protobuf with invalid archetype ID
		invalidPb := &cardinalv1.CardinalSnapshot{
			Archetypes:       []*cardinalv1.Archetype{}, // Empty archetypes
			NextId:           1,
			FreeIds:          []uint32{},
			EntityArchetypes: map[uint32]uint64{0: 999}, // Invalid archetype ID 999
		}

		err := ws.deserialize(invalidPb)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid archetype ID 999 for entity 0")
	})

	t.Run("archetype deserialization failure", func(t *testing.T) {
		t.Parallel()

		w := NewWorld()
		RegisterComponent[Health](w)
		ws := newWorldState(w)

		// Create protobuf with invalid archetype data
		invalidPb := &cardinalv1.CardinalSnapshot{
			Archetypes: []*cardinalv1.Archetype{
				{
					Id:               0,
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
				},
			},
			NextId:           1,
			FreeIds:          []uint32{},
			EntityArchetypes: map[uint32]uint64{},
		}

		err := ws.deserialize(invalidPb)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to deserialize archetype 0")
	})

	t.Run("nil protobuf", func(t *testing.T) {
		t.Parallel()

		w := NewWorld()
		RegisterComponent[Health](w)
		ws := newWorldState(w)

		// This should not panic - protobuf Get methods handle nil gracefully
		err := ws.deserialize(nil)
		require.NoError(t, err) // No error for empty protobuf
		assert.Equal(t, EntityID(0), ws.entities.nextID)
		assert.Empty(t, ws.entities.free)
		assert.Empty(t, ws.entities.entityArch)
		assert.Empty(t, ws.archetypes)
	})

	t.Run("archetype index out of bounds", func(t *testing.T) {
		t.Parallel()

		w := NewWorld()
		RegisterComponent[Health](w)
		ws := newWorldState(w)

		// Create protobuf where entity refers to archetype that doesn't exist
		invalidPb := &cardinalv1.CardinalSnapshot{
			Archetypes: []*cardinalv1.Archetype{
				{
					Id:               0,
					EntitiesBitmap:   []byte{},
					ComponentsBitmap: []byte{},
					Columns:          []*cardinalv1.Column{},
				},
			},
			NextId:           1,
			FreeIds:          []uint32{},
			EntityArchetypes: map[uint32]uint64{0: 5}, // Archetype ID 5 doesn't exist (only 0 exists)
		}

		err := ws.deserialize(invalidPb)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid archetype ID 5 for entity 0")
	})
}
