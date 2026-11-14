package scenario

import (
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// Dormant covers the velocity-survival contract for a temporarily disabled body: setting Active=false
// must not wipe the gameplay-authored Velocity2D, and on re-enable the held velocity must take
// effect so the body resumes moving instead of freezing at its last position forever.
//
// The existing "flags" scenario checks only that an inactive body does not fall and that re-enabling
// resumes gravity; it never authors a velocity on the inactive body, so it cannot catch the
// velocity wipe. This scenario closes that gap with two bodies:
//
//   - "cooled-mover": spawned moving, disabled mid-flight, then re-enabled. While disabled its ECS
//     velocity must stay at the authored value and its position must freeze; after re-enable it
//     must resume moving at that velocity.
//   - "born-dormant": spawned with Active=false and a nonzero velocity. While disabled the velocity
//     must survive in ECS; after re-enable it must be applied so the body moves.
//
// Both bodies use GravityScale=0 and FixedRotation so the only motion is the authored linear
// velocity, isolating the effect from gravity and spin.
func Dormant() harness.Scenario {
	var s struct {
		cooledMover cardinal.EntityID
		bornDormant cardinal.EntityID
		frozenX     float64
		dormantX    float64
	}

	const (
		disableTick = 40
		frozenTick  = 50
		enableTick  = 50
		resumeTick  = 110
		moverSpeed  = 5.0
	)

	// zeroGDrifter is a dynamic circle that isolates authored linear motion: no gravity, no spin.
	zeroGDrifter := func(active bool) physics.PhysicsBody2D {
		pb := body(physics.BodyTypeDynamic, circle(0.5))
		pb.GravityScale = 0
		pb.FixedRotation = true
		pb.SleepingAllowed = false
		pb.Active = active
		return pb
	}

	return harness.Scenario{
		Name: "dormant",
		Setup: func(c *harness.Ctx) {
			// Row y=10: spawned moving, disabled mid-flight, re-enabled.
			s.cooledMover = c.SpawnMoving("cooled-mover", 0, 10, moverSpeed, 0, zeroGDrifter(true))

			// Row y=20: created disabled with a nonzero velocity.
			s.bornDormant = c.SpawnFull("born-dormant",
				physics.Transform2D{Position: physics.Vec2{X: 0, Y: 20}},
				physics.Velocity2D{Linear: physics.Vec2{X: moverSpeed, Y: 0}},
				zeroGDrifter(false))
		},
		Steps: []harness.Step{
			{Tick: disableTick, Do: func(c *harness.Ctx) {
				// The mover has been travelling since spawn; record its position and disable it.
				s.frozenX = c.Pos(s.cooledMover).X
				c.Greater("mover travelled before being disabled", s.frozenX, 1.0)
				c.EditBody(s.cooledMover, func(pb *physics.PhysicsBody2D) { pb.Active = false })
			}},
			{Tick: frozenTick, Do: func(c *harness.Ctx) {
				// While disabled the ECS velocity must NOT be stomped to zero, and the position
				// must be frozen at the disable-tick position.
				c.Near("disabled mover keeps its authored ECS velocity",
					c.Vel(s.cooledMover).X, moverSpeed, 1e-9)
				c.Near("disabled mover position is frozen",
					c.Pos(s.cooledMover).X, s.frozenX, 1e-9)

				// The born-dormant body has been disabled since creation: its velocity must also
				// survive in ECS, and it must not have moved.
				c.Near("body created disabled keeps its authored ECS velocity",
					c.Vel(s.bornDormant).X, moverSpeed, 1e-9)
				s.dormantX = c.Pos(s.bornDormant).X
				c.Near("body created disabled never moved", s.dormantX, 0, 1e-9)

				// Re-enable both bodies.
				c.EditBody(s.cooledMover, func(pb *physics.PhysicsBody2D) { pb.Active = true })
				c.EditBody(s.bornDormant, func(pb *physics.PhysicsBody2D) { pb.Active = true })
			}},
			{Tick: resumeTick, Do: func(c *harness.Ctx) {
				// After re-enable, both bodies must resume moving at the held velocity, not be
				// frozen at their disabled positions forever.
				moverX := c.Pos(s.cooledMover).X
				c.Greater("re-enabled mover resumed moving beyond its frozen position",
					moverX, s.frozenX+1.0)
				c.Near("re-enabled mover keeps its authored velocity",
					c.Vel(s.cooledMover).X, moverSpeed, 0.05)

				dormantX := c.Pos(s.bornDormant).X
				c.Greater("born-dormant body moved after being enabled",
					dormantX, s.dormantX+1.0)
				c.Near("born-dormant body keeps its authored velocity after enable",
					c.Vel(s.bornDormant).X, moverSpeed, 0.05)
			}},
		},
	}
}
