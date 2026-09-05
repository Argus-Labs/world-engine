package scenario

import (
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// BodyTypes covers the four body kinds and, just as importantly, the writeback
// rules that go with them. Static and manual bodies must never have their
// Transform2D or Velocity2D overwritten by the physics step; dynamic and
// kinematic bodies must.
//
// Manual is the kind most likely to break in a port: it is a Box2D kinematic
// body underneath, but ECS owns its pose, Box2D is told its velocity is zero
// every tick, and writeback is skipped entirely.
func BodyTypes() harness.Scenario {
	var s struct {
		floor      cardinal.EntityID
		static     cardinal.EntityID
		dynamic    cardinal.EntityID
		kinematic  cardinal.EntityID
		pusher     cardinal.EntityID
		pushed     cardinal.EntityID
		manualPose cardinal.EntityID
		sweeper    cardinal.EntityID
		sweepBall  cardinal.EntityID
		dozer      cardinal.EntityID
		sleeper    cardinal.EntityID
		wall       cardinal.EntityID
		ghost      cardinal.EntityID
	}

	const (
		manualPoseStartX = 14.0
		manualPoseStep   = 0.02
		manualPoseTicks  = 120

		sweeperStartX = 26.0
		sweeperStopX  = 20.6
		dozerStartX   = 50.0
		dozerStopX    = 44.6
		ghostStartX   = 41.0
		ghostStopX    = 31.0
		manualSpeed   = 0.05
	)

	return harness.Scenario{
		Name: "bodytypes",
		Setup: func(c *harness.Ctx) {
			s.floor = c.Spawn("floor", 0, groundY, ground(60))

			s.static = c.Spawn("static-box", -20, 2,
				body(physics.BodyTypeStatic, box(0.5, 0.5)))
			s.dynamic = c.Spawn("dynamic-box", -14, 8,
				body(physics.BodyTypeDynamic, box(0.5, 0.5)))

			// Kinematic: velocity-driven, gravity-immune, integrated by Box2D.
			// GravityScale is deliberately left at 1 to prove it is ignored.
			s.kinematic = c.SpawnMoving("kinematic-mover", -6, 5, 0.5, 0,
				body(physics.BodyTypeKinematic, box(0.5, 0.5)))

			// A kinematic body must still be able to shove a dynamic one.
			s.pusher = c.SpawnMoving("kinematic-pusher", 2, 1, 1, 0,
				body(physics.BodyTypeKinematic, box(0.5, 1)))
			s.pushed = c.Spawn("pushed-box", 6, 0.5,
				body(physics.BodyTypeDynamic, withFriction(box(0.5, 0.5), 0.1)))

			// Manual: gameplay writes the pose every tick and physics must not
			// touch it. The velocity below is pure gameplay bookkeeping — Box2D
			// is told this body's velocity is zero.
			s.manualPose = c.SpawnMoving("manual-pose", manualPoseStartX, 5, 7, -3,
				body(physics.BodyTypeManual, box(0.5, 0.5)))

			// Manual bodies must still generate contacts against dynamic bodies.
			// This target is kept awake so the contact cannot be masked by Box2D
			// sleeping: it isolates "does manual-vs-dynamic collide at all".
			awakeTarget := body(physics.BodyTypeDynamic, circle(0.5))
			awakeTarget.SleepingAllowed = false
			s.sweepBall = c.Spawn("sweep-target-awake", 20, 0.5, awakeTarget)
			s.sweeper = c.Spawn("manual-sweeper", sweeperStartX, 0.5,
				body(physics.BodyTypeManual, box(0.5, 0.5)))

			// Same setup, but the target is allowed to fall asleep resting on the
			// floor. A manual body is repositioned with World.SetBodyTransform,
			// which moves the broad-phase proxy without waking anything it moves
			// into, so this is where a character walking into a settled prop would
			// silently miss its contact.
			s.sleeper = c.Spawn("sweep-target-asleep", 44, 0.5,
				body(physics.BodyTypeDynamic, circle(0.5)))
			s.dozer = c.Spawn("manual-dozer", dozerStartX, 0.5,
				body(physics.BodyTypeManual, box(0.5, 0.5)))

			// ...but must pass straight through static geometry, which is what
			// Box2D does for kinematic-vs-static and is easy to get wrong.
			s.wall = c.Spawn("static-wall", 35, 1.5,
				body(physics.BodyTypeStatic, box(0.2, 2)))
			s.ghost = c.Spawn("manual-ghost", ghostStartX, 1.5,
				body(physics.BodyTypeManual, box(0.5, 0.5)))
		},
		EachTick: func(c *harness.Ctx) {
			tick := float64(c.Tick())

			// Drive the manual bodies from gameplay, before the physics pipeline
			// reconciles, exactly the way a character controller would.
			steps := tick
			if steps > manualPoseTicks {
				steps = manualPoseTicks
			}
			c.SetPos(s.manualPose, manualPoseStartX+steps*manualPoseStep, 5)

			sweepX := sweeperStartX - tick*manualSpeed
			if sweepX < sweeperStopX {
				sweepX = sweeperStopX
			}
			c.SetPos(s.sweeper, sweepX, 0.5)

			dozerX := dozerStartX - tick*manualSpeed
			if dozerX < dozerStopX {
				dozerX = dozerStopX
			}
			c.SetPos(s.dozer, dozerX, 0.5)

			ghostX := ghostStartX - tick*manualSpeed
			if ghostX < ghostStopX {
				ghostX = ghostStopX
			}
			c.SetPos(s.ghost, ghostX, 1.5)
		},
		Steps: []harness.Step{
			{Tick: 10, Do: func(c *harness.Ctx) {
				// A velocity on a static body is meaningless to Box2D. Setting one
				// proves the writeback skip is real rather than incidental.
				c.SetVel(s.static, 5, 5)
			}},
			{Tick: 120, Do: func(c *harness.Ctx) {
				// 120 ticks is 2s: a kinematic body at 0.5 m/s has covered 1 m.
				pos := c.Pos(s.kinematic)
				c.Near("kinematic body integrates its velocity", pos.X, -6+1.0, 0.02)
				c.Near("kinematic body ignores gravity", pos.Y, 5, 1e-6)
				c.NearVec("kinematic velocity survives writeback", c.Vel(s.kinematic),
					vec(0.5, 0), 1e-6)
			}},
			{Tick: 200, Do: func(c *harness.Ctx) {
				c.Near("dynamic body falls and rests on the floor", c.Pos(s.dynamic).Y, 0.5, 0.15)
				c.Less("dynamic body comes to rest", c.Speed(s.dynamic), 0.2)

				c.NearVec("static body never moves", c.Pos(s.static), vec(-20, 2), 1e-9)
				c.NearVec("static body is never written back", c.Vel(s.static), vec(5, 5), 1e-9)
			}},
			{Tick: 300, Do: func(c *harness.Ctx) {
				// The pusher started at x=2 moving 1 m/s, so at 5s it is at x=7 and
				// the box it met at x=6 must be ahead of it.
				pusherX := c.Pos(s.pusher).X
				c.Near("kinematic pusher keeps its own path", pusherX, 7, 0.05)
				c.Greater("kinematic body pushes a dynamic body", c.Pos(s.pushed).X, 7.0)
				c.IntAtLeast("kinematic/dynamic contact fires",
					c.CountBetween(harness.ContactBegin, s.pusher, s.pushed), 1)

				// Manual pose: ECS is authoritative, so the position must be
				// exactly the scripted one and the velocity untouched.
				wantX := manualPoseStartX + manualPoseTicks*manualPoseStep
				c.NearVec("manual body keeps the pose gameplay set",
					c.Pos(s.manualPose), vec(wantX, 5), 1e-9)
				c.NearVec("manual body's velocity is never written back",
					c.Vel(s.manualPose), vec(7, -3), 1e-9)
				c.Near("manual body never picks up rotation", c.Rot(s.manualPose), 0, 1e-9)

				// Manual vs an awake dynamic body: contact fires and the ball moves.
				c.IntAtLeast("manual body contacts an awake dynamic body",
					c.CountBetween(harness.ContactBegin, s.sweeper, s.sweepBall), 1)
				c.Less("manual body displaces the awake dynamic body it meets",
					c.Pos(s.sweepBall).X, 19.9)

				// Manual vs a sleeping dynamic body. The dozer is overlapping the
				// sleeper by 0.4 m by now, so anything short of a contact means the
				// sleeping body was never woken by the body driven into it.
				c.IntAtLeast("manual body wakes the sleeping dynamic body it drives into",
					c.CountBetween(harness.ContactBegin, s.dozer, s.sleeper), 1)
				c.Less("manual body displaces a sleeping dynamic body",
					c.Pos(s.sleeper).X, 43.9)

				// Manual vs static: no contact, and it drives straight through.
				c.Int("manual body raises no contact with static geometry",
					c.CountBetween(harness.ContactBegin, s.ghost, s.wall), 0)
				c.Near("manual body passes through static geometry",
					c.Pos(s.ghost).X, ghostStopX, 1e-9)
				c.NearVec("static wall is undisturbed", c.Pos(s.wall), vec(35, 1.5), 1e-9)
			}},
		},
	}
}
