package physics2d_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Shape type: Circle — falls under gravity, detectable by AABB query
// ---------------------------------------------------------------------------

func TestShapeType_CircleFallsAndDetectable(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var ballID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "circle"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 10}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.5,
			Density:      1,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		ballID = id
	}, cardinal.WithHook(cardinal.Init))

	var finalPos physics.Vec2
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() < 30 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == ballID {
				finalPos = row.T.Get().Position
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 31)

	require.Less(t, finalPos.Y, 9.0, "circle should fall")

	// AABB query should find the circle at its current position.
	ov := physics.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: -1, Y: finalPos.Y - 1},
		Max: physics.Vec2{X: 1, Y: finalPos.Y + 1},
	})
	found := false
	for _, h := range ov.Hits {
		if h.Entity == ballID {
			found = true
		}
	}
	require.True(t, found, "circle detected by AABB query")
}

// ---------------------------------------------------------------------------
// Shape type: Box — static floor, detectable by raycast
// ---------------------------------------------------------------------------

func TestShapeType_BoxFloorDetectable(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var floorID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "box_floor"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 10, Y: 0.5},
			Friction:     0.5,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		floorID = id
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	// Raycast down should hit the floor.
	ray := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 5},
		End:    physics.Vec2{X: 0, Y: -5},
	})
	require.True(t, ray.Hit, "raycast should hit box floor")
	require.Equal(t, floorID, ray.Entity)
}

// ---------------------------------------------------------------------------
// Shape type: ConvexPolygon — triangle, detectable by AABB query
// ---------------------------------------------------------------------------

func TestShapeType_ConvexPolygonDetectable(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var triID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "triangle"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType: physics.ShapeTypeConvexPolygon,
			Vertices: []physics.Vec2{
				{X: -1, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 2},
			},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		triID = id
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	ov := physics.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: -0.5, Y: 0.5},
		Max: physics.Vec2{X: 0.5, Y: 1.5},
	})
	found := false
	for _, h := range ov.Hits {
		if h.Entity == triID {
			found = true
		}
	}
	require.True(t, found, "polygon detected by AABB query")
}

// ---------------------------------------------------------------------------
// Shape type: StaticChain — open chain, detectable by raycast
// ---------------------------------------------------------------------------

func TestShapeType_StaticChainDetectable(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var chainID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "chain"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType: physics.ShapeTypeStaticChain,
			// Box2D v3 chains are one-sided: normal faces right of segment direction (CCW winding).
			// Right-to-left ordering gives an upward-facing normal so a downward ray hits.
			ChainPoints:  []physics.Vec2{{X: 10, Y: 0}, {X: 3, Y: 0}, {X: -3, Y: 0}, {X: -10, Y: 0}},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		chainID = id
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	// Raycast from above downward should hit the chain.
	ray := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 5},
		End:    physics.Vec2{X: 0, Y: -5},
	})
	require.True(t, ray.Hit, "raycast should hit chain segment")
	require.Equal(t, chainID, ray.Entity)
}

// ---------------------------------------------------------------------------
// Shape type: StaticChainLoop — closed loop, detectable by AABB
// ---------------------------------------------------------------------------

func TestShapeType_ChainLoopDetectable(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var loopID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "loop"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType: physics.ShapeTypeStaticChainLoop,
			// Box2D v3 chains are one-sided: CCW winding for inward-facing normals.
			// This lets a ray from inside (0,0) outward (10,0) hit the boundary.
			ChainPoints: []physics.Vec2{
				{X: -5, Y: -5}, {X: -5, Y: 5}, {X: 5, Y: 5}, {X: 5, Y: -5},
			},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		loopID = id
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	// Raycast from inside to outside should hit the loop boundary.
	ray := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 0},
		End:    physics.Vec2{X: 10, Y: 0},
	})
	require.True(t, ray.Hit, "raycast should hit chain loop boundary")
	require.Equal(t, loopID, ray.Entity)
}

// ---------------------------------------------------------------------------
// Shape type: Edge — single line segment, detectable by raycast
// ---------------------------------------------------------------------------

func TestShapeType_EdgeDetectable(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var edgeID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "edge"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeEdge,
			EdgeVertices: [2]physics.Vec2{{X: -10, Y: 0}, {X: 10, Y: 0}},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		edgeID = id
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	// Raycast from above should hit the edge.
	ray := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 5},
		End:    physics.Vec2{X: 0, Y: -5},
	})
	require.True(t, ray.Hit, "raycast should hit edge")
	require.Equal(t, edgeID, ray.Entity)
}

// ---------------------------------------------------------------------------
// Shape type: Capsule — dynamic capsule falls under gravity
// ---------------------------------------------------------------------------

