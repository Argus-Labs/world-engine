// Behavior tests for the float64 port of the Box2D v3.2.0 solver core
// (src/solver.c, src/contact_solver.c, src/physics_world.c step portion):
// settling, restitution, friction, stacking, events, sleeping, island
// splitting and cross-world determinism.

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

const (
	stepDT       = 1.0 / 60.0
	stepSubSteps = 4
)

func newTestWorld(t *testing.T) *box2d.World {
	t.Helper()

	def := box2d.DefaultWorldDef()
	def.Gravity = box2d.Vec2{X: 0.0, Y: -10.0}
	w := box2d.NewWorld(&def)
	t.Cleanup(w.Destroy)
	return w
}

// createGround creates the standard static ground: a 100x20 box whose top
// surface is the line y = 0.
func createGround(t *testing.T, w *box2d.World) {
	t.Helper()

	bd := box2d.DefaultBodyDef()
	bd.Position = box2d.Vec2{X: 0.0, Y: -10.0}
	ground := w.CreateBody(&bd)

	groundBox := box2d.MakeBox(50.0, 10.0)
	sd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(ground, &sd, &groundBox)
}

// TestHelloWorld mirrors the upstream HelloWorld sample (test_world.c): a
// dynamic box falls onto the static ground under gravity {0,-10} at 60Hz with
// 4 sub-steps and settles at the analytic resting height, then sleeps, then
// wakes on impulse.
func TestHelloWorld(t *testing.T) {
	w := newTestWorld(t)
	createGround(t, w)

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: 0.0, Y: 4.0}
	body := w.CreateBody(&bd)

	box := box2d.MakeBox(1.0, 1.0)
	sd := box2d.DefaultShapeDef()
	sd.Density = 1.0
	sd.Material.Friction = 0.3
	w.CreatePolygonShape(body, &sd, &box)

	for range 90 {
		w.Step(stepDT, stepSubSteps)
	}

	// Analytic resting height: ground top (y=0) + half height (1).
	pos := w.BodyPosition(body)
	vel := w.BodyLinearVelocity(body)
	require.InDelta(t, 1.0, pos.Y, 2.0*box2d.LinearSlop, "resting height")
	require.InDelta(t, 0.0, pos.X, 2.0*box2d.LinearSlop, "no lateral drift")
	require.Less(t, math.Abs(vel.X), 0.01)
	require.Less(t, math.Abs(vel.Y), 0.01)
	require.Less(t, math.Abs(w.BodyAngularVelocity(body)), 0.01)

	// With sleep enabled the body must fall asleep.
	require.True(t, w.IsSleepingEnabled())
	asleep := false
	for range 120 {
		w.Step(stepDT, stepSubSteps)
		if !w.IsBodyAwake(body) {
			asleep = true
			break
		}
	}
	require.True(t, asleep, "body must fall asleep after settling")

	// Wake on impulse.
	w.ApplyBodyLinearImpulseToCenter(body, box2d.Vec2{X: 0.0, Y: 8.0}, true)
	require.True(t, w.IsBodyAwake(body))
	w.Step(stepDT, stepSubSteps)
	require.True(t, w.IsBodyAwake(body))
	require.Greater(t, w.BodyLinearVelocity(body).Y, 0.0)
}

// dropBall drops a circle of radius 0.5 from center height 10.5 with the
// given restitution and returns the peak center height reached after the
// first bounce (or the resting height if it never bounces).
func dropBall(t *testing.T, restitution float64) float64 {
	t.Helper()

	w := newTestWorld(t)
	createGround(t, w)

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: 0.0, Y: 10.5}
	ball := w.CreateBody(&bd)

	circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
	sd := box2d.DefaultShapeDef()
	sd.Density = 1.0
	sd.Material.Restitution = restitution
	w.CreateCircleShape(ball, &sd, &circle)

	bounced := false
	peak := 0.0
	for range 600 {
		w.Step(stepDT, stepSubSteps)
		v := w.BodyLinearVelocity(ball)
		y := w.BodyPosition(ball).Y

		if !bounced && v.Y > 0.1 {
			bounced = true
		}

		if bounced {
			if y > peak {
				peak = y
			}

			// stop once it is heading down again well below the peak
			if v.Y < -0.1 && y < peak-0.1 {
				return peak
			}
		}
	}

	if !bounced {
		return w.BodyPosition(ball).Y
	}
	return peak
}

