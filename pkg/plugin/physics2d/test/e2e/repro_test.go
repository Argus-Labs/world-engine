package e2e_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	physcomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/scenario"
	"github.com/stretchr/testify/require"
)

// reproConfig is the suite config with gravity off: these repros isolate the velocity
// the reconciler pushes, so free fall would only add noise.
func reproConfig(t *testing.T) harness.Config {
	t.Helper()
	cfg := e2eConfig(t, 0)
	cfg.Gravity = physics.Vec2{}
	return cfg
}

// runRepro runs one repro scenario and fails the test if any check did.
func runRepro(t *testing.T, sc harness.Scenario) {
	t.Helper()
	_, _, code := runSuite(t, []harness.Scenario{sc}, reproConfig(t))
	require.Zero(t, code, "check(s) failed — bug reproduced")
}

// TestReproManualReleaseVelocityDrop guards the parity contract between the incremental
// reconciler (ReconcileFromECS) and the full rebuild (FullRebuildFromECS): for an identical
// ECS snapshot, Box2D must end up in the same state either way.
//
// While a body is BodyTypeManual, Box2D's velocity is forced to {0,0} every tick and the
// shadow stores the gameplay Velocity2D untouched (writeback is skipped for Manual bodies).
// When the body is flipped to Dynamic (or Kinematic) without rewriting Velocity2D in the
// same tick, prev.VelocityDiffers(e.Velocity) reports "no change" (the shadow still holds
// the gameplay velocity), so the reconciler must still push that velocity into Box2D —
// otherwise the body keeps the Manual-imposed {0,0} and ReconcileFromECS diverges from
// FullRebuildFromECS, which would have created the body born-moving at the gameplay
// velocity (see CreateBody in internal/create.go).
//
// The "control" body is spawned already Dynamic at the release tick, so it goes through
// CreateBody and is the FullRebuild-equivalent. The two "released" bodies start Manual
// (the gameplay velocity {vx,0} is pure bookkeeping) and are flipped to Dynamic at the
// release tick via EditBody without touching Velocity2D. After enough dynamic ticks all
// three must have travelled the same distance, in both the plain and FixedRotation
// (Manual+FixedRotation -> Dynamic+FixedRotation) variants — both non-Manual branches of
// the reconciler velocity switch are affected.
func TestReproManualReleaseVelocityDrop(t *testing.T) {
	t.Parallel()
	const releaseTick, checkTick = uint64(20), uint64(60)
	const vx = 7.0
	wantX := vx * float64(checkTick-releaseTick) / 60.0
	var s struct{ released, releasedFR, control cardinal.EntityID }
	box := func(c *harness.Ctx) physics.PhysicsBody2D {
		pb := physcomp.NewPhysicsBody2D(physcomp.BodyTypeManual, scenario.Box(0.5, 0.5).Spawn(c))
		pb.GravityScale = 0
		return pb
	}
	sc := harness.Scenario{
		Name: "repro-manual-release",
		Setup: func(c *harness.Ctx) {
			s.released = c.SpawnMoving("released", 0, 10, vx, 0, box(c))
			fr := box(c)
			fr.FixedRotation = true
			s.releasedFR = c.SpawnMoving("released-fr", 0, 15, vx, 0, fr)
		},
		Steps: []harness.Step{
			{Tick: releaseTick, Do: func(c *harness.Ctx) {
				c.EditBody(s.released, func(pb *physics.PhysicsBody2D) { pb.BodyType = physics.BodyTypeDynamic })
				c.EditBody(s.releasedFR, func(pb *physics.PhysicsBody2D) { pb.BodyType = physics.BodyTypeDynamic })
				ctrl := box(c)
				ctrl.BodyType = physics.BodyTypeDynamic
				s.control = c.SpawnMoving("control", 0, 5, vx, 0, ctrl) // FullRebuild-equivalent: born Dynamic with {7,0}
			}},
			{Tick: checkTick, Do: func(c *harness.Ctx) {
				t.Logf("positions after %d dynamic ticks: control.X=%.4f released.X=%.4f releasedFR.X=%.4f want=%.4f",
					checkTick-releaseTick, c.Pos(s.control).X, c.Pos(s.released).X, c.Pos(s.releasedFR).X, wantX)
				c.Near("control (FullRebuild-equivalent) reaches want X", c.Pos(s.control).X, wantX, 0.1)
				c.Near("released (Manual->Dynamic) matches control X", c.Pos(s.released).X, c.Pos(s.control).X, 0.1)
				c.Near("released-FR (Manual->Dynamic+FixedRotation) matches", c.Pos(s.releasedFR).X, c.Pos(s.control).X, 0.1)
			}},
		},
	}
	runRepro(t, sc)
}

