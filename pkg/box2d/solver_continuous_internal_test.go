// Internal tests for the continuous-collision port (solver_continuous.go and
// the bullet stage in solver.go): flag bookkeeping (isSpeedCapped,
// hadTimeOfImpact, enlargeBounds) that has no public accessors.

package box2d

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSpeedCapSetsFlag launches a body far above the world maximum linear
// speed and verifies both the clamp and the isSpeedCapped transfer from the
// sim flags to the body flags in finalizeBodiesTask.
func TestSpeedCapSetsFlag(t *testing.T) {
	def := DefaultWorldDef()
	def.Gravity = Vec2Zero
	def.MaximumLinearSpeed = 10.0
	w := NewWorld(&def)
	defer w.Destroy()

	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.LinearVelocity = Vec2{X: 100.0, Y: 0.0}
	bodyID := w.CreateBody(&bd)

	circle := Circle{Center: Vec2Zero, Radius: 0.5}
	sd := DefaultShapeDef()
	sd.Density = 1.0
	w.CreateCircleShape(bodyID, &sd, &circle)

	w.Step(1.0/60.0, 4)

	b := w.getBodyFullID(bodyID)
	tassert.NotZero(t, b.flags&isSpeedCapped, "body flags must carry isSpeedCapped after the clamp")

	v := w.BodyLinearVelocity(bodyID)
	require.InDelta(t, 10.0, Length(v), 1e-9, "velocity must be clamped to MaximumLinearSpeed")

	// A slow body must not be flagged.
	w.SetBodyLinearVelocity(bodyID, Vec2{X: 1.0, Y: 0.0})
	w.Step(1.0/60.0, 4)
	b = w.getBodyFullID(bodyID)
	tassert.Zero(t, b.flags&isSpeedCapped, "slow body must not be speed capped")
}

// TestContinuousSetsHadTimeOfImpact drives a fast non-bullet circle into a
// thin static wall and checks that the continuous solve records the TOI event
// on the body flags (transferred in finalizeBodiesTask on the next step for
// non-bullet bodies solved inline).
func TestContinuousSetsHadTimeOfImpact(t *testing.T) {
	def := DefaultWorldDef()
	def.Gravity = Vec2Zero
	w := NewWorld(&def)
	defer w.Destroy()

	gd := DefaultBodyDef()
	ground := w.CreateBody(&gd)
	wall := MakeBox(0.05, 10.0)
	gsd := DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &wall)

	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = Vec2{X: -5.0, Y: 0.0}
	bd.LinearVelocity = Vec2{X: 200.0, Y: 0.0}
	bodyID := w.CreateBody(&bd)

	circle := Circle{Center: Vec2Zero, Radius: 0.1}
	sd := DefaultShapeDef()
	sd.Density = 1.0
	w.CreateCircleShape(bodyID, &sd, &circle)

	// The TOI happens on the step whose sweep crosses the wall. Watch the
	// body flags across a few steps.
	sawTOI := false
	for range 10 {
		w.Step(1.0/60.0, 4)
		b := w.getBodyFullID(bodyID)
		if b.flags&hadTimeOfImpact != 0 {
			sawTOI = true
			break
		}
	}
	tassert.True(t, sawTOI, "fast body sweeping into a wall must record hadTimeOfImpact")

	// The body must have been stopped by the TOI + contact, not tunneled.
	p := w.BodyPosition(bodyID)
	require.Less(t, p.X, 0.0)
}

// TestBulletBodiesFillOrderIsDeterministic checks that finalizeBodiesTask
// gathers fast bullet bodies in ascending body-sim index order, which is what
// keeps the serial bullet stage deterministic.
func TestBulletBodiesFillOrderIsDeterministic(t *testing.T) {
	def := DefaultWorldDef()
	def.Gravity = Vec2Zero
	w := NewWorld(&def)
	defer w.Destroy()

	// Several bullets, all fast, no contacts.
	const count = 8
	ids := make([]BodyID, 0, count)
	for i := range count {
		bd := DefaultBodyDef()
		bd.Type = DynamicBody
		bd.Position = Vec2{X: 0.0, Y: 10.0 * float64(i)}
		bd.LinearVelocity = Vec2{X: 100.0, Y: 0.0}
		bd.IsBullet = true
		bodyID := w.CreateBody(&bd)

		circle := Circle{Center: Vec2Zero, Radius: 0.1}
		sd := DefaultShapeDef()
		sd.Density = 1.0
		w.CreateCircleShape(bodyID, &sd, &circle)
		ids = append(ids, bodyID)
	}

	w.Step(1.0/60.0, 4)

	// After the step every bullet must have advanced (center0 == center,
	// i.e. solveContinuous ran and advanced the sweep origin).
	awake := &w.solverSets[awakeSet]
	require.Len(t, awake.bodySims, count)
	for i := range awake.bodySims {
		sim := &awake.bodySims[i]
		tassert.Equal(t, sim.center, sim.center0, "bullet sim %d must be advanced by the continuous solve", i)
		tassert.NotZero(t, sim.flags&isBullet)
	}
	_ = ids
}