// TestRestitution checks that a ball with restitution 0.8 dropped from height
// h bounces back to roughly e^2*h, and a ball with restitution 0 does not
// bounce.
func TestRestitution(t *testing.T) {
	// drop height above the resting position: 10.5 - 0.5 = 10
	const h = 10.0

	peak := dropBall(t, 0.8)
	bounceHeight := peak - 0.5
	// e^2 * h = 0.64 * 10 = 6.4, allow an energy tolerance band
	require.Greater(t, bounceHeight, 0.50*h, "restitution 0.8 must bounce high")
	require.Less(t, bounceHeight, 0.78*h, "restitution 0.8 must not gain energy")

	peak = dropBall(t, 0.0)
	// never bounced: peak is the resting height (~0.5)
	require.Less(t, peak, 0.7, "restitution 0 must not bounce")
}

// inclineSlide places a box on an incline with the given friction and
// returns its total displacement after settling and simulating.
func inclineSlide(t *testing.T, friction float64) float64 {
	t.Helper()

	w := newTestWorld(t)

	// Incline: static box rotated by 20 degrees. tan(20 deg) ~= 0.36, so
	// friction 1 holds and friction 0 slides.
	angle := 20.0 * box2d.Pi / 180.0
	rot := box2d.MakeRot(angle)

	bd := box2d.DefaultBodyDef()
	bd.Rotation = rot
	ground := w.CreateBody(&bd)
	groundBox := box2d.MakeBox(20.0, 1.0)
	gsd := box2d.DefaultShapeDef()
	gsd.Material.Friction = friction
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	// Place the box flush on the incline surface.
	local := box2d.Vec2{X: 0.0, Y: 1.5}
	world := box2d.RotateVector(rot, local)

	bd2 := box2d.DefaultBodyDef()
	bd2.Type = box2d.DynamicBody
	bd2.Position = world
	bd2.Rotation = rot
	boxBody := w.CreateBody(&bd2)
	box := box2d.MakeBox(0.5, 0.5)
	sd := box2d.DefaultShapeDef()
	sd.Density = 1.0
	sd.Material.Friction = friction
	w.CreatePolygonShape(boxBody, &sd, &box)

	// settle
	for range 30 {
		w.Step(stepDT, stepSubSteps)
	}
	start := w.BodyPosition(boxBody)

	for range 120 {
		w.Step(stepDT, stepSubSteps)
	}
	end := w.BodyPosition(boxBody)

	return box2d.Distance(start, end)
}

// TestFriction checks that high friction holds a box on an incline while zero
// friction lets it slide.
func TestFriction(t *testing.T) {
	held := inclineSlide(t, 1.0)
	require.Less(t, held, 0.05, "high friction must hold the box")

	slid := inclineSlide(t, 0.0)
	require.Greater(t, slid, 1.0, "zero friction must let the box slide")
}

// TestStack checks that a 5-box stack stays standing for 500 steps within a
// drift bound and eventually sleeps.
func TestStack(t *testing.T) {
	w := newTestWorld(t)
	createGround(t, w)

	const count = 5
	bodies := make([]box2d.BodyID, count)
	for i := range count {
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: 0.0, Y: 0.5 + float64(1.01*float64(i))}
		bodies[i] = w.CreateBody(&bd)

		box := box2d.MakeBox(0.5, 0.5)
		sd := box2d.DefaultShapeDef()
		sd.Density = 1.0
		w.CreatePolygonShape(bodies[i], &sd, &box)
	}

	for range 500 {
		w.Step(stepDT, stepSubSteps)
	}

	for i := range count {
		pos := w.BodyPosition(bodies[i])
		require.InDelta(t, 0.0, pos.X, 0.1, "box %d lateral drift", i)
		require.InDelta(t, 0.5+float64(i), pos.Y, 0.1, "box %d height", i)
	}

	// The stack must go to sleep.
	asleep := false
	for range 500 {
		w.Step(stepDT, stepSubSteps)
		allAsleep := true
		for i := range count {
			if w.IsBodyAwake(bodies[i]) {
				allAsleep = false
				break
			}
		}
		if allAsleep {
			asleep = true
			break
		}
	}
	require.True(t, asleep, "stack must fall asleep")
}

