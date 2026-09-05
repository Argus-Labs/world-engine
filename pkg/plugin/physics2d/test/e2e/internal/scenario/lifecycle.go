package scenario

import (
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// Lifecycle covers everything that can change after a body exists: entities
// created and destroyed mid-run, poses and velocities written from gameplay,
// geometry edited in place, shapes added and removed, filters and sensor flags
// flipped, materials retuned, and body types swapped.
//
// The reconciler decides which of these can be applied to the live Box2D body
// and which need the fixture rebuilt, by diffing the component against a shadow
// copy. Anything the diff does not look at is a field a game can change with no
// effect and no error, so each mutation here is paired with an observation that
// would notice.
func Lifecycle() harness.Scenario {
	var s struct {
		lateWall    cardinal.EntityID
		doomedWall  cardinal.EntityID
		mover       cardinal.EntityID
		growCircle  cardinal.EntityID
		growBox     cardinal.EntityID
		growCapsule cardinal.EntityID
		fatCapsule  cardinal.EntityID
		filterWall  cardinal.EntityID
		filterBox   cardinal.EntityID
		controlWall cardinal.EntityID
		controlBox  cardinal.EntityID
		sensorWall  cardinal.EntityID
		sensorBox   cardinal.EntityID
		freezer     cardinal.EntityID
		freezerY    float64
		thawFloor   cardinal.EntityID
		thawed      cardinal.EntityID
		multiShape  cardinal.EntityID
		slickPad    cardinal.EntityID
		gripLater   cardinal.EntityID
		gripControl cardinal.EntityID
		slickPad2   cardinal.EntityID
	}

	const (
		mutateTick = 30
		removeTick = 90
		earlyCheck = 45
		lateCheck  = 260
		crossSpeed = 2.0
		slideSpeed = 5.0
	)

	// hits reports whether a short vertical ray at x through row y finds id.
	hits := func(c *harness.Ctx, id cardinal.EntityID, x, y float64) bool {
		res := c.Raycast(x, y+4, x, y-4, nil)
		return res.Hit && res.Entity == id
	}

	// spans reports whether a horizontal ray across row y at height h finds id.
	// A vertical ray cannot tell a fat capsule from a thin one; a ray skimming
	// above the thin one can.
	spans := func(c *harness.Ctx, id cardinal.EntityID, x, h float64) bool {
		res := c.Raycast(x-6, h, x+6, h, nil)
		return res.Hit && res.Entity == id
	}

	zeroG := func(shapes ...physics.ColliderShape) physics.PhysicsBody2D {
		pb := body(physics.BodyTypeDynamic, shapes...)
		pb.GravityScale = 0
		pb.FixedRotation = true
		return pb
	}

	return harness.Scenario{
		Name: "lifecycle",
		Setup: func(c *harness.Ctx) {
			// Row y=10 — destroyed mid-run. Nothing ever touches it, because
			// destroying a body that holds a live contact is its own case (see
			// the robustness scenario).
			s.doomedWall = c.Spawn("doomed-wall", 0, 10,
				body(physics.BodyTypeStatic, box(1, 1)))

			// Row y=20 — pose and velocity written straight from gameplay.
			s.mover = c.Spawn("teleporter", 0, 20, zeroG(box(0.5, 0.5)))

			// Rows y=30..50 — geometry edited in place. Each starts too small to
			// reach the probe point and is grown into it.
			s.growCircle = c.Spawn("growing-circle", 0, 30,
				body(physics.BodyTypeStatic, circle(0.5)))
			s.growBox = c.Spawn("growing-box", 0, 40,
				body(physics.BodyTypeStatic, box(0.5, 0.5)))
			s.growCapsule = c.Spawn("lengthening-capsule", 0, 50,
				body(physics.BodyTypeStatic, capsule(vec(-0.5, 0), vec(0.5, 0), 0.3)))
			// Control for the capsule: growing the radius, which the reconciler's
			// structural diff does compare.
			s.fatCapsule = c.Spawn("fattening-capsule", 10, 50,
				body(physics.BodyTypeStatic, capsule(vec(-0.5, 0), vec(0.5, 0), 0.3)))

			// Row y=60 — a wall whose filter is changed so it stops colliding.
			s.filterWall = c.Spawn("refiltered-wall", 5, 60,
				body(physics.BodyTypeStatic, box(0.5, 2)))
			s.filterBox = c.SpawnMoving("refilter-probe", 0, 60, crossSpeed, 0,
				zeroG(withFilter(box(0.5, 0.5), 0x1, 0x1, 0)))

			// Row y=70 — the control: same geometry, filter left alone.
			s.controlWall = c.Spawn("control-wall", 5, 70,
				body(physics.BodyTypeStatic, withFilter(box(0.5, 2), 0x1, 0x1, 0)))
			s.controlBox = c.SpawnMoving("control-probe", 0, 70, crossSpeed, 0,
				zeroG(withFilter(box(0.5, 0.5), 0x1, 0x1, 0)))

			// Row y=80 — a solid wall turned into a sensor mid-run.
			s.sensorWall = c.Spawn("wall-turned-sensor", 5, 80,
				body(physics.BodyTypeStatic, box(0.5, 2)))
			s.sensorBox = c.SpawnMoving("sensor-probe", 0, 80, crossSpeed, 0,
				zeroG(box(0.5, 0.5)))

			// Row y=90 — a falling body frozen by turning it static.
			s.freezer = c.Spawn("frozen-mid-fall", 0, 90,
				body(physics.BodyTypeDynamic, circle(0.5)))

			// Row y=100 — a static body thawed into a dynamic one, with a pad to
			// land on so it does not fall out of the scene.
			s.thawFloor = c.Spawn("thaw-pad", 20, 70, body(physics.BodyTypeStatic, box(3, 1)))
			s.thawed = c.Spawn("thawed-body", 20, 100,
				body(physics.BodyTypeStatic, box(0.5, 0.5)))

			// Row y=110 — a body that gains and then loses a second fixture.
			s.multiShape = c.Spawn("growing-compound", 0, 110,
				body(physics.BodyTypeStatic, box(0.5, 0.5)))

			// Rows y=120/130 — friction retuned on a body that is already sliding.
			s.slickPad = c.Spawn("retune-pad", 0, 119,
				body(physics.BodyTypeStatic, withFriction(box(30, 1), 0.6)))
			s.gripLater = c.SpawnMoving("gains-friction", -20, 120.5, slideSpeed, 0,
				body(physics.BodyTypeDynamic, withFriction(box(0.5, 0.5), 0)))

			s.slickPad2 = c.Spawn("retune-control-pad", 0, 129,
				body(physics.BodyTypeStatic, withFriction(box(30, 1), 0.6)))
			s.gripControl = c.SpawnMoving("keeps-no-friction", -20, 130.5, slideSpeed, 0,
				body(physics.BodyTypeDynamic, withFriction(box(0.5, 0.5), 0)))
		},
		Steps: []harness.Step{
			{Tick: 3, Do: func(c *harness.Ctx) {
				c.False("nothing is at the late-create site before it is created",
					hits(c, s.lateWall, 0, 0), "found a body before it was spawned")
				c.True("the doomed wall exists before it is destroyed",
					hits(c, s.doomedWall, 0, 10), "the wall was never built")

				// Probe points each growing shape does not yet reach.
				c.False("the small circle does not reach its probe point",
					hits(c, s.growCircle, 1.2, 30), "circle already covers x=1.2")
				c.False("the small box does not reach its probe point",
					hits(c, s.growBox, 1.2, 40), "box already covers x=1.2")
				c.False("the short capsule does not reach its probe point",
					hits(c, s.growCapsule, 2, 50), "capsule already covers x=2")
				c.False("the thin capsule does not reach its probe height",
					spans(c, s.fatCapsule, 10, 51.5), "capsule already reaches y=51.5")
				c.False("the single-shape body has no second fixture yet",
					hits(c, s.multiShape, 3, 110), "a second shape already exists")

				s.freezerY = c.Pos(s.freezer).Y
			}},
			{Tick: mutateTick, Do: func(c *harness.Ctx) {
				s.lateWall = c.Spawn("late-wall", 0, 0, body(physics.BodyTypeStatic, box(1, 1)))
				c.True("destroying an uncontacted entity reports success",
					c.Destroy(s.doomedWall), "Destroy returned false")

				c.SetPos(s.mover, 8, 20)

				c.EditBody(s.growCircle, func(pb *physics.PhysicsBody2D) {
					pb.Shapes[0].Radius = 2
				})
				c.EditBody(s.growBox, func(pb *physics.PhysicsBody2D) {
					pb.Shapes[0].HalfExtents = vec(2, 0.5)
				})
				c.EditBody(s.growCapsule, func(pb *physics.PhysicsBody2D) {
					pb.Shapes[0].CapsuleCenter1 = vec(-3, 0)
					pb.Shapes[0].CapsuleCenter2 = vec(3, 0)
				})
				c.EditBody(s.fatCapsule, func(pb *physics.PhysicsBody2D) {
					pb.Shapes[0].Radius = 1.8
				})

				c.EditBody(s.filterWall, func(pb *physics.PhysicsBody2D) {
					pb.Shapes[0].CategoryBits = 0x4
					pb.Shapes[0].MaskBits = 0x4
				})
				c.EditBody(s.sensorWall, func(pb *physics.PhysicsBody2D) {
					pb.Shapes[0].IsSensor = true
				})
				c.EditBody(s.freezer, func(pb *physics.PhysicsBody2D) {
					pb.BodyType = physics.BodyTypeStatic
				})
				c.EditBody(s.thawed, func(pb *physics.PhysicsBody2D) {
					pb.BodyType = physics.BodyTypeDynamic
				})
				c.EditBody(s.multiShape, func(pb *physics.PhysicsBody2D) {
					pb.Shapes = append(pb.Shapes, atOffset(box(0.5, 0.5), 3, 0))
				})
				c.EditBody(s.gripLater, func(pb *physics.PhysicsBody2D) {
					pb.Shapes[0].Friction = 0.9
				})
			}},
			{Tick: earlyCheck, Do: func(c *harness.Ctx) {
				c.True("an entity created mid-run gets a body", hits(c, s.lateWall, 0, 0),
					"the wall spawned at tick %d was never simulated", mutateTick)
				c.False("a destroyed entity loses its body", hits(c, s.doomedWall, 0, 10),
					"a raycast still finds the destroyed wall")
				c.False("a destroyed entity is gone from ECS", c.Alive(s.doomedWall),
					"the entity is still in the world")

				c.NearVec("a pose written from gameplay moves the body",
					c.Pos(s.mover), vec(8, 20), 1e-9)

				c.True("growing a circle's radius updates its geometry",
					hits(c, s.growCircle, 1.2, 30), "the enlarged circle is not there")
				c.True("growing a box's half-extents updates its geometry",
					hits(c, s.growBox, 1.2, 40), "the enlarged box is not there")
				c.True("growing a capsule's radius updates its geometry",
					spans(c, s.fatCapsule, 10, 51.5),
					"the capsule's radius went from 0.3 to 1.8 but a ray at y=51.5 misses it")
				c.True("moving a capsule's end centers updates its geometry",
					hits(c, s.growCapsule, 2, 50),
					"the capsule was lengthened from +/-0.5 to +/-3 but a ray at "+
						"x=2 still misses it. The reconciler's structural shape "+
						"comparison does not include CapsuleCenter1/CapsuleCenter2, "+
						"so the change is silently dropped and no error is reported")

				c.True("appending a shape adds a fixture", hits(c, s.multiShape, 3, 110),
					"the appended shape is not there")
				res := c.Raycast(3, 114, 3, 106, nil)
				if res.Hit && res.Entity == s.multiShape {
					c.Int("an appended shape takes the next slot index", res.ShapeIndex, 1)
				}

				c.Less("turning a body static stops it mid-fall",
					s.freezerY-c.Pos(s.freezer).Y, 2.0)
				// 15 ticks of fall is only ~0.3 m, so the threshold is small on
				// purpose; the landing check at lateCheck is the strong one.
				c.Less("turning a body dynamic starts it falling", c.Pos(s.thawed).Y, 99.9)
			}},
			{Tick: removeTick, Do: func(c *harness.Ctx) {
				frozenY := c.Pos(s.freezer).Y
				c.EditBody(s.multiShape, func(pb *physics.PhysicsBody2D) {
					pb.Shapes = pb.Shapes[:1]
				})
				c.SetVel(s.mover, crossSpeed, 0)
				s.freezerY = frozenY
			}},
			{Tick: lateCheck, Do: func(c *harness.Ctx) {
				c.False("removing a shape removes its fixture", hits(c, s.multiShape, 3, 110),
					"a ray still finds the removed shape")
				c.True("removing a shape leaves the other fixtures alone",
					hits(c, s.multiShape, 0, 110), "the remaining shape disappeared too")

				// The velocity written at removeTick must have been applied.
				wantX := 8 + crossSpeed*float64(lateCheck-removeTick)/harness.TickRate
				c.Near("a velocity written from gameplay is applied",
					c.Pos(s.mover).X, wantX, 0.05)

				c.Near("a body left static stays exactly where it froze",
					c.Pos(s.freezer).Y, s.freezerY, 1e-9)
				c.Near("a thawed body lands on the pad below it",
					c.Pos(s.thawed).Y, 71.5, 0.15)

				// Refiltered wall: the probe must sail past where it used to stop.
				c.Int("a refiltered wall stops colliding",
					c.CountBetween(harness.ContactBegin, s.filterBox, s.filterWall), 0)
				c.Greater("the probe passes through the refiltered wall",
					c.Pos(s.filterBox).X, 6.0)

				// Control: identical geometry, filter untouched.
				c.IntAtLeast("the control wall still collides",
					c.CountBetween(harness.ContactBegin, s.controlBox, s.controlWall), 1)
				c.Less("the control probe is stopped by its wall",
					c.Pos(s.controlBox).X, 4.6)

				// Sensor toggle: triggers instead of contacts, and no blocking.
				c.Int("a wall turned sensor raises no contact",
					c.CountBetween(harness.ContactBegin, s.sensorBox, s.sensorWall), 0)
				c.IntAtLeast("a wall turned sensor raises a trigger",
					c.CountBetween(harness.TriggerBegin, s.sensorBox, s.sensorWall), 1)
				c.Greater("the probe passes through the wall turned sensor",
					c.Pos(s.sensorBox).X, 6.0)

				// Friction retuned mid-slide.
				c.Less("friction raised mid-slide brings the body to a stop",
					c.Speed(s.gripLater), 0.05)
				c.Near("friction left alone keeps the control sliding",
					c.Speed(s.gripControl), slideSpeed, 0.05)
				c.Note("retuned friction stopped the box after %.2f m; the control "+
					"has travelled %.2f m and is still going",
					c.Pos(s.gripLater).X+20, c.Pos(s.gripControl).X+20)
			}},
		},
	}
}
