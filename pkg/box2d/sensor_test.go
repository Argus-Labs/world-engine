// Behavior tests for the float64 port of Box2D v3.2.0 src/sensor.c: the
// end-of-step sensor overlap pass, its begin/end touch event model, the
// enableSensorEvents flags on both shapes, sensor and visitor destruction,
// the double buffered end-event slices and cross-world determinism.

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

const (
	sensorDT       = 1.0 / 60.0
	sensorSubSteps = 4
)

// sensorEventLog holds the sensor events reported for a single step. The
// slices are copies because the world buffers are transient.
type sensorEventLog struct {
	begins []box2d.SensorBeginTouchEvent
	ends   []box2d.SensorEndTouchEvent
}

func (l sensorEventLog) empty() bool {
	return len(l.begins) == 0 && len(l.ends) == 0
}

// newSensorWorld creates a gravity-free world so sensor tests can drive
// visitors with explicit velocities.
func newSensorWorld(t *testing.T) *box2d.World {
	t.Helper()

	def := box2d.DefaultWorldDef()
	def.Gravity = box2d.Vec2{X: 0.0, Y: 0.0}
	w := box2d.NewWorld(&def)
	t.Cleanup(w.Destroy)

	return w
}

// sensorAddBox creates a single-shape box body. isSensor selects a sensor
// shape and enableEvents sets ShapeDef.EnableSensorEvents.
func sensorAddBox(
	w *box2d.World, bodyType box2d.BodyType, position box2d.Vec2,
	halfWidth, halfHeight float64, isSensor, enableEvents bool,
) (box2d.BodyID, box2d.ShapeID) {
	bd := box2d.DefaultBodyDef()
	bd.Type = bodyType
	bd.Position = position
	body := w.CreateBody(&bd)

	sd := box2d.DefaultShapeDef()
	sd.IsSensor = isSensor
	sd.EnableSensorEvents = enableEvents

	poly := box2d.MakeBox(halfWidth, halfHeight)

	return body, w.CreatePolygonShape(body, &sd, &poly)
}

// sensorStep runs one step and copies out the sensor events it produced.
func sensorStep(w *box2d.World) sensorEventLog {
	w.Step(sensorDT, sensorSubSteps)
	events := w.SensorEvents()

	return sensorEventLog{
		begins: append([]box2d.SensorBeginTouchEvent(nil), events.BeginEvents...),
		ends:   append([]box2d.SensorEndTouchEvent(nil), events.EndEvents...),
	}
}

// sensorRun runs the world for the given number of steps and returns the
// per-step event log.
func sensorRun(w *box2d.World, steps int) []sensorEventLog {
	log := make([]sensorEventLog, 0, steps)
	for range steps {
		log = append(log, sensorStep(w))
	}

	return log
}

// sensorFirstBegin returns the index of the first step with a begin event, or
// -1.
func sensorFirstBegin(log []sensorEventLog) int {
	for i := range log {
		if len(log[i].begins) > 0 {
			return i
		}
	}

	return -1
}

// sensorFirstEnd returns the index of the first step with an end event, or -1.
func sensorFirstEnd(log []sensorEventLog) int {
	for i := range log {
		if len(log[i].ends) > 0 {
			return i
		}
	}

	return -1
}

func TestSensorBeginEndLifecycle(t *testing.T) {
	t.Parallel()

	w := newSensorWorld(t)

	_, sensorShape := sensorAddBox(w, box2d.StaticBody, box2d.Vec2{X: 0.0, Y: 0.0}, 1.0, 1.0, true, true)
	visitorBody, visitorShape := sensorAddBox(
		w, box2d.DynamicBody, box2d.Vec2{X: -5.0, Y: 0.0}, 0.25, 0.25, false, true,
	)
	w.SetBodyLinearVelocity(visitorBody, box2d.Vec2{X: 5.0, Y: 0.0})

	// Before overlap: no events at all.
	require.True(t, sensorStep(w).empty())

	log := sensorRun(w, 200)

	beginStep := sensorFirstBegin(log)
	require.GreaterOrEqual(t, beginStep, 0, "expected a begin touch event")
	require.Len(t, log[beginStep].begins, 1)
	require.Empty(t, log[beginStep].ends)
	require.Equal(t, sensorShape, log[beginStep].begins[0].SensorShapeID)
	require.Equal(t, visitorShape, log[beginStep].begins[0].VisitorShapeID)

	endStep := sensorFirstEnd(log)
	require.Greater(t, endStep, beginStep, "expected the end event after the begin event")
	require.Len(t, log[endStep].ends, 1)
	require.Equal(t, sensorShape, log[endStep].ends[0].SensorShapeID)
	require.Equal(t, visitorShape, log[endStep].ends[0].VisitorShapeID)

	// Exactly one begin and one end for the pass-through.
	begins, ends := 0, 0
	for i := range log {
		begins += len(log[i].begins)
		ends += len(log[i].ends)
	}
	require.Equal(t, 1, begins)
	require.Equal(t, 1, ends)

	// Re-entry emits again.
	w.SetBodyLinearVelocity(visitorBody, box2d.Vec2{X: -5.0, Y: 0.0})
	log2 := sensorRun(w, 200)

	begin2 := sensorFirstBegin(log2)
	require.GreaterOrEqual(t, begin2, 0, "expected a second begin touch event on re-entry")
	require.Equal(t, visitorShape, log2[begin2].begins[0].VisitorShapeID)

	end2 := sensorFirstEnd(log2)
	require.Greater(t, end2, begin2)
	require.Equal(t, visitorShape, log2[end2].ends[0].VisitorShapeID)
}