// TestPyramid checks that a 6-row pyramid is stable.
func TestPyramid(t *testing.T) {
	w := newTestWorld(t)
	createGround(t, w)

	const rows = 6
	var bodies []box2d.BodyID
	var expected []box2d.Vec2

	h := 0.5
	shift := 1.0 * h
	for i := range rows {
		y := float64((2.0*float64(i)+1.0)*h) + float64(0.02*float64(i))
		for j := i; j < rows; j++ {
			x := float64(float64(i)*shift) + float64(float64(2.0*h)*float64(j-i)) - float64(h*float64(rows-i-1))

			bd := box2d.DefaultBodyDef()
			bd.Type = box2d.DynamicBody
			bd.Position = box2d.Vec2{X: x, Y: y}
			body := w.CreateBody(&bd)

			box := box2d.MakeBox(h, h)
			sd := box2d.DefaultShapeDef()
			sd.Density = 1.0
			w.CreatePolygonShape(body, &sd, &box)

			bodies = append(bodies, body)
			expected = append(expected, bd.Position)
		}
	}

	for range 400 {
		w.Step(stepDT, stepSubSteps)
	}

	for i, body := range bodies {
		pos := w.BodyPosition(body)
		require.InDelta(t, expected[i].X, pos.X, 0.2, "pyramid box %d x", i)
		require.InDelta(t, expected[i].Y, pos.Y, 0.2, "pyramid box %d y", i)
	}
}

// TestContactEvents checks begin/end touch events and hit events across a
// bounce.
func TestContactEvents(t *testing.T) {
	w := newTestWorld(t)

	bd := box2d.DefaultBodyDef()
	bd.Position = box2d.Vec2{X: 0.0, Y: -10.0}
	ground := w.CreateBody(&bd)
	groundBox := box2d.MakeBox(50.0, 10.0)
	gsd := box2d.DefaultShapeDef()
	gsd.EnableContactEvents = true
	groundShape := w.CreatePolygonShape(ground, &gsd, &groundBox)

	bd2 := box2d.DefaultBodyDef()
	bd2.Type = box2d.DynamicBody
	bd2.Position = box2d.Vec2{X: 0.0, Y: 3.0}
	ball := w.CreateBody(&bd2)
	circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
	sd := box2d.DefaultShapeDef()
	sd.Density = 1.0
	sd.Material.Restitution = 0.8
	sd.EnableContactEvents = true
	sd.EnableHitEvents = true
	ballShape := w.CreateCircleShape(ball, &sd, &circle)

	sawBegin := false
	sawEnd := false
	sawHit := false
	for range 300 {
		w.Step(stepDT, stepSubSteps)
		events := w.ContactEvents()

		for _, e := range events.BeginEvents {
			sawBegin = true
			ids := []box2d.ShapeID{e.ShapeIDA, e.ShapeIDB}
			require.Contains(t, ids, groundShape)
			require.Contains(t, ids, ballShape)
		}

		for _, e := range events.EndEvents {
			sawEnd = true
			ids := []box2d.ShapeID{e.ShapeIDA, e.ShapeIDB}
			require.Contains(t, ids, groundShape)
			require.Contains(t, ids, ballShape)
		}

		for _, e := range events.HitEvents {
			sawHit = true
			require.Greater(t, e.ApproachSpeed, w.HitEventThreshold())
			ids := []box2d.ShapeID{e.ShapeIDA, e.ShapeIDB}
			require.Contains(t, ids, groundShape)
			require.Contains(t, ids, ballShape)
		}

		if sawBegin && sawEnd && sawHit {
			break
		}
	}

	require.True(t, sawBegin, "begin touch event")
	require.True(t, sawEnd, "end touch event (bounce separation)")
	require.True(t, sawHit, "hit event (impact speed above threshold)")
}

