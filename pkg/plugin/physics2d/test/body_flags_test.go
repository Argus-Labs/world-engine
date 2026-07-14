package physics2d_test

import (
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Active=false — body excluded from simulation (no fall, no contacts)
// ---------------------------------------------------------------------------

func TestBodyFlag_InactiveBodyDoesNotFall(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var bodyID cardinal.EntityID
	spawnPos := physics.Vec2{X: 0, Y: 10}

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		pb := newRigid(physics.BodyTypeDynamic, circleColliderShapes()...)
		pb.Active = false
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "inactive"})
		row.T.Set(physics.Transform2D{Position: spawnPos})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(pb)
		bodyID = id
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
			if eid == bodyID {
				finalPos = row.T.Get().Position
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 61)

	approxVec2(t, finalPos, spawnPos, "inactive body should not move")
}

// ---------------------------------------------------------------------------
// Active toggled mid-sim: false → true starts falling
// ---------------------------------------------------------------------------

func TestBodyFlag_ActivateMidSim(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var bodyID cardinal.EntityID
	spawnPos := physics.Vec2{X: 0, Y: 20}

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		pb := newRigid(physics.BodyTypeDynamic, circleColliderShapes()...)
		pb.Active = false
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "activate_later"})
		row.T.Set(physics.Transform2D{Position: spawnPos})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(pb)
		bodyID = id
	}, cardinal.WithHook(cardinal.Init))

	// Activate at tick 30.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 30 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == bodyID {
				pb := row.PB.Get()
				pb.Active = true
				row.PB.Set(pb)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	posAtActivate := physics.Vec2{}
	var finalPos physics.Vec2
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		for eid, row := range state.Spawn.Iter() {
			if eid == bodyID {
				pos := row.T.Get().Position
				if state.Tick() == 30 {
					posAtActivate = pos
				}
				if state.Tick() == 90 {
					finalPos = pos
				}
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 91)

	approxVec2(t, posAtActivate, spawnPos, "body unchanged while inactive")
	require.Less(t, finalPos.Y, spawnPos.Y-1.0, "body should fall after activation")
}

// ---------------------------------------------------------------------------
// FixedRotation — body does not rotate despite angular velocity
// ---------------------------------------------------------------------------

func TestBodyFlag_FixedRotation(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var bodyID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		pb := newRigidNoGravity(physics.BodyTypeDynamic, circleColliderShapes()...)
		pb.FixedRotation = true
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "fixed_rot"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{Angular: 10.0})
		row.PB.Set(pb)
		bodyID = id
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
			if eid == bodyID {
				finalRotation = row.T.Get().Rotation
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 61)

	require.InDelta(t, 0, finalRotation, epsilon,
		"FixedRotation body should not rotate despite angular velocity")
}

// ---------------------------------------------------------------------------
// FixedRotation toggled mid-sim: true → false, body starts rotating
// ---------------------------------------------------------------------------

func TestBodyFlag_FixedRotationToggle(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var bodyID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		pb := newRigidNoGravity(physics.BodyTypeDynamic, circleColliderShapes()...)
		pb.FixedRotation = true
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "toggle_rot"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{Angular: 5.0})
		row.PB.Set(pb)
		bodyID = id
	}, cardinal.WithHook(cardinal.Init))

	// Unlock rotation at tick 30.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 30 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == bodyID {
				pb := row.PB.Get()
				pb.FixedRotation = false
				row.PB.Set(pb)
				// Re-set angular velocity since Box2D may have zeroed it.
				row.V.Set(physics.Velocity2D{Angular: 5.0})
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	rotAtUnlock := 0.0
	var finalRotation float64
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		for eid, row := range state.Spawn.Iter() {
			if eid == bodyID {
				rot := row.T.Get().Rotation
				if state.Tick() == 30 {
					rotAtUnlock = rot
				}
				if state.Tick() == 90 {
					finalRotation = rot
				}
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 91)

	require.InDelta(t, 0, rotAtUnlock, epsilon, "rotation locked before toggle")
	require.Greater(t, math.Abs(finalRotation), 0.5, "rotation should change after unlocking")
}

