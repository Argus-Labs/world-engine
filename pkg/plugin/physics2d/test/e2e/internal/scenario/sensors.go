package scenario

import (
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// Sensors covers the trigger half of the event system: a sensor fixture must
// report overlaps without blocking anything, must route to Trigger events rather
// than Contact events, and must keep working in the cases Box2D handles
// specially — sleeping bodies, static visitors, compound bodies and disabled
// bodies.
//
// Overlaps that already exist when the world is first built are deliberately
// suppressed by the plugin (otherwise every rebuild would replay the whole world
// as new contacts), so every "does this overlap register" case here creates its
// second body partway through the run rather than at setup.
func Sensors() harness.Scenario {
	var s struct {
		floor       cardinal.EntityID
		fallSensor  cardinal.EntityID
		faller      cardinal.EntityID
		compound    cardinal.EntityID
		visitor     cardinal.EntityID
		staticGate  cardinal.EntityID
		staticBlock cardinal.EntityID
		platform    cardinal.EntityID
		sleepy      cardinal.EntityID
		sleepVisit  cardinal.EntityID
		offGate     cardinal.EntityID
		offBlock    cardinal.EntityID
		gateA       cardinal.EntityID
		gateB       cardinal.EntityID
		doomedGate  cardinal.EntityID
		gateSitter  cardinal.EntityID
	}

	const (
		spawnLate    = 30
		visitorStart = 8.0
		visitorStop  = 2.0
		visitorSpeed = 0.05
		sleepStart   = 90
		checkTick    = 260
	)

	// sensorBody is a static body whose single fixture is a sensor.
	sensorBody := func(shape physics.ColliderShape) physics.PhysicsBody2D {
		return body(physics.BodyTypeStatic, asSensor(shape))
	}

	return harness.Scenario{
		Name: "sensors",
		Setup: func(c *harness.Ctx) {
			// Row y=0 — a ball falls clean through a sensor and lands below it.
			s.floor = c.Spawn("catch-floor", 0, -15, body(physics.BodyTypeStatic, box(6, 1)))
			s.fallSensor = c.Spawn("fall-sensor", 0, 0, sensorBody(circle(2)))
			s.faller = c.Spawn("faller", 0, 10, body(physics.BodyTypeDynamic, circle(0.5)))

			// Row y=20 — a compound body whose slot 1 is the sensor. The event has
			// to name slot 1, not slot 0 and not the body as a whole.
			s.compound = c.Spawn("compound-gate", 0, 20, body(physics.BodyTypeStatic,
				box(0.5, 0.5),
				asSensor(circle(2)),
			))
			s.visitor = c.Spawn("gate-visitor", visitorStart, 20,
				body(physics.BodyTypeManual, box(0.5, 0.5)))

			// Row y=40 — a static solid body appears inside a sensor. Box2D v3
			// queries the static, kinematic and dynamic trees for sensor overlap,
			// so a static visitor must register.
			s.staticGate = c.Spawn("static-gate", 0, 40, sensorBody(box(2, 2)))

			// Row y=60 — a sensor riding a body that has gone to sleep. Sensors
			// run in their own overlap pass that ignores sleep state, so this must
			// fire even though nothing about the sleeping body has changed.
			s.platform = c.Spawn("sleep-platform", 0, 59,
				body(physics.BodyTypeStatic, box(3, 1)))
			s.sleepy = c.Spawn("sleeping-sensor-body", 0, 60.5, body(physics.BodyTypeDynamic,
				box(0.5, 0.5),
				asSensor(circle(2)),
			))
			s.sleepVisit = c.Spawn("sleep-visitor", visitorStart, 60.5,
				body(physics.BodyTypeManual, box(0.5, 0.5)))

			// Row y=80 — the one case Box2D does skip: a disabled body.
			off := sensorBody(box(2, 2))
			off.Active = false
			s.offGate = c.Spawn("disabled-gate", 0, 80, off)

			// Row y=100 — sensor against sensor.
			s.gateA = c.Spawn("sensor-a", 0, 100, sensorBody(box(2, 2)))

			// Row y=120 — a sensor destroyed while something is still inside it.
			// The engine reports the end-touch for a destroyed shape, but by the
			// time it is drained the shape is gone and the record cannot be
			// resolved to an entity, so the runtime has to synthesize the
			// TriggerEnd from the persisted pair instead. Without that, anything
			// latching state on TriggerBegin (an "in the zone" flag) would never
			// be told the overlap ended.
			s.doomedGate = c.Spawn("doomed-gate", 0, 120, sensorBody(box(2, 2)))
		},
		EachTick: func(c *harness.Ctx) {
			tick := float64(c.Tick())

			x := visitorStart - tick*visitorSpeed
			if x < visitorStop {
				x = visitorStop
			}
			c.SetPos(s.visitor, x, 20)

			// The sleeping-sensor visitor waits until the sensor body has had
			// time to settle and drop out of the awake set.
			if c.Tick() >= sleepStart {
				sx := visitorStart - (tick-sleepStart)*visitorSpeed
				if sx < visitorStop {
					sx = visitorStop
				}
				c.SetPos(s.sleepVisit, sx, 60.5)
			}
		},
		Steps: []harness.Step{
			{Tick: spawnLate, Do: func(c *harness.Ctx) {
				s.staticBlock = c.Spawn("static-visitor", 1.5, 40,
					body(physics.BodyTypeStatic, box(0.5, 0.5)))
				s.offBlock = c.Spawn("disabled-gate-visitor", 1.5, 80,
					body(physics.BodyTypeStatic, box(0.5, 0.5)))
				s.gateB = c.Spawn("sensor-b", 1.5, 100, sensorBody(box(2, 2)))
				// Created after the world exists, so the overlap counts as new (see
				// the note above about build-time overlaps).
				s.gateSitter = c.Spawn("gate-sitter", 0, 120,
					body(physics.BodyTypeStatic, box(0.5, 0.5)))
			}},
			{Tick: 120, Do: func(c *harness.Ctx) {
				c.IntAtLeast("the doomed sensor registered its visitor before being destroyed",
					c.CountBetween(harness.TriggerBegin, s.doomedGate, s.gateSitter), 1)
				c.True("destroying a sensor that holds a live overlap reports success",
					c.Destroy(s.doomedGate), "Destroy returned false")
			}},
			{Tick: 140, Do: func(c *harness.Ctx) {
				c.IntAtLeast("destroying a sensor mid-overlap still reports TriggerEnd",
					c.CountBetween(harness.TriggerEnd, s.doomedGate, s.gateSitter), 1)
			}},
			{Tick: 141, Do: func(c *harness.Ctx) {
				// The faller crosses the sensor between roughly t=76 and t=90.
				begins := c.EventsBetween(harness.TriggerBegin, s.faller, s.fallSensor)
				ends := c.EventsBetween(harness.TriggerEnd, s.faller, s.fallSensor)

				c.IntAtLeast("passing through a sensor raises TriggerBegin", len(begins), 1)
				c.IntAtLeast("leaving a sensor raises TriggerEnd", len(ends), 1)
				c.Int("a sensor never raises a solid ContactBegin",
					c.CountBetween(harness.ContactBegin, s.faller, s.fallSensor), 0)
				c.Int("a sensor never raises a solid ContactEnd",
					c.CountBetween(harness.ContactEnd, s.faller, s.fallSensor), 0)

				if len(begins) > 0 && len(ends) > 0 {
					c.Greater("TriggerEnd arrives after TriggerBegin",
						float64(ends[0].Tick), float64(begins[0].Tick)-0.5)
				}
			}},
			{Tick: checkTick, Do: func(c *harness.Ctx) {
				// The sensor must not have slowed the faller down at all.
				c.Near("a sensor does not block the body passing through it",
					c.Pos(s.faller).Y, -13.5, 0.2)
				c.IntAtLeast("the solid floor below the sensor does raise a contact",
					c.CountBetween(harness.ContactBegin, s.faller, s.floor), 1)

				// Compound: the event must point at the sensor slot.
				gate := c.EventsBetween(harness.TriggerBegin, s.visitor, s.compound)
				if c.IntAtLeast("a compound body's sensor slot triggers", len(gate), 1) {
					idx, ok := gate[0].ShapeIndexFor(s.compound)
					c.True("the trigger event names the compound body", ok,
						"event carried entities %d and %d, not %d",
						gate[0].Payload.EntityA, gate[0].Payload.EntityB, s.compound)
					c.Int("the trigger event names the sensor's shape slot", idx, 1)
				}
				c.Int("the compound body's solid slot raises no contact with the visitor",
					c.CountBetween(harness.ContactBegin, s.visitor, s.compound), 0)

				// Static visitor.
				staticTriggers := c.CountBetween(harness.TriggerBegin, s.staticGate, s.staticBlock)
				c.IntAtLeast("a sensor detects a static body placed inside it", staticTriggers, 1)

				// Sleeping sensor body.
				sleepTriggers := c.CountBetween(harness.TriggerBegin, s.sleepy, s.sleepVisit)
				c.IntAtLeast("a sensor on a sleeping body still triggers", sleepTriggers, 1)

				// Disabled body: the documented exception.
				c.Int("a sensor on a disabled body never triggers",
					c.CountTouching(harness.TriggerBegin, s.offGate), 0)

				// Sensor against sensor.
				sensorPair := c.CountBetween(harness.TriggerBegin, s.gateA, s.gateB)
				c.Note("sensor-vs-sensor overlap produced %d TriggerBegin event(s) "+
					"(Box2D v3.1 has no sensor-vs-sensor exclusion; v3.0 did)", sensorPair)
				c.Int("overlapping sensors never raise a solid contact",
					c.CountBetween(harness.ContactBegin, s.gateA, s.gateB), 0)
			}},
		},
	}
}