// TestBodyMoveEvents checks that every moving body reports a move event and
// that the fellAsleep flag is raised on the sleep transition.
func TestBodyMoveEvents(t *testing.T) {
	w := newTestWorld(t)
	createGround(t, w)

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: 0.0, Y: 2.0}
	body := w.CreateBody(&bd)
	bd.Position = box2d.Vec2{X: 3.0, Y: 2.0}
	body2 := w.CreateBody(&bd)

	box := box2d.MakeBox(0.5, 0.5)
	sd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(body, &sd, &box)
	w.CreatePolygonShape(body2, &sd, &box)

	w.Step(stepDT, stepSubSteps)
	events := w.BodyEvents()
	require.Len(t, events.MoveEvents, 2, "both awake bodies report move events")
	seen := map[box2d.BodyID]bool{}
	for _, e := range events.MoveEvents {
		seen[e.BodyID] = true
		require.False(t, e.FellAsleep)
	}
	require.True(t, seen[body])
	require.True(t, seen[body2])

	// Step until asleep and require the fellAsleep flag on the transition.
	fellAsleep := map[box2d.BodyID]bool{}
	for range 600 {
		w.Step(stepDT, stepSubSteps)
		for _, e := range w.BodyEvents().MoveEvents {
			if e.FellAsleep {
				fellAsleep[e.BodyID] = true
			}
		}
		if !w.IsBodyAwake(body) && !w.IsBodyAwake(body2) {
			break
		}
	}

	require.False(t, w.IsBodyAwake(body))
	require.False(t, w.IsBodyAwake(body2))
	require.True(t, fellAsleep[body], "fellAsleep flag for body 1")
	require.True(t, fellAsleep[body2], "fellAsleep flag for body 2")

	// Sleeping bodies do not report move events.
	w.Step(stepDT, stepSubSteps)
	require.Empty(t, w.BodyEvents().MoveEvents)
}

// TestIslandSplitAndSelectiveWake builds a three-body contact chain (one
// island), removes the middle body by teleporting it away, and checks that
// the island splits so waking one body leaves the others asleep.
func TestIslandSplitAndSelectiveWake(t *testing.T) {
	w := newTestWorld(t)
	createGround(t, w)

	makeBox := func(x float64) box2d.BodyID {
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: x, Y: 0.5}
		body := w.CreateBody(&bd)
		box := box2d.MakeBox(0.5, 0.5)
		sd := box2d.DefaultShapeDef()
		w.CreatePolygonShape(body, &sd, &box)
		return body
	}

	a := makeBox(-1.0)
	b := makeBox(0.0)
	c := makeBox(1.0)

	// Let the contact chain form: A-B and B-C merge the islands into one.
	for range 30 {
		w.Step(stepDT, stepSubSteps)
	}
	require.Equal(t, 1, w.Counters().IslandCount, "chain forms one island")

	// Teleport the middle body far away (but still above the ground). The
	// A-B and B-C contacts are destroyed which flags the island for
	// splitting.
	w.SetBodyTransform(b, box2d.Vec2{X: 20.0, Y: 0.5}, box2d.MakeRot(0.0))

	// Let everything settle and sleep. The island must split before it can
	// sleep, leaving three single-body islands.
	for range 600 {
		w.Step(stepDT, stepSubSteps)
		if !w.IsBodyAwake(a) && !w.IsBodyAwake(b) && !w.IsBodyAwake(c) {
			break
		}
	}
	require.False(t, w.IsBodyAwake(a))
	require.False(t, w.IsBodyAwake(b))
	require.False(t, w.IsBodyAwake(c))
	require.Equal(t, 3, w.Counters().IslandCount, "island must split into three")

	// Waking one body wakes its island only.
	w.ApplyBodyLinearImpulseToCenter(a, box2d.Vec2{X: 0.0, Y: 2.0}, true)
	require.True(t, w.IsBodyAwake(a))
	require.False(t, w.IsBodyAwake(b), "waking A must not wake B")
	require.False(t, w.IsBodyAwake(c), "waking A must not wake C")
}

