package physics2d_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Raycast: closest hit returned, further hits ignored
// ---------------------------------------------------------------------------

func TestQuery_RaycastClosestHit(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var nearID, farID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Near wall at X=5.
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "near"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 5, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 0.5, Y: 2},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		nearID = id

		// Far wall at X=15.
		id2, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "far"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 15, Y: 0}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 0.5, Y: 2},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		farID = id2
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	ray := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 0},
		End:    physics.Vec2{X: 20, Y: 0},
	})
	require.True(t, ray.Hit)
	require.Equal(t, nearID, ray.Entity, "should hit nearest wall")
	require.Greater(t, ray.Fraction, 0.0)
	require.Less(t, ray.Fraction, 1.0)
	_ = farID
}

// ---------------------------------------------------------------------------
// Raycast: miss when no fixtures in path
// ---------------------------------------------------------------------------

func TestQuery_RaycastMiss(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		_, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "wall"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 100, Y: 100}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 1, Y: 1},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	ray := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 0},
		End:    physics.Vec2{X: 10, Y: 0},
	})
	require.False(t, ray.Hit, "ray should miss when nothing in path")
}

// ---------------------------------------------------------------------------
// Raycast: zero-length segment returns no hit
// ---------------------------------------------------------------------------

func TestQuery_RaycastZeroLength(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		_, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "box"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 5, Y: 5},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	ray := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 0},
		End:    physics.Vec2{X: 0, Y: 0},
	})
	require.False(t, ray.Hit, "zero-length ray should return no hit")
}

// ---------------------------------------------------------------------------
// Raycast: hit point and normal populated
// ---------------------------------------------------------------------------

func TestQuery_RaycastHitPointAndNormal(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		_, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "wall"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 5, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 0.5, Y: 5},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	ray := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 0},
		End:    physics.Vec2{X: 10, Y: 0},
	})
	require.True(t, ray.Hit)
	// Hit point X should be at wall's left face (X=4.5).
	require.InDelta(t, 4.5, ray.Point.X, 0.1, "hit point X near wall face")
	// Normal should point left (toward ray origin).
	require.InDelta(t, -1.0, ray.Normal.X, 0.1, "normal X should point left")
	require.InDelta(t, 0.0, ray.Normal.Y, 0.1, "normal Y should be ~0")
}

// ---------------------------------------------------------------------------
// Raycast: nil runtime (ResetRuntime) returns no hit
// ---------------------------------------------------------------------------

func TestQuery_RaycastAfterResetRuntime(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		_, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "box"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 5, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, boxColliderShapes(1, 1)...))
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	// Confirm hit before reset.
	ray := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 0},
		End:    physics.Vec2{X: 10, Y: 0},
	})
	require.True(t, ray.Hit, "hit before reset")

	// Reset and query immediately — world is nil.
	physics.ResetRuntime()
	rayAfter := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 0},
		End:    physics.Vec2{X: 10, Y: 0},
	})
	require.False(t, rayAfter.Hit, "no hit after reset (world nil)")

	// After ticking, world should be rebuilt.
	tickN(t, w, 3)
	rayRebuilt := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 0},
		End:    physics.Vec2{X: 10, Y: 0},
	})
	require.True(t, rayRebuilt.Hit, "hit after world rebuilt")
}

// ---------------------------------------------------------------------------
// AABB: swapped min/max handled correctly
// ---------------------------------------------------------------------------

func TestQuery_AABBSwappedMinMax(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var boxID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "box"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 1, Y: 1},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		boxID = id
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	// Swapped: Min > Max on both axes.
	ov := physics.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: 2, Y: 2},
		Max: physics.Vec2{X: -2, Y: -2},
	})
	found := false
	for _, h := range ov.Hits {
		if h.Entity == boxID {
			found = true
		}
	}
	require.True(t, found, "swapped min/max should auto-correct and find box")
}