func TestSensorEventsDisabledOnVisitor(t *testing.T) {
	t.Parallel()

	w := newSensorWorld(t)

	sensorAddBox(w, box2d.StaticBody, box2d.Vec2{X: 0.0, Y: 0.0}, 1.0, 1.0, true, true)
	visitorBody, _ := sensorAddBox(w, box2d.DynamicBody, box2d.Vec2{X: -5.0, Y: 0.0}, 0.25, 0.25, false, false)
	w.SetBodyLinearVelocity(visitorBody, box2d.Vec2{X: 5.0, Y: 0.0})

	for _, log := range sensorRun(w, 200) {
		require.True(t, log.empty(), "visitor with EnableSensorEvents=false must not produce events")
	}
}

func TestSensorEventsDisabledOnSensor(t *testing.T) {
	t.Parallel()

	w := newSensorWorld(t)

	sensorAddBox(w, box2d.StaticBody, box2d.Vec2{X: 0.0, Y: 0.0}, 1.0, 1.0, true, false)
	visitorBody, _ := sensorAddBox(w, box2d.DynamicBody, box2d.Vec2{X: -5.0, Y: 0.0}, 0.25, 0.25, false, true)
	w.SetBodyLinearVelocity(visitorBody, box2d.Vec2{X: 5.0, Y: 0.0})

	for _, log := range sensorRun(w, 200) {
		require.True(t, log.empty(), "sensor with EnableSensorEvents=false must not produce events")
	}
}

func TestSensorEventFlagToggledMidSim(t *testing.T) {
	t.Parallel()

	w := newSensorWorld(t)

	_, sensorShape := sensorAddBox(w, box2d.StaticBody, box2d.Vec2{X: 0.0, Y: 0.0}, 1.0, 1.0, true, false)
	_, visitorShape := sensorAddBox(w, box2d.StaticBody, box2d.Vec2{X: 0.5, Y: 0.0}, 0.25, 0.25, false, true)

	// Sensor events are off on the sensor: the overlap is invisible.
	require.True(t, sensorStep(w).empty())

	// Enabling the sensor makes the standing overlap begin.
	w.EnableShapeSensorEvents(sensorShape, true)
	log := sensorStep(w)
	require.Len(t, log.begins, 1)
	require.Empty(t, log.ends)
	require.Equal(t, sensorShape, log.begins[0].SensorShapeID)
	require.Equal(t, visitorShape, log.begins[0].VisitorShapeID)

	// A persisting overlap is silent.
	require.True(t, sensorStep(w).empty())

	// Disabling the visitor drops it from the query results: end event.
	w.EnableShapeSensorEvents(visitorShape, false)
	log = sensorStep(w)
	require.Empty(t, log.begins)
	require.Len(t, log.ends, 1)
	require.Equal(t, visitorShape, log.ends[0].VisitorShapeID)

	// Re-enabling the visitor begins again.
	w.EnableShapeSensorEvents(visitorShape, true)
	log = sensorStep(w)
	require.Len(t, log.begins, 1)
	require.Empty(t, log.ends)

	// Disabling the sensor drops every overlap it holds: end event.
	w.EnableShapeSensorEvents(sensorShape, false)
	log = sensorStep(w)
	require.Empty(t, log.begins)
	require.Len(t, log.ends, 1)
	require.Equal(t, visitorShape, log.ends[0].VisitorShapeID)

	// A disabled sensor stays quiet.
	require.True(t, sensorStep(w).empty())
}

