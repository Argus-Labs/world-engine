package scenario

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// Compound covers multi-fixture bodies: each child's LocalOffset and
// LocalRotation must place real geometry in the world, the slot index must stay
// attached to the child that produced a contact, and the children's densities
// must combine into one centre of mass.
//
// Slot identity is the part worth guarding. The plugin documents shape identity
// as "index i in Shapes is fixture slot i", and every contact event a game
// receives is only useful if that mapping holds.
func Compound() harness.Scenario {
	var s struct {
		shelf    cardinal.EntityID
		tilted   cardinal.EntityID
		pad      cardinal.EntityID
		dumbbell cardinal.EntityID
		targets  cardinal.EntityID
		dropper  cardinal.EntityID
		lopsided cardinal.EntityID
		evenTop  cardinal.EntityID
	}

	const (
		// Centre of mass of the lopsided spinner: (20*1 - 1*1) / 21 metres out
		// along +x from the body origin.
		lopsidedCOM = 19.0 / 21.0
		spinRate    = 4.0
		tiltAngle   = math.Pi / 4
		settleTick  = 200
		comTick     = 30
		wrapTick    = 89
	)

	return harness.Scenario{
		Name: "compound",
		Setup: func(c *harness.Ctx) {
			// Row y=0 — three children spread along the body. Each must be
			// findable at its own offset and nowhere else.
			s.shelf = c.Spawn("three-part-shelf", 0, 0, body(physics.BodyTypeStatic,
				atOffset(box(0.5, 0.5), -3, 0),
				circle(0.5),
				atOffset(box(0.5, 0.5), 3, 0),
			))

			// Row y=10 — a long thin child rotated 45 degrees in body space.
			// A probe along its new long axis must hit; the perpendicular
			// direction, which the unrotated box would also have covered, must not.
			s.tilted = c.Spawn("tilted-plank", 0, 10, body(physics.BodyTypeStatic,
				rotatedBy(box(2, 0.2), tiltAngle),
			))

			// Row y=20 — two equal children either side of the origin. Equal mass
			// either side means the body must settle perfectly level, and its
			// resting height is set by the children's half-height, not the body
			// origin, which is only true if the offsets really reached Box2D.
			s.pad = c.Spawn("dumbbell-pad", 0, 19, body(physics.BodyTypeStatic, box(5, 1)))
			s.dumbbell = c.Spawn("dumbbell", 0, 20.6, body(physics.BodyTypeDynamic,
				atOffset(box(0.5, 0.5), -1, 0),
				atOffset(box(0.5, 0.5), 1, 0),
			))

			// Row y=30 — a ball dropped onto slot 1 of a two-slot body.
			s.targets = c.Spawn("two-slot-target", 0, 30, body(physics.BodyTypeStatic,
				atOffset(box(0.5, 0.5), -3, 0),
				atOffset(box(0.5, 0.5), 3, 0),
			))
			s.dropper = c.Spawn("slot-1-dropper", 3, 36,
				body(physics.BodyTypeDynamic, circle(0.5)))

			// Rows y=40 and y=140 — where the centre of mass actually is.
			//
			// Both bodies are spawned with zero linear velocity and the same
			// spin. Box2D applies b2BodyDef.linearVelocity to the body origin,
			// then, once the shapes fix the centre of mass, adjusts the stored
			// velocity by omega x (centre - origin) so the origin's velocity is
			// preserved. So a body whose mass sits d metres off its origin comes
			// out of setup with a centre-of-mass velocity of exactly omega*d,
			// which makes the reported velocity a direct measurement of d.
			//
			// The rows are far apart because the lopsided body really does
			// translate away as a result, which is the point.
			spinner := func(leftDensity, rightDensity float64) physics.PhysicsBody2D {
				pb := body(physics.BodyTypeDynamic,
					atOffset(withDensity(box(0.5, 0.5), leftDensity), -1, 0),
					atOffset(withDensity(box(0.5, 0.5), rightDensity), 1, 0),
				)
				pb.GravityScale = 0
				return pb
			}
			s.lopsided = c.SpawnSpinning("lopsided-spinner", 0, 40, spinRate, spinner(1, 20))
			s.evenTop = c.SpawnSpinning("even-spinner", 0, 140, spinRate, spinner(10, 10))
		},
		Steps: []harness.Step{
			{Tick: 3, Do: func(c *harness.Ctx) {
				// Raycasts, not AABB overlaps: a ray runs the real narrow phase,
				// so "hit" means geometry is genuinely there. Each probe drops a
				// short vertical ray through where a child should — or should
				// not — be.
				probe := func(name string, x, y float64, wantSlot int, wantHit bool) {
					res := c.Raycast(x, y+2, x, y-2, nil)
					hit := res.Hit && res.Entity == s.shelf
					if !c.True("compound: "+name, hit == wantHit,
						"ray down x=%.2f: hit=%v (entity %d), want hit=%v",
						x, res.Hit, res.Entity, wantHit) {
						return
					}
					if wantHit {
						c.Int("compound: "+name+" reports its own slot", res.ShapeIndex, wantSlot)
					}
				}
				probe("slot 0 sits at its -3 offset", -3, 0, 0, true)
				probe("slot 1 sits at the body origin", 0, 0, 1, true)
				probe("slot 2 sits at its +3 offset", 3, 0, 2, true)
				probe("nothing sits between slot 0 and slot 1", -1.5, 0, 0, false)
				probe("nothing sits between slot 1 and slot 2", 1.5, 0, 0, false)

				// A 4x0.4 plank turned 45 degrees runs along y=x through the body
				// origin. It must be found on that diagonal and nowhere near the
				// axis-aligned strip it used to occupy.
				along := c.Raycast(1.2, 13, 1.2, 10.5, nil)
				c.True("compound: LocalRotation puts the child on its new axis",
					along.Hit && along.Entity == s.tilted,
					"a ray down the rotated plank's diagonal found nothing")

				flat := c.Raycast(1.9, 10.15, 1.9, 9.85, nil)
				c.False("compound: a rotated child no longer covers its old axis",
					flat.Hit && flat.Entity == s.tilted,
					"the plank is still where it would be with LocalRotation ignored")

				mirrored := c.Raycast(1.2, 9.5, 1.2, 7, nil)
				c.False("compound: LocalRotation turns one way, not both",
					mirrored.Hit && mirrored.Entity == s.tilted,
					"the plank was found on the diagonal opposite the one it was rotated onto")
			}},
			{Tick: settleTick, Do: func(c *harness.Ctx) {
				// Equal children either side: level, and resting on the radius.
				pos := c.Pos(s.dumbbell)
				c.Near("compound: symmetric body settles level", c.Rot(s.dumbbell), 0, 0.02)
				c.Near("compound: symmetric body rests on its children's half-height",
					pos.Y, 20.5, 0.1)
				c.Near("compound: symmetric body does not drift sideways", pos.X, 0, 0.1)

				// Slot identity through a contact event.
				hits := c.EventsBetween(harness.ContactBegin, s.dropper, s.targets)
				if c.IntAtLeast("compound: dropping onto slot 1 raises a contact",
					len(hits), 1) {
					idx, ok := hits[0].ShapeIndexFor(s.targets)
					c.True("compound: the contact event names the compound body", ok,
						"event carried entities %d and %d",
						hits[0].Payload.EntityA, hits[0].Payload.EntityB)
					c.Int("compound: the contact event names the struck slot", idx, 1)

					dropperIdx, _ := hits[0].ShapeIndexFor(s.dropper)
					c.Int("compound: the single-shape body reports slot 0", dropperIdx, 0)
				}
				c.Near("compound: the ball rests on slot 1", c.Pos(s.dropper).Y, 31, 0.15)
			}},
			{Tick: comTick, Do: func(c *harness.Ctx) {
				// v = omega x d, so v/omega recovers the centre-of-mass offset
				// the two children's densities imply.
				v := c.Vel(s.lopsided)
				c.Near("compound: children's densities set the centre of mass",
					v.Y/spinRate, lopsidedCOM, 0.01)
				c.Near("compound: the centre-of-mass offset is purely along the "+
					"axis the children sit on", v.X, 0, 1e-6)
				c.NearVec("compound: an evenly loaded body has no centre-of-mass offset",
					c.Vel(s.evenTop), vec(0, 0), 1e-6)
				c.Note("centre of mass measured from omega x d: %.4f m "+
					"(predicted %.4f m from densities 1 and 20 at +/-1 m)",
					v.Y/spinRate, lopsidedCOM)
				c.Note("a compound body spawned with Velocity2D.Linear zero comes out "+
					"of setup translating at %.3f m/s, because Box2D applies the "+
					"spawn velocity to the body origin and then re-expresses it "+
					"about the centre of mass", v.Y)

				c.Near("compound: a free spinner keeps its angular velocity",
					c.AngVel(s.lopsided), spinRate, 1e-3)
			}},
			{Tick: wrapTick, Do: func(c *harness.Ctx) {
				// Rotation comes back through Box2D's cos/sin b2Rot, so the angle
				// written into Transform2D is always the wrapped one. A game that
				// accumulates rotation cannot read it back from the component.
				spun := spinRate * float64(wrapTick+1) / harness.TickRate
				wrapped := math.Mod(spun+math.Pi, 2*math.Pi) - math.Pi
				got := c.Rot(s.evenTop)
				c.LessEq("Transform2D.Rotation stays within [-pi, pi]", math.Abs(got), math.Pi)
				c.Near("Transform2D.Rotation is the wrapped angle, not the accumulated one",
					got, wrapped, 0.05)
				c.Note("after %.2f rad of spin the component reports %.3f rad: "+
					"rotation is wrapped to [-pi, pi] on writeback", spun, got)
			}},
		},
	}
}