// ---------------------------------------------------------------------------
// AABB: empty region (zero area) returns nothing
// ---------------------------------------------------------------------------

func TestQuery_AABBZeroArea(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		_, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "box"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, boxColliderShapes(5, 5)...))
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	// Point AABB (zero area).
	ov := physics.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: 0, Y: 0},
		Max: physics.Vec2{X: 0, Y: 0},
	})
	require.Empty(t, ov.Hits, "zero-area AABB should return no hits")
}

// ---------------------------------------------------------------------------
// CircleSweep: hit closest fixture
// ---------------------------------------------------------------------------

func TestQuery_CircleSweepClosestHit(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var nearID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "near"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 5, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 0.5, Y: 2},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		nearID = id

		_, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "far"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 15, Y: 0}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 0.5, Y: 2},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	sweep := physics.CircleSweep(physics.CircleSweepRequest{
		Start:  physics.Vec2{X: 0, Y: 0},
		End:    physics.Vec2{X: 20, Y: 0},
		Radius: 0.3,
	})
	require.True(t, sweep.Hit)
	require.Equal(t, nearID, sweep.Entity, "sweep hits nearest fixture")
}

// ---------------------------------------------------------------------------
// CircleSweep: zero radius returns no hit
// ---------------------------------------------------------------------------

func TestQuery_CircleSweepZeroRadius(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		_, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "wall"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 5, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, boxColliderShapes(2, 2)...))
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	sweep := physics.CircleSweep(physics.CircleSweepRequest{
		Start:  physics.Vec2{X: 0, Y: 0},
		End:    physics.Vec2{X: 10, Y: 0},
		Radius: 0,
	})
	require.False(t, sweep.Hit, "zero radius sweep returns no hit")
}

// ---------------------------------------------------------------------------
// CircleSweep: zero-length segment returns no hit
// ---------------------------------------------------------------------------

func TestQuery_CircleSweepZeroLength(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		_, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "wall"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, boxColliderShapes(5, 5)...))
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	sweep := physics.CircleSweep(physics.CircleSweepRequest{
		Start:  physics.Vec2{X: 0, Y: 0},
		End:    physics.Vec2{X: 0, Y: 0},
		Radius: 1.0,
	})
	require.False(t, sweep.Hit, "zero-length sweep returns no hit")
}

// ---------------------------------------------------------------------------
// CircleSweep: MaxFraction limits search distance
// ---------------------------------------------------------------------------

func TestQuery_CircleSweepMaxFraction(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Wall at X=10 (far away).
		_, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "far_wall"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 10, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 0.5, Y: 2},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	// Full sweep should hit.
	sweepFull := physics.CircleSweep(physics.CircleSweepRequest{
		Start:  physics.Vec2{X: 0, Y: 0},
		End:    physics.Vec2{X: 20, Y: 0},
		Radius: 0.2,
	})
	require.True(t, sweepFull.Hit, "full sweep should hit")

	// MaxFraction=0.2 limits to first 20% of segment (X=0 to X=4) — should miss wall at X=10.
	sweepLimited := physics.CircleSweep(physics.CircleSweepRequest{
		Start:       physics.Vec2{X: 0, Y: 0},
		End:         physics.Vec2{X: 20, Y: 0},
		Radius:      0.2,
		MaxFraction: 0.2,
	})
	require.False(t, sweepLimited.Hit, "limited sweep should miss far wall")
}

// ---------------------------------------------------------------------------
// Raycast: ShapeIndex correctly reported on compound body
// ---------------------------------------------------------------------------

