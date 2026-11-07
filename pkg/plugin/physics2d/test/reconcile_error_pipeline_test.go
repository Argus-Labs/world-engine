package physics2d_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// TestReconcileErrorDoesNotSkipLaterEntities_Pipeline is the plugin-level companion to
// internal.TestReconcileErrorDoesNotSkipLaterEntities. It drives the real PhysicsPipelineSystem
// (PreUpdate reconcile -> step -> writeback) via makeWorld + tickN instead of calling the Runtime
// phases directly, so it catches drift in the pipeline's gathering and ordering.
//
// Scenario: spawn A (lower EntityID) then B (higher EntityID) as dynamic boxes. After a few
// ticks, break A's collider (empty shapes) and give A a pending teleport while teleporting B.
// The pipeline must still reconcile B (A's failure does not abort the loop), B's teleport sticks,
// and A's pending teleport survives (writeback skips A). Repairing A's collider resumes writeback
// for A.
func TestReconcileErrorDoesNotSkipLaterEntities_Pipeline(t *testing.T) {
	t.Parallel()

	const (
		phaseApplyBreak   = 2 // PostUpdate: break A + pending teleport on A + teleport B.
		phaseVerifyFail   = 3 // PostUpdate: assert A frozen, B teleported; repair A.
		phaseVerifyResume = 4 // PostUpdate: assert A writeback resumed.
		lastTick          = phaseVerifyResume
	)

	boxShape := physics.ColliderShape{
		ShapeType:    physics.ShapeTypeBox,
		HalfExtents:  physics.Vec2{X: 0.5, Y: 0.5},
		Density:      1,
		CategoryBits: 0xFFFF,
		MaskBits:     0xFFFF,
	}

	var (
		idA, idB       cardinal.EntityID
		brokeA         bool
		repairedA      bool
		verifiedFail   bool
		verifiedResume bool
	)

	w, _ := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	// Spawn A (lower ID) then B (higher ID). The plugin's Init system runs first and creates the
	// physics singleton, so A and B get the next IDs in order.
	w.RegisterSystem(&reconcileErrorPipelineSpawn{run: func(w *cardinal.World) {
		if w.TickHeight() != 0 {
			return
		}
		spawn := func(x float64) cardinal.EntityID {
			row := w.Create[spawnArchetype]()
			id := row.ID()
			row.Set(harnessTag{Role: "box"})
			row.Set(physics.Transform2D{Position: physics.Vec2{X: x, Y: 50}})
			row.Set(physics.Velocity2D{})
			row.Set(newRigid(physics.BodyTypeDynamic, boxShape))
			return id
		}
		idA = spawn(0)
		idB = spawn(100)
	}}, cardinal.WithHook(cardinal.Init))

	w.RegisterSystem(&reconcileErrorPipelineDriver{run: func(w *cardinal.World) {
		req := require.New(t)
		tick := w.TickHeight()

		read := func(id cardinal.EntityID) physics.Transform2D {
			for row := range w.Exact[spawnArchetype]().Iter() {
				if row.ID() == id {
					return row.Get[physics.Transform2D]()
				}
			}
			req.FailNowf("entity not found", "entity %d", id)
			return physics.Transform2D{}
		}
		setBody := func(id cardinal.EntityID, shapes ...physics.ColliderShape) {
			for row := range w.Exact[spawnArchetype]().Iter() {
				if row.ID() == id {
					row.Set(newRigid(physics.BodyTypeDynamic, shapes...))
					return
				}
			}
		}

		if tick == phaseApplyBreak && !brokeA {
			for row := range w.Exact[spawnArchetype]().Iter() {
				switch row.ID() {
				case idA:
					row.Set(physics.Transform2D{Position: physics.Vec2{X: 50, Y: 7}})
					row.Set(newRigid(physics.BodyTypeDynamic)) // empty shapes -> reconcile fails
				case idB:
					row.Set(physics.Transform2D{Position: physics.Vec2{X: 200, Y: 200}})
				}
			}
			brokeA = true
		}

		if tick == phaseVerifyFail && !verifiedFail {
			trA := read(idA)
			req.InDelta(50, trA.Position.X, epsilon, "A ECS X frozen at pending teleport: %v", trA)
			req.InDelta(7, trA.Position.Y, epsilon, "A ECS Y frozen at pending teleport (writeback skipped A): %v", trA)
			trB := read(idB)
			req.InDelta(200, trB.Position.X, epsilon, "B ECS X teleported (loop continued past A failure): %v", trB)
			req.Less(trB.Position.Y, 200.0, "B ECS Y fell from teleport (writeback ran for B): %v", trB)

			// Repair A: valid collider again, keep the pending teleport.
			setBody(idA, boxShape)
			repairedA = true
			verifiedFail = true
		}

		if tick == phaseVerifyResume && repairedA && !verifiedResume {
			trA := read(idA)
			req.InDelta(50, trA.Position.X, epsilon, "A ECS X at teleport X after repair: %v", trA)
			req.Less(trA.Position.Y, 7.0, "A ECS Y fell after repair (writeback resumed for A): %v", trA)
			verifiedResume = true
		}
	}}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, lastTick+1)

	require.True(t, verifiedResume, "resume phase did not run; lastTick must reach %d", phaseVerifyResume)
}

type reconcileErrorPipelineSpawn struct{ run func(w *cardinal.World) }

func (s *reconcileErrorPipelineSpawn) Run(w *cardinal.World) { s.run(w) }

type reconcileErrorPipelineDriver struct{ run func(w *cardinal.World) }

func (s *reconcileErrorPipelineDriver) Run(w *cardinal.World) { s.run(w) }
