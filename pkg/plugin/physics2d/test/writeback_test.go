// Package physics2d_test holds writeback and BodyTypeManual integration tests.
// Each test builds a fresh Cardinal world, registers the physics plugin, spawns
// entities, ticks, and asserts ECS components reflect (or don't reflect) Box2D state.
package physics2d_test

import (
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test: Dynamic body writeback — gravity pulls ball down, ECS reflects it
// ---------------------------------------------------------------------------

func TestWriteback_DynamicGravity(t *testing.T) {
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
		row.Tag.Set(harnessTag{Role: "dyn_ball"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 10}})
		row.V.Set(physics.Velocity2D{})
		row.R.Set(newRigid(physics.BodyTypeDynamic))
		row.C.Set(circleCollider())
		ballID = id
	}, cardinal.WithHook(cardinal.Init))

	// Read ECS after writeback.
	var finalPos physics.Vec2
	var finalVel physics.Vec2
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
				finalVel = row.V.Get().Linear
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 31)

	// After 30 ticks at 60Hz with gravity -10, ball should have fallen significantly.
	require.Less(t, finalPos.Y, 9.0, "dynamic ball should have fallen from Y=10")
	require.Less(t, finalVel.Y, -0.5, "dynamic ball should have downward velocity")
}

// ---------------------------------------------------------------------------
// Test: Static body — no writeback, position unchanged
// ---------------------------------------------------------------------------

func TestWriteback_StaticNoWriteback(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var floorID cardinal.EntityID
	spawnPos := physics.Vec2{X: 5, Y: -1}

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "floor"})
		row.T.Set(physics.Transform2D{Position: spawnPos})
		row.V.Set(physics.Velocity2D{})
		row.R.Set(newRigid(physics.BodyTypeStatic))
		row.C.Set(boxCollider(20, 0.5))
		floorID = id
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
			if eid == floorID {
				finalPos = row.T.Get().Position
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 31)

	approxVec2(t, finalPos, spawnPos, "static body position unchanged")
}

// ---------------------------------------------------------------------------
// Test: Kinematic body with velocity — Box2D integrates, writeback updates ECS
// ---------------------------------------------------------------------------

func TestWriteback_KinematicVelocityIntegration(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var kinID cardinal.EntityID
	startPos := physics.Vec2{X: 0, Y: 0}
	kinVel := physics.Vec2{X: 3, Y: 0} // 3 m/s rightward

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "kin_mover"})
		row.T.Set(physics.Transform2D{Position: startPos})
		row.V.Set(physics.Velocity2D{Linear: kinVel})
		row.R.Set(newRigid(physics.BodyTypeKinematic))
		row.C.Set(circleCollider())
		kinID = id
	}, cardinal.WithHook(cardinal.Init))

	var finalPos physics.Vec2
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() < 60 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == kinID {
				finalPos = row.T.Get().Position
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 61)

	// After 60 ticks at 60Hz = 1 second, at 3 m/s → X ≈ 3.0
	require.InDelta(t, 3.0, finalPos.X, 0.1, "kinematic body should move via velocity integration")
	require.InDelta(t, 0.0, finalPos.Y, epsilon, "kinematic body Y unchanged (no gravity effect)")
}

// ---------------------------------------------------------------------------
// Test: Manual body — ECS drives position, writeback skipped
// ---------------------------------------------------------------------------

func TestWriteback_ManualNoWriteback(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var manualID cardinal.EntityID
	spawnPos := physics.Vec2{X: 0, Y: 5}

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "manual_body"})
		row.T.Set(physics.Transform2D{Position: spawnPos})
		// Set non-zero velocity in ECS — should NOT be applied to Box2D.
		row.V.Set(physics.Velocity2D{Linear: physics.Vec2{X: 100, Y: 100}})
		row.R.Set(newRigid(physics.BodyTypeManual))
		row.C.Set(circleCollider())
		manualID = id
	}, cardinal.WithHook(cardinal.Init))

	var finalPos physics.Vec2
	var finalVel physics.Vec2
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() < 60 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == manualID {
				finalPos = row.T.Get().Position
				finalVel = row.V.Get().Linear
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 61)

	// Manual body: position must NOT have moved despite gravity and ECS velocity.
	approxVec2(t, finalPos, spawnPos, "manual body position unchanged after 60 ticks")
	// ECS velocity must remain what gameplay set (writeback didn't touch it).
	approxVec2(t, finalVel, physics.Vec2{X: 100, Y: 100}, "manual body ECS velocity preserved")
}

// ---------------------------------------------------------------------------
// Test: Manual body — gameplay moves position each tick, ECS stays authoritative
// ---------------------------------------------------------------------------