// buildMixedScene fills a world with a deterministic mixed scene: ground box,
// a segment ramp and falling circles, boxes and capsules.
func buildMixedScene(w *box2d.World) []box2d.BodyID {
	var bodies []box2d.BodyID

	// ground
	bd := box2d.DefaultBodyDef()
	bd.Position = box2d.Vec2{X: 0.0, Y: -10.0}
	ground := w.CreateBody(&bd)
	groundBox := box2d.MakeBox(50.0, 10.0)
	gsd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	// segment ramp
	sbd := box2d.DefaultBodyDef()
	sramp := w.CreateBody(&sbd)
	segment := box2d.Segment{Point1: box2d.Vec2{X: -8.0, Y: 4.0}, Point2: box2d.Vec2{X: -2.0, Y: 1.0}}
	ssd := box2d.DefaultShapeDef()
	w.CreateSegmentShape(sramp, &ssd, &segment)

	// falling shapes
	for i := range 8 {
		fi := float64(i)

		cbd := box2d.DefaultBodyDef()
		cbd.Type = box2d.DynamicBody
		cbd.Position = box2d.Vec2{X: -6.0 + float64(1.3*fi), Y: 6.0 + float64(0.31*fi)}
		ball := w.CreateBody(&cbd)
		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.35}
		csd := box2d.DefaultShapeDef()
		csd.Density = 1.0
		csd.Material.Restitution = 0.3
		w.CreateCircleShape(ball, &csd, &circle)
		bodies = append(bodies, ball)

		bbd := box2d.DefaultBodyDef()
		bbd.Type = box2d.DynamicBody
		bbd.Position = box2d.Vec2{X: -5.5 + float64(1.4*fi), Y: 9.0 + float64(0.17*fi)}
		bbd.Rotation = box2d.MakeRot(0.1 * fi)
		boxBody := w.CreateBody(&bbd)
		box := box2d.MakeBox(0.4, 0.3)
		bsd := box2d.DefaultShapeDef()
		bsd.Density = 1.0
		w.CreatePolygonShape(boxBody, &bsd, &box)
		bodies = append(bodies, boxBody)
	}

	for i := range 4 {
		fi := float64(i)

		kbd := box2d.DefaultBodyDef()
		kbd.Type = box2d.DynamicBody
		kbd.Position = box2d.Vec2{X: -3.0 + float64(2.1*fi), Y: 12.0 + float64(0.5*fi)}
		capBody := w.CreateBody(&kbd)
		capsule := box2d.Capsule{
			Center1: box2d.Vec2{X: -0.3, Y: 0.0},
			Center2: box2d.Vec2{X: 0.3, Y: 0.0},
			Radius:  0.25,
		}
		ksd := box2d.DefaultShapeDef()
		ksd.Density = 1.0
		w.CreateCapsuleShape(capBody, &ksd, &capsule)
		bodies = append(bodies, capBody)
	}

	return bodies
}

