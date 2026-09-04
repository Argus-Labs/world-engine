package scenario

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// Stability is the numerical-sanity scenario: the cases where a port that is
// otherwise correct starts producing garbage. A tall stack has to settle instead
// of jittering apart, deeply overlapped bodies have to push out at a bounded
// speed instead of exploding, very small and very large shapes have to behave,
// and float32 has to hold up thousands of metres from the origin.
//
// The harness also watches every body in every scenario for NaN and Inf on every
// tick, so anything that does blow up is reported against the body that did it.
func Stability() harness.Scenario {
	const (
		stackCount  = 10
		boxHalf     = 0.5
		stackGap    = 0.02
		settleTick  = 400
		overlapTick = 200
		// Negative, so this probe lands to the *left* of lane 0 no matter how
		// many scenarios are added. Lanes only ever extend in +x.
		farX = -6000.0
	)

	var s struct {
		floor     cardinal.EntityID
		stack     []cardinal.EntityID
		jamFloor  cardinal.EntityID
		jamA      cardinal.EntityID
		jamB      cardinal.EntityID
		jamPeak   float64
		tinyFloor cardinal.EntityID
		tiny      cardinal.EntityID
		bigFloor  cardinal.EntityID
		big       cardinal.EntityID
		hugeFloor cardinal.EntityID
		huge      cardinal.EntityID
		farFloor  cardinal.EntityID
		farBody   cardinal.EntityID
		restPad   cardinal.EntityID
		rester    cardinal.EntityID
	}

	return harness.Scenario{
		Name: "stability",
		Setup: func(c *harness.Ctx) {
			// Row y=0 — a ten-box stack, each box dropped a hair above the last.
			s.floor = c.Spawn("stack-floor", 0, groundY,
				body(physics.BodyTypeStatic, withFriction(box(10, 1), 0.6)))
			for i := range stackCount {
				y := boxHalf + float64(i)*(2*boxHalf+stackGap)
				s.stack = append(s.stack, c.Spawn("stack-box", 0, y,
					body(physics.BodyTypeDynamic, withFriction(box(boxHalf, boxHalf), 0.6))))
			}

			// Row y=30 — two boxes spawned almost entirely inside each other.
			// Box2D pushes overlap out at a bounded speed; a port that solved
			// this as a spring would fire them across the map.
			s.jamFloor = c.Spawn("jam-floor", 30, 29,
				body(physics.BodyTypeStatic, box(10, 1)))
			s.jamA = c.Spawn("jammed-a", 30, 30.5, body(physics.BodyTypeDynamic, box(0.5, 0.5)))
			s.jamB = c.Spawn("jammed-b", 30.1, 30.5, body(physics.BodyTypeDynamic, box(0.5, 0.5)))

			// Row y=60 — a 2 cm ball. Small shapes are where Box2D's linear slop
			// and speculative margins start to matter.
			s.tinyFloor = c.Spawn("tiny-floor", 0, 59, body(physics.BodyTypeStatic, box(2, 1)))
			s.tiny = c.Spawn("tiny-ball", 0, 62, body(physics.BodyTypeDynamic, circle(0.01)))

			// Row y=100 — a 10 m box, the top of the size range Box2D is tuned
			// for, and the one this scenario holds to a real tolerance.
			s.bigFloor = c.Spawn("big-floor", 0, 99, body(physics.BodyTypeStatic, box(20, 1)))
			s.big = c.Spawn("big-box", 0, 108, body(physics.BodyTypeDynamic, box(5, 5)))

			// Row y=300 — a 100 m box, well outside the range Box2D is tuned for
			// and two orders of magnitude larger than the stack boxes, on its own
			// floor with nothing else within a hundred metres.
			s.hugeFloor = c.Spawn("huge-floor", 0, 299, body(physics.BodyTypeStatic, box(80, 1)))
			s.huge = c.Spawn("huge-box", 0, 355, body(physics.BodyTypeDynamic, box(50, 50)))

			// Row y=0, far away — the same drop several kilometres from the
			// origin, where float32 has about a millimetre of resolution left.
			s.farFloor = c.Spawn("far-floor", farX, groundY,
				body(physics.BodyTypeStatic, box(10, 1)))
			s.farBody = c.Spawn("far-ball", farX, 8, body(physics.BodyTypeDynamic, circle(0.5)))

			// Row y=200 — a body that starts at rest and must stay at rest. Any
			// drift here is energy appearing from nowhere.
			s.restPad = c.Spawn("rest-pad", 0, 199, body(physics.BodyTypeStatic, box(3, 1)))
			s.rester = c.Spawn("resting-box", 0, 200.5,
				body(physics.BodyTypeDynamic, box(0.5, 0.5)))
		},
		EachTick: func(c *harness.Ctx) {
			// The jammed pair's separation speed, sampled every tick: the peak is
			// what a bad push-out solver would blow up.
			if sp := c.Speed(s.jamA); sp > s.jamPeak {
				s.jamPeak = sp
			}
		},
		Steps: []harness.Step{
			{Tick: overlapTick, Do: func(c *harness.Ctx) {
				a := c.Pos(s.jamA)
				b := c.Pos(s.jamB)
				sep := math.Abs(a.X - b.X)

				c.Greater("deeply overlapped bodies push apart", sep, 0.9)
				c.Less("deeply overlapped bodies do not overshoot", sep, 1.6)
				c.Less("overlap recovery speed stays bounded", s.jamPeak, 5.0)
				c.Less("a body pushed out of an overlap stays near where it started",
					math.Abs(a.X-30), 2.0)
				c.Note("two boxes spawned 0.1 m apart separated to %.3f m, peaking at "+
					"%.3f m/s during recovery", sep, s.jamPeak)
			}},
			{Tick: settleTick, Do: func(c *harness.Ctx) {
				// The stack: still stacked, still in order, and quiet.
				var maxSpeed, maxDrift float64
				for i, id := range s.stack {
					pos := c.Pos(id)
					wantY := boxHalf + float64(i)*2*boxHalf
					c.Near("stack box rests at its layer height", pos.Y, wantY, 0.2)
					if d := math.Abs(pos.X); d > maxDrift {
						maxDrift = d
					}
					if sp := c.Speed(id); sp > maxSpeed {
						maxSpeed = sp
					}
				}
				c.Less("a settled stack stops moving", maxSpeed, 0.05)
				c.Less("a settled stack does not walk sideways", maxDrift, 0.3)
				c.Note("stack of %d after %d ticks: max speed %.4f m/s, max sideways "+
					"drift %.4f m", stackCount, settleTick, maxSpeed, maxDrift)

				// Extremes of scale.
				c.Near("a 2 cm ball rests on its own radius", c.Pos(s.tiny).Y, 60.01, 0.03)
				c.Less("a 2 cm ball comes to rest", c.Speed(s.tiny), 0.05)
				c.Near("a 10 m box rests on its own half-height", c.Pos(s.big).Y, 105, 0.05)
				c.Less("a 10 m box comes to rest", c.Speed(s.big), 0.05)

				hugeY := c.Pos(s.huge).Y
				c.Near("a 100 m box rests on its own half-height", hugeY, 350, 0.05)
				c.Less("a 100 m box comes to rest", c.Speed(s.huge), 0.05)
				c.Note("scale sanity: a 2 cm ball and a 100 m box both settle on "+
					"their own geometry, %.4f m and %.4f m of penetration",
					60.01-c.Pos(s.tiny).Y, 350-hugeY)

				// Far from the origin.
				far := c.Pos(s.farBody)
				c.Near("a body kilometres from the origin still rests on the floor",
					far.Y, 0.5, 0.2)
				c.Near("a body kilometres from the origin does not drift sideways",
					far.X, farX, 0.2)
				c.Less("a body kilometres from the origin comes to rest",
					c.Speed(s.farBody), 0.05)

				// Nothing from nothing.
				c.NearVec("a body at rest stays where it was put",
					c.Pos(s.rester), vec(0, 200.5), 0.05)
				c.Less("a body at rest gains no speed", c.Speed(s.rester), 0.02)
				c.Near("a body at rest gains no spin", math.Abs(c.AngVel(s.rester)), 0, 0.02)
			}},
		},
	}
}
