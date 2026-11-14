package internal

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/stretchr/testify/require"
)

// velocity2D builds a Velocity2D from linear XY and angular, for terser test setup.
func velocity2D(lx, ly, ang float64) component.Velocity2D {
	return component.Velocity2D{Linear: component.Vec2{X: lx, Y: ly}, Angular: ang}
}

// tickPipeline runs one full reconcile -> step -> writeback tick against rt for the given entities,
// mirroring the PhysicsPipelineSystem phase order. Reconcile is required to succeed.
func tickPipeline(t *testing.T, rt *Runtime, ents ...cardinal.Entity) {
	t.Helper()
	require.NoError(t, rt.ReconcileFromECS(rebuildEntriesFromECS(ents...)),
		"reconcile must succeed for the pipeline tick")
	rt.Step()
	rt.WritebackFromStepResults(writebackEntries(ents...))
}

// engineLinearVel reads the engine's linear velocity for the entity's body.
func engineLinearVel(rt *Runtime, id cardinal.EntityID) component.Vec2 {
	lv := rt.World.BodyLinearVelocity(rt.Bodies[id])
	return component.Vec2{X: lv.X, Y: lv.Y}
}

// engineAngularVel reads the engine's angular velocity for the entity's body.
func engineAngularVel(rt *Runtime, id cardinal.EntityID) float64 {
	return rt.World.BodyAngularVelocity(rt.Bodies[id])
}

// requireVel asserts a Velocity2D matches within a tight epsilon. The tight bound still catches the
// bug: the buggy writeback stomps velocity to exactly {0,0}, which is far outside 1e-9 of any
// authored nonzero velocity, while tolerating negligible float drift from an active step.
func requireVel(t *testing.T, phase string, got, want component.Velocity2D) {
	t.Helper()
	require.InDelta(t, want.Linear.X, got.Linear.X, 1e-9,
		"%s: linear.X = %v, want %v", phase, got.Linear.X, want.Linear.X)
	require.InDelta(t, want.Linear.Y, got.Linear.Y, 1e-9,
		"%s: linear.Y = %v, want %v", phase, got.Linear.Y, want.Linear.Y)
	require.InDelta(t, want.Angular, got.Angular, 1e-9,
		"%s: angular = %v, want %v", phase, got.Angular, want.Angular)
}

// spawnBoxWithVelocity creates a dynamic box at the origin and sets the given velocity. Must run
// inside a system on the world's system goroutine.
func spawnBoxWithVelocity(w *cardinal.World, vel component.Velocity2D) cardinal.Entity {
	e := spawnBoxAt(w, component.Vec2{})
	e.Set(vel)
	return e
}

// spawnPhysics creates a physics entity with the given transform, velocity, and body params.
func spawnPhysics(
	w *cardinal.World, tf component.Transform2D, vel component.Velocity2D, pb component.PhysicsBody2D,
) cardinal.Entity {
	e := w.Create[physicsArchetype]()
	e.Set(tf)
	e.Set(vel)
	e.Set(pb)
	return e
}

// kinematicBox returns a kinematic body with the shared box shape and Box2D-compatible defaults.
func kinematicBox() component.PhysicsBody2D {
	return component.NewPhysicsBody2D(component.BodyTypeKinematic, boxShape)
}

// fixedRotationBox returns a dynamic box with FixedRotation enabled.
func fixedRotationBox() component.PhysicsBody2D {
	pb := boxBody()
	pb.FixedRotation = true
	return pb
}

// disabledBody returns a copy of pb with Active=false.
func disabledBody(pb component.PhysicsBody2D) component.PhysicsBody2D {
	pb.Active = false
	return pb
}