// TestReproManualReleaseKinematic extends the parity guard to the Manual->Kinematic
// transition (cameFromManual keys on != BodyTypeManual, so Kinematic is covered by the
// same fix). A Manual body is released to Kinematic without touching Velocity2D; a control
// is born Kinematic with the same velocity (the FullRebuild-equivalent). Box2D integrates
// kinematic linear velocity, so both must cover the same ground.
func TestReproManualReleaseKinematic(t *testing.T) {
	t.Parallel()
	const releaseTick, checkTick = uint64(20), uint64(60)
	const vx = 5.0
	wantX := vx * float64(checkTick-releaseTick) / 60.0
	var s struct{ released, control cardinal.EntityID }
	mk := func(c *harness.Ctx) physics.PhysicsBody2D {
		pb := physcomp.NewPhysicsBody2D(physcomp.BodyTypeManual, scenario.Box(0.5, 0.5).Spawn(c))
		return pb
	}
	sc := harness.Scenario{
		Name: "repro-manual-release-kinematic",
		Setup: func(c *harness.Ctx) {
			s.released = c.SpawnMoving("released", 0, 10, vx, 0, mk(c))
		},
		Steps: []harness.Step{
			{Tick: releaseTick, Do: func(c *harness.Ctx) {
				c.EditBody(s.released, func(pb *physics.PhysicsBody2D) { pb.BodyType = physics.BodyTypeKinematic })
				ctrl := mk(c)
				ctrl.BodyType = physics.BodyTypeKinematic
				s.control = c.SpawnMoving("control", 0, 5, vx, 0, ctrl) // FullRebuild-equivalent: born Kinematic with {5,0}
			}},
			{Tick: checkTick, Do: func(c *harness.Ctx) {
				t.Logf("positions after %d kinematic ticks: control.X=%.4f released.X=%.4f want=%.4f",
					checkTick-releaseTick, c.Pos(s.control).X, c.Pos(s.released).X, wantX)
				c.Near("control (FullRebuild-equivalent) reaches want X", c.Pos(s.control).X, wantX, 0.1)
				c.Near("released (Manual->Kinematic) matches control X", c.Pos(s.released).X, c.Pos(s.control).X, 0.1)
			}},
		},
	}
	runRepro(t, sc)
}

// TestReproManualReleaseAngular exercises the plain (non-FixedRotation) branch's angular
// push on a Manual->Dynamic transition. While Manual, the gameplay Angular velocity is
// bookkeeping and Box2D's angular velocity is forced to 0; on release to Dynamic (with
// FixedRotation=false), the cameFromManual path must push AngularVelocity too, matching
// CreateBody. The control is born Dynamic with the same angular velocity. Zero gravity,
// zero damping, an isolated circle and SleepingAllowed=false keep angular velocity
// constant so rotation is linear in time.
func TestReproManualReleaseAngular(t *testing.T) {
	t.Parallel()
	const releaseTick, checkTick = uint64(20), uint64(60)
	const av = 1.0 // rad/s
	wantRot := av * float64(checkTick-releaseTick) / 60.0
	var s struct{ released, control cardinal.EntityID }
	mk := func(c *harness.Ctx) physics.PhysicsBody2D {
		pb := physcomp.NewPhysicsBody2D(physcomp.BodyTypeManual, scenario.SampleShape(scenario.KindCircle).Spawn(c))
		pb.SleepingAllowed = false // isolate the angular push from solver sleep
		return pb
	}
	sc := harness.Scenario{
		Name: "repro-manual-release-angular",
		Setup: func(c *harness.Ctx) {
			s.released = c.SpawnSpinning("released", 0, 10, av, mk(c))
		},
		Steps: []harness.Step{
			{Tick: releaseTick, Do: func(c *harness.Ctx) {
				c.EditBody(s.released, func(pb *physics.PhysicsBody2D) { pb.BodyType = physics.BodyTypeDynamic })
				ctrl := mk(c)
				ctrl.BodyType = physics.BodyTypeDynamic
				s.control = c.SpawnSpinning("control", 0, 5, av, ctrl) // FullRebuild-equivalent: born Dynamic with av
			}},
			{Tick: checkTick, Do: func(c *harness.Ctx) {
				t.Logf("rotations after %d dynamic ticks: control=%.4f released=%.4f want=%.4f",
					checkTick-releaseTick, c.Rot(s.control), c.Rot(s.released), wantRot)
				c.Near("control (FullRebuild-equivalent) reaches want rotation", c.Rot(s.control), wantRot, 0.01)
				c.Near("released (Manual->Dynamic) matches control rotation", c.Rot(s.released), c.Rot(s.control), 0.01)
			}},
		},
	}
	runRepro(t, sc)
}