func TestSensorOnDynamicBodySweepsStaticShapes(t *testing.T) {
	t.Parallel()

	w := newSensorWorld(t)

	sensorBody, sensorShape := sensorAddBox(
		w, box2d.DynamicBody, box2d.Vec2{X: -10.0, Y: 0.0}, 0.25, 0.25, true, true,
	)
	w.SetBodyLinearVelocity(sensorBody, box2d.Vec2{X: 5.0, Y: 0.0})

	targets := make([]box2d.ShapeID, 0, 3)
	for i := range 3 {
		_, id := sensorAddBox(
			w, box2d.StaticBody, box2d.Vec2{X: float64(i) * 2.0, Y: 0.0}, 0.25, 0.25, false, true,
		)
		targets = append(targets, id)
	}

	visited := make([]box2d.ShapeID, 0, 3)
	closed := make([]box2d.ShapeID, 0, 3)
	for _, log := range sensorRun(w, 300) {
		for _, e := range log.begins {
			require.Equal(t, sensorShape, e.SensorShapeID)
			visited = append(visited, e.VisitorShapeID)
		}

		for _, e := range log.ends {
			require.Equal(t, sensorShape, e.SensorShapeID)
			closed = append(closed, e.VisitorShapeID)
		}
	}

	// The sensor sweeps left to right, so it meets the targets in order.
	require.Equal(t, targets, visited)
	require.Equal(t, targets, closed)
}

func TestSensorDetectsOtherSensor(t *testing.T) {
	t.Parallel()

	w := newSensorWorld(t)

	movingBody, movingShape := sensorAddBox(
		w, box2d.DynamicBody, box2d.Vec2{X: -5.0, Y: 0.0}, 0.5, 0.5, true, true,
	)
	w.SetBodyLinearVelocity(movingBody, box2d.Vec2{X: 5.0, Y: 0.0})

	_, staticShape := sensorAddBox(w, box2d.StaticBody, box2d.Vec2{X: 0.0, Y: 0.0}, 1.0, 1.0, true, true)

	type pair struct {
		sensorID  box2d.ShapeID
		visitorID box2d.ShapeID
	}

	begins := make([]pair, 0, 2)
	ends := make([]pair, 0, 2)
	for _, log := range sensorRun(w, 200) {
		for _, e := range log.begins {
			begins = append(begins, pair{e.SensorShapeID, e.VisitorShapeID})
		}

		for _, e := range log.ends {
			ends = append(ends, pair{e.SensorShapeID, e.VisitorShapeID})
		}
	}

	// Upstream v3.2 does not skip sensor visitors, so both sensors report the
	// other one. Events are published in sensor-array index order, which is
	// creation order.
	require.Equal(t, []pair{
		{movingShape, staticShape},
		{staticShape, movingShape},
	}, begins)
	require.Equal(t, []pair{
		{movingShape, staticShape},
		{staticShape, movingShape},
	}, ends)
}

func TestSensorIgnoresSameBodyShapes(t *testing.T) {
	t.Parallel()

	w := newSensorWorld(t)

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.StaticBody
	bd.Position = box2d.Vec2{X: 0.0, Y: 0.0}
	body := w.CreateBody(&bd)

	sensorDef := box2d.DefaultShapeDef()
	sensorDef.IsSensor = true
	sensorDef.EnableSensorEvents = true
	sensorPoly := box2d.MakeBox(1.0, 1.0)
	w.CreatePolygonShape(body, &sensorDef, &sensorPoly)

	// A second shape on the same body, fully inside the sensor.
	solidDef := box2d.DefaultShapeDef()
	solidDef.EnableSensorEvents = true
	solidPoly := box2d.MakeBox(0.25, 0.25)
	w.CreatePolygonShape(body, &solidDef, &solidPoly)

	for _, log := range sensorRun(w, 10) {
		require.True(t, log.empty(), "sensors must not detect shapes on their own body")
	}
}