// TestDisabledVelocity_LinearSurvivesDisableEditAndReenable is the canonical repro: a moving body
// set Active=false must keep its authored Velocity2D in ECS and the shadow, a velocity edit made
// while disabled must be preserved (not swallowed and erased), and on re-enable the authored
// velocity must be applied to the engine so the body resumes moving instead of freezing forever.
//
// Tick timeline (zero gravity, initial velocity {8,0}):
//
//	tick 1: active, body moves normally
//	tick 2: Active=false -> ECS velocity must stay {8,0}, engine reads {0,0} for the disabled body
//	tick 3: edit velocity to {5,0} while disabled -> ECS must hold {5,0}, engine still {0,0}
//	tick 4: Active=true  -> engine must receive {5,0}, body moves again
//	tick 5: body keeps moving at {5,0}
func TestDisabledVelocity_LinearSurvivesDisableEditAndReenable(t *testing.T) {
	t.Parallel()

	gravity := component.Vec2{} // zero gravity: velocity is the only motion
	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)

	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		registerPhysicsWorld(w)
	})

	var ent cardinal.Entity
	w.RunSystem(&runOnceSystem{func(w *cardinal.World) {
		ent = spawnBoxWithVelocity(w, velocity2D(8, 0, 0))
	}})

	id := ent.ID()
	dt := defaultFixedDT

	w.RunSystem(&runOnceSystem{func(*cardinal.World) {
		require.NoError(t, rt.FullRebuildFromECS(gravity, rebuildEntriesFromECS(ent)))

		// tick 1: active body moves at {8,0}.
		tickPipeline(t, rt, ent)
		requireVel(t, "tick1 ECS vel", ent.Get[component.Velocity2D](), velocity2D(8, 0, 0))
		requireVel(
			t,
			"tick1 engine vel",
			velocity2D(engineLinearVel(rt, id).X, engineLinearVel(rt, id).Y, 0),
			velocity2D(8, 0, 0),
		)
		pos1 := ent.Get[component.Transform2D]().Position
		requireXY(t, "tick1 pos", pos1.X, pos1.Y, 8*dt, 0)
		requireVel(t, "tick1 shadow vel", rt.Shadow[id].Velocity, velocity2D(8, 0, 0))

		// tick 2: disable. ECS velocity must NOT be stomped to zero.
		ent.Set(disabledBody(ent.Get[component.PhysicsBody2D]()))
		tickPipeline(t, rt, ent)
		requireVel(t, "tick2 ECS vel after disable", ent.Get[component.Velocity2D](), velocity2D(8, 0, 0))
		requireVel(t, "tick2 shadow vel after disable", rt.Shadow[id].Velocity, velocity2D(8, 0, 0))
		require.Equal(t, component.Vec2{}, engineLinearVel(rt, id),
			"disabled body has no solver state; engine velocity reads zero")
		requireXY(t, "tick2 pos frozen while disabled",
			ent.Get[component.Transform2D]().Position.X, ent.Get[component.Transform2D]().Position.Y,
			pos1.X, pos1.Y)

		// tick 3: edit velocity while disabled. The edit must survive (not be swallowed and erased).
		ent.Set(velocity2D(5, 0, 0))
		tickPipeline(t, rt, ent)
		requireVel(t, "tick3 ECS vel after edit while disabled", ent.Get[component.Velocity2D](), velocity2D(5, 0, 0))
		requireVel(t, "tick3 shadow vel after edit while disabled", rt.Shadow[id].Velocity, velocity2D(5, 0, 0))
		require.Equal(t, component.Vec2{}, engineLinearVel(rt, id),
			"velocity edit must not reach the engine while the body is disabled")

		// tick 4: re-enable. The authored velocity must be applied to the engine.
		pb := ent.Get[component.PhysicsBody2D]()
		pb.Active = true
		ent.Set(pb)
		tickPipeline(t, rt, ent)
		requireVel(t, "tick4 ECS vel after reenable", ent.Get[component.Velocity2D](), velocity2D(5, 0, 0))
		requireVel(t, "tick4 engine vel after reenable",
			velocity2D(engineLinearVel(rt, id).X, engineLinearVel(rt, id).Y, 0), velocity2D(5, 0, 0))
		pos4 := ent.Get[component.Transform2D]().Position
		requireXY(t, "tick4 pos advanced after reenable", pos4.X, pos4.Y, pos1.X+5*dt, 0)

		// tick 5: body keeps moving; it is not frozen.
		tickPipeline(t, rt, ent)
		pos5 := ent.Get[component.Transform2D]().Position
		requireXY(t, "tick5 pos keeps moving", pos5.X, pos5.Y, pos1.X+2*5*dt, 0)
		requireVel(
			t,
			"tick5 engine vel",
			velocity2D(engineLinearVel(rt, id).X, engineLinearVel(rt, id).Y, 0),
			velocity2D(5, 0, 0),
		)
	}})
}