func TestWriteback_ManualGameplayDrivesPosition(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var manualID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "manual_mover"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.R.Set(newRigid(physics.BodyTypeManual))
		row.C.Set(circleCollider())
		manualID = id
	}, cardinal.WithHook(cardinal.Init))

	// Gameplay system moves the body 0.1 units right each tick (Update hook).
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		for eid, row := range state.Spawn.Iter() {
			if eid == manualID {
				tr := row.T.Get()
				tr.Position.X += 0.1
				row.T.Set(tr)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	var finalPos physics.Vec2
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() < 60 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == manualID {
				finalPos = row.T.Get().Position
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 61)

	// 60 ticks × 0.1 = 6.0 (tick 0 is init, so ~60 updates)
	require.InDelta(t, 6.0, finalPos.X, 0.5, "manual body X driven by gameplay")
	require.InDelta(t, 0.0, finalPos.Y, epsilon, "manual body Y unchanged (gravity ignored)")
}

// ---------------------------------------------------------------------------
// Test: Manual body does NOT drift — velocity zero'd prevents Box2D integration
// ---------------------------------------------------------------------------

func TestWriteback_ManualNoDrift(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var manualID cardinal.EntityID
	spawnPos := physics.Vec2{X: 5, Y: 5}

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "nodrift"})
		row.T.Set(physics.Transform2D{Position: spawnPos})
		// Large ECS velocity that must NOT cause Box2D drift.
		row.V.Set(physics.Velocity2D{Linear: physics.Vec2{X: 999, Y: 999}})
		row.R.Set(newRigid(physics.BodyTypeManual))
		row.C.Set(circleCollider())
		manualID = id
	}, cardinal.WithHook(cardinal.Init))

	// Check every tick that position hasn't drifted.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		for eid, row := range state.Spawn.Iter() {
			if eid == manualID {
				pos := row.T.Get().Position
				if math.Abs(pos.X-spawnPos.X) > epsilon || math.Abs(pos.Y-spawnPos.Y) > epsilon {
					testRequire.FailNowf("manual body drifted",
						"tick %d: pos=(%f,%f) want=(%f,%f)",
						state.Tick(), pos.X, pos.Y, spawnPos.X, spawnPos.Y)
				}
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	testRequire = require.New(t)
	t.Cleanup(func() { testRequire = nil })

	initCardinalECS(w)
	tickN(t, w, 120)
}

// ---------------------------------------------------------------------------
// Test: Dynamic body writeback updates shadow — reconciler doesn't fight
// ---------------------------------------------------------------------------

func TestWriteback_DynamicShadowSync(t *testing.T) {
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
		row.Tag.Set(harnessTag{Role: "shadow_ball"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 50}})
		row.V.Set(physics.Velocity2D{})
		row.R.Set(newRigid(physics.BodyTypeDynamic))
		row.C.Set(circleCollider())
		ballID = id
	}, cardinal.WithHook(cardinal.Init))

	// Record positions over time. If shadow is wrong, reconciler snaps body back,
	// causing Y to oscillate instead of monotonically decreasing.
	positions := make([]float64, 0, 120)
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		for eid, row := range state.Spawn.Iter() {
			if eid == ballID {
				positions = append(positions, row.T.Get().Position.Y)
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 120)

	require.GreaterOrEqual(t, len(positions), 100, "enough samples")
	// Y must be monotonically non-increasing (gravity pulls down, no floor to bounce).
	for i := 1; i < len(positions); i++ {
		require.LessOrEqual(t, positions[i], positions[i-1]+epsilon,
			"tick %d: Y should be non-increasing (shadow desync would cause snap-back)", i)
	}
	// Should have fallen significantly.
	require.Less(t, positions[len(positions)-1], 30.0, "ball should have fallen far")
}

// ---------------------------------------------------------------------------
// Test: Kinematic writeback doesn't fight reconciler (no snap-back)
// ---------------------------------------------------------------------------

func TestWriteback_KinematicNoSnapBack(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var kinID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "kin_shadow"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{Linear: physics.Vec2{X: 5, Y: 0}})
		row.R.Set(newRigid(physics.BodyTypeKinematic))
		row.C.Set(circleCollider())
		kinID = id
	}, cardinal.WithHook(cardinal.Init))

	positions := make([]float64, 0, 120)
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		for eid, row := range state.Spawn.Iter() {
			if eid == kinID {
				positions = append(positions, row.T.Get().Position.X)
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 120)

	require.GreaterOrEqual(t, len(positions), 100, "enough samples")
	// X must be monotonically non-decreasing (constant rightward velocity).
	for i := 1; i < len(positions); i++ {
		require.GreaterOrEqual(t, positions[i], positions[i-1]-epsilon,
			"tick %d: X should be non-decreasing (reconciler snap-back detected)", i)
	}
	// After 120 ticks at 60Hz = 2s at 5m/s → X ≈ 10.0
	require.Greater(t, positions[len(positions)-1], 8.0, "kinematic should have moved ~10 units")
}

// ---------------------------------------------------------------------------
// Test: Manual body + dynamic collectable — contact detection works
// ---------------------------------------------------------------------------