// ---------------------------------------------------------------------------
// GravityScale=0 — body does not fall despite world gravity
// ---------------------------------------------------------------------------

func TestBodyFlag_GravityScaleZero(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var bodyID cardinal.EntityID
	spawnPos := physics.Vec2{X: 0, Y: 10}

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "no_gravity"})
		row.T.Set(physics.Transform2D{Position: spawnPos})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigidNoGravity(physics.BodyTypeDynamic, circleColliderShapes()...))
		bodyID = id
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
			if eid == bodyID {
				finalPos = row.T.Get().Position
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 61)

	approxVec2(t, finalPos, spawnPos, "gravity_scale=0 body should not fall")
}

// ---------------------------------------------------------------------------
// GravityScale=2 — body falls faster than GravityScale=1
// ---------------------------------------------------------------------------

func TestBodyFlag_GravityScaleDouble(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var normalID, doubleID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Normal gravity body.
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "normal_grav"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: -5, Y: 30}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeDynamic, circleColliderShapes()...))
		normalID = id

		// Double gravity body.
		pb := newRigid(physics.BodyTypeDynamic, circleColliderShapes()...)
		pb.GravityScale = 2
		id2, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "double_grav"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 5, Y: 30}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(pb)
		doubleID = id2
	}, cardinal.WithHook(cardinal.Init))

	var normalY, doubleY float64
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() < 60 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == normalID {
				normalY = row.T.Get().Position.Y
			}
			if eid == doubleID {
				doubleY = row.T.Get().Position.Y
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 61)

	require.Less(t, doubleY, normalY, "double gravity body should be lower")
}

// ---------------------------------------------------------------------------
// LinearDamping — body with damping slows down faster
// ---------------------------------------------------------------------------

func TestBodyFlag_LinearDamping(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var undampedID, dampedID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Undamped body moving right.
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "undamped"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{Linear: physics.Vec2{X: 10, Y: 0}})
		row.PB.Set(newRigidNoGravity(physics.BodyTypeDynamic, circleColliderShapes()...))
		undampedID = id

		// Damped body moving right.
		pb := newRigidNoGravity(physics.BodyTypeDynamic, circleColliderShapes()...)
		pb.LinearDamping = 5.0
		id2, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "damped"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 5}})
		row2.V.Set(physics.Velocity2D{Linear: physics.Vec2{X: 10, Y: 0}})
		row2.PB.Set(pb)
		dampedID = id2
	}, cardinal.WithHook(cardinal.Init))

	var undampedX, dampedX float64
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() < 60 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == undampedID {
				undampedX = row.T.Get().Position.X
			}
			if eid == dampedID {
				dampedX = row.T.Get().Position.X
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 61)

	require.Greater(t, undampedX, dampedX, "damped body should travel less distance")
	require.Greater(t, dampedX, 0.0, "damped body should still have moved forward")
}

// ---------------------------------------------------------------------------
// AngularDamping — body with angular damping spins down faster
// ---------------------------------------------------------------------------

func TestBodyFlag_AngularDamping(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var undampedID, dampedID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Undamped spinner.
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "undamped_spin"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{Angular: 10.0})
		row.PB.Set(newRigidNoGravity(physics.BodyTypeDynamic, circleColliderShapes()...))
		undampedID = id

		// Damped spinner.
		pb := newRigidNoGravity(physics.BodyTypeDynamic, circleColliderShapes()...)
		pb.AngularDamping = 5.0
		id2, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "damped_spin"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 5, Y: 0}})
		row2.V.Set(physics.Velocity2D{Angular: 10.0})
		row2.PB.Set(pb)
		dampedID = id2
	}, cardinal.WithHook(cardinal.Init))

	var undampedRot, dampedRot float64
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() < 60 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == undampedID {
				undampedRot = math.Abs(row.T.Get().Rotation)
			}
			if eid == dampedID {
				dampedRot = math.Abs(row.T.Get().Rotation)
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 61)

	require.Greater(t, undampedRot, dampedRot, "damped body should spin less")
	require.Greater(t, dampedRot, 0.0, "damped body should still have spun some")
}