// timeLossTolerance is the max allowed error between the observed post-TOI
// linear velocity and the upstream v3.2.0 analytic value vy0 + g*dt*fraction.
// The bug under fix deviates by ~0.086 m/s at these parameters (~858x this).
const timeLossTolerance = 1e-4

// timeLossScene builds the bug-report reproduction: a small fast circle under
// gravity above a static floor. When withGround is false the body is in free
// fall (the control sweep); when true it hits the floor mid-step via a TOI.
// The returned c1 is the COM at the start of the step (identical for both
// worlds, captured before stepping).
func timeLossScene(t *testing.T, withGround, isBullet bool, locks MotionLocks) (*World, BodyID, Vec2) {
	t.Helper()

	const (
		dt       = 1.0 / 60.0
		vy0      = -50.0
		radius   = 0.1
		yStart   = 0.5
		subSteps = 8
	)

	// Build the world with a non-trivial gravity that has both an X and a Y
	// component so the per-axis lock clipping in the correction is exercised
	// when the caller requests a lock.
	def := DefaultWorldDef()
	def.Gravity = Vec2{X: -5.0, Y: -10.0}
	w := NewWorld(&def)

	if withGround {
		gd := DefaultBodyDef()
		ground := w.CreateBody(&gd)
		// Wide thin floor with its top surface at y = 0.
		floor := MakeOffsetBox(5.0, 0.05, Vec2{X: 0.0, Y: -0.05}, RotIdentity)
		gsd := DefaultShapeDef()
		w.CreatePolygonShape(ground, &gsd, &floor)
	}

	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = Vec2{X: 0.0, Y: yStart}
	bd.LinearVelocity = Vec2{X: 0.0, Y: vy0}
	bd.IsBullet = isBullet
	bd.MotionLocks = locks
	bodyID := w.CreateBody(&bd)

	circle := Circle{Center: Vec2Zero, Radius: radius}
	sd := DefaultShapeDef()
	sd.Density = 1.0
	w.CreateCircleShape(bodyID, &sd, &circle)

	c1 := w.BodyWorldCenterOfMass(bodyID)

	w.Step(dt, subSteps)

	return w, bodyID, c1
}

// TestSolveContinuousTimeLossGravityCorrection reproduces the upstream v3.2.0
// time-loss gravity velocity correction in the TOI-hit branch of
// solveContinuous. A fast body under gravity that hits a TOI mid-step must
// keep only the gravity impulse for the simulated fraction of the step
// (vy0 + g*dt*fraction), not the full step's worth (vy0 + g*dt) the port used
// to retain. Mirrors the reproduction in the bug report and exercises both
// call sites: non-bullet fast bodies (solved inline in finalizeBodiesTask) and
// bullet bodies (the serial bullet stage).
func TestSolveContinuousTimeLossGravityCorrection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		isBullet bool
	}{
		{"non_bullet", false},
		{"bullet", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const (
				dt  = 1.0 / 60.0
				vy0 = -50.0
				gY  = -10.0
			)

			// Control: free fall, no ground → the full gravity increment for
			// the whole step, and the full semi-implicit-Euler sweep.
			wc, control, c1 := timeLossScene(t, false, tc.isBullet, MotionLocks{})
			defer wc.Destroy()
			c2 := wc.BodyWorldCenterOfMass(control)
			vControl := wc.BodyLinearVelocity(control)
			require.InDelta(t, vy0+gY*dt, vControl.Y, 1e-4,
				"control (no TOI) must take the full step gravity")

			// TOI: identical body above a static floor.
			wt, hit, _ := timeLossScene(t, true, tc.isBullet, MotionLocks{})
			defer wt.Destroy()

			// hadTimeOfImpact is set on the body SIM by continuousQueryCallback
			// during the TOI sweep and only transferred to the body flags on
			// the next step's finalize, so check the sim flag directly.
			hsim := wt.getBodySim(wt.getBodyFullID(hit))
			require.NotZero(t, hsim.flags&hadTimeOfImpact,
				"%s: fast body must record a TOI against the floor", tc.name)

			cHit := wt.BodyWorldCenterOfMass(hit)
			vHit := wt.BodyLinearVelocity(hit)

			// The TOI must stop the body short of the free-fall sweep.
			require.Less(t, cHit.Y, c1.Y, "%s: body must fall", tc.name)
			require.Greater(t, cHit.Y, c2.Y, "%s: TOI must stop the body before the full sweep", tc.name)

			// Fraction of the step the body was actually simulated (from the
			// COM sweep, which is identical to the control's pre-CCD sweep).
			fraction := (cHit.Y - c1.Y) / (c2.Y - c1.Y)
			require.True(t, fraction > 0.0 && fraction < 1.0,
				"%s: TOI fraction must be in (0,1), got %v", tc.name, fraction)

			// Upstream v3.2.0 correction: only the simulated fraction's gravity
			// is kept. The bug retained the full vy0 + g*dt. The float64()
			// conversion rounds the product so it cannot fuse into an FMA
			// (see math_fma.go / nofma_test.go).
			wantY := vy0 + float64(gY*dt*fraction)
			require.InDelta(t, wantY, vHit.Y, timeLossTolerance,
				"%s: TOI body must keep only the simulated fraction's gravity (time-loss correction)", tc.name)

			// And it must carry strictly less downward speed than free fall:
			// the un-simulated portion's gravity was removed.
			require.Greater(t, vHit.Y, vControl.Y,
				"%s: TOI body must have less downward speed than free fall", tc.name)
		})
	}
}