func TestWriteback_ManualDynamicContact(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var manualID, dynamicID cardinal.EntityID
	contactDetected := false

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Manual body (kinematic under the hood).
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "manual_player"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.R.Set(newRigid(physics.BodyTypeManual))
		row.C.Set(physics.Collider2D{Shapes: []physics.ColliderShape{{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.5,
			Density:      1,
			CategoryBits: 0x0001,
			MaskBits:     0xFFFF,
		}}})
		manualID = id

		// Dynamic sensor (like a collectable) — overlapping the manual body.
		id2, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "collectable"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 5, Y: 0}})
		row2.V.Set(physics.Velocity2D{})
		row2.R.Set(newRigidNoGravity(physics.BodyTypeDynamic))
		row2.C.Set(physics.Collider2D{Shapes: []physics.ColliderShape{{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.5,
			IsSensor:     true,
			Density:      1,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}}})
		dynamicID = id2
	}, cardinal.WithHook(cardinal.Init))

	// Move manual body toward the dynamic sensor.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		for eid, row := range state.Spawn.Iter() {
			if eid == manualID {
				tr := row.T.Get()
				tr.Position.X += 0.2 // move toward collectable at X=5
				row.T.Set(tr)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	// Listen for trigger events.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		TriggerBeginRx cardinal.WithSystemEventReceiver[physics.TriggerBeginEvent]
	}) {
		for e := range state.TriggerBeginRx.Iter() {
			if pairHas(e.EntityA, e.EntityB, manualID, dynamicID) {
				contactDetected = true
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 60)

	require.True(t, contactDetected,
		"manual (kinematic) body should trigger contact with dynamic sensor")
}

// ---------------------------------------------------------------------------
// Test: Body type change from Manual to Dynamic mid-sim
// ---------------------------------------------------------------------------

func TestWriteback_ManualToDynamic(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var entityID cardinal.EntityID
	spawnPos := physics.Vec2{X: 0, Y: 20}

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "type_switch"})
		row.T.Set(physics.Transform2D{Position: spawnPos})
		row.V.Set(physics.Velocity2D{})
		row.R.Set(newRigid(physics.BodyTypeManual))
		row.C.Set(circleCollider())
		entityID = id
	}, cardinal.WithHook(cardinal.Init))

	// At tick 30, switch from Manual to Dynamic. Body should start falling.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 30 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == entityID {
				rb := row.R.Get()
				rb.BodyType = physics.BodyTypeDynamic
				row.R.Set(rb)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	posAtSwitch := physics.Vec2{}
	var finalPos physics.Vec2
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		for eid, row := range state.Spawn.Iter() {
			if eid == entityID {
				pos := row.T.Get().Position
				if state.Tick() == 30 {
					posAtSwitch = pos
				}
				if state.Tick() == 90 {
					finalPos = pos
				}
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 91)

	// Before switch: should not have moved (Manual).
	approxVec2(t, posAtSwitch, spawnPos, "position unchanged while Manual")
	// After switch: should have fallen (Dynamic + gravity).
	require.Less(t, finalPos.Y, spawnPos.Y-1.0,
		"body should fall after switching to Dynamic")
}

// ---------------------------------------------------------------------------
// Test: Rotation writeback for dynamic body
// ---------------------------------------------------------------------------

func TestWriteback_DynamicRotation(t *testing.T) {
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
		row.Tag.Set(harnessTag{Role: "spinner"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}, Rotation: 0})
		row.V.Set(physics.Velocity2D{Angular: 2.0}) // 2 rad/s
		row.R.Set(newRigidNoGravity(physics.BodyTypeDynamic))
		row.C.Set(circleCollider())
		entityID = id
	}, cardinal.WithHook(cardinal.Init))

	var finalRotation float64
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() < 60 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == entityID {
				finalRotation = row.T.Get().Rotation
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 61)

	// 60 ticks at 60Hz = 1s at 2 rad/s → ~2 radians (with some damping)
	require.Greater(t, math.Abs(finalRotation), 1.0,
		"dynamic body rotation should be written back from angular velocity")
}

// ---------------------------------------------------------------------------
// Test: Manual body rotation stays unchanged
// ---------------------------------------------------------------------------

func TestWriteback_ManualRotationUnchanged(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var entityID cardinal.EntityID
	spawnRot := 1.5

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "manual_rot"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}, Rotation: spawnRot})
		row.V.Set(physics.Velocity2D{Angular: 10.0}) // high angular vel — should be ignored
		row.R.Set(newRigid(physics.BodyTypeManual))
		row.C.Set(circleCollider())
		entityID = id
	}, cardinal.WithHook(cardinal.Init))

	var finalRotation float64
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() < 60 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == entityID {
				finalRotation = row.T.Get().Rotation
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 61)

	require.InDelta(t, spawnRot, finalRotation, epsilon,
		"manual body rotation should not change despite angular velocity in ECS")
}