// TestReproManualReleaseParityVsRebuild is the direct parity spot-check for the
// reconcile.go:14-15 contract: on identical ECS inputs, ReconcileFromECS (the incremental
// release path) and FullRebuildFromECS (after Plugin.Reset) must produce the same Box2D
// velocity. It reads the engine's own linear velocity (not the component) for the released
// body and a control born Dynamic, asserts both equal the gameplay velocity before Reset,
// then forces a full rebuild with Plugin.Reset and asserts the velocities still match —
// proving the two paths agree. Before the fix, the released body's engine velocity stays
// {0,0} after the incremental release while the control's is {vx,0}.
func TestReproManualReleaseParityVsRebuild(t *testing.T) {
	t.Parallel()
	const releaseTick, readTick, resetTick, recheckTick = uint64(20), uint64(25), uint64(30), uint64(35)
	const vx = 7.0
	var s struct{ released, control cardinal.EntityID }
	engVelX := func(c *harness.Ctx, id cardinal.EntityID) float64 {
		bodyID, ok := c.Plugin().BodyID(id)
		if !c.True("body has a Box2D id", ok, "entity %d has no engine body", id) {
			return 0
		}
		v := c.Plugin().Engine().BodyLinearVelocity(bodyID)
		return v.X
	}
	mk := func(c *harness.Ctx) physics.PhysicsBody2D {
		pb := physcomp.NewPhysicsBody2D(physcomp.BodyTypeManual, scenario.Box(0.5, 0.5).Spawn(c))
		pb.SleepingAllowed = false
		return pb
	}
	sc := harness.Scenario{
		Name: "repro-manual-release-parity",
		Setup: func(c *harness.Ctx) {
			s.released = c.SpawnMoving("released", 0, 10, vx, 0, mk(c))
		},
		Steps: []harness.Step{
			{Tick: releaseTick, Do: func(c *harness.Ctx) {
				c.EditBody(s.released, func(pb *physics.PhysicsBody2D) { pb.BodyType = physics.BodyTypeDynamic })
				ctrl := mk(c)
				ctrl.BodyType = physics.BodyTypeDynamic
				s.control = c.SpawnMoving("control", 0, 5, vx, 0, ctrl) // FullRebuild-equivalent: born Dynamic with {7,0}
			}},
			{Tick: readTick, Do: func(c *harness.Ctx) {
				relV := engVelX(c, s.released)
				ctlV := engVelX(c, s.control)
				t.Logf("pre-reset engine velocities: released.X=%.4f control.X=%.4f want=%.4f",
					relV, ctlV, vx)
				c.Near("control (CreateBody path) has the gameplay velocity", ctlV, vx, 0.01)
				c.Near("released (ReconcileFromECS path) adopts the gameplay velocity", relV, vx, 0.01)
				c.Near("reconcile and CreateBody agree on the released body's velocity", relV, ctlV, 0.01)
			}},
			{Tick: resetTick, Do: func(c *harness.Ctx) {
				c.ExpectWorldReset()
				c.Plugin().Reset() // forces FullRebuildFromECS next tick
			}},
			{Tick: recheckTick, Do: func(c *harness.Ctx) {
				relV := engVelX(c, s.released)
				ctlV := engVelX(c, s.control)
				t.Logf("post-rebuild engine velocities: released.X=%.4f control.X=%.4f want=%.4f",
					relV, ctlV, vx)
				c.Near("control still has the gameplay velocity after FullRebuild", ctlV, vx, 0.01)
				c.Near("released keeps the gameplay velocity after FullRebuild", relV, vx, 0.01)
				c.Near("ReconcileFromECS and FullRebuildFromECS agree on identical ECS inputs", relV, ctlV, 0.01)
			}},
		},
	}
	runRepro(t, sc)
}