func TestSensorVisitorShapeDestroyedWhileOverlapped(t *testing.T) {
	t.Parallel()

	w := newSensorWorld(t)

	_, sensorShape := sensorAddBox(w, box2d.StaticBody, box2d.Vec2{X: 0.0, Y: 0.0}, 1.0, 1.0, true, true)
	_, visitorShape := sensorAddBox(w, box2d.StaticBody, box2d.Vec2{X: 0.5, Y: 0.0}, 0.25, 0.25, false, true)

	log := sensorStep(w)
	require.Len(t, log.begins, 1)
	require.Equal(t, visitorShape, log.begins[0].VisitorShapeID)

	// Destroying a visitor mid-overlap does not report anything by itself.
	w.DestroyShape(visitorShape, true)
	require.False(t, w.IsShapeValid(visitorShape))

	// The next step finds no overlap and closes it out. The event carries the
	// stale visitor id: SensorEndTouchEvent documents that the shape "may have
	// been destroyed", so callers must check IsShapeValid.
	log = sensorStep(w)
	require.Empty(t, log.begins)
	require.Len(t, log.ends, 1)
	require.Equal(t, sensorShape, log.ends[0].SensorShapeID)
	require.Equal(t, visitorShape, log.ends[0].VisitorShapeID)
	require.False(t, w.IsShapeValid(log.ends[0].VisitorShapeID))

	// No stale repeats.
	for _, l := range sensorRun(w, 5) {
		require.True(t, l.empty())
	}
}

func TestSensorShapeDestroyedWhileOverlapped(t *testing.T) {
	t.Parallel()

	w := newSensorWorld(t)

	_, sensorShape := sensorAddBox(w, box2d.StaticBody, box2d.Vec2{X: 0.0, Y: 0.0}, 1.0, 1.0, true, true)
	_, visitorShape := sensorAddBox(w, box2d.StaticBody, box2d.Vec2{X: 0.5, Y: 0.0}, 0.25, 0.25, false, true)

	log := sensorStep(w)
	require.Len(t, log.begins, 1)

	// destroySensor pushes end events into the buffer the *next* step
	// publishes, so the current event view is unchanged.
	w.DestroyShape(sensorShape, true)
	require.False(t, w.IsShapeValid(sensorShape))
	require.Empty(t, w.SensorEvents().EndEvents)

	log = sensorStep(w)
	require.Empty(t, log.begins)
	require.Len(t, log.ends, 1)
	require.Equal(t, sensorShape, log.ends[0].SensorShapeID)
	require.Equal(t, visitorShape, log.ends[0].VisitorShapeID)
	require.False(t, w.IsShapeValid(log.ends[0].SensorShapeID))

	// The sensor is gone: stepping stays quiet and must not panic.
	for _, l := range sensorRun(w, 5) {
		require.True(t, l.empty())
	}
}

// sensorDestroyedSensorIndexFixup covers the removeSwap fixup in
// destroySensor: destroying the first of three sensors moves the last one
// into its slot and its shape must keep reporting.
func TestSensorDestroyMiddleSensorKeepsOthersWorking(t *testing.T) {
	t.Parallel()

	w := newSensorWorld(t)

	sensors := make([]box2d.ShapeID, 0, 3)
	for i := range 3 {
		_, id := sensorAddBox(
			w, box2d.StaticBody, box2d.Vec2{X: float64(i) * 10.0, Y: 0.0}, 1.0, 1.0, true, true,
		)
		sensors = append(sensors, id)
	}

	visitors := make([]box2d.ShapeID, 0, 3)
	for i := range 3 {
		_, id := sensorAddBox(
			w, box2d.StaticBody, box2d.Vec2{X: float64(i) * 10.0, Y: 0.5}, 0.25, 0.25, false, true,
		)
		visitors = append(visitors, id)
	}

	log := sensorStep(w)
	require.Len(t, log.begins, 3)

	// Destroy the first sensor. The last sensor is swapped into its slot.
	w.DestroyShape(sensors[0], true)

	log = sensorStep(w)
	require.Len(t, log.ends, 1)
	require.Equal(t, sensors[0], log.ends[0].SensorShapeID)
	require.Equal(t, visitors[0], log.ends[0].VisitorShapeID)

	// The surviving sensors still track their visitors: moving one away must
	// still report an end event from the swapped sensor.
	w.DestroyShape(visitors[2], true)

	log = sensorStep(w)
	require.Len(t, log.ends, 1)
	require.Equal(t, sensors[2], log.ends[0].SensorShapeID)
	require.Equal(t, visitors[2], log.ends[0].VisitorShapeID)

	// Sensor overlap queries still answer for the untouched sensor.
	require.Equal(t, 1, w.ShapeSensorCapacity(sensors[1]))
	ids := make([]box2d.ShapeID, 1)
	require.Equal(t, 1, w.ShapeSensorData(sensors[1], ids))
	require.Equal(t, visitors[1], ids[0])
}