// TestDisabledVelocity_HeldAtDisableMomentResumesOnReenable covers the case where no velocity edit is
// made while disabled: the velocity the body had at the moment of disable must survive untouched
// and resume motion on re-enable, rather than leaving the body dead-still at its last position.
func TestDisabledVelocity_HeldAtDisableMomentResumesOnReenable(t *testing.T) {
	t.Parallel()

	gravity := component.Vec2{}
	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)

	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		registerPhysicsWorld(w)
	})

	var ent cardinal.Entity
	w.RunSystem(&runOnceSystem{func(w *cardinal.World) {
		ent = spawnBoxWithVelocity(w, velocity2D(8, 0, 0))
	}})

	id := ent.ID()
	dt := defaultFixedDT

	w.RunSystem(&runOnceSystem{func(*cardinal.World) {
		require.NoError(t, rt.FullRebuildFromECS(gravity, rebuildEntriesFromECS(ent)))

		tickPipeline(t, rt, ent) // tick 1: moves to 8*dt
		pos1 := ent.Get[component.Transform2D]().Position

		// ticks 2-3: disabled, no velocity edit.
		ent.Set(disabledBody(ent.Get[component.PhysicsBody2D]()))
		tickPipeline(t, rt, ent) // tick 2: disable
		requireVel(t, "tick2 ECS vel preserved", ent.Get[component.Velocity2D](), velocity2D(8, 0, 0))
		tickPipeline(t, rt, ent) // tick 3: still disabled
		requireVel(t, "tick3 ECS vel preserved", ent.Get[component.Velocity2D](), velocity2D(8, 0, 0))
		requireXY(t, "pos frozen across disabled ticks",
			ent.Get[component.Transform2D]().Position.X, ent.Get[component.Transform2D]().Position.Y,
			pos1.X, pos1.Y)

		// tick 4: re-enable WITHOUT editing velocity. The held velocity must resume.
		pb := ent.Get[component.PhysicsBody2D]()
		pb.Active = true
		ent.Set(pb)
		tickPipeline(t, rt, ent)
		requireVel(
			t,
			"tick4 engine vel resumed",
			velocity2D(engineLinearVel(rt, id).X, engineLinearVel(rt, id).Y, 0),
			velocity2D(8, 0, 0),
		)
		pos4 := ent.Get[component.Transform2D]().Position
		requireXY(t, "tick4 pos advanced on resume", pos4.X, pos4.Y, pos1.X+8*dt, 0)

		// tick 5: still moving — the body did not freeze.
		tickPipeline(t, rt, ent)
		requireXY(
			t,
			"tick5 pos keeps moving",
			ent.Get[component.Transform2D]().Position.X,
			ent.Get[component.Transform2D]().Position.Y,
			pos1.X+2*8*dt,
			0,
		)
	}})
}

// TestDisabledVelocity_BodyCreatedDisabledWithVelocityAppliesOnReenable covers a body created with
// Active=false and a nonzero velocity: the velocity must survive the disabled period in ECS and the
// shadow (writeback must not stomp it), and on re-enable the reconciler must force-push it into the
// engine. Without the force-push the shadow would already agree with ECS and the push would be
// skipped, leaving the engine at the zero velocity EnableBody restarts it with.
func TestDisabledVelocity_BodyCreatedDisabledWithVelocityAppliesOnReenable(t *testing.T) {
	t.Parallel()

	gravity := component.Vec2{}
	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)

	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		registerPhysicsWorld(w)
	})

	var ent cardinal.Entity
	w.RunSystem(&runOnceSystem{func(w *cardinal.World) {
		ent = spawnPhysics(w,
			component.Transform2D{},
			velocity2D(5, 0, 0),
			disabledBody(boxBody()))
	}})

	id := ent.ID()
	dt := defaultFixedDT

	w.RunSystem(&runOnceSystem{func(*cardinal.World) {
		require.NoError(t, rt.FullRebuildFromECS(gravity, rebuildEntriesFromECS(ent)))

		// tick 1: disabled at creation. Velocity must be preserved, not stomped to zero.
		tickPipeline(t, rt, ent)
		requireVel(
			t,
			"tick1 ECS vel preserved (created disabled)",
			ent.Get[component.Velocity2D](),
			velocity2D(5, 0, 0),
		)
		requireVel(t, "tick1 shadow vel preserved (created disabled)", rt.Shadow[id].Velocity, velocity2D(5, 0, 0))
		require.Equal(t, component.Vec2{}, engineLinearVel(rt, id),
			"engine has no velocity state for a body created disabled")
		requireXY(
			t,
			"tick1 pos frozen",
			ent.Get[component.Transform2D]().Position.X,
			ent.Get[component.Transform2D]().Position.Y,
			0,
			0,
		)

		// tick 2: re-enable. The authored velocity must be force-pushed into the engine.
		pb := ent.Get[component.PhysicsBody2D]()
		pb.Active = true
		ent.Set(pb)
		tickPipeline(t, rt, ent)
		requireVel(t, "tick2 engine vel after reenable",
			velocity2D(engineLinearVel(rt, id).X, engineLinearVel(rt, id).Y, 0), velocity2D(5, 0, 0))
		requireXY(t, "tick2 pos moved after reenable",
			ent.Get[component.Transform2D]().Position.X, ent.Get[component.Transform2D]().Position.Y, 5*dt, 0)

		// tick 3: keeps moving.
		tickPipeline(t, rt, ent)
		requireXY(t, "tick3 pos keeps moving",
			ent.Get[component.Transform2D]().Position.X, ent.Get[component.Transform2D]().Position.Y, 2*5*dt, 0)
	}})
}