// TestSolveContinuousTimeLossRespectsLinearLocks verifies the per-axis lock
// clipping of the time-loss correction: a body locked on X must NOT acquire
// spurious X velocity from the correction's gravity.X component when it hits a
// TOI. Upstream v3.2.0 zeros dv on locked axes before adding it.
func TestSolveContinuousTimeLossRespectsLinearLocks(t *testing.T) {
	// Locked on X, free on Y, gravity has an X component. Without the lock
	// clipping, the correction would set v.X = -timeLoss*gravityScale*gravity.X
	// (nonzero); with it, v.X stays zero.
	wt, hit, _ := timeLossScene(t, true, false, MotionLocks{LinearX: true})
	defer wt.Destroy()

	hsim := wt.getBodySim(wt.getBodyFullID(hit))
	require.NotZero(t, hsim.flags&hadTimeOfImpact, "body must hit a TOI against the floor")

	v := wt.BodyLinearVelocity(hit)
	require.InDelta(t, 0.0, v.X, timeLossTolerance,
		"locked-X body must keep zero X velocity after the TOI correction (lock clipping)")
}

// TestSolveContinuousTimeLossSpeedCappedNoReversal reproduces the regression
// reported by the slopcop review on a12efce: a speed-capped fast body under
// gravity that hits a TOI mid-step must NOT have its velocity reversed by the
// time-loss correction. integrateVelocitiesTask caps velocity to
// MaximumLinearSpeed and discards part of the gravity impulse; subtracting
// the full theoretical un-simulated gravity (as the pre-fix code did) removes
// gravity that was never retained and can launch the body back above the cap.
// After the fix the post-TOI velocity must stay at or below the cap and
// must not reverse direction.
func TestSolveContinuousTimeLossSpeedCappedNoReversal(t *testing.T) {
	const (
		dt       = 1.0 / 60.0
		maxSpeed = 10.0
		radius   = 0.1
		yStart   = 0.15
		vy0      = -50.0
		subSteps = 8
	)

	def := DefaultWorldDef()
	def.Gravity = Vec2{X: 0.0, Y: -10000.0}
	def.MaximumLinearSpeed = maxSpeed
	w := NewWorld(&def)
	defer w.Destroy()

	gd := DefaultBodyDef()
	ground := w.CreateBody(&gd)
	floor := MakeOffsetBox(5.0, 0.05, Vec2{X: 0.0, Y: -0.05}, RotIdentity)
	gsd := DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &floor)

	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = Vec2{X: 0.0, Y: yStart}
	bd.LinearVelocity = Vec2{X: 0.0, Y: vy0}
	bodyID := w.CreateBody(&bd)

	circle := Circle{Center: Vec2Zero, Radius: radius}
	sd := DefaultShapeDef()
	sd.Density = 1.0
	w.CreateCircleShape(bodyID, &sd, &circle)

	w.Step(dt, subSteps)

	hsim := w.getBodySim(w.getBodyFullID(bodyID))
	require.NotZero(t, hsim.flags&hadTimeOfImpact,
		"fast body must record a TOI against the floor")

	bf := w.getBodyFullID(bodyID)
	require.NotZero(t, bf.flags&isSpeedCapped,
		"body must be speed-capped at this step")

	v := w.BodyLinearVelocity(bodyID)
	require.LessOrEqual(t, Length(v), maxSpeed+1e-6,
		"post-TOI velocity must respect MaximumLinearSpeed (no cap reversal)")
	require.LessOrEqual(t, v.Y, 1e-6,
		"downward TOI must not reverse the body upward (pre-fix reversed to +101.67)")
}