func TestSensorMultipleVisitorsSortedDeterministically(t *testing.T) {
	t.Parallel()

	build := func() (*box2d.World, box2d.ShapeID, []box2d.ShapeID) {
		def := box2d.DefaultWorldDef()
		def.Gravity = box2d.Vec2{X: 0.0, Y: 0.0}
		w := box2d.NewWorld(&def)

		_, sensorShape := sensorAddBox(w, box2d.StaticBody, box2d.Vec2{X: 0.0, Y: 0.0}, 2.0, 2.0, true, true)

		// Visitors are created in an order that does not match their spatial
		// layout, so the sorted event order must come from the shape ids.
		offsets := []float64{1.5, -1.5, 0.75, -0.75, 0.0}
		visitors := make([]box2d.ShapeID, 0, len(offsets))
		for _, dx := range offsets {
			_, id := sensorAddBox(
				w, box2d.StaticBody, box2d.Vec2{X: dx, Y: 0.0}, 0.2, 0.2, false, true,
			)
			visitors = append(visitors, id)
		}

		return w, sensorShape, visitors
	}

	wA, sensorA, visitorsA := build()
	defer wA.Destroy()

	wB, _, _ := build()
	defer wB.Destroy()

	logA := sensorStep(wA)
	logB := sensorStep(wB)

	require.Len(t, logA.begins, len(visitorsA))
	require.Equal(t, logA, logB, "identical worlds must emit identical event sequences")

	// The visitor list is sorted by shape id, which is creation order here.
	for i, e := range logA.begins {
		require.Equal(t, sensorA, e.SensorShapeID)
		require.Equal(t, visitorsA[i], e.VisitorShapeID)
	}

	// The sensor overlap accessor sees the same sorted list.
	require.Equal(t, len(visitorsA), wA.ShapeSensorCapacity(sensorA))
	ids := make([]box2d.ShapeID, len(visitorsA))
	require.Equal(t, len(visitorsA), wA.ShapeSensorData(sensorA, ids))
	require.Equal(t, visitorsA, ids)
}

func TestSensorEventsAreDoubleBuffered(t *testing.T) {
	t.Parallel()

	w := newSensorWorld(t)

	_, sensorShape := sensorAddBox(w, box2d.StaticBody, box2d.Vec2{X: 0.0, Y: 0.0}, 1.0, 1.0, true, true)
	visitorBody, visitorShape := sensorAddBox(
		w, box2d.DynamicBody, box2d.Vec2{X: 0.5, Y: 0.0}, 0.25, 0.25, false, true,
	)

	w.Step(sensorDT, sensorSubSteps)
	events := w.SensorEvents()
	require.Len(t, events.BeginEvents, 1)
	require.Empty(t, events.EndEvents)
	require.Equal(t, sensorShape, events.BeginEvents[0].SensorShapeID)

	// Reading twice without stepping returns the same set.
	require.Equal(t, events, w.SensorEvents())

	// Move the visitor out. The next step replaces the event set: the begin
	// events are gone and the end event is visible.
	w.SetBodyTransform(visitorBody, box2d.Vec2{X: 20.0, Y: 0.0}, box2d.MakeRot(0.0))
	w.Step(sensorDT, sensorSubSteps)

	events = w.SensorEvents()
	require.Empty(t, events.BeginEvents)
	require.Len(t, events.EndEvents, 1)
	require.Equal(t, visitorShape, events.EndEvents[0].VisitorShapeID)

	// And the step after that clears it again.
	w.Step(sensorDT, sensorSubSteps)
	events = w.SensorEvents()
	require.Empty(t, events.BeginEvents)
	require.Empty(t, events.EndEvents)
}

// sensorBodyState is a bit-exact snapshot of a body's simulation state.
type sensorBodyState struct {
	PX, PY   uint64
	RC, RS   uint64
	VX, VY   uint64
	Omega    uint64
	Sleeping bool
}

func sensorSnapshot(w *box2d.World, bodies []box2d.BodyID) []sensorBodyState {
	states := make([]sensorBodyState, 0, len(bodies))
	for _, id := range bodies {
		xf := w.BodyTransform(id)
		v := w.BodyLinearVelocity(id)
		states = append(states, sensorBodyState{
			PX:       math.Float64bits(xf.P.X),
			PY:       math.Float64bits(xf.P.Y),
			RC:       math.Float64bits(xf.Q.C),
			RS:       math.Float64bits(xf.Q.S),
			VX:       math.Float64bits(v.X),
			VY:       math.Float64bits(v.Y),
			Omega:    math.Float64bits(w.BodyAngularVelocity(id)),
			Sleeping: !w.IsBodyAwake(id),
		})
	}

	return states
}

