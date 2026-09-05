package scenario

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// Flags checks every boolean and scalar on PhysicsBody2D that has to survive the
// trip into Box2D: Active, Awake, SleepingAllowed, Bullet, FixedRotation,
// GravityScale, LinearDamping and AngularDamping.
//
// Each one is checked by its observable consequence rather than by reading the
// component back, because the component is exactly where a flag that never
// reached C still looks correct.
func Flags() harness.Scenario {
	var s struct {
		floor       cardinal.EntityID
		inactive    cardinal.EntityID
		asleep      cardinal.EntityID
		forcedAwake cardinal.EntityID
		teleSleep   cardinal.EntityID
		gravity0    cardinal.EntityID
		gravity1    cardinal.EntityID
		gravity2    cardinal.EntityID
		gravityUp   cardinal.EntityID
		staticWall  cardinal.EntityID
		staticShot  cardinal.EntityID
		bulletWall  cardinal.EntityID
		bullet      cardinal.EntityID
		plainWall   cardinal.EntityID
		notBullet   cardinal.EntityID
		spinLocked  cardinal.EntityID
		spinFree    cardinal.EntityID
		damped      cardinal.EntityID
		undamped    cardinal.EntityID
		spinDamped  cardinal.EntityID
		spinKeeps   cardinal.EntityID
		overspeed   cardinal.EntityID
	}

	const (
		spawnY      = 15.0
		wakeTick    = 60
		bulletSpeed = 300.0
		bulletWallX = 20.0
		// The projectiles live far above every other scenario, each pair on its
		// own row, so a body that does tunnel cannot reach anything else.
		staticPairY  = 200.0
		bulletPairY  = 210.0
		plainPairY   = 220.0
		bulletCheck  = 6
		dampCheck    = 120
		gravityCheck = 30
	)

	// zeroG returns a dynamic body that ignores world gravity, so a test can
	// isolate one effect (damping, CCD) from the fall.
	zeroG := func(shapes ...physics.ColliderShape) physics.PhysicsBody2D {
		pb := body(physics.BodyTypeDynamic, shapes...)
		pb.GravityScale = 0
		return pb
	}

	return harness.Scenario{
		Name: "flags",
		Setup: func(c *harness.Ctx) {
			s.floor = c.Spawn("floor", 0, groundY, ground(60))

			// Active=false must remove the body from the simulation entirely.
			inactive := body(physics.BodyTypeDynamic, circle(0.5))
			inactive.Active = false
			s.inactive = c.Spawn("inactive", -26, spawnY, inactive)

			// Box2D decides the initial sleep state as
			// (isAwake || !enableSleep) && isEnabled, so these two bodies differ
			// only in SleepingAllowed and must behave in opposite ways.
			asleep := body(physics.BodyTypeDynamic, circle(0.5))
			asleep.Awake = false
			s.asleep = c.Spawn("asleep", -20, spawnY, asleep)

			forced := body(physics.BodyTypeDynamic, circle(0.5))
			forced.Awake = false
			forced.SleepingAllowed = false
			s.forcedAwake = c.Spawn("sleep-disabled", -14, spawnY, forced)

			// Teleport + explicit sleep in one tick: reconcile treats a game
			// teleport as a disturbance and wakes the body, but a same-tick
			// Awake=false write is explicit intent and must win over that wake.
			s.teleSleep = c.Spawn("teleport-sleep", -29, spawnY,
				body(physics.BodyTypeDynamic, circle(0.5)))

			// GravityScale: 0 hovers, 1 is the reference, 2 falls twice as far in
			// the same time, -1 rises by what 1 falls.
			g0 := body(physics.BodyTypeDynamic, circle(0.5))
			g0.GravityScale = 0
			s.gravity0 = c.Spawn("gravity-scale-0", -8, spawnY, g0)

			s.gravity1 = c.Spawn("gravity-scale-1", -2, spawnY,
				body(physics.BodyTypeDynamic, circle(0.5)))

			g2 := body(physics.BodyTypeDynamic, circle(0.5))
			g2.GravityScale = 2
			s.gravity2 = c.Spawn("gravity-scale-2", 4, spawnY, g2)

			gUp := body(physics.BodyTypeDynamic, circle(0.5))
			gUp.GravityScale = -1
			s.gravityUp = c.Spawn("gravity-scale-negative", 10, spawnY, gUp)

			// Continuous collision detection. Every projectile below is fired at
			// a 4 cm wall at 200 m/s — 3.3 m per tick, far more than the wall is
			// thick — so nothing but CCD can stop it.
			//
			// Box2D v3 sweeps every fast dynamic body against *static* geometry
			// whether or not it is a bullet; the Bullet flag is what additionally
			// sweeps it against the kinematic and dynamic trees. So a static wall
			// cannot tell the two apart. The discriminating walls are kinematic:
			// reachable only via the bullet path, and infinitely massive, so a
			// projectile that detects one cannot simply shove it aside.
			thinWall := func(kind physics.BodyType) physics.PhysicsBody2D {
				pb := body(kind, box(0.02, 1))
				pb.GravityScale = 0
				pb.SleepingAllowed = false
				return pb
			}

			s.staticWall = c.Spawn("static-wall", bulletWallX, staticPairY,
				body(physics.BodyTypeStatic, box(0.02, 1)))
			s.staticShot = c.SpawnMoving("shot-at-static-wall", 10, staticPairY,
				bulletSpeed, 0, zeroG(circle(0.25)))

			s.bulletWall = c.Spawn("kinematic-wall-vs-bullet", bulletWallX, bulletPairY,
				thinWall(physics.BodyTypeKinematic))
			bullet := zeroG(circle(0.25))
			bullet.Bullet = true
			s.bullet = c.SpawnMoving("bullet", 10, bulletPairY, bulletSpeed, 0, bullet)

			s.plainWall = c.Spawn("kinematic-wall-vs-plain", bulletWallX, plainPairY,
				thinWall(physics.BodyTypeKinematic))
			s.notBullet = c.SpawnMoving("not-bullet", 10, plainPairY,
				bulletSpeed, 0, zeroG(circle(0.25)))

			// FixedRotation. Both bodies are handed the same spin; only the free
			// one may keep it. The plugin zeroes angular velocity on the Box2D
			// side for fixed-rotation bodies, so the lock must hold even against
			// an explicit angular velocity.
			locked := zeroG(box(0.5, 0.5))
			locked.FixedRotation = true
			s.spinLocked = c.SpawnSpinning("rotation-locked", 16, 60, 5, locked)
			s.spinFree = c.SpawnSpinning("rotation-free", 20, 64, 5, zeroG(box(0.5, 0.5)))

			// Damping, isolated from gravity so the only force is the damping.
			// These two travel along +x for the whole run, so their rows are kept
			// clear of every other body in this lane.
			damped := zeroG(circle(0.5))
			damped.LinearDamping = 0.5
			s.damped = c.SpawnMoving("linear-damped", 26, 20, 5, 0, damped)
			s.undamped = c.SpawnMoving("linear-undamped", 26, 24, 5, 0, zeroG(circle(0.5)))

			spinDamped := zeroG(circle(0.5))
			spinDamped.AngularDamping = 1.0
			s.spinDamped = c.SpawnSpinning("angular-damped", 34, 50, 10, spinDamped)
			s.spinKeeps = c.SpawnSpinning("angular-undamped", 34, 54, 10, zeroG(circle(0.5)))

			// Box2D v3 clamps linear speed to b2WorldDef.maximumLinearSpeed,
			// which defaults to 400 m/s, and the plugin exposes no way to raise
			// it. Anything faster is silently slowed, so pin the limit: a game
			// that ships a 600 m/s projectile needs to know it will not get one.
			s.overspeed = c.SpawnMoving("overspeed", 0, 300, 1000, 0, zeroG(circle(0.25)))
		},
		Steps: []harness.Step{
			{Tick: gravityCheck, Do: func(c *harness.Ctx) {
				fell1 := spawnY - c.Pos(s.gravity1).Y
				fell2 := spawnY - c.Pos(s.gravity2).Y
				rose := c.Pos(s.gravityUp).Y - spawnY

				c.Near("GravityScale=0 leaves the body hovering", c.Pos(s.gravity0).Y, spawnY, 1e-9)
				c.Near("GravityScale=0 leaves velocity at zero", c.Vel(s.gravity0).Y, 0, 1e-9)
				c.Greater("GravityScale=1 falls", fell1, 1.0)
				c.Near("GravityScale=2 falls twice as far", fell2/fell1, 2, 0.02)
				c.Near("GravityScale=-1 rises by what GravityScale=1 falls", rose/fell1, 1, 0.02)
				c.Note("gravity-scale drop at t=%d: x1=%.4f m, x2=%.4f m, up=%.4f m",
					gravityCheck, fell1, fell2, rose)
			}},
			{Tick: bulletCheck, Do: func(c *harness.Ctx) {
				staticX := c.Pos(s.staticShot).X
				bulletX := c.Pos(s.bullet).X
				plainX := c.Pos(s.notBullet).X

				// Fast body vs static geometry: v3 stops this one with or without
				// the flag, so a tunnel here means continuous collision is off
				// entirely, not that the Bullet flag was dropped.
				c.Less("fast body does not tunnel through static geometry", staticX, bulletWallX)
				c.IntAtLeast("fast body reports its contact with static geometry",
					c.CountBetween(harness.ContactBegin, s.staticShot, s.staticWall), 1)

				// Fast body vs a dynamic wall: only the Bullet flag saves this one,
				// so this is the check that the flag actually reached Box2D.
				c.Less("Bullet=true stops a projectile at a thin kinematic wall",
					bulletX, bulletWallX)
				c.IntAtLeast("Bullet=true reports the kinematic-wall contact",
					c.CountBetween(harness.ContactBegin, s.bullet, s.bulletWall), 1)

				// The control. If this one also stops, the bullet result above
				// proves nothing, so say so loudly rather than quietly passing.
				if plainX > bulletWallX {
					c.Note("control: Bullet=false tunnelled the kinematic wall to x=%.2f "+
						"while the bullet stopped at x=%.2f — the flag is doing real work",
						plainX, bulletX)
				} else {
					c.Note("control: Bullet=false also stopped, at x=%.2f — this run "+
						"cannot distinguish CCD from a lucky sub-step; treat the "+
						"Bullet checks above as unproven", plainX)
				}

				// Park the projectiles. They are deliberately not destroyed:
				// destroying a body while it holds a live contact is its own
				// (currently fatal) case, covered by the robustness scenario.
				c.SetVel(s.staticShot, 0, 0)
				c.SetVel(s.bullet, 0, 0)
				c.SetVel(s.notBullet, 0, 0)
			}},
			{Tick: 3, Do: func(c *harness.Ctx) {
				speed := c.Speed(s.overspeed)
				c.Near("Box2D clamps linear speed to its 400 m/s world maximum",
					speed, 400, 1.0)
				c.Note("a body launched at 1000 m/s is simulating at %.1f m/s; "+
					"b2World_SetMaximumLinearSpeed is not exposed by the plugin", speed)
				c.SetVel(s.overspeed, 0, 0)
			}},
			{Tick: 40, Do: func(c *harness.Ctx) {
				// Well before the wake, and long enough that a live body would
				// have fallen over a metre.
				c.Near("Active=false keeps the body out of the simulation",
					c.Pos(s.inactive).Y, spawnY, 1e-9)
				c.Near("Awake=false with sleeping allowed keeps the body asleep",
					c.Pos(s.asleep).Y, spawnY, 1e-9)
				c.Less("Awake=false with SleepingAllowed=false still falls",
					c.Pos(s.forcedAwake).Y, spawnY-1.0)
			}},
			{Tick: wakeTick, Do: func(c *harness.Ctx) {
				// Flip both flags at runtime; the reconciler must push them through.
				c.EditBody(s.inactive, func(pb *physics.PhysicsBody2D) { pb.Active = true })
				c.EditBody(s.asleep, func(pb *physics.PhysicsBody2D) { pb.Awake = true })
			}},
			{Tick: wakeTick + 60, Do: func(c *harness.Ctx) {
				c.Less("Active flipped to true resumes simulation",
					c.Pos(s.inactive).Y, spawnY-1.0)
				c.Less("Awake flipped to true wakes the body",
					c.Pos(s.asleep).Y, spawnY-1.0)
			}},
			{Tick: 62, Do: func(c *harness.Ctx) {
				// Mid-fall, teleport and explicitly sleep in the same tick. The
				// teleport alone must wake (bodytypes pins that); pairing it
				// with an explicit Awake=false must not.
				c.SetPos(s.teleSleep, -29, 18)
				c.EditBody(s.teleSleep, func(pb *physics.PhysicsBody2D) { pb.Awake = false })
			}},
			{Tick: 102, Do: func(c *harness.Ctx) {
				c.NearVec("teleport with a same-tick explicit Awake=false stays asleep at the target",
					c.Pos(s.teleSleep), physics.Vec2{X: -29, Y: 18}, 1e-9)
				c.False("the awake mirror does not overwrite an explicit sleep",
					c.Body(s.teleSleep).Awake, "component Awake read back true")
				c.EditBody(s.teleSleep, func(pb *physics.PhysicsBody2D) { pb.Awake = true })
			}},
			{Tick: 142, Do: func(c *harness.Ctx) {
				c.Less("the slept teleport target resumes simulating once re-woken",
					c.Pos(s.teleSleep).Y, 17)
			}},
			{Tick: dampCheck, Do: func(c *harness.Ctx) {
				// Rotation locks.
				c.Near("FixedRotation=true keeps rotation at zero", c.Rot(s.spinLocked), 0, 1e-9)
				c.Near("FixedRotation=true keeps angular velocity at zero",
					c.AngVel(s.spinLocked), 0, 1e-9)
				c.Greater("FixedRotation=false lets the body spin",
					math.Abs(c.AngVel(s.spinFree)), 4.0)
				c.Greater("FixedRotation=false accumulates rotation",
					math.Abs(c.Rot(s.spinFree)), 0.1)

				// Linear damping: same start velocity, less distance and less speed.
				dampedSpeed := c.Speed(s.damped)
				freeSpeed := c.Speed(s.undamped)
				c.Near("undamped body keeps its speed", freeSpeed, 5, 1e-3)
				c.Less("LinearDamping bleeds off speed", dampedSpeed, freeSpeed-0.5)
				c.Less("LinearDamping shortens the distance travelled",
					c.Pos(s.damped).X, c.Pos(s.undamped).X-0.5)
				c.Note("linear damping at t=%d: damped %.3f m/s vs undamped %.3f m/s",
					dampCheck, dampedSpeed, freeSpeed)

				// Angular damping, same shape of test.
				dampedSpin := math.Abs(c.AngVel(s.spinDamped))
				freeSpin := math.Abs(c.AngVel(s.spinKeeps))
				c.Near("undamped body keeps its spin", freeSpin, 10, 1e-3)
				c.Less("AngularDamping bleeds off spin", dampedSpin, freeSpin-1.0)
				c.Note("angular damping at t=%d: damped %.3f rad/s vs undamped %.3f rad/s",
					dampCheck, dampedSpin, freeSpin)
			}},
			{Tick: 200, Do: func(c *harness.Ctx) {
				// Every body that was ever meant to fall must be on the floor by
				// now; a flag that half-applied would leave one stuck mid-air.
				for _, id := range []cardinal.EntityID{s.inactive, s.asleep, s.forcedAwake, s.gravity1} {
					c.Near("body "+c.Label(id)+" ends up resting on the floor",
						c.Pos(id).Y, 0.5, 0.15)
				}
				c.Greater("GravityScale=-1 body is still climbing", c.Pos(s.gravityUp).Y, spawnY+5)
				c.Near("GravityScale=0 body never moved", c.Pos(s.gravity0).Y, spawnY, 1e-9)
			}},
		},
	}
}