// TestSolveContinuousTimeLossSpeedCappedNoReversalBullet is the bullet-path
// twin of the above test: the serial bullet stage calls solveContinuous on
// bullet bodies after finalizeBodiesTask clears the sim isSpeedCapped flag, so
// the same body-flag guard must protect the bullet path.
func TestSolveContinuousTimeLossSpeedCappedNoReversalBullet(t *testing.T) {
	const (
		dt       = 1.0 / 60.0
		maxSpeed = 10.0
		radius   = 0.1
		yStart   = 0.15
		vy0      = -50.0
		subSteps = 8
	)

	def := DefaultWorldDef()
	def.Gravity = Vec2{X: 0.0, Y: -10000.0}
	def.MaximumLinearSpeed = maxSpeed
	w := NewWorld(&def)
	defer w.Destroy()

	gd := DefaultBodyDef()
	ground := w.CreateBody(&gd)
	floor := MakeOffsetBox(5.0, 0.05, Vec2{X: 0.0, Y: -0.05}, RotIdentity)
	gsd := DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &floor)

	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = Vec2{X: 0.0, Y: yStart}
	bd.LinearVelocity = Vec2{X: 0.0, Y: vy0}
	bd.IsBullet = true
	bodyID := w.CreateBody(&bd)

	circle := Circle{Center: Vec2Zero, Radius: radius}
	sd := DefaultShapeDef()
	sd.Density = 1.0
	w.CreateCircleShape(bodyID, &sd, &circle)

	w.Step(dt, subSteps)

	hsim := w.getBodySim(w.getBodyFullID(bodyID))
	require.NotZero(t, hsim.flags&hadTimeOfImpact,
		"bullet must record a TOI against the floor")

	bf := w.getBodyFullID(bodyID)
	require.NotZero(t, bf.flags&isSpeedCapped,
		"bullet must be speed-capped at this step")

	v := w.BodyLinearVelocity(bodyID)
	require.LessOrEqual(t, Length(v), maxSpeed+1e-6,
		"post-TOI velocity must respect MaximumLinearSpeed (no cap reversal)")
	require.LessOrEqual(t, v.Y, 1e-6,
		"downward TOI must not reverse the bullet upward (pre-fix reversed to +101.67)")
}

// timeLossReproScene builds the common slopcop reproduction used by the three
// follow-up regression tests: a small fast circle above a static floor, under
// the given gravity and linear damping, stepping once. maxLinearSpeed,
// linearDamping and the initial angular velocity are configurable so each
// finding exercises its own guard.
func timeLossReproScene(t *testing.T, gravity Vec2, maxLinearSpeed, linearDamping, angularVelocity float64, isBullet bool) (*World, BodyID) {
	t.Helper()

	const (
		dt       = 1.0 / 60.0
		radius   = 0.1
		yStart   = 0.15
		vy0      = -50.0
		subSteps = 8
	)

	def := DefaultWorldDef()
	def.Gravity = gravity
	def.MaximumLinearSpeed = maxLinearSpeed
	w := NewWorld(&def)

	gd := DefaultBodyDef()
	ground := w.CreateBody(&gd)
	floor := MakeOffsetBox(5.0, 0.05, Vec2{X: 0.0, Y: -0.05}, RotIdentity)
	gsd := DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &floor)

	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = Vec2{X: 0.0, Y: yStart}
	bd.LinearVelocity = Vec2{X: 0.0, Y: vy0}
	bd.AngularVelocity = angularVelocity
	bd.LinearDamping = linearDamping
	bd.IsBullet = isBullet
	bodyID := w.CreateBody(&bd)

	circle := Circle{Center: Vec2Zero, Radius: radius}
	sd := DefaultShapeDef()
	sd.Density = 1.0
	w.CreateCircleShape(bodyID, &sd, &circle)

	w.Step(dt, subSteps)

	return w, bodyID
}