// buildSensorDeterminismScene builds a sensor-heavy scene: a grid of static
// sensor boxes with a column of dynamic visitors raining through them, plus a
// pair of dynamic sensors sweeping across the field.
func buildSensorDeterminismScene(w *box2d.World) []box2d.BodyID {
	bodies := make([]box2d.BodyID, 0, 32)

	for i := range 4 {
		for j := range 3 {
			body, _ := sensorAddBox(
				w, box2d.StaticBody,
				box2d.Vec2{X: float64(i)*2.0 - 3.0, Y: float64(j) * -3.0},
				0.9, 0.9, true, true,
			)
			bodies = append(bodies, body)
		}
	}

	for i := range 6 {
		body, _ := sensorAddBox(
			w, box2d.DynamicBody,
			box2d.Vec2{X: float64(i)*1.3 - 3.2, Y: 4.0},
			0.2, 0.2, false, true,
		)
		w.SetBodyLinearVelocity(body, box2d.Vec2{X: 0.15 * float64(i%3), Y: -3.0})
		bodies = append(bodies, body)
	}

	for i := range 2 {
		body, _ := sensorAddBox(
			w, box2d.DynamicBody,
			box2d.Vec2{X: -8.0, Y: float64(i) * -3.0},
			0.4, 0.4, true, true,
		)
		w.SetBodyLinearVelocity(body, box2d.Vec2{X: 2.0, Y: 0.0})
		bodies = append(bodies, body)
	}

	return bodies
}

func TestSensorDeterminismAcrossWorlds(t *testing.T) {
	t.Parallel()

	build := func() (*box2d.World, []box2d.BodyID) {
		def := box2d.DefaultWorldDef()
		def.Gravity = box2d.Vec2{X: 0.0, Y: -10.0}
		w := box2d.NewWorld(&def)

		return w, buildSensorDeterminismScene(w)
	}

	wA, bodiesA := build()
	defer wA.Destroy()

	wB, bodiesB := build()
	defer wB.Destroy()

	logA := sensorRun(wA, 200)
	logB := sensorRun(wB, 200)

	require.Equal(t, logA, logB, "sensor event sequences must be identical")
	require.Equal(t, sensorSnapshot(wA, bodiesA), sensorSnapshot(wB, bodiesB))

	// Sanity: the scene must actually exercise the sensor pass.
	total := 0
	for i := range logA {
		total += len(logA[i].begins) + len(logA[i].ends)
	}
	require.Positive(t, total, "sensor determinism scene produced no events")
}

// TestSensorBulletHit proves the continuous-collision sensor-hit path: a
// bullet crossing a thin sensor entirely within one step never overlaps it at
// a step boundary, so only the TOI sweep (solveContinuous -> taskContext
// sensorHits -> sensor.hits -> overlap pass) can report it (upstream: the
// sensor-hit report block in b2Solve).
func TestSensorBulletHit(t *testing.T) {
	t.Parallel()

	w := newSensorWorld(t)

	// Thin static sensor wall at x = 0.
	_, sensorShape := sensorAddBox(w, box2d.StaticBody, box2d.Vec2{X: 0.0, Y: 0.0}, 0.05, 4.0, true, true)

	// Bullet circle moving fast enough to cross the wall inside one step:
	// 200 m/s * (1/60 s) ≈ 3.33 m per step vs a 0.1 m thick sensor.
	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: -5.0, Y: 0.0}
	bd.LinearVelocity = box2d.Vec2{X: 200.0, Y: 0.0}
	bd.IsBullet = true
	body := w.CreateBody(&bd)

	sd := box2d.DefaultShapeDef()
	sd.EnableSensorEvents = true
	circle := box2d.Circle{Radius: 0.1}
	visitorShape := w.CreateCircleShape(body, &sd, &circle)

	log := sensorRun(w, 20)

	beginStep := sensorFirstBegin(log)
	require.GreaterOrEqual(t, beginStep, 0, "bullet crossing must produce a sensor begin event via the TOI hit path")
	require.Equal(t, sensorShape, log[beginStep].begins[0].SensorShapeID)
	require.Equal(t, visitorShape, log[beginStep].begins[0].VisitorShapeID)

	endStep := sensorFirstEnd(log)
	require.Greater(t, endStep, beginStep, "begin must be followed by an end once the bullet is past")

	// The bullet really did end up on the far side.
	require.Greater(t, w.BodyPosition(body).X, 1.0)
}