func TestQuery_RaycastShapeIndex(t *testing.T) {
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
		row.Tag.Set(harnessTag{Role: "compound"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic,
			// Shape 0: box at center (X=-1..1).
			physics.ColliderShape{
				ShapeType:    physics.ShapeTypeBox,
				HalfExtents:  physics.Vec2{X: 1, Y: 1},
				CategoryBits: 0xFFFF,
				MaskBits:     0xFFFF,
			},
			// Shape 1: circle at X=10.
			physics.ColliderShape{
				ShapeType:    physics.ShapeTypeCircle,
				Radius:       1.0,
				LocalOffset:  physics.Vec2{X: 10, Y: 0},
				CategoryBits: 0xFFFF,
				MaskBits:     0xFFFF,
			},
		))
		bodyID = id
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	// Ray toward center → shape 0.
	rayCenter := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: -5, Y: 0},
		End:    physics.Vec2{X: 5, Y: 0},
	})
	require.True(t, rayCenter.Hit)
	require.Equal(t, bodyID, rayCenter.Entity)
	require.Equal(t, 0, rayCenter.ShapeIndex, "center ray should hit shape 0")

	// Ray toward X=10 → shape 1.
	rayRight := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 5, Y: 0},
		End:    physics.Vec2{X: 15, Y: 0},
	})
	require.True(t, rayRight.Hit)
	require.Equal(t, bodyID, rayRight.Entity)
	require.Equal(t, 1, rayRight.ShapeIndex, "right ray should hit shape 1")
}

// ---------------------------------------------------------------------------
// AABB: multiple distinct entities returned
// ---------------------------------------------------------------------------

func TestQuery_AABBMultipleEntities(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var id1, id2, id3 cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		a, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "a"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, boxColliderShapes(1, 1)...))
		id1 = a

		b, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "b"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 3, Y: 0}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(newRigid(physics.BodyTypeStatic, boxColliderShapes(1, 1)...))
		id2 = b

		c, row3 := state.Spawn.Create()
		row3.Tag.Set(harnessTag{Role: "c"})
		row3.T.Set(physics.Transform2D{Position: physics.Vec2{X: 100, Y: 100}})
		row3.V.Set(physics.Velocity2D{})
		row3.PB.Set(newRigid(physics.BodyTypeStatic, boxColliderShapes(1, 1)...))
		id3 = c
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	ov := physics.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: -5, Y: -5},
		Max: physics.Vec2{X: 5, Y: 5},
	})

	entities := make(map[cardinal.EntityID]bool)
	for _, h := range ov.Hits {
		entities[h.Entity] = true
	}
	require.True(t, entities[id1], "entity 1 found")
	require.True(t, entities[id2], "entity 2 found")
	require.False(t, entities[id3], "entity 3 NOT found (far away)")
}

// ---------------------------------------------------------------------------
// Body() API — returns Box2D body for entity, nil for unknown
// ---------------------------------------------------------------------------

func TestQuery_BodyAPIAccess(t *testing.T) {
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
		row.Tag.Set(harnessTag{Role: "ball"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 10}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeDynamic, circleColliderShapes()...))
		ballID = id
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 5)

	// Body for known entity should be non-nil.
	body := physics.Body(ballID)
	require.NotNil(t, body, "Body() should return non-nil for physics entity")

	// Check some read-only queries.
	pos := body.GetPosition()
	require.Less(t, pos.Y, 10.0, "body should have fallen")
	require.True(t, body.IsAwake(), "dynamic body should be awake")

	// Body for unknown entity should be nil.
	unknown := physics.Body(99999)
	require.Nil(t, unknown, "Body() for unknown entity should be nil")
}

// ---------------------------------------------------------------------------
// PhysicsWorld() — returns world or nil
// ---------------------------------------------------------------------------

func TestQuery_PhysicsWorldAPI(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		_, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "dummy"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, boxColliderShapes(1, 1)...))
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	world := physics.PhysicsWorld()
	require.NotNil(t, world, "world should exist after init")

	physics.ResetRuntime()
	worldAfterReset := physics.PhysicsWorld()
	require.Nil(t, worldAfterReset, "world should be nil after reset")

	tickN(t, w, 3)
	worldRebuilt := physics.PhysicsWorld()
	require.NotNil(t, worldRebuilt, "world should be rebuilt after ticks")
}