// TestReproStaticReleaseParityVsRebuild is the Static half of the same parity contract.
// Writeback skips Static bodies exactly as it skips Manual ones, so a Static body's shadow
// also holds a gameplay Velocity2D that Box2D never adopted. Releasing it to Dynamic
// without rewriting Velocity2D hits the identical VelocityDiffers blind spot, and the
// zero then propagates into Velocity2D via writeback, so a later FullRebuildFromECS cannot
// recover it either. The control is born Dynamic with the same velocity (the CreateBody
// path); the two must agree both before and after a forced full rebuild.
func TestReproStaticReleaseParityVsRebuild(t *testing.T) {
	t.Parallel()
	const releaseTick, readTick, resetTick, recheckTick = uint64(20), uint64(25), uint64(30), uint64(35)
	const vx = 7.0
	var s struct{ released, control cardinal.EntityID }
	engVelX := func(c *harness.Ctx, id cardinal.EntityID) float64 {
		bodyID, ok := c.Plugin().BodyID(id)
		if !c.True("body has a Box2D id", ok, "entity %d has no engine body", id) {
			return 0
		}
		return c.Plugin().Engine().BodyLinearVelocity(bodyID).X
	}
	mk := func(c *harness.Ctx, bt physics.BodyType) physics.PhysicsBody2D {
		pb := physcomp.NewPhysicsBody2D(bt, scenario.Box(0.5, 0.5).Spawn(c))
		pb.SleepingAllowed = false
		return pb
	}
	sc := harness.Scenario{
		Name: "repro-static-release-parity",
		Setup: func(c *harness.Ctx) {
			s.released = c.SpawnMoving("released", 0, 10, vx, 0, mk(c, physcomp.BodyTypeStatic))
		},
		Steps: []harness.Step{
			{Tick: releaseTick, Do: func(c *harness.Ctx) {
				c.EditBody(s.released, func(pb *physics.PhysicsBody2D) { pb.BodyType = physics.BodyTypeDynamic })
				s.control = c.SpawnMoving("control", 0, 5, vx, 0, mk(c, physcomp.BodyTypeDynamic))
			}},
			{Tick: readTick, Do: func(c *harness.Ctx) {
				relV, ctlV := engVelX(c, s.released), engVelX(c, s.control)
				t.Logf("pre-reset engine velocities: released.X=%.4f control.X=%.4f want=%.4f", relV, ctlV, vx)
				c.Near("control (CreateBody path) has the gameplay velocity", ctlV, vx, 0.01)
				c.Near("released (Static->Dynamic) adopts the gameplay velocity", relV, vx, 0.01)
			}},
			{Tick: resetTick, Do: func(c *harness.Ctx) {
				c.ExpectWorldReset()
				c.Plugin().Reset() // forces FullRebuildFromECS next tick
			}},
			{Tick: recheckTick, Do: func(c *harness.Ctx) {
				relV, ctlV := engVelX(c, s.released), engVelX(c, s.control)
				t.Logf("post-rebuild engine velocities: released.X=%.4f control.X=%.4f want=%.4f", relV, ctlV, vx)
				c.Near("released keeps the gameplay velocity after FullRebuild", relV, vx, 0.01)
				c.Near("ReconcileFromECS and FullRebuildFromECS agree on identical ECS inputs", relV, ctlV, 0.01)
			}},
		},
	}
	runRepro(t, sc)
}