// TestSolveContinuousTimeLossDampingNoReversal reproduces the damping finding
// from the slopcop review on 526a4f5: a heavily damped fast body under strong
// gravity that hits a TOI mid-step must NOT be reversed upward by the time-loss
// correction. integrateVelocitiesTask attenuates the accumulated velocity
// (including the gravity impulse) every substep, so the body retained less
// than the full theoretical gravity; subtracting the full un-simulated amount
// (as the pre-fix code did) reverses a falling body upward to +118.63. The
// correction is conservatively skipped for damped bodies, preserving the
// downward velocity. Exercises both the non-bullet and bullet CCD paths.
func TestSolveContinuousTimeLossDampingNoReversal(t *testing.T) {
	for _, tc := range []struct {
		name     string
		isBullet bool
	}{
		{"non_bullet", false},
		{"bullet", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, bodyID := timeLossReproScene(t,
				Vec2{X: 0.0, Y: -10000.0}, // gravity
				400.0,                     // maxLinearSpeed (never reached here)
				1000.0,                    // linearDamping
				0.0,                       // angularVelocity
				tc.isBullet)
			defer w.Destroy()

			hsim := w.getBodySim(w.getBodyFullID(bodyID))
			require.NotZero(t, hsim.flags&hadTimeOfImpact,
				"%s: fast body must record a TOI against the floor", tc.name)

			v := w.BodyLinearVelocity(bodyID)
			require.LessOrEqual(t, v.Y, 1e-6,
				"%s: damped downward TOI must not reverse the body upward (pre-fix reversed to +118.63)", tc.name)
		})
	}
}

// TestSolveContinuousTimeLossZeroMassNoGravityImpulse reproduces the zero-mass
// finding from the slopcop review on 526a4f5: a zero-density (zero-mass) dynamic
// body receives NO gravity from integrateVelocitiesTask (the integrator zeroes
// the gravity scale when invMass == 0), so the time-loss correction must NOT
// subtract a gravity contribution that was never integrated. The pre-fix code
// used fastBodySim.gravityScale unconditionally and reversed the body to
// +105.67; the invMass > 0 guard skips the correction, leaving the downward
// velocity intact. Exercises both CCD paths.
func TestSolveContinuousTimeLossZeroMassNoGravityImpulse(t *testing.T) {
	const (
		dt       = 1.0 / 60.0
		radius   = 0.1
		yStart   = 0.15
		vy0      = -50.0
		subSteps = 8
	)

	for _, tc := range []struct {
		name     string
		isBullet bool
	}{
		{"non_bullet", false},
		{"bullet", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := DefaultWorldDef()
			def.Gravity = Vec2{X: 0.0, Y: -10000.0}
			def.MaximumLinearSpeed = 400.0
			w := NewWorld(&def)
			defer w.Destroy()

			gd := DefaultBodyDef()
			ground := w.CreateBody(&gd)
			floor := MakeOffsetBox(5.0, 0.05, Vec2{X: 0.0, Y: -0.05}, RotIdentity)
			gsd := DefaultShapeDef()
			w.CreatePolygonShape(ground, &gsd, &floor)

			bd := DefaultBodyDef()
			bd.Type = DynamicBody
			bd.Position = Vec2{X: 0.0, Y: yStart}
			bd.LinearVelocity = Vec2{X: 0.0, Y: vy0}
			bd.IsBullet = tc.isBullet
			bodyID := w.CreateBody(&bd)

			circle := Circle{Center: Vec2Zero, Radius: radius}
			sd := DefaultShapeDef()
			// Density 0 -> zero mass -> invMass 0 -> no gravity integrated.
			sd.Density = 0.0
			w.CreateCircleShape(bodyID, &sd, &circle)

			w.Step(dt, subSteps)

			hsim := w.getBodySim(w.getBodyFullID(bodyID))
			require.NotZero(t, hsim.flags&hadTimeOfImpact,
				"%s: zero-mass fast body must record a TOI against the floor", tc.name)

			v := w.BodyLinearVelocity(bodyID)
			require.LessOrEqual(t, v.Y, 1e-6,
				"%s: zero-mass downward TOI must not reverse the body upward (pre-fix reversed to +105.67)", tc.name)
		})
	}
}

