package scenario

import (
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// Reset covers Plugin.Reset, the call a game makes after restoring a
// snapshot or rebuilding its world. It throws away every derived Box2D object
// and lets the next PreUpdate rebuild the whole simulation from ECS, so it is
// the one operation that exercises the ECS -> Box2D path from a cold start with
// a world that is already full of moving bodies and live contacts.
//
// The risks it has to avoid are all invisible ones: bodies that come back in the
// wrong place, contacts that replay as new Begin events, contacts that quietly
// disappear without an End, and queries that keep returning stale results.
//
// Reset only touches this plugin instance's runtime, but the scenario still runs
// after every other one has finished asserting: the whole world shares a plugin,
// so the rebuild is visible to every lane.
func Reset() harness.Scenario {
	var s struct {
		pad         cardinal.EntityID
		rester      cardinal.EntityID
		drifter     cardinal.EntityID
		gate        cardinal.EntityID
		visitor     cardinal.EntityID
		wall        cardinal.EntityID
		restPos     physics.Vec2
		driftPos    physics.Vec2
		driftVel    physics.Vec2
		beginsAtCut int
		endsAtCut   int
		trigsAtCut  int
		trigEndsCut int
		rayFraction float64
	}

	const (
		resetTick = 450
		afterTick = 451
		lateTick  = 520
		driftVX   = 1.0
	)

	return harness.Scenario{
		Name: "reset",
		Setup: func(c *harness.Ctx) {
			// A live contact that must survive the rebuild without replaying.
			s.pad = c.Spawn("reset-pad", 0, -1, body(physics.BodyTypeStatic, box(4, 1)))
			s.rester = c.Spawn("reset-rester", 0, 3,
				body(physics.BodyTypeDynamic, box(0.5, 0.5)))

			// A body in motion, so the rebuild has to carry velocity across too.
			drift := body(physics.BodyTypeDynamic, circle(0.5))
			drift.GravityScale = 0
			drift.SleepingAllowed = false
			s.drifter = c.SpawnMoving("reset-drifter", -40, 20, driftVX, 0, drift)

			// A live sensor overlap, which the plugin tracks separately from
			// solid contacts in its ActiveContacts baseline.
			s.gate = c.Spawn("reset-gate", 20, 40,
				body(physics.BodyTypeStatic, asSensor(box(3, 3))))
			s.visitor = c.Spawn("reset-visitor", 20, 40,
				body(physics.BodyTypeDynamic, circle(0.5)))

			// Something to raycast at, before and after.
			s.wall = c.Spawn("reset-wall", 10, 60, body(physics.BodyTypeStatic, box(1, 3)))
		},
		Steps: []harness.Step{
			{Tick: 60, Do: func(c *harness.Ctx) {
				// Give the visitor a nudge so its overlap with the sensor is a
				// real Begin event rather than a suppressed build-time one.
				c.SetPos(s.visitor, 24, 40)
			}},
			{Tick: 120, Do: func(c *harness.Ctx) {
				c.SetPos(s.visitor, 20, 40)
			}},
			{Tick: resetTick, Do: func(c *harness.Ctx) {
				s.restPos = c.Pos(s.rester)
				s.driftPos = c.Pos(s.drifter)
				s.driftVel = c.Vel(s.drifter)
				s.beginsAtCut = c.CountBetween(harness.ContactBegin, s.rester, s.pad)
				s.endsAtCut = c.CountBetween(harness.ContactEnd, s.rester, s.pad)
				s.trigsAtCut = c.CountBetween(harness.TriggerBegin, s.visitor, s.gate)
				s.trigEndsCut = c.CountBetween(harness.TriggerEnd, s.visitor, s.gate)

				before := c.Raycast(10, 66, 10, 54, nil)
				c.True("the wall is there before the reset",
					before.Hit && before.Entity == s.wall, "the pre-reset raycast missed")
				s.rayFraction = before.Fraction

				c.IntAtLeast("the resting pair had a contact before the reset",
					s.beginsAtCut, 1)
				c.IntAtLeast("the sensor pair had a trigger before the reset",
					s.trigsAtCut, 1)

				// Announce the reset so the runner's world watchdog does not
				// report the missing world as a failure.
				c.ExpectWorldReset()
				c.Plugin().Reset()

				c.True("Reset drops the Box2D world", c.Plugin().Engine() == nil,
					"Plugin.Engine() is still live immediately after Reset")
				c.False("a raycast with no world returns no hit",
					c.Raycast(10, 66, 10, 54, nil).Hit,
					"a query answered from a world that no longer exists")
				c.Int("an overlap with no world returns nothing",
					len(c.OverlapAABB(8, 56, 12, 64, nil).Hits), 0)
				c.False("a sweep with no world returns no hit",
					c.CircleSweep(10, 66, 10, 54, 0.5, 0, nil).Hit,
					"a sweep answered from a world that no longer exists")
			}},
			{Tick: afterTick, Do: func(c *harness.Ctx) {
				c.True("the next tick rebuilds the Box2D world",
					c.Plugin().Engine() != nil,
					"Plugin.Engine() is still nil a full tick after Reset")

				// ECS is authoritative, so a body that was not moving must come
				// back exactly where it was.
				c.NearVec("a resting body survives the rebuild in place",
					c.Pos(s.rester), s.restPos, 1e-6)
				c.Less("a resting body is not woken by the rebuild",
					c.Speed(s.rester), 0.05)

				// A moving body keeps its velocity across the rebuild.
				c.NearVec("a moving body keeps its velocity across the rebuild",
					c.Vel(s.drifter), s.driftVel, 1e-3)

				// The rebuilding tick reconciles ECS into a fresh Box2D world and
				// returns without stepping, so simulated time stands still for one
				// tick. Worth knowing when physics has to line up with a tick
				// count, as in client-side prediction.
				c.NearVec("the rebuild tick reconciles but does not advance the world",
					c.Pos(s.drifter), s.driftPos, 1e-9)
				c.Note("the tick that rebuilds the Box2D world does not step it, so " +
					"one tick of simulated time is lost after every ResetRuntime")

				// The rebuild must not replay existing overlaps as new events.
				c.Int("the rebuild replays no ContactBegin",
					c.CountBetween(harness.ContactBegin, s.rester, s.pad), s.beginsAtCut)
				c.Int("the rebuild invents no ContactEnd",
					c.CountBetween(harness.ContactEnd, s.rester, s.pad), s.endsAtCut)
				c.Int("the rebuild replays no TriggerBegin",
					c.CountBetween(harness.TriggerBegin, s.visitor, s.gate), s.trigsAtCut)
				c.Int("the rebuild invents no TriggerEnd",
					c.CountBetween(harness.TriggerEnd, s.visitor, s.gate), s.trigEndsCut)

				after := c.Raycast(10, 66, 10, 54, nil)
				c.True("queries work again after the rebuild",
					after.Hit && after.Entity == s.wall, "the post-reset raycast missed")
				c.Near("the rebuilt geometry is in the same place",
					after.Fraction, s.rayFraction, 1e-4)
			}},
			{Tick: lateTick, Do: func(c *harness.Ctx) {
				// And the rebuilt world keeps simulating.
				c.NearVec("a resting body stays resting after the rebuild",
					c.Pos(s.rester), s.restPos, 0.02)
				// One fewer tick of travel than the elapsed ticks, because the
				// rebuild tick above did not step.
				c.Near("a moving body keeps moving after the rebuild",
					c.Pos(s.drifter).X,
					s.driftPos.X+driftVX*float64(lateTick-resetTick-1)/harness.TickRate, 0.05)
				c.Int("the rebuild does not leak repeated contact events",
					c.CountBetween(harness.ContactBegin, s.rester, s.pad), s.beginsAtCut)
			}},
		},
	}
}