// ---------------------------------------------------------------------------
// Body type switches: Dynamic→Static, Dynamic→Kinematic, Kinematic→Dynamic
// ---------------------------------------------------------------------------

func TestBodyTypeSwitch_DynamicToStatic(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var bodyID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "dyn_to_static"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 30}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeDynamic, circleColliderShapes()...))
		bodyID = id
	}, cardinal.WithHook(cardinal.Init))

	// Switch at tick 30.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 30 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == bodyID {
				pb := row.PB.Get()
				pb.BodyType = physics.BodyTypeStatic
				row.PB.Set(pb)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	var posAtSwitch, finalPos physics.Vec2
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		for eid, row := range state.Spawn.Iter() {
			if eid == bodyID {
				if state.Tick() == 30 {
					posAtSwitch = row.T.Get().Position
				}
				if state.Tick() == 90 {
					finalPos = row.T.Get().Position
				}
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 91)

	require.Less(t, posAtSwitch.Y, 30.0, "body should have fallen before switch")
	// After switching to static, position should be frozen.
	approxVec2(t, finalPos, posAtSwitch, "body should stop after switching to static")
}

func TestBodyTypeSwitch_DynamicToKinematic(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var bodyID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "dyn_to_kin"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 30}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeDynamic, circleColliderShapes()...))
		bodyID = id
	}, cardinal.WithHook(cardinal.Init))

	// Switch to kinematic with rightward velocity at tick 30.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 30 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == bodyID {
				pb := row.PB.Get()
				pb.BodyType = physics.BodyTypeKinematic
				row.PB.Set(pb)
				row.V.Set(physics.Velocity2D{Linear: physics.Vec2{X: 5, Y: 0}})
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	var posAtSwitch, finalPos physics.Vec2
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		for eid, row := range state.Spawn.Iter() {
			if eid == bodyID {
				if state.Tick() == 31 {
					posAtSwitch = row.T.Get().Position
				}
				if state.Tick() == 90 {
					finalPos = row.T.Get().Position
				}
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 91)

	// After switch, should move right (kinematic integrates velocity) but not fall (no gravity).
	require.Greater(t, finalPos.X, posAtSwitch.X+1.0, "kinematic should move right")
	// Y should be roughly constant after switch (kinematic ignores gravity).
	require.InDelta(t, posAtSwitch.Y, finalPos.Y, 0.5, "kinematic Y should not change much")
}

func TestBodyTypeSwitch_KinematicToDynamic(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var bodyID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "kin_to_dyn"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 20}})
		row.V.Set(physics.Velocity2D{Linear: physics.Vec2{X: 3, Y: 0}})
		row.PB.Set(newRigid(physics.BodyTypeKinematic, circleColliderShapes()...))
		bodyID = id
	}, cardinal.WithHook(cardinal.Init))

	// Switch to dynamic at tick 30.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 30 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == bodyID {
				pb := row.PB.Get()
				pb.BodyType = physics.BodyTypeDynamic
				row.PB.Set(pb)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	var posAtSwitch, finalPos physics.Vec2
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		for eid, row := range state.Spawn.Iter() {
			if eid == bodyID {
				if state.Tick() == 30 {
					posAtSwitch = row.T.Get().Position
				}
				if state.Tick() == 90 {
					finalPos = row.T.Get().Position
				}
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 91)

	// Before switch: kinematic at Y=20 (no gravity).
	require.InDelta(t, 20.0, posAtSwitch.Y, 0.5, "kinematic Y should stay near 20")
	// After switch: dynamic body falls.
	require.Less(t, finalPos.Y, posAtSwitch.Y-1.0, "body should fall after switching to dynamic")
}