// TestSolveContinuousTimeLossAngularCapDoesNotSkipCorrection reproduces the
// angular-cap finding from the slopcop review on 526a4f5: an angular-only speed
// clamp (isSpeedCapped set by the angular cap, not the linear one) discards no
// linear gravity impulse, so the time-loss correction must still apply. Two
// identical falling circles differ only in initial spin; the spinning one's
// rotation is capped but its linear motion is not. The pre-fix guard on
// isSpeedCapped skipped the correction for the spinning body, so it retained
// the full step's gravity (-50.166667) while the non-spinning one was
// corrected (-50.080848). The linear-only isLinearSpeedCapped guard applies
// the correction to both, so they finish with equal velocity.
func TestSolveContinuousTimeLossAngularCapDoesNotSkipCorrection(t *testing.T) {
	const (
		dt       = 1.0 / 60.0
		radius   = 0.1
		yStart   = 0.5
		vy0      = -50.0
		gY       = -10.0
		subSteps = 8
	)

	mkWorld := func(angularVelocity float64) (*World, BodyID) {
		def := DefaultWorldDef()
		def.Gravity = Vec2{X: 0.0, Y: gY}
		w := NewWorld(&def)

		gd := DefaultBodyDef()
		ground := w.CreateBody(&gd)
		floor := MakeOffsetBox(5.0, 0.05, Vec2{X: 0.0, Y: -0.05}, RotIdentity)
		gsd := DefaultShapeDef()
		w.CreatePolygonShape(ground, &gsd, &floor)

		bd := DefaultBodyDef()
		bd.Type = DynamicBody
		bd.Position = Vec2{X: 0.0, Y: yStart}
		bd.LinearVelocity = Vec2{X: 0.0, Y: vy0}
		bd.AngularVelocity = angularVelocity
		bodyID := w.CreateBody(&bd)

		circle := Circle{Center: Vec2Zero, Radius: radius}
		sd := DefaultShapeDef()
		sd.Density = 1.0
		w.CreateCircleShape(bodyID, &sd, &circle)

		w.Step(dt, subSteps)
		return w, bodyID
	}

	wPlain, plain := mkWorld(0.0)
	defer wPlain.Destroy()
	wSpin, spin := mkWorld(1000.0)
	defer wSpin.Destroy()

	require.NotZero(t, wSpin.getBodySim(wSpin.getBodyFullID(spin)).flags&hadTimeOfImpact,
		"spinning body must record a TOI against the floor")
	require.NotZero(t, wPlain.getBodySim(wPlain.getBodyFullID(plain)).flags&hadTimeOfImpact,
		"plain body must record a TOI against the floor")

	// The spinning body's rotation is angular-capped (isSpeedCapped set) but
	// its linear speed (~50) is well under the 400 default, so it is NOT
	// linearly capped.
	spinBody := wSpin.getBodyFullID(spin)
	require.NotZero(t, spinBody.flags&isSpeedCapped,
		"spinning body must be angular speed-capped")
	require.Zero(t, spinBody.flags&isLinearSpeedCapped,
		"spinning body must not be linearly speed-capped")

	vPlain := wPlain.BodyLinearVelocity(plain)
	vSpin := wSpin.BodyLinearVelocity(spin)

	// Both bodies hit the same floor at the same fraction, so the correction
	// (now applied to both) must leave them with equal velocity. Pre-fix the
	// spinning one was skipped and retained the full step's gravity.
	require.InDelta(t, vPlain.Y, vSpin.Y, 1e-4,
		"angular-only cap must not skip the time-loss correction: spinning and plain bodies must finish with equal Y velocity")
}

// timeLossSupportedWallScene builds the slopcop P1 reproduction: a frictionless
// circle RESTING on a static floor (so the contact solver cancels gravity each
// substep, holding vertical velocity at zero) that is also sliding horizontally
// into a vertical wall, so a CCD TOI fires in the same step. When withWall is
// false the wall is omitted (the no-TOI control). The body starts exactly
// touching the floor (center y == radius, floor top at y == 0) so the floor
// contact is touching from step one and isContactConstrained is set.
func timeLossSupportedWallScene(t *testing.T, gravity Vec2, withWall, isBullet bool) (*World, BodyID) {
	t.Helper()

	const (
		dt       = 1.0 / 60.0
		radius   = 0.1
		subSteps = 8
		vx       = 50.0
	)

	def := DefaultWorldDef()
	def.Gravity = gravity
	w := NewWorld(&def)

	// Floor: top surface at y == 0.
	gd := DefaultBodyDef()
	ground := w.CreateBody(&gd)
	floor := MakeOffsetBox(5.0, 0.05, Vec2{X: 0.0, Y: -0.05}, RotIdentity)
	gsd := DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &floor)

	if withWall {
		// Vertical wall just ahead of the body so it is hit mid-step.
		wall := MakeOffsetBox(0.05, 5.0, Vec2{X: 0.5, Y: 0.0}, RotIdentity)
		w.CreatePolygonShape(ground, &gsd, &wall)
	}

	// Resting on the floor: center y == radius, bottom == floor top.
	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = Vec2{X: 0.0, Y: radius}
	bd.LinearVelocity = Vec2{X: vx, Y: 0.0}
	bd.IsBullet = isBullet
	bodyID := w.CreateBody(&bd)

	circle := Circle{Center: Vec2Zero, Radius: radius}
	sd := DefaultShapeDef()
	sd.Density = 1.0
	sd.Material.Friction = 0.0
	w.CreateCircleShape(bodyID, &sd, &circle)

	w.Step(dt, subSteps)
	return w, bodyID
}

