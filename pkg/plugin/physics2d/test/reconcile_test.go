package physics2d_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Reconcile: entity destroyed → body removed from queries
// ---------------------------------------------------------------------------

func TestReconcile_DestroyEntityRemovesBody(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var entityID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "doomed"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 2, Y: 2},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		entityID = id
	}, cardinal.WithHook(cardinal.Init))

	foundBefore := false
	foundAfter := false

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() == 10 {
			state.Spawn.Destroy(entityID)
		}
	}, cardinal.WithHook(cardinal.Update))

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
	}) {
		ov := physics.OverlapAABB(physics.AABBOverlapRequest{
			Min: physics.Vec2{X: -3, Y: -3},
			Max: physics.Vec2{X: 3, Y: 3},
		})
		for _, h := range ov.Hits {
			if h.Entity == entityID {
				if state.Tick() == 5 {
					foundBefore = true
				}
				if state.Tick() >= 12 {
					foundAfter = true
				}
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 15)

	require.True(t, foundBefore, "entity visible before destroy")
	require.False(t, foundAfter, "entity gone after destroy")
}

// ---------------------------------------------------------------------------
// Reconcile: ECS transform change → Box2D position updated
// ---------------------------------------------------------------------------

func TestReconcile_TransformChangeMovesBody(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var entityID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "mover"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 1, Y: 1},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		entityID = id
	}, cardinal.WithHook(cardinal.Init))

	// Move entity at tick 10.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 10 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == entityID {
				tr := row.T.Get()
				tr.Position = physics.Vec2{X: 50, Y: 0}
				row.T.Set(tr)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	hitAtOldPos := false
	hitAtNewPos := false

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
	}) {
		if state.Tick() != 15 {
			return
		}
		// Should NOT be at old position.
		ovOld := physics.OverlapAABB(physics.AABBOverlapRequest{
			Min: physics.Vec2{X: -2, Y: -2},
			Max: physics.Vec2{X: 2, Y: 2},
		})
		for _, h := range ovOld.Hits {
			if h.Entity == entityID {
				hitAtOldPos = true
			}
		}

		// Should be at new position.
		ovNew := physics.OverlapAABB(physics.AABBOverlapRequest{
			Min: physics.Vec2{X: 48, Y: -2},
			Max: physics.Vec2{X: 52, Y: 2},
		})
		for _, h := range ovNew.Hits {
			if h.Entity == entityID {
				hitAtNewPos = true
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 16)

	require.False(t, hitAtOldPos, "entity should not be at old position")
	require.True(t, hitAtNewPos, "entity should be at new position after transform change")
}

// ---------------------------------------------------------------------------
// Reconcile: new entity created mid-sim → Box2D body appears
// ---------------------------------------------------------------------------

func TestReconcile_MidSimEntityCreation(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var newID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Start with an empty world (no physics entities).
	}, cardinal.WithHook(cardinal.Init))

	// Create entity at tick 10.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 10 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "new_entity"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 2, Y: 2},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		newID = id
	}, cardinal.WithHook(cardinal.Update))

	found := false
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
	}) {
		if state.Tick() < 12 {
			return
		}
		ov := physics.OverlapAABB(physics.AABBOverlapRequest{
			Min: physics.Vec2{X: -3, Y: -3},
			Max: physics.Vec2{X: 3, Y: 3},
		})
		for _, h := range ov.Hits {
			if h.Entity == newID {
				found = true
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 15)

	require.True(t, found, "mid-sim entity should be detectable by query")
}

// ---------------------------------------------------------------------------
// Reconcile: mutable fixture change (friction) — no structural rebuild
// ---------------------------------------------------------------------------

func TestReconcile_MutableFrictionChange(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var entityID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "friction_box"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 2, Y: 2},
			Friction:     0.3,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		entityID = id
	}, cardinal.WithHook(cardinal.Init))

	// Change friction at tick 10 — mutable change, no fixture rebuild.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 10 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == entityID {
				pb := row.PB.Get()
				pb.Shapes[0].Friction = 0.9
				row.PB.Set(pb)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	// Verify entity still queryable after mutable change (no accidental destruction).
	found := false
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
	}) {
		if state.Tick() != 15 {
			return
		}
		ov := physics.OverlapAABB(physics.AABBOverlapRequest{
			Min: physics.Vec2{X: -3, Y: -3},
			Max: physics.Vec2{X: 3, Y: 3},
		})
		for _, h := range ov.Hits {
			if h.Entity == entityID {
				found = true
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 16)

	require.True(t, found, "entity should remain after mutable friction change")
}

// ---------------------------------------------------------------------------
// Reconcile: structural shape change — radius change triggers fixture rebuild
// ---------------------------------------------------------------------------

func TestReconcile_StructuralRadiusChange(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var entityID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "resizable"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.5,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		entityID = id
	}, cardinal.WithHook(cardinal.Init))

	// Change radius at tick 10 → structural change.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 10 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == entityID {
				pb := row.PB.Get()
				pb.Shapes[0].Radius = 5.0
				row.PB.Set(pb)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	// After resize, a wider AABB query should find the entity.
	foundSmall := false
	foundLarge := false
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
	}) {
		if state.Tick() == 5 {
			// Before resize — small radius, check at X=3 should miss.
			ov := physics.OverlapAABB(physics.AABBOverlapRequest{
				Min: physics.Vec2{X: 2, Y: -1},
				Max: physics.Vec2{X: 4, Y: 1},
			})
			for _, h := range ov.Hits {
				if h.Entity == entityID {
					foundSmall = true
				}
			}
		}
		if state.Tick() == 15 {
			// After resize to R=5, check at X=3 should hit.
			ov := physics.OverlapAABB(physics.AABBOverlapRequest{
				Min: physics.Vec2{X: 2, Y: -1},
				Max: physics.Vec2{X: 4, Y: 1},
			})
			for _, h := range ov.Hits {
				if h.Entity == entityID {
					foundLarge = true
				}
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 16)

	require.False(t, foundSmall, "small circle should not reach X=3")
	require.True(t, foundLarge, "large circle (R=5) should reach X=3")
}

// ---------------------------------------------------------------------------
// Reconcile: add second shape (shape count change → structural rebuild)
// ---------------------------------------------------------------------------

func TestReconcile_AddShape(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var entityID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "expandable"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 1, Y: 1},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		entityID = id
	}, cardinal.WithHook(cardinal.Init))

	// Add second shape at X=20 at tick 10.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 10 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == entityID {
				pb := row.PB.Get()
				pb.Shapes = append(pb.Shapes, physics.ColliderShape{
					ShapeType:    physics.ShapeTypeCircle,
					Radius:       1.0,
					LocalOffset:  physics.Vec2{X: 20, Y: 0},
					CategoryBits: 0xFFFF,
					MaskBits:     0xFFFF,
				})
				row.PB.Set(pb)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	foundNewShape := false
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
	}) {
		if state.Tick() != 15 {
			return
		}
		ov := physics.OverlapAABB(physics.AABBOverlapRequest{
			Min: physics.Vec2{X: 18, Y: -2},
			Max: physics.Vec2{X: 22, Y: 2},
		})
		for _, h := range ov.Hits {
			if h.Entity == entityID && h.ShapeIndex == 1 {
				foundNewShape = true
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 16)

	require.True(t, foundNewShape, "new shape should be detectable after add")
}

// ---------------------------------------------------------------------------
// Reconcile: damping change mid-sim (body param change)
// ---------------------------------------------------------------------------

func TestReconcile_DampingChangeMidSim(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var entityID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "damping_change"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{Linear: physics.Vec2{X: 10, Y: 0}})
		row.PB.Set(newRigidNoGravity(physics.BodyTypeDynamic, circleColliderShapes()...))
		entityID = id
	}, cardinal.WithHook(cardinal.Init))

	// Add heavy damping at tick 30.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 30 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == entityID {
				pb := row.PB.Get()
				pb.LinearDamping = 10.0
				row.PB.Set(pb)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	posAtChange := 0.0
	var finalPos float64
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		for eid, row := range state.Spawn.Iter() {
			if eid == entityID {
				if state.Tick() == 30 {
					posAtChange = row.T.Get().Position.X
				}
				if state.Tick() == 90 {
					finalPos = row.T.Get().Position.X
				}
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 91)

	// Body should have moved before damping change.
	require.Greater(t, posAtChange, 3.0, "body moved before damping")
	// After heavy damping, speed decreases rapidly — final position not much further.
	require.Less(t, finalPos-posAtChange, posAtChange,
		"distance after damping should be much less than distance before")
}

