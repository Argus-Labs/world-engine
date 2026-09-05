package scenario

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// Contacts checks what a ContactBegin/ContactEnd event actually carries: the
// right pair of entities, the right shape slots, a usable normal and point, and
// the fixture filters that produced the contact. It also pins the event
// lifecycle — one Begin per touch, an End when the bodies part, and no per-tick
// re-firing while two bodies rest against each other.
//
// The contact point is checked against world space on purpose. The manifold
// stores it as an anchor relative to body A's centre of mass, and turning that
// into the world point a game actually wants is a one-line addition the plugin
// has to make. Every scenario runs in its own lane hundreds of metres from the
// origin, which makes the two frames impossible to confuse.
func Contacts() harness.Scenario {
	var s struct {
		floor      cardinal.EntityID
		ball       cardinal.EntityID
		liftPad    cardinal.EntityID
		lifted     cardinal.EntityID
		bouncePad  cardinal.EntityID
		bouncer    cardinal.EntityID
		restPad    cardinal.EntityID
		rester     cardinal.EntityID
		ballCat    uint64
		floorCat   uint64
		ballGroup  int32
		floorGroup int32
	}
	s.ballCat, s.floorCat = 0x100, 0x200
	s.ballGroup, s.floorGroup = 3, -2

	const (
		dropY      = 6.0
		restSpawn  = 30
		liftTick   = 150
		earlyCheck = 140
		lateCheck  = 300
	)

	awake := func(pb physics.PhysicsBody2D) physics.PhysicsBody2D {
		pb.SleepingAllowed = false
		return pb
	}

	return harness.Scenario{
		Name: "contacts",
		Setup: func(c *harness.Ctx) {
			// Row y=0 — one clean landing, with distinctive filters on both sides
			// so the event's filter fields have something to be wrong about.
			s.floor = c.Spawn("floor", 0, groundY, body(physics.BodyTypeStatic,
				withFilter(box(20, 1), s.floorCat, maskAll, s.floorGroup)))
			s.ball = c.Spawn("landing-ball", 0, dropY, body(physics.BodyTypeDynamic,
				withFilter(circle(0.5), s.ballCat, maskAll, s.ballGroup)))

			// Row y=20 — a box that is launched off its pad partway through, so
			// the End event has an unambiguous cause.
			s.liftPad = c.Spawn("lift-pad", 0, 19, body(physics.BodyTypeStatic, box(3, 1)))
			s.lifted = c.Spawn("lifted-box", 0, 26,
				awake(body(physics.BodyTypeDynamic, box(0.5, 0.5))))

			// Row y=40 — a bouncing ball must produce a Begin/End pair per bounce.
			s.bouncePad = c.Spawn("bounce-pad", 0, 39, body(physics.BodyTypeStatic, box(3, 1)))
			s.bouncer = c.Spawn("bouncing-ball", 0, 48,
				body(physics.BodyTypeDynamic, withRestitution(circle(0.5), 0.75)))

			// Row y=60 — the resting pad. Its box is created mid-run (see below)
			// because an overlap that exists at world build time is suppressed.
			s.restPad = c.Spawn("rest-pad", 0, 59, body(physics.BodyTypeStatic, box(3, 1)))
		},
		Steps: []harness.Step{
			{Tick: restSpawn, Do: func(c *harness.Ctx) {
				// Dropped from 5 cm so it lands once and stays put.
				s.rester = c.Spawn("resting-box", 0, 60.55,
					body(physics.BodyTypeDynamic, box(0.5, 0.5)))
			}},
			{Tick: earlyCheck, Do: func(c *harness.Ctx) {
				begins := c.EventsBetween(harness.ContactBegin, s.ball, s.floor)
				if !c.IntAtLeast("landing raises ContactBegin", len(begins), 1) {
					return
				}
				e := begins[0]

				ballIdx, ballOK := e.ShapeIndexFor(s.ball)
				floorIdx, floorOK := e.ShapeIndexFor(s.floor)
				c.True("ContactBegin names the ball", ballOK, "event lacked entity %d", s.ball)
				c.True("ContactBegin names the floor", floorOK, "event lacked entity %d", s.floor)
				c.Int("ContactBegin reports the ball's only shape slot", ballIdx, 0)
				c.Int("ContactBegin reports the floor's only shape slot", floorIdx, 0)

				c.True("ContactBegin carries a valid normal", e.Payload.NormalValid,
					"NormalValid was false on a solid landing")
				n := e.Payload.Normal
				c.Near("the contact normal is a unit vector", math.Hypot(n.X, n.Y), 1, 0.01)
				c.Less("a flat landing produces a vertical normal", math.Abs(n.X), 0.05)
				c.True("ContactBegin carries a valid point", e.Payload.PointValid,
					"PointValid was false on a solid landing")

				// Filter round-trip: what Box2D reports must be what ECS asked for.
				if bf, ok := e.FilterFor(s.ball); ok {
					c.True("event reports the ball's category bits",
						bf.CategoryBits == s.ballCat, "got %#x, want %#x", bf.CategoryBits, s.ballCat)
					c.True("event reports the ball's mask bits",
						bf.MaskBits == maskAll, "got %#x, want %#x", bf.MaskBits, maskAll)
					c.True("event reports the ball's group index",
						bf.GroupIndex == s.ballGroup, "got %d, want %d", bf.GroupIndex, s.ballGroup)
				}
				if ff, ok := e.FilterFor(s.floor); ok {
					c.True("event reports the floor's category bits",
						ff.CategoryBits == s.floorCat, "got %#x, want %#x", ff.CategoryBits, s.floorCat)
					c.True("event reports the floor's negative group index",
						ff.GroupIndex == s.floorGroup, "got %d, want %d", ff.GroupIndex, s.floorGroup)
				}

				// Frame check. In world space the point sits at this lane's X,
				// give or take the ball's radius; relative to a body centre it
				// sits within a metre of zero.
				p := e.Payload.Point
				worldish := math.Abs(p.X-c.Lane()) < 2
				bodyish := math.Abs(p.X) < 2
				c.True("ContactBegin.Point is a world-space point", worldish,
					"got (%.4f, %.4f); this lane's origin is x=%.0f. "+
						"internal/contact_listener.go copies Manifold.Points[0].AnchorA "+
						"straight through, and that anchor is relative to body A's "+
						"centre of mass, so a caller has to add body A's centre to "+
						"use it — and nothing in the plugin says so",
					p.X, p.Y, c.Lane())
				c.Note("contact point reported as (%.4f, %.4f) with lane origin x=%.0f "+
					"— world-space reading %v, body-relative reading %v",
					p.X, p.Y, c.Lane(), worldish, bodyish)
			}},
			{Tick: liftTick, Do: func(c *harness.Ctx) {
				c.Int("a resting pair does not re-fire ContactBegin every tick",
					c.CountBetween(harness.ContactBegin, s.lifted, s.liftPad), 1)
				c.Int("a resting pair raises no ContactEnd while it rests",
					c.CountBetween(harness.ContactEnd, s.lifted, s.liftPad), 0)
				c.SetVel(s.lifted, 0, 9)
			}},
			{Tick: lateCheck, Do: func(c *harness.Ctx) {
				c.IntAtLeast("leaving a surface raises ContactEnd",
					c.CountBetween(harness.ContactEnd, s.lifted, s.liftPad), 1)

				bounceBegins := c.CountBetween(harness.ContactBegin, s.bouncer, s.bouncePad)
				bounceEnds := c.CountBetween(harness.ContactEnd, s.bouncer, s.bouncePad)
				c.IntAtLeast("a bouncing ball raises a Begin per bounce", bounceBegins, 2)
				c.IntAtLeast("a bouncing ball raises an End per bounce", bounceEnds, 1)
				c.LessEq("ends never outnumber begins", float64(bounceEnds), float64(bounceBegins))
				c.Note("bouncing ball produced %d ContactBegin and %d ContactEnd events",
					bounceBegins, bounceEnds)

				c.Int("a settled box raises exactly one ContactBegin",
					c.CountBetween(harness.ContactBegin, s.rester, s.restPad), 1)
				c.Int("a settled box raises no ContactEnd",
					c.CountBetween(harness.ContactEnd, s.rester, s.restPad), 0)
				c.Near("the settled box is still on its pad", c.Pos(s.rester).Y, 60.5, 0.1)
			}},
		},
	}
}
