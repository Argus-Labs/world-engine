package physics2d_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// TestDisabledVelocitySurvivesDisableEnable_Pipeline is the plugin-level companion to the internal
// whitebox tests in internal.TestDisabledVelocity_*. It drives the real PhysicsPipelineSystem
// (PreUpdate reconcile -> step -> writeback) via makeWorld + tickN instead of calling the Runtime
// phases directly, so it catches drift in the pipeline's gathering and ordering for the
// disabled-body velocity survival behavior.
//
// Scenario: a zero-gravity dynamic body is spawned moving at 8 m/s. After it has moved, it is set
// Active=false. While disabled its ECS Velocity2D must stay at 8 m/s (not be stomped to zero by
// writeback) and its position must freeze. Re-enabling must apply the held velocity so the body
// resumes moving instead of remaining dead-still at its last position.
func TestDisabledVelocitySurvivesDisableEnable_Pipeline(t *testing.T) {
	t.Parallel()

	const (
		phaseDisable = 5  // PostUpdate: the body has moved; disable it.
		phaseFrozen  = 8  // PostUpdate: assert velocity preserved + position frozen; re-enable.
		phaseResumed = 20 // PostUpdate: assert the body resumed moving at the held velocity.
		lastTick     = phaseResumed
	)

	boxShape := physics.ColliderShape{
		ShapeType:    physics.ShapeTypeBox,
		HalfExtents:  physics.Vec2{X: 0.5, Y: 0.5},
		Density:      1,
		CategoryBits: 0xFFFF,
		MaskBits:     0xFFFF,
	}

	var (
		id              cardinal.EntityID
		disabled        bool
		verifiedFrozen  bool
		verifiedResumed bool
		frozenX         float64
	)

	w, p := makeWorld(t, physics.Vec2{X: 0, Y: 0}) // zero gravity: velocity is the only motion

	// Spawn the moving body at Init.
	w.RegisterSystem(&disabledVelSpawn{run: func(w *cardinal.World) {
		if w.TickHeight() != 0 {
			return
		}
		pb := newRigidNoGravity(physics.BodyTypeDynamic, boxShape)
		pb.SleepingAllowed = false // keep it awake throughout, no sleep interaction
		row := w.Create[spawnArchetype]()
		id = row.ID()
		row.Set(harnessTag{Role: "mover"})
		row.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.Set(physics.Velocity2D{Linear: physics.Vec2{X: 8, Y: 0}})
		row.Set(pb)
	}}, cardinal.WithHook(cardinal.Init))

	w.RegisterSystem(&disabledVelDriver{run: func(w *cardinal.World) {
		req := require.New(t)
		tick := w.TickHeight()

		read := func(id cardinal.EntityID) (physics.Transform2D, physics.Velocity2D) {
			for row := range w.Exact[spawnArchetype]().Iter() {
				if row.ID() == id {
					return row.Get[physics.Transform2D](), row.Get[physics.Velocity2D]()
				}
			}
			req.FailNowf("entity not found", "entity %d", id)
			return physics.Transform2D{}, physics.Velocity2D{}
		}
		setActive := func(id cardinal.EntityID, active bool) {
			for row := range w.Exact[spawnArchetype]().Iter() {
				if row.ID() == id {
					pb := row.Get[physics.PhysicsBody2D]()
					pb.Active = active
					row.Set(pb)
					return
				}
			}
		}
		engineVel := func(id cardinal.EntityID) physics.Vec2 {
			bid, ok := p.BodyID(id)
			if !ok || p.Engine() == nil {
				return physics.Vec2{}
			}
			lv := p.Engine().BodyLinearVelocity(bid)
			return physics.Vec2{X: lv.X, Y: lv.Y}
		}

		// phaseDisable: the body has been moving for several ticks. Record its position and disable.
		if tick == phaseDisable && !disabled {
			tr, _ := read(id)
			frozenX = tr.Position.X
			req.Greater(frozenX, 0.5, "body should have moved before disable: %v", tr)
			setActive(id, false)
			disabled = true
		}

		// phaseFrozen: velocity must survive (not stomped to zero), position must be frozen; re-enable.
		if tick == phaseFrozen && !verifiedFrozen {
			tr, vel := read(id)
			req.InDelta(8, vel.Linear.X, epsilon, "ECS velocity must survive disable: %v", vel)
			req.InDelta(frozenX, tr.Position.X, epsilon, "position must be frozen while disabled: %v", tr)
			req.Zero(engineVel(id).X, "engine velocity reads zero for a disabled body")
			setActive(id, true)
			verifiedFrozen = true
		}

		// phaseResumed: the body must have resumed moving at the held velocity, not frozen forever.
		if tick == phaseResumed && verifiedFrozen && !verifiedResumed {
			tr, vel := read(id)
			req.InDelta(8, vel.Linear.X, epsilon, "ECS velocity after re-enable: %v", vel)
			req.InDelta(8, engineVel(id).X, epsilon, "engine velocity must be applied on re-enable")
			req.Greater(tr.Position.X, frozenX+1.0,
				"body must resume moving after re-enable, frozen at %.4f now %.4f", frozenX, tr.Position.X)
			verifiedResumed = true
		}
	}}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, lastTick+1)

	require.True(t, verifiedResumed, "resume phase did not run; lastTick must reach %d", phaseResumed)
}

type disabledVelSpawn struct{ run func(w *cardinal.World) }

func (s *disabledVelSpawn) Run(w *cardinal.World) { s.run(w) }

type disabledVelDriver struct{ run func(w *cardinal.World) }

func (s *disabledVelDriver) Run(w *cardinal.World) { s.run(w) }