// TestDeterminism steps two identical worlds through a 300-step mixed scene
// and requires bit-for-bit identical body state every 50 steps.
func TestDeterminism(t *testing.T) {
	w1 := newTestWorld(t)
	w2 := newTestWorld(t)

	bodies1 := buildMixedScene(w1)
	bodies2 := buildMixedScene(w2)
	require.Len(t, bodies2, len(bodies1))

	for step := 1; step <= 300; step++ {
		w1.Step(stepDT, stepSubSteps)
		w2.Step(stepDT, stepSubSteps)

		require.Len(t, w2.ContactEvents().BeginEvents, len(w1.ContactEvents().BeginEvents), "step %d", step)
		require.Len(t, w2.ContactEvents().EndEvents, len(w1.ContactEvents().EndEvents), "step %d", step)

		if step%50 != 0 && step != 300 {
			continue
		}

		c1 := w1.Counters()
		c2 := w2.Counters()
		require.Equal(t, c1.ContactCount, c2.ContactCount, "step %d contact count", step)
		require.Equal(t, c1.IslandCount, c2.IslandCount, "step %d island count", step)

		for i := range bodies1 {
			p1 := w1.BodyPosition(bodies1[i])
			p2 := w2.BodyPosition(bodies2[i])
			q1 := w1.BodyRotation(bodies1[i])
			q2 := w2.BodyRotation(bodies2[i])
			v1 := w1.BodyLinearVelocity(bodies1[i])
			v2 := w2.BodyLinearVelocity(bodies2[i])
			av1 := w1.BodyAngularVelocity(bodies1[i])
			av2 := w2.BodyAngularVelocity(bodies2[i])

			require.Equal(t, math.Float64bits(p1.X), math.Float64bits(p2.X), "step %d body %d pos.X", step, i)
			require.Equal(t, math.Float64bits(p1.Y), math.Float64bits(p2.Y), "step %d body %d pos.Y", step, i)
			require.Equal(t, math.Float64bits(q1.C), math.Float64bits(q2.C), "step %d body %d rot.C", step, i)
			require.Equal(t, math.Float64bits(q1.S), math.Float64bits(q2.S), "step %d body %d rot.S", step, i)
			require.Equal(t, math.Float64bits(v1.X), math.Float64bits(v2.X), "step %d body %d vel.X", step, i)
			require.Equal(t, math.Float64bits(v1.Y), math.Float64bits(v2.Y), "step %d body %d vel.Y", step, i)
			require.Equal(t, math.Float64bits(av1), math.Float64bits(av2), "step %d body %d angVel", step, i)
		}
	}
}

// TestZeroTimeStep checks the upstream early-return path for dt == 0.
func TestZeroTimeStep(t *testing.T) {
	w := newTestWorld(t)
	createGround(t, w)

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: 0.0, Y: 2.0}
	body := w.CreateBody(&bd)
	box := box2d.MakeBox(0.5, 0.5)
	sd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(body, &sd, &box)

	before := w.BodyPosition(body)
	w.Step(0.0, stepSubSteps)
	after := w.BodyPosition(body)

	require.Equal(t, math.Float64bits(before.X), math.Float64bits(after.X))
	require.Equal(t, math.Float64bits(before.Y), math.Float64bits(after.Y))
	require.Empty(t, w.BodyEvents().MoveEvents)
}

// TestBodyMoveIndexInvalidatedOnAwakeSetExit is the regression test for a
// stale bodyMoveIndex found by the E14 op-sequence fuzzer: a body that left
// the awake set through SetBodyType kept the move-event index assigned by an
// earlier, larger step, and a later forced sleep indexed a shorter buffer
// (upstream C has the same gap and reads out of bounds unchecked).
func TestBodyMoveIndexInvalidatedOnAwakeSetExit(t *testing.T) {
	t.Parallel()

	def := box2d.DefaultWorldDef()
	w := box2d.NewWorld(&def)
	defer w.Destroy()

	mkDynamic := func(x float64) box2d.BodyID {
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: x, Y: 0}
		body := w.CreateBody(&bd)
		sd := box2d.DefaultShapeDef()
		circle := box2d.Circle{Radius: 0.5}
		w.CreateCircleShape(body, &sd, &circle)
		return body
	}

	a := mkDynamic(0)
	b := mkDynamic(10)
	x := mkDynamic(20)

	w.Step(1.0/60.0, 4) // three move events; x holds index 2

	w.SetBodyType(x, box2d.StaticBody) // x leaves the awake set
	w.DestroyBody(a)
	w.DestroyBody(b)

	w.Step(1.0/60.0, 4) // zero move events this step

	w.SetBodyType(x, box2d.DynamicBody) // re-enters the awake set

	// Must not panic; before the fix this indexed bodyMoveEvents[2] with
	// length 0.
	w.SetBodyAwake(x, false)
	require.False(t, w.IsBodyAwake(x))
}