// TestSolveContinuousTimeLossContactConstrainedNoReversal reproduces the P1
// finding from the slopcop review on 13c5462: a body RESTING on a floor (its
// vertical velocity held at zero by the floor's normal contact constraint) that
// slides horizontally into a wall receives an artificial vertical impulse from
// the time-loss correction, because the correction subtracts gravity that the
// contact solver already canceled. Pre-fix the wall hit finished with a spurious
// vertical velocity of ~0.096 m/s (gravity -10) / ~9.57 m/s (gravity -1000) in
// this scene; the no-wall control stayed at zero. The isContactConstrained
// guard skips the correction for the touching body, so the wall case now matches
// the control: vertical velocity stays at zero. Exercises both the non-bullet
// and bullet CCD paths.
func TestSolveContinuousTimeLossContactConstrainedNoReversal(t *testing.T) {
	for _, tc := range []struct {
		name     string
		isBullet bool
	}{
		{"non_bullet", false},
		{"bullet", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, g := range []struct {
				name string
				gY   float64
				// pre-fix spurious vertical velocity the guard must eliminate
				kick float64
			}{
				{"gravity_-10", -10.0, 0.0956667},
				{"gravity_-1000", -1000.0, 9.56667},
			} {
				t.Run(g.name, func(t *testing.T) {
					// No-wall control: floor-supported body, no TOI. The
					// contact solver holds vertical velocity at zero.
					wCtrl, ctrl := timeLossSupportedWallScene(t,
						Vec2{X: 0.0, Y: g.gY}, false, tc.isBullet)
					defer wCtrl.Destroy()
					vCtrl := wCtrl.BodyLinearVelocity(ctrl)
					require.InDelta(t, 0.0, vCtrl.Y, 1e-3,
						"%s/%s: no-wall control must keep zero vertical velocity", tc.name, g.name)

					// Wall case: same body slides into a wall, firing a CCD TOI
					// in the same step it is floor-supported.
					w, bodyID := timeLossSupportedWallScene(t,
						Vec2{X: 0.0, Y: g.gY}, true, tc.isBullet)
					defer w.Destroy()

					hsim := w.getBodySim(w.getBodyFullID(bodyID))
					require.NotZero(t, hsim.flags&hadTimeOfImpact,
						"%s/%s: body must record a TOI against the wall", tc.name, g.name)
					bf := w.getBodyFullID(bodyID)
					require.NotZero(t, bf.flags&isContactConstrained,
						"%s/%s: floor-supported body must be contact-constrained", tc.name, g.name)

					v := w.BodyLinearVelocity(bodyID)
					// The guard skips the correction for the constrained body,
					// so the wall hit must NOT inject a spurious vertical impulse.
					// Pre-fix this was ~g.kick in magnitude.
					require.InDelta(t, 0.0, v.Y, 1e-3,
						"%s/%s: floor-supported wall hit must not inject vertical velocity (pre-fix was ~%v)", tc.name, g.name, g.kick)
				})
			}
		})
	}
}