// TestDisabledVelocity_AngularSurvivesDisable covers the angular component: a spinning body set
// Active=false must keep its angular velocity in ECS, and on re-enable the spin must resume.
func TestDisabledVelocity_AngularSurvivesDisable(t *testing.T) {
	t.Parallel()

	gravity := component.Vec2{}
	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)

	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		registerPhysicsWorld(w)
	})

	var ent cardinal.Entity
	w.RunSystem(&runOnceSystem{func(w *cardinal.World) {
		ent = spawnBoxWithVelocity(w, velocity2D(0, 0, 3))
	}})

	id := ent.ID()
	dt := defaultFixedDT

	w.RunSystem(&runOnceSystem{func(*cardinal.World) {
		require.NoError(t, rt.FullRebuildFromECS(gravity, rebuildEntriesFromECS(ent)))

		tickPipeline(t, rt, ent) // tick 1: spinning
		rot1 := box2dRotAngle(rt, id)
		require.InDelta(t, 3*dt, rot1, 1e-4, "tick1 rotation advanced by angular*dt")

		// tick 2: disable. Angular velocity must be preserved.
		ent.Set(disabledBody(ent.Get[component.PhysicsBody2D]()))
		tickPipeline(t, rt, ent)
		requireVel(t, "tick2 ECS angular vel preserved", ent.Get[component.Velocity2D](), velocity2D(0, 0, 3))
		requireVel(t, "tick2 shadow angular vel preserved", rt.Shadow[id].Velocity, velocity2D(0, 0, 3))
		require.InDelta(t, 0, engineAngularVel(rt, id), 1e-9, "disabled body engine angular velocity is zero")
		require.InDelta(t, rot1, box2dRotAngle(rt, id), 1e-9, "rotation frozen while disabled")

		// tick 3: re-enable. Spin must resume.
		pb := ent.Get[component.PhysicsBody2D]()
		pb.Active = true
		ent.Set(pb)
		tickPipeline(t, rt, ent)
		require.InDelta(t, 3, engineAngularVel(rt, id), 1e-9, "tick3 engine angular vel resumed")
		require.InDelta(t, rot1+3*dt, box2dRotAngle(rt, id), 1e-4, "tick3 rotation advanced one step from frozen")
	}})
}

// box2dRotAngle reads the engine rotation angle (radians) for the entity's body.
func box2dRotAngle(rt *Runtime, id cardinal.EntityID) float64 {
	return box2d.RotGetAngle(rt.World.BodyRotation(rt.Bodies[id]))
}