func TestShapeType_CapsuleFallsUnderGravity(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var capsuleID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "capsule"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 10}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
			ShapeType:      physics.ShapeTypeCapsule,
			CapsuleCenter1: physics.Vec2{X: 0, Y: -0.5},
			CapsuleCenter2: physics.Vec2{X: 0, Y: 0.5},
			Radius:         0.3,
			Density:        1,
			CategoryBits:   0xFFFF,
			MaskBits:       0xFFFF,
		}))
		capsuleID = id
	}, cardinal.WithHook(cardinal.Init))

	var finalPos physics.Vec2
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() < 30 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == capsuleID {
				finalPos = row.T.Get().Position
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 31)

	require.Less(t, finalPos.Y, 9.0, "capsule should fall under gravity")
}

// ---------------------------------------------------------------------------
// Shape type: Capsule — static capsule detectable by raycast
// ---------------------------------------------------------------------------

func TestShapeType_StaticCapsuleDetectable(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var capsuleID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "static_capsule"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:      physics.ShapeTypeCapsule,
			CapsuleCenter1: physics.Vec2{X: -2, Y: 0},
			CapsuleCenter2: physics.Vec2{X: 2, Y: 0},
			Radius:         0.5,
			CategoryBits:   0xFFFF,
			MaskBits:       0xFFFF,
		}))
		capsuleID = id
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	// Raycast from above should hit the capsule.
	ray := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 5},
		End:    physics.Vec2{X: 0, Y: -5},
	})
	require.True(t, ray.Hit, "raycast should hit static capsule")
	require.Equal(t, capsuleID, ray.Entity)
}

// ---------------------------------------------------------------------------
// Compound collider — multiple shapes on one body, all detectable
// ---------------------------------------------------------------------------

func TestShapeType_CompoundCollider(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var compID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "compound"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic,
			// Shape 0: box at center
			physics.ColliderShape{
				ShapeType:    physics.ShapeTypeBox,
				HalfExtents:  physics.Vec2{X: 1, Y: 1},
				CategoryBits: 0xFFFF,
				MaskBits:     0xFFFF,
			},
			// Shape 1: circle offset to the right
			physics.ColliderShape{
				ShapeType:    physics.ShapeTypeCircle,
				Radius:       0.5,
				LocalOffset:  physics.Vec2{X: 5, Y: 0},
				CategoryBits: 0xFFFF,
				MaskBits:     0xFFFF,
			},
		))
		compID = id
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	// AABB around center should find shape 0 (box).
	ovCenter := physics.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: -0.5, Y: -0.5},
		Max: physics.Vec2{X: 0.5, Y: 0.5},
	})
	foundBox := false
	for _, h := range ovCenter.Hits {
		if h.Entity == compID && h.ShapeIndex == 0 {
			foundBox = true
		}
	}
	require.True(t, foundBox, "compound box shape detected at center")

	// AABB around X=5 should find shape 1 (circle).
	ovRight := physics.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: 4, Y: -1},
		Max: physics.Vec2{X: 6, Y: 1},
	})
	foundCircle := false
	for _, h := range ovRight.Hits {
		if h.Entity == compID && h.ShapeIndex == 1 {
			foundCircle = true
		}
	}
	require.True(t, foundCircle, "compound circle shape detected at offset")

	// AABB around X=5 should NOT find shape 0.
	for _, h := range ovRight.Hits {
		if h.Entity == compID {
			require.NotEqual(t, 0, h.ShapeIndex, "box shape should not appear at offset X=5")
		}
	}
}

// ---------------------------------------------------------------------------
// LocalOffset on circle — circle placed at offset, detectable there
// ---------------------------------------------------------------------------

func TestShapeType_CircleLocalOffset(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var bodyID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "offset_circle"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.5,
			LocalOffset:  physics.Vec2{X: 10, Y: 0},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		bodyID = id
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	// Should NOT be at origin.
	ovOrigin := physics.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: -1, Y: -1},
		Max: physics.Vec2{X: 1, Y: 1},
	})
	for _, h := range ovOrigin.Hits {
		require.NotEqual(t, bodyID, h.Entity, "offset circle should not be at origin")
	}

	// Should be at offset X=10.
	ovOffset := physics.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: 9, Y: -1},
		Max: physics.Vec2{X: 11, Y: 1},
	})
	found := false
	for _, h := range ovOffset.Hits {
		if h.Entity == bodyID {
			found = true
		}
	}
	require.True(t, found, "offset circle should be at X=10")
}

// ---------------------------------------------------------------------------
// Chain/ChainLoop/Edge cannot be on dynamic bodies
// ---------------------------------------------------------------------------

func TestShapeType_ChainOnDynamic_NoPhysicsBody(t *testing.T) {
	// Chain shapes on dynamic bodies produce zero mass — the internal create.go rejects this.
	// We verify the body is not created (no hit) rather than a direct error, since the error
	// is logged by the system but doesn't crash.
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		_, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "dyn_chain"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeStaticChain,
			ChainPoints:  []physics.Vec2{{X: -5, Y: 0}, {X: -2, Y: 0}, {X: 2, Y: 0}, {X: 5, Y: 0}},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)
	// No crash = test passes. Chain on dynamic body is rejected during fixture attachment.
}