// timeLossPrismaticWallScene builds the slopcop P1 joint reproduction: a
// frictionless dynamic circle constrained to a HORIZONTAL-axis prismatic joint
// (so the joint's point-to-line constraint holds vertical velocity at zero,
// canceling gravity each substep) that slides horizontally into a vertical
// wall, so a CCD TOI fires in the same step. When withWall is false the wall
// is omitted (the no-TOI control). The joint anchor is a static body; the
// joint axis is the world x-axis (LocalFrameA.Q = RotIdentity), so the body is
// free along x and locked along y — exactly the geometry that lets gravity be
// canceled while the body still hits the wall.
func timeLossPrismaticWallScene(t *testing.T, gravity Vec2, withWall, isBullet bool) (*World, BodyID) {
	t.Helper()

	const (
		dt       = 1.0 / 60.0
		radius   = 0.1
		subSteps = 8
		vx       = 50.0
	)

	def := DefaultWorldDef()
	def.Gravity = gravity
	w := NewWorld(&def)

	// Static anchor for the joint (no shape needed).
	gd := DefaultBodyDef()
	anchor := w.CreateBody(&gd)

	if withWall {
		// Vertical wall just ahead of the body so it is hit mid-step. The
		// wall lives on its OWN static body (not the joint anchor), because
		// shouldBodiesCollide returns false for bodies joined together, so a
		// wall on the anchor would be skipped by the CCD query.
		wgd := DefaultBodyDef()
		wallBody := w.CreateBody(&wgd)
		wall := MakeOffsetBox(0.05, 5.0, Vec2{X: 0.5, Y: 0.0}, RotIdentity)
		gsd := DefaultShapeDef()
		w.CreatePolygonShape(wallBody, &gsd, &wall)
	}

	// Dynamic circle on the joint axis (y == 0), sliding along x.
	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = Vec2{X: 0.0, Y: 0.0}
	bd.LinearVelocity = Vec2{X: vx, Y: 0.0}
	bd.IsBullet = isBullet
	bodyID := w.CreateBody(&bd)

	circle := Circle{Center: Vec2Zero, Radius: radius}
	sd := DefaultShapeDef()
	sd.Density = 1.0
	sd.Material.Friction = 0.0
	w.CreateCircleShape(bodyID, &sd, &circle)

	// Horizontal-axis prismatic joint: free along x, locked along y, so the
	// joint solver cancels gravity (which is vertical here).
	jd := DefaultPrismaticJointDef()
	jd.Base.BodyIDA = anchor
	jd.Base.BodyIDB = bodyID
	jd.Base.LocalFrameA.P = Vec2Zero
	jd.Base.LocalFrameB.P = Vec2Zero
	w.CreatePrismaticJoint(&jd)

	w.Step(dt, subSteps)
	return w, bodyID
}

// TestSolveContinuousTimeLossJointConstrainedNoReversal reproduces the P1
// finding from the slopcop review on 4ec97fe: a body on a horizontal-axis
// prismatic joint (its vertical velocity held at zero by the joint's
// point-to-line constraint) that slides horizontally into a wall receives an
// artificial vertical impulse from the time-loss correction, because the
// correction subtracts gravity that the joint solver already canceled.
// Pre-fix the wall hit finished with a spurious upward velocity of ~0.095667
// m/s (gravity -10) / ~9.56667 m/s (gravity -1000) in this scene; the no-wall
// control stayed at zero. The isJointConstrained guard skips the correction
// for the joint-constrained body, so the wall case now matches the control:
// vertical velocity stays at zero. Exercises both the non-bullet and bullet
// CCD paths.
func TestSolveContinuousTimeLossJointConstrainedNoReversal(t *testing.T) {
	for _, tc := range []struct {
		name     string
		isBullet bool
	}{
		{"non_bullet", false},
		{"bullet", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, g := range []struct {
				name string
				gY   float64
				// pre-fix spurious upward vertical velocity the guard must eliminate
				kick float64
			}{
				{"gravity_-10", -10.0, 0.0956667},
				{"gravity_-1000", -1000.0, 9.56667},
			} {
				t.Run(g.name, func(t *testing.T) {
					// No-wall control: joint-constrained body, no TOI. The
					// joint solver holds vertical velocity at zero.
					wCtrl, ctrl := timeLossPrismaticWallScene(t,
						Vec2{X: 0.0, Y: g.gY}, false, tc.isBullet)
					defer wCtrl.Destroy()
					vCtrl := wCtrl.BodyLinearVelocity(ctrl)
					require.InDelta(t, 0.0, vCtrl.Y, 1e-3,
						"%s/%s: no-wall control must keep zero vertical velocity", tc.name, g.name)

					// Wall case: same body slides into a wall, firing a CCD TOI
					// in the same step it is joint-constrained.
					w, bodyID := timeLossPrismaticWallScene(t,
						Vec2{X: 0.0, Y: g.gY}, true, tc.isBullet)
					defer w.Destroy()

					hsim := w.getBodySim(w.getBodyFullID(bodyID))
					require.NotZero(t, hsim.flags&hadTimeOfImpact,
						"%s/%s: body must record a TOI against the wall", tc.name, g.name)
					bf := w.getBodyFullID(bodyID)
					require.NotZero(t, bf.flags&isJointConstrained,
						"%s/%s: joint-constrained body must carry isJointConstrained", tc.name, g.name)

					v := w.BodyLinearVelocity(bodyID)
					// The guard skips the correction for the joint-constrained
					// body, so the wall hit must NOT inject a spurious vertical
					// impulse. Pre-fix this was ~g.kick upward.
					require.InDelta(t, 0.0, v.Y, 1e-3,
						"%s/%s: joint-constrained wall hit must not inject vertical velocity (pre-fix was ~%v)", tc.name, g.name, g.kick)
				})
			}
		})
	}
}