// ---------------------------------------------------------------------------
// Reconcile: velocity change in ECS for dynamic body → Box2D velocity updated
// ---------------------------------------------------------------------------

func TestReconcile_VelocityChangeInECS(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var entityID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "vel_change"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{Linear: physics.Vec2{X: 5, Y: 0}})
		row.PB.Set(newRigidNoGravity(physics.BodyTypeDynamic, circleColliderShapes()...))
		entityID = id
	}, cardinal.WithHook(cardinal.Init))

	// Reverse velocity at tick 30.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 30 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == entityID {
				row.V.Set(physics.Velocity2D{Linear: physics.Vec2{X: -5, Y: 0}})
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	posAtReverse := 0.0
	var finalPos float64
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		for eid, row := range state.Spawn.Iter() {
			if eid == entityID {
				if state.Tick() == 30 {
					posAtReverse = row.T.Get().Position.X
				}
				if state.Tick() == 90 {
					finalPos = row.T.Get().Position.X
				}
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 91)

	// Body moved right before reverse.
	require.Greater(t, posAtReverse, 1.0, "body moved right")
	// After reverse, body should have moved left — final X less than at reverse.
	require.Less(t, finalPos, posAtReverse, "body should move back after velocity reversal")
}

// ---------------------------------------------------------------------------
// Reconcile: rotation change in ECS → Box2D rotation updated
// ---------------------------------------------------------------------------

func TestReconcile_RotationChange(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var entityID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "rotator"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}, Rotation: 0})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 5, Y: 0.1},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		entityID = id
	}, cardinal.WithHook(cardinal.Init))

	// Rotate box 90 degrees at tick 10 (5 tall, 0.1 wide → becomes 0.1 tall, 5 wide).
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 10 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == entityID {
				tr := row.T.Get()
				tr.Rotation = 1.5708 // ~π/2
				row.T.Set(tr)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	// Before rotation: horizontal ray at Y=3 should miss (box is 0.1 tall, spans Y=-0.1..0.1).
	// After rotation: horizontal ray at Y=3 should HIT (box now spans Y=-5..5, X=-0.1..0.1).
	// Note: vertical rays fail here because Box2D raycasts don't detect shapes whose interior
	// contains the ray origin, which happens when the ray starts inside the rotated box.
	hitBefore := false
	hitAfter := false
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
	}) {
		ray := physics.Raycast(physics.RaycastRequest{
			Origin: physics.Vec2{X: -5, Y: 3},
			End:    physics.Vec2{X: 5, Y: 3},
		})
		if state.Tick() == 5 {
			hitBefore = ray.Hit && ray.Entity == entityID
		}
		if state.Tick() == 15 {
			hitAfter = ray.Hit && ray.Entity == entityID
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 16)

	require.False(t, hitBefore, "unrotated thin box should not be hit at Y=3")
	require.True(t, hitAfter, "rotated box should be hit at Y=3")
}