// TestDisabledVelocity_KinematicSurvivesDisable covers kinematic bodies: kinematic velocity is
// gameplay-authored and integrated by Box2D; it must survive a disable and resume on re-enable.
func TestDisabledVelocity_KinematicSurvivesDisable(t *testing.T) {
	t.Parallel()

	gravity := component.Vec2{}
	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)

	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		registerPhysicsWorld(w)
	})

	var ent cardinal.Entity
	w.RunSystem(&runOnceSystem{func(w *cardinal.World) {
		ent = spawnPhysics(w, component.Transform2D{}, velocity2D(5, 0, 0), kinematicBox())
	}})

	id := ent.ID()
	dt := defaultFixedDT

	w.RunSystem(&runOnceSystem{func(*cardinal.World) {
		require.NoError(t, rt.FullRebuildFromECS(gravity, rebuildEntriesFromECS(ent)))

		tickPipeline(t, rt, ent) // tick 1: kinematic moves
		pos1 := ent.Get[component.Transform2D]().Position
		requireXY(t, "tick1 pos", pos1.X, pos1.Y, 5*dt, 0)

		// tick 2: disable. Velocity preserved.
		ent.Set(disabledBody(ent.Get[component.PhysicsBody2D]()))
		tickPipeline(t, rt, ent)
		requireVel(t, "tick2 ECS vel preserved (kinematic)", ent.Get[component.Velocity2D](), velocity2D(5, 0, 0))
		requireVel(t, "tick2 shadow vel preserved (kinematic)", rt.Shadow[id].Velocity, velocity2D(5, 0, 0))

		// tick 3: re-enable. Velocity applied, body moves.
		pb := ent.Get[component.PhysicsBody2D]()
		pb.Active = true
		ent.Set(pb)
		tickPipeline(t, rt, ent)
		requireVel(t, "tick3 engine vel resumed (kinematic)",
			velocity2D(engineLinearVel(rt, id).X, engineLinearVel(rt, id).Y, 0), velocity2D(5, 0, 0))
		requireXY(
			t,
			"tick3 pos moved",
			ent.Get[component.Transform2D]().Position.X,
			ent.Get[component.Transform2D]().Position.Y,
			pos1.X+5*dt,
			0,
		)

		tickPipeline(t, rt, ent) // tick 4: keeps moving
		requireXY(t, "tick4 pos keeps moving",
			ent.Get[component.Transform2D]().Position.X, ent.Get[component.Transform2D]().Position.Y, pos1.X+2*5*dt, 0)
	}})
}

// TestDisabledVelocity_FixedRotationLinearSurvives covers a FixedRotation body: its angular velocity
// is always zero in Box2D, but its linear velocity must survive a disable and resume on re-enable.
func TestDisabledVelocity_FixedRotationLinearSurvives(t *testing.T) {
	t.Parallel()

	gravity := component.Vec2{}
	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)

	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		registerPhysicsWorld(w)
	})

	var ent cardinal.Entity
	w.RunSystem(&runOnceSystem{func(w *cardinal.World) {
		ent = spawnPhysics(w, component.Transform2D{}, velocity2D(5, 0, 0), fixedRotationBox())
	}})

	id := ent.ID()
	dt := defaultFixedDT

	w.RunSystem(&runOnceSystem{func(*cardinal.World) {
		require.NoError(t, rt.FullRebuildFromECS(gravity, rebuildEntriesFromECS(ent)))

		tickPipeline(t, rt, ent) // tick 1: moves linearly
		pos1 := ent.Get[component.Transform2D]().Position
		requireXY(t, "tick1 pos", pos1.X, pos1.Y, 5*dt, 0)

		// tick 2: disable. Linear velocity preserved.
		ent.Set(disabledBody(ent.Get[component.PhysicsBody2D]()))
		tickPipeline(t, rt, ent)
		require.InDelta(t, 5, ent.Get[component.Velocity2D]().Linear.X, 1e-9,
			"tick2 ECS linear vel preserved (FixedRotation)")
		require.InDelta(t, 5, rt.Shadow[id].Velocity.Linear.X, 1e-9,
			"tick2 shadow linear vel preserved (FixedRotation)")

		// tick 3: re-enable. Linear velocity force-pushed; body moves linearly.
		pb := ent.Get[component.PhysicsBody2D]()
		pb.Active = true
		ent.Set(pb)
		tickPipeline(t, rt, ent)
		require.InDelta(t, 5, engineLinearVel(rt, id).X, 1e-9, "tick3 engine linear vel resumed (FixedRotation)")
		requireXY(
			t,
			"tick3 pos moved",
			ent.Get[component.Transform2D]().Position.X,
			ent.Get[component.Transform2D]().Position.Y,
			pos1.X+5*dt,
			0,
		)

		tickPipeline(t, rt, ent) // tick 4: keeps moving
		requireXY(t, "tick4 pos keeps moving",
			ent.Get[component.Transform2D]().Position.X, ent.Get[component.Transform2D]().Position.Y, pos1.X+2*5*dt, 0)
	}})
}

