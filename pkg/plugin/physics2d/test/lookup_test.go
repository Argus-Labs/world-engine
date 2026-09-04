package physics2d_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Entity -> engine object lookup (Plugin.BodyID / Plugin.ShapeIDs), the
// supported read-only companion to Plugin.Engine().
// ---------------------------------------------------------------------------

// spawnTwoShapeBody registers an Init system that creates one static entity with two collider
// slots (box in slot 0, circle in slot 1) and returns a pointer to the assigned entity id.
func spawnTwoShapeBody(t *testing.T, w *cardinal.World) *cardinal.EntityID {
	t.Helper()
	entityID := new(cardinal.EntityID)
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "lookup"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 1, Y: 2}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic,
			physics.ColliderShape{
				ShapeType:    physics.ShapeTypeBox,
				HalfExtents:  physics.Vec2{X: 0.5, Y: 0.5},
				Density:      1,
				CategoryBits: 0xFFFF,
				MaskBits:     0xFFFF,
			},
			physics.ColliderShape{
				ShapeType:    physics.ShapeTypeCircle,
				Radius:       0.25,
				Density:      1,
				LocalOffset:  physics.Vec2{X: 1, Y: 0},
				CategoryBits: 0xFFFF,
				MaskBits:     0xFFFF,
			},
		))
		*entityID = id
	}, cardinal.WithHook(cardinal.Init))
	return entityID
}

// TestLookup_BodyAndShapeIDsForSpawnedEntity checks the lookups resolve to the engine objects
// the reconciler actually created for the entity (verified through Box2D userdata).
func TestLookup_BodyAndShapeIDsForSpawnedEntity(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{X: 0, Y: 0})
	entityID := spawnTwoShapeBody(t, w)

	initCardinalECS(w)
	tickN(t, w, 3)

	engine := p.Engine()
	require.NotNil(t, engine, "world should exist after reconcile")

	bodyID, ok := p.BodyID(*entityID)
	require.True(t, ok, "spawned entity should have a body")
	require.Equal(t, *entityID, cardinal.EntityID(uint32(engine.BodyUserData(bodyID))),
		"body userdata should carry the entity id")
	require.Equal(t, 2, engine.BodyShapeCount(bodyID))

	shapeIDs, ok := p.ShapeIDs(*entityID)
	require.True(t, ok, "spawned entity should have shape ids")
	require.Len(t, shapeIDs, 2, "one shape id per collider slot")
	for slot, shapeID := range shapeIDs {
		require.Equal(t, bodyID, engine.ShapeBody(shapeID), "slot %d belongs to the entity body", slot)
		require.Equal(t, slot, int(uint32(engine.ShapeUserData(shapeID))),
			"shape userdata should carry the collider slot index")
	}
}

// TestLookup_UnknownAndDestroyedEntity checks both lookups report ok=false for entities the
// runtime does not track.
func TestLookup_UnknownAndDestroyedEntity(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{X: 0, Y: 0})
	entityID := spawnTwoShapeBody(t, w)

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() == 5 {
			require.True(t, state.Spawn.Destroy(*entityID), "Destroy(lookup entity)")
		}
	}, cardinal.WithHook(cardinal.Update))

	initCardinalECS(w)
	tickN(t, w, 3)

	// Never-spawned entity id.
	unknown := cardinal.EntityID(999_999)
	_, ok := p.BodyID(unknown)
	require.False(t, ok, "unknown entity has no body")
	shapeIDs, ok := p.ShapeIDs(unknown)
	require.False(t, ok, "unknown entity has no shapes")
	require.Nil(t, shapeIDs)

	// Destroyed entity: reconcile drops the body on the tick after Destroy.
	tickN(t, w, 5)
	_, ok = p.BodyID(*entityID)
	require.False(t, ok, "destroyed entity should no longer resolve to a body")
	shapeIDs, ok = p.ShapeIDs(*entityID)
	require.False(t, ok, "destroyed entity should no longer resolve to shapes")
	require.Nil(t, shapeIDs)
}

// TestLookup_ShapeIDsReturnsCopy checks the caller cannot corrupt the runtime's own slice.
func TestLookup_ShapeIDsReturnsCopy(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{X: 0, Y: 0})
	entityID := spawnTwoShapeBody(t, w)

	initCardinalECS(w)
	tickN(t, w, 3)

	first, ok := p.ShapeIDs(*entityID)
	require.True(t, ok)
	require.Len(t, first, 2)

	original := make([]box2d.ShapeID, len(first))
	copy(original, first)

	// Scribble over the returned slice. Guard that zeroing is a real change, so the final
	// comparison cannot pass trivially.
	zeroed := make([]box2d.ShapeID, len(first))
	require.NotEqual(t, zeroed, original, "shape ids should not already be zero values")
	copy(first, zeroed)

	second, ok := p.ShapeIDs(*entityID)
	require.True(t, ok)
	require.Equal(t, original, second, "mutating the returned slice must not affect the plugin")
}

// TestLookup_EngineNilBeforeInitAndAfterReset pins the Engine() lifetime contract that the
// lookups inherit.
func TestLookup_EngineNilBeforeInitAndAfterReset(t *testing.T) {
	t.Parallel()

	// Unregistered plugin: no runtime at all.
	require.Nil(t, physics.NewPlugin(physics.Config{}).Engine())

	w, p := makeWorld(t, physics.Vec2{X: 0, Y: 0})
	entityID := spawnTwoShapeBody(t, w)

	// Registered but not yet reconciled: no world.
	require.Nil(t, p.Engine(), "no world before the first reconcile")
	_, ok := p.BodyID(*entityID)
	require.False(t, ok, "no lookups before the first reconcile")

	initCardinalECS(w)
	tickN(t, w, 3)
	require.NotNil(t, p.Engine())

	p.Reset()
	require.Nil(t, p.Engine(), "no world after Reset")
	_, ok = p.BodyID(*entityID)
	require.False(t, ok, "no lookups after Reset")
	shapeIDs, ok := p.ShapeIDs(*entityID)
	require.False(t, ok)
	require.Nil(t, shapeIDs)
}
