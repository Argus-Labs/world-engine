package scenario

import (
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// Material covers the three per-fixture material values — restitution, friction
// and density — and, more importantly, the rules Box2D uses to combine two
// fixtures' values on contact. Box2D v3 mixes friction as sqrt(fA*fB) and
// restitution as max(rA, rB); a port that reached for the other obvious choice
// (max for friction, multiply for restitution) would still look plausible in
// isolation and only show up in a test like this one.
func Material() harness.Scenario {
	var s struct {
		floor      cardinal.EntityID
		deadBall   cardinal.EntityID
		bouncyBall cardinal.EntityID
		slickFloor cardinal.EntityID
		slickBox   cardinal.EntityID
		grippyFlr  cardinal.EntityID
		grippyBox  cardinal.EntityID
		heavy      cardinal.EntityID
		light      cardinal.EntityID
		evenA      cardinal.EntityID
		evenB      cardinal.EntityID
		apex       map[cardinal.EntityID]float64
	}
	s.apex = map[cardinal.EntityID]float64{}

	const (
		dropY        = 6.0
		firstImpact  = 66
		bounceCheck  = 200
		slideCheck   = 180
		impactCheck  = 200
		floorFrict   = 0.6
		slideSpeed   = 5.0
		headOnSpeed  = 3.0
		heavyDensity = 10.0
	)

	zeroG := func(shapes ...physics.ColliderShape) physics.PhysicsBody2D {
		pb := body(physics.BodyTypeDynamic, shapes...)
		pb.GravityScale = 0
		return pb
	}

	return harness.Scenario{
		Name: "material",
		Setup: func(c *harness.Ctx) {
			// Each sub-test gets its own row. Sharing a floor would let a sliding
			// box wander into a bouncing ball and quietly ruin both results.

			// Row y=0 — bounce. The floor's restitution is 0. Box2D takes the
			// *max* of the two restitutions, so a bouncy ball must still bounce
			// off it; that single fact is the combine-rule test.
			s.floor = c.Spawn("bounce-floor", -30, groundY,
				body(physics.BodyTypeStatic, withFriction(box(20, 1), floorFrict)))
			s.deadBall = c.Spawn("restitution-0", -40, dropY,
				body(physics.BodyTypeDynamic, circle(0.5)))
			s.bouncyBall = c.Spawn("restitution-0.8", -30, dropY,
				body(physics.BodyTypeDynamic, withRestitution(circle(0.5), 0.8)))

			// Row y=10 — a frictionless box on a gritty platform. sqrt(0*0.6) is
			// exactly 0, so it must not lose a millimetre per second.
			s.slickFloor = c.Spawn("slick-platform", 0, 9,
				body(physics.BodyTypeStatic, withFriction(box(30, 1), floorFrict)))
			s.slickBox = c.SpawnMoving("friction-0", -25, 10.5, slideSpeed, 0,
				body(physics.BodyTypeDynamic, withFriction(box(0.5, 0.5), 0)))

			// Row y=20 — same platform material, a gripping box.
			s.grippyFlr = c.Spawn("grippy-platform", 0, 19,
				body(physics.BodyTypeStatic, withFriction(box(30, 1), floorFrict)))
			s.grippyBox = c.SpawnMoving("friction-0.9", -5, 20.5, slideSpeed, 0,
				body(physics.BodyTypeDynamic, withFriction(box(0.5, 0.5), 0.9)))

			// Rows y=30 and y=40 — density becomes mass becomes momentum. Two
			// head-on pairs at the same closing speed: the mismatched pair must
			// end up moving the heavy body's way, the matched pair must cancel.
			s.heavy = c.SpawnMoving("density-10", 40, 30, headOnSpeed, 0,
				zeroG(withDensity(box(0.5, 0.5), heavyDensity)))
			s.light = c.SpawnMoving("density-1", 50, 30, -headOnSpeed, 0,
				zeroG(box(0.5, 0.5)))

			s.evenA = c.SpawnMoving("even-mass-a", 40, 40, headOnSpeed, 0,
				zeroG(box(0.5, 0.5)))
			s.evenB = c.SpawnMoving("even-mass-b", 50, 40, -headOnSpeed, 0,
				zeroG(box(0.5, 0.5)))
		},
		EachTick: func(c *harness.Ctx) {
			// Record how high each ball gets back after its first landing.
			if c.Tick() < firstImpact {
				return
			}
			for _, id := range []cardinal.EntityID{s.deadBall, s.bouncyBall} {
				if y := c.Pos(id).Y; y > s.apex[id] {
					s.apex[id] = y
				}
			}
		},
		Steps: []harness.Step{
			{Tick: slideCheck, Do: func(c *harness.Ctx) {
				slickX := c.Pos(s.slickBox).X
				grippyX := c.Pos(s.grippyBox).X
				slickSpeed := c.Speed(s.slickBox)

				// sqrt(0 * 0.6) == 0: a frictionless box on a gritty floor is
				// frictionless. Under a max() rule it would have ground to a halt.
				c.Near("friction combines as sqrt(a*b), so friction 0 stays frictionless",
					slickSpeed, slideSpeed, 0.05)
				c.Near("frictionless box keeps sliding at its launch speed",
					slickX, -25+slideSpeed*float64(slideCheck)/harness.TickRate, 0.3)

				c.Less("friction 0.9 brings the box to a stop", c.Speed(s.grippyBox), 0.05)
				c.Less("friction 0.9 stops the box within a couple of metres",
					grippyX+5, 3.0)
				c.Greater("friction 0.9 box still moved before stopping", grippyX+5, 0.2)
				c.Note("slide at t=%d: frictionless box travelled %.2f m, "+
					"friction-0.9 box travelled %.2f m", slideCheck, slickX+25, grippyX+5)
			}},
			{Tick: bounceCheck, Do: func(c *harness.Ctx) {
				dead := s.apex[s.deadBall]
				bouncy := s.apex[s.bouncyBall]

				c.Greater("restitution combines as max(a,b), so a bouncy ball "+
					"bounces off a dead floor", bouncy, 1.5)
				c.Less("restitution 0 does not bounce", dead, 0.75)
				c.Greater("bouncy ball out-bounces the dead ball", bouncy-dead, 1.0)
				c.Note("bounce apex: restitution-0 ball %.3f m, restitution-0.8 ball %.3f m "+
					"(Box2D ignores bounces below its 1 m/s restitution threshold)",
					dead, bouncy)

				c.Near("dead ball settles on the floor", c.Pos(s.deadBall).Y, 0.5, 0.15)
			}},
			{Tick: impactCheck, Do: func(c *harness.Ctx) {
				heavyV := c.Vel(s.heavy).X
				lightV := c.Vel(s.light).X

				c.IntAtLeast("head-on pair actually collided",
					c.CountBetween(harness.ContactBegin, s.heavy, s.light), 1)
				c.Greater("the denser body keeps going its own way", heavyV, 1.0)
				c.Greater("the lighter body is turned around", lightV, 0.5)
				c.Less("the denser body is slowed by the impact", heavyV, headOnSpeed)
				c.Note("density 10 vs density 1 head-on: heavy %.3f m/s, light %.3f m/s "+
					"(inelastic prediction %.3f m/s)",
					heavyV, lightV, headOnSpeed*(heavyDensity-1)/(heavyDensity+1))

				// Equal masses at equal and opposite speeds: nothing survives.
				c.IntAtLeast("even-mass pair actually collided",
					c.CountBetween(harness.ContactBegin, s.evenA, s.evenB), 1)
				c.Near("equal masses cancel out head-on", c.Vel(s.evenA).X, 0, 0.2)
				c.Near("equal masses cancel out head-on (other side)",
					c.Vel(s.evenB).X, 0, 0.2)
			}},
		},
	}
}