// TestDisabledVelocity_ControlTransformTeleportSticksWhileDisabled is the control: a transform
// teleport on a disabled body must reach the engine and hold (SetBodyTransform patches the body sim
// the disabled set retains), proving the data loss is velocity-specific and not a general
// disabled-body writeback artifact.
func TestDisabledVelocity_ControlTransformTeleportSticksWhileDisabled(t *testing.T) {
	t.Parallel()

	gravity := component.Vec2{}
	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)

	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		registerPhysicsWorld(w)
	})

	var ent cardinal.Entity
	w.RunSystem(&runOnceSystem{func(w *cardinal.World) {
		ent = spawnBoxAt(w, component.Vec2{})
	}})

	w.RunSystem(&runOnceSystem{func(*cardinal.World) {
		require.NoError(t, rt.FullRebuildFromECS(gravity, rebuildEntriesFromECS(ent)))

		// Disable and teleport in the same tick the way gameplay would.
		ent.Set(disabledBody(ent.Get[component.PhysicsBody2D]()))
		ent.Set(component.Transform2D{Position: component.Vec2{X: 50, Y: 3}})
		tickPipeline(t, rt, ent)

		// Engine followed the teleport and holds it.
		pos := ent.Get[component.Transform2D]().Position
		requireXY(t, "ECS transform after teleport+disable", pos.X, pos.Y, 50, 3)
		ePos := rt.World.BodyPosition(rt.Bodies[ent.ID()])
		requireXY(t, "engine transform after teleport+disable", ePos.X, ePos.Y, 50, 3)

		// A second disabled tick must not drift the transform.
		tickPipeline(t, rt, ent)
		pos = ent.Get[component.Transform2D]().Position
		requireXY(t, "ECS transform held while disabled", pos.X, pos.Y, 50, 3)
		ePos = rt.World.BodyPosition(rt.Bodies[ent.ID()])
		requireXY(t, "engine transform held while disabled", ePos.X, ePos.Y, 50, 3)
	}})
}

// TestDisabledVelocity_DisableTickDoesNotStompVelocity pins the exact moment the bug manifested: on
// the very tick Active flips to false, writeback runs for the (now disabled) body and must NOT
// overwrite ECS Velocity2D with the zero the engine reads for it. This is the regression guard that
// fails on the unfixed code.
func TestDisabledVelocity_DisableTickDoesNotStompVelocity(t *testing.T) {
	t.Parallel()

	gravity := component.Vec2{}
	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)

	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		registerPhysicsWorld(w)
	})

	var ent cardinal.Entity
	w.RunSystem(&runOnceSystem{func(w *cardinal.World) {
		ent = spawnBoxWithVelocity(w, velocity2D(8, 2, 1))
	}})

	id := ent.ID()

	w.RunSystem(&runOnceSystem{func(*cardinal.World) {
		require.NoError(t, rt.FullRebuildFromECS(gravity, rebuildEntriesFromECS(ent)))

		// One active tick so the body has a live, moving state.
		tickPipeline(t, rt, ent)
		authored := velocity2D(8, 2, 1)
		requireVel(t, "active tick ECS vel", ent.Get[component.Velocity2D](), authored)

		// The disable tick: writeback must not stomp the velocity to zero.
		ent.Set(disabledBody(ent.Get[component.PhysicsBody2D]()))
		tickPipeline(t, rt, ent)
		requireVel(t, "disable-tick ECS vel not stomped", ent.Get[component.Velocity2D](), authored)
		requireVel(t, "disable-tick shadow vel not stomped", rt.Shadow[id].Velocity, authored)

		// The engine legitimately reads zero for the disabled body; the fix is that this zero is
		// no longer mirrored back into ECS.
		require.Equal(t, component.Vec2{}, engineLinearVel(rt, id), "engine reads zero while disabled")
		require.InDelta(t, 0, engineAngularVel(rt, id), 1e-9, "engine angular reads zero while disabled")
	}})
}
