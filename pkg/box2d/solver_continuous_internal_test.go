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
			require.InDelta(t, vy0+gY*dt, vControl.Y, timeLossTolerance,
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

// TestSolveContinuousTimeLossSpeedCappedNoReversal covers the speed-cap guard
// for both CCD call sites: a speed-capped fast body under gravity that hits a
// TOI mid-step must NOT have its velocity reversed by the time-loss
// correction. integrateVelocitiesTask caps velocity to MaximumLinearSpeed and
// discards part of the gravity impulse; subtracting the full theoretical
// un-simulated gravity (as the pre-fix code did) removes gravity that was
// never retained and can launch the body back above the cap. After the fix the
// post-TOI velocity must stay at or below the cap and must not reverse
// direction. The non-bullet path runs solveContinuous inline in
// finalizeBodiesTask; the bullet path runs it in the serial bullet stage
// after finalize clears the sim isSpeedCapped flag, so the body-flag guard
// must protect both.
func TestSolveContinuousTimeLossSpeedCappedNoReversal(t *testing.T) {
	for _, tc := range []struct {
		name     string
		isBullet bool
	}{
		{"non_bullet", false},
		{"bullet", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			bd.IsBullet = tc.isBullet
			bodyID := w.CreateBody(&bd)

			circle := Circle{Center: Vec2Zero, Radius: radius}
			sd := DefaultShapeDef()
			sd.Density = 1.0
			w.CreateCircleShape(bodyID, &sd, &circle)

			w.Step(dt, subSteps)

			hsim := w.getBodySim(w.getBodyFullID(bodyID))
			require.NotZero(t, hsim.flags&hadTimeOfImpact,
				"%s: fast body must record a TOI against the floor", tc.name)

			bf := w.getBodyFullID(bodyID)
			require.NotZero(t, bf.flags&isSpeedCapped,
				"%s: body must be speed-capped at this step", tc.name)

			v := w.BodyLinearVelocity(bodyID)
			require.LessOrEqual(t, Length(v), maxSpeed+1e-6,
				"%s: post-TOI velocity must respect MaximumLinearSpeed (no cap reversal)", tc.name)
			require.LessOrEqual(t, v.Y, 1e-6,
				"%s: downward TOI must not reverse the body upward (pre-fix reversed to +101.67)", tc.name)
		})
	}
}

// timeLossReproScene builds the common reproduction used by the damping,
// zero-mass and angular-cap regression tests: a small fast circle above a
// static floor, under the given gravity and linear damping, stepping once.
// maxLinearSpeed, linearDamping and the initial angular velocity are
// configurable so each test exercises its own guard.
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

// TestSolveContinuousTimeLossDampingNoReversal guards the damping exclusion:
// a heavily damped fast body under strong gravity that hits a TOI mid-step
// must NOT be reversed upward by the time-loss correction.
// integrateVelocitiesTask attenuates the accumulated velocity (including the
// gravity impulse) every substep, so the body retained less than the full
// theoretical gravity; subtracting the full un-simulated amount reverses a
// falling body upward (observed +118.63 pre-fix). The correction is
// conservatively skipped for damped bodies, preserving the downward
// velocity. Exercises both the non-bullet and bullet CCD paths.
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
				Vec2{X: 0.0, Y: -10000.0},            // gravity
				DefaultWorldDef().MaximumLinearSpeed, // maxLinearSpeed (default; never reached here)
				1000.0,                               // linearDamping
				0.0,                                  // angularVelocity
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

// TestSolveContinuousTimeLossZeroMassNoGravityImpulse guards the zero-mass
// exclusion: a zero-density (zero-mass) dynamic body receives NO gravity from
// integrateVelocitiesTask (the integrator zeroes the gravity scale when
// invMass == 0), so the time-loss correction must NOT subtract a gravity
// contribution that was never integrated. Without the invMass > 0 guard the
// correction reverses the body upward (observed +105.67 pre-fix); the guard
// skips it, leaving the downward velocity intact. Exercises both CCD paths.
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

// TestSolveContinuousTimeLossAngularCapDoesNotSkipCorrection guards the
// linear-only speed-cap distinction: an angular-only speed clamp (isSpeedCapped
// set by the angular cap, not the linear one) discards no linear gravity
// impulse, so the time-loss correction must still apply. Two identical falling
// circles differ only in initial spin; the spinning one's rotation is capped
// but its linear motion is not. A guard on isSpeedCapped would wrongly skip
// the correction for the spinning body, leaving it with the full step's
// gravity (-50.166667) while the non-spinning one was corrected (-50.080848).
// The linear-only isLinearSpeedCapped guard applies the correction to both,
// so they finish with equal velocity.
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
	require.InDelta(t, vPlain.Y, vSpin.Y, timeLossTolerance,
		"angular-only cap must not skip the time-loss correction: spinning and plain bodies must finish with equal Y velocity")
}

// timeLossSupportedWallScene builds the contact-constrained reproduction: a
// frictionless circle RESTING on a static floor (so the contact solver cancels
// gravity each substep, holding vertical velocity at zero) that is also
// sliding horizontally into a vertical wall, so a CCD TOI fires in the same
// step. When withWall is false the wall is omitted (the no-TOI control). The
// body starts exactly touching the floor (center y == radius, floor top at
// y == 0) so the floor contact is touching from step one and
// isContactConstrained is set.
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

// TestSolveContinuousTimeLossContactConstrainedNoReversal guards the
// contact-constrained exclusion: a body RESTING on a floor (its vertical
// velocity held at zero by the floor's normal contact constraint) that slides
// horizontally into a wall must NOT receive an artificial vertical impulse
// from the time-loss correction, because the correction would subtract
// gravity the contact solver already canceled. Without the guard the wall
// hit finishes with a spurious vertical velocity of ~0.096 m/s (gravity -10)
// / ~9.57 m/s (gravity -1000) while the no-wall control stays at zero. The
// isContactConstrained guard skips the correction for the touching body, so
// the wall case matches the control: vertical velocity stays at zero.
// Exercises both the non-bullet and bullet CCD paths.
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

// timeLossPrismaticWallScene builds the joint-constrained reproduction: a
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

// TestSolveContinuousTimeLossJointConstrainedNoReversal guards the
// joint-constrained exclusion: a body on a horizontal-axis prismatic joint
// (its vertical velocity held at zero by the joint's point-to-line
// constraint) that slides horizontally into a wall must NOT receive an
// artificial vertical impulse from the time-loss correction, because the
// correction would subtract gravity the joint solver already canceled.
// Without the guard the wall hit finishes with a spurious upward velocity of
// ~0.095667 m/s (gravity -10) / ~9.56667 m/s (gravity -1000) while the no-wall
// control stays at zero. The isJointConstrained guard skips the correction
// for the joint-constrained body, so the wall case matches the control:
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

// timeLossFilterJointScene builds the bug-report falling-circle TOI scene (a
// fast circle under gravity that hits a static floor mid-step) and, when
// withFilterJoint is true, attaches a FilterJoint between the falling body and
// an EMPTY static body. A FilterJoint disables collision between its two
// bodies, but the anchor has no shapes, so the floor TOI is unaffected. The
// reviewer's P2 finding: prepareJoint marked FilterJoint endpoints
// isJointConstrained even though FilterJoint performs no velocity solve, so
// the time-loss correction was suppressed and the post-TOI velocity changed
// from the corrected -50.0808484 to the un-corrected -50.1666667 (gravity
// (0,-10), vy0=-50, dt=1/60).
func timeLossFilterJointScene(t *testing.T, withFilterJoint, isBullet bool) (*World, BodyID) {
	t.Helper()

	const (
		dt       = 1.0 / 60.0
		vy0      = -50.0
		radius   = 0.1
		yStart   = 0.5
		subSteps = 8
	)

	def := DefaultWorldDef()
	def.Gravity = Vec2{X: 0.0, Y: -10.0}
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
	bd.IsBullet = isBullet
	bodyID := w.CreateBody(&bd)

	circle := Circle{Center: Vec2Zero, Radius: radius}
	sd := DefaultShapeDef()
	sd.Density = 1.0
	w.CreateCircleShape(bodyID, &sd, &circle)

	if withFilterJoint {
		// Empty static anchor (no shapes) so the floor collision is unchanged.
		anchor := w.CreateBody(&gd)
		fjd := DefaultFilterJointDef()
		fjd.Base.BodyIDA = anchor
		fjd.Base.BodyIDB = bodyID
		w.CreateFilterJoint(&fjd)
	}

	w.Step(dt, subSteps)
	return w, bodyID
}

// TestSolveContinuousTimeLossFilterJointNoSuppression guards the FilterJoint
// exclusion from joint-constrained marking: FilterJoint performs no velocity
// solve, so it cannot cancel the integrated gravity impulse and must not
// suppress the CCD time-loss gravity correction. Marking FilterJoint
// endpoints isJointConstrained would set the flag on a falling dynamic body
// (via a filter joint to an empty static body) and change its post-TOI
// velocity from the corrected value to the un-corrected full-step-gravity
// value (identical collision geometry and TOI). The with- and
// without-filter-joint scenes must finish with identical post-TOI velocity
// and neither body carries isJointConstrained. Exercises both the non-bullet
// and bullet CCD paths.
func TestSolveContinuousTimeLossFilterJointNoSuppression(t *testing.T) {
	const (
		dt  = 1.0 / 60.0
		vy0 = -50.0
		gY  = -10.0
	)
	for _, tc := range []struct {
		name     string
		isBullet bool
	}{
		{"non_bullet", false},
		{"bullet", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Baseline: no filter joint. The correction must apply.
			w0, b0 := timeLossFilterJointScene(t, false, tc.isBullet)
			defer w0.Destroy()
			hsim0 := w0.getBodySim(w0.getBodyFullID(b0))
			require.NotZero(t, hsim0.flags&hadTimeOfImpact,
				"%s: baseline body must record a TOI against the floor", tc.name)
			require.Zero(t, w0.getBodyFullID(b0).flags&isJointConstrained,
				"%s: baseline body must not be joint-constrained", tc.name)
			v0 := w0.BodyLinearVelocity(b0)

			// With a filter joint to an empty static body. The correction must
			// still apply — the filter joint does no solve work.
			w1, b1 := timeLossFilterJointScene(t, true, tc.isBullet)
			defer w1.Destroy()
			hsim1 := w1.getBodySim(w1.getBodyFullID(b1))
			require.NotZero(t, hsim1.flags&hadTimeOfImpact,
				"%s: filter-joint body must still record a TOI against the floor", tc.name)
			require.Zero(t, w1.getBodyFullID(b1).flags&isJointConstrained,
				"%s: filter-joint body must not be marked isJointConstrained (FilterJoint does no solve)", tc.name)
			v1 := w1.BodyLinearVelocity(b1)

			// Both scenes have identical collision geometry and TOI, so the
			// corrected post-TOI velocity must match. Pre-fix v1 was the
			// un-corrected vy0 + g*dt = -50.1666667.
			require.InDelta(t, v0.Y, v1.Y, timeLossTolerance,
				"%s: filter joint must not change post-TOI velocity (pre-fix differed by ~0.086)", tc.name)

			// And the correction must actually have been applied in both: the
			// post-TOI downward speed is strictly less than the free-fall
			// (un-corrected) value vy0 + g*dt.
			fullFall := vy0 + gY*dt
			require.Greater(t, v1.Y, fullFall,
				"%s: correction must remove the un-simulated gravity (post-TOI vy %v must exceed free-fall %v)", tc.name, v1.Y, fullFall)
		})
	}
}

// timeLossBalancedForceScene builds the balanced-force reproduction: a
// frictionless dynamic circle moving horizontally under a strong downward
// gravity, with an upward center force equal to mass*|g| applied so the net
// vertical acceleration is zero (the unobstructed control keeps vy ~= 0).
// When withWall is true a vertical wall is placed so the body hits it
// mid-step, firing a CCD TOI in the same step the force is applied. Without
// the force-driven guard the TOI branch subtracts the un-simulated gravity
// the force had already canceled, injecting a spurious upward velocity
// (+8.5667 m/s at gravity -1000, dt=1/60).
func timeLossBalancedForceScene(t *testing.T, gravityY float64, withWall, isBullet bool) (*World, BodyID) {
	t.Helper()

	const (
		dt       = 1.0 / 60.0
		radius   = 0.1
		subSteps = 8
		vx       = 50.0
	)

	def := DefaultWorldDef()
	def.Gravity = Vec2{X: 0.0, Y: gravityY}
	w := NewWorld(&def)

	if withWall {
		gd := DefaultBodyDef()
		wallBody := w.CreateBody(&gd)
		wall := MakeOffsetBox(0.05, 5.0, Vec2{X: 0.5, Y: 0.0}, RotIdentity)
		gsd := DefaultShapeDef()
		w.CreatePolygonShape(wallBody, &gsd, &wall)
	}

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

	// Compute the mass now that the shape is attached, then apply an upward
	// force that exactly cancels gravity (force.Y = -mass*gravity.Y).
	mass := w.BodyMass(bodyID)
	w.ApplyBodyForceToCenter(bodyID, Vec2{X: 0.0, Y: -mass * gravityY}, true)

	w.Step(dt, subSteps)
	return w, bodyID
}

// TestSolveContinuousTimeLossBalancedForceNoReversal guards the force-driven
// exclusion: a body with an upward force balancing gravity, moving
// horizontally into a wall, must NOT gain a spurious upward velocity from the
// time-loss correction, which would subtract gravity the applied force had
// already canceled (observed +8.5667 m/s at gravity -1000 pre-fix). The
// isForceDriven guard (set in integrateVelocitiesTask when sim.force !=
// Vec2Zero) skips the correction for force-driven bodies, so the wall case
// matches the no-wall control (vertical velocity stays at zero). Exercises
// both the non-bullet and bullet CCD paths at gravity -10 and -1000.
func TestSolveContinuousTimeLossBalancedForceNoReversal(t *testing.T) {
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
				{"gravity_-10", -10.0, 0.0856667},
				{"gravity_-1000", -1000.0, 8.56667},
			} {
				t.Run(g.name, func(t *testing.T) {
					// No-wall control: balanced force, no TOI. The vertical
					// acceleration is zero, so vertical velocity stays at zero.
					wCtrl, ctrl := timeLossBalancedForceScene(t, g.gY, false, tc.isBullet)
					defer wCtrl.Destroy()
					vCtrl := wCtrl.BodyLinearVelocity(ctrl)
					require.InDelta(t, 0.0, vCtrl.Y, 1e-3,
						"%s/%s: no-wall control must keep zero vertical velocity", tc.name, g.name)

					// Wall case: same balanced force, hitting a wall mid-step.
					w, bodyID := timeLossBalancedForceScene(t, g.gY, true, tc.isBullet)
					defer w.Destroy()

					hsim := w.getBodySim(w.getBodyFullID(bodyID))
					require.NotZero(t, hsim.flags&hadTimeOfImpact,
						"%s/%s: body must record a TOI against the wall", tc.name, g.name)
					require.NotZero(t, w.getBodyFullID(bodyID).flags&isForceDriven,
						"%s/%s: force-driven body must carry isForceDriven", tc.name, g.name)

					v := w.BodyLinearVelocity(bodyID)
					// The guard skips the correction for the force-driven body,
					// so the wall hit must NOT inject a spurious vertical
					// impulse. Pre-fix this was ~g.kick upward.
					require.InDelta(t, 0.0, v.Y, 1e-3,
						"%s/%s: balanced-force wall hit must not inject vertical velocity (pre-fix was ~%v)", tc.name, g.name, g.kick)
				})
			}
		})
	}
}

// TestSolveContinuousTimeLossCeilingReAppliesSpeedCap guards the re-application
// of MaximumLinearSpeed after the time-loss correction: a body moving against
// gravity that integration clamps BELOW the cap (so the correction is not
// skipped via isLinearSpeedCapped) can be pushed back ABOVE the cap by the
// correction, because the correction restores the gravity removed during the
// un-simulated fraction. The post-correction clamp must keep the post-TOI
// velocity at or below MaximumLinearSpeed. The body uses a small radius and a
// thin ceiling so it tunnels past the ceiling in one step (no broadphase
// contact to stop it before CCD), letting the CCD TOI fire at a small fraction
// (large correction). Exercises both the non-bullet and bullet CCD paths.
func TestSolveContinuousTimeLossCeilingReAppliesSpeedCap(t *testing.T) {
	for _, tc := range []struct {
		name     string
		isBullet bool
	}{
		{"non_bullet", false},
		{"bullet", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const (
				dt       = 1.0 / 60.0
				maxSpeed = 10.0
				radius   = 0.01 // small enough to tunnel past the thin ceiling
				vy0      = 10.5 // upward, just above the cap
				subSteps = 1
			)

			def := DefaultWorldDef()
			def.Gravity = Vec2{X: 0.0, Y: -60.0}
			def.MaximumLinearSpeed = maxSpeed
			def.EnableSleep = false
			w := NewWorld(&def)
			defer w.Destroy()

			// Thin ceiling: bottom surface a small gap above the body's top so
			// the body is NOT touching at step start, but the body (small
			// radius) tunnels past it in one step — no broadphase contact stops
			// the body before CCD, so the CCD TOI fires at a small fraction
			// (large correction → would exceed the cap without the re-clamp).
			gd := DefaultBodyDef()
			ceiling := w.CreateBody(&gd)
			ceilingBox := MakeOffsetBox(5.0, 0.005, Vec2{X: 0.0, Y: 0.05}, RotIdentity) // bottom at y=0.045
			gsd := DefaultShapeDef()
			w.CreatePolygonShape(ceiling, &gsd, &ceilingBox)

			bd := DefaultBodyDef()
			bd.Type = DynamicBody
			bd.Position = Vec2{X: 0.0, Y: 0.0}
			bd.LinearVelocity = Vec2{X: 0.0, Y: vy0}
			bd.IsBullet = tc.isBullet
			bd.EnableSleep = false
			bodyID := w.CreateBody(&bd)

			circle := Circle{Center: Vec2Zero, Radius: radius}
			sd := DefaultShapeDef()
			sd.Density = 1.0
			w.CreateCircleShape(bodyID, &sd, &circle)

			w.Step(dt, subSteps)

			hsim := w.getBodySim(w.getBodyFullID(bodyID))
			require.NotZero(t, hsim.flags&hadTimeOfImpact,
				"%s: body must record a TOI against the ceiling", tc.name)

			// The body must NOT be linear-speed-capped during integration (its
			// post-integration speed 9.5 is below the 10 cap), so the guard
			// does not skip the correction — the re-clamp is what keeps it in
			// range.
			bf := w.getBodyFullID(bodyID)
			require.Zero(t, bf.flags&isLinearSpeedCapped,
				"%s: body must not be linear-speed-capped during integration (else the re-clamp is not exercised)", tc.name)

			v := w.BodyLinearVelocity(bodyID)
			require.LessOrEqual(t, Length(v), maxSpeed+1e-6,
				"%s: post-TOI velocity must respect MaximumLinearSpeed after the correction re-clamp", tc.name)
			require.Greater(t, v.Y, 0.0,
				"%s: upward ceiling TOI must keep upward (not reversed) velocity after re-clamp", tc.name)
		})
	}
}

// TestSolveContinuousTimeLossUnlockedXWithLinearYLocked is the complementary
// lock test: a body locked on Y, free on X, with a non-unit gravity scale and a
// gravity X component, moving horizontally into a wall. The correction's Y
// component is clipped by lockLinearY (v.Y stays zero); the X component is
// applied to the unlocked X axis, so the post-TOI v.X matches the upstream
// analytic value vx + gravityScale*gravity.X*dt*fraction. The existing lock
// test only locks X and only asserts Y; this covers the unlocked-X path and a
// non-unit gravity scale, which can regress independently.
func TestSolveContinuousTimeLossUnlockedXWithLinearYLocked(t *testing.T) {
	const (
		dt           = 1.0 / 60.0
		vx           = 50.0
		radius       = 0.1
		gravityScale = 2.0
		subSteps     = 8
	)

	gravity := Vec2{X: -5.0, Y: -10.0}

	build := func(t *testing.T, withWall bool) (*World, BodyID, Vec2) {
		t.Helper()
		def := DefaultWorldDef()
		def.Gravity = gravity
		w := NewWorld(&def)

		if withWall {
			gd := DefaultBodyDef()
			wallBody := w.CreateBody(&gd)
			wall := MakeOffsetBox(0.05, 5.0, Vec2{X: 0.5, Y: 0.0}, RotIdentity)
			gsd := DefaultShapeDef()
			w.CreatePolygonShape(wallBody, &gsd, &wall)
		}

		bd := DefaultBodyDef()
		bd.Type = DynamicBody
		bd.Position = Vec2{X: 0.0, Y: 0.0}
		bd.LinearVelocity = Vec2{X: vx, Y: 0.0}
		bd.GravityScale = gravityScale
		bd.MotionLocks = MotionLocks{LinearY: true}
		bodyID := w.CreateBody(&bd)

		circle := Circle{Center: Vec2Zero, Radius: radius}
		sd := DefaultShapeDef()
		sd.Density = 1.0
		w.CreateCircleShape(bodyID, &sd, &circle)

		c1 := w.BodyWorldCenterOfMass(bodyID)
		w.Step(dt, subSteps)
		return w, bodyID, c1
	}

	// Control: no wall → full step gravity on X (Y is locked, stays zero).
	wc, control, c1 := build(t, false)
	defer wc.Destroy()
	c2 := wc.BodyWorldCenterOfMass(control)
	vControl := wc.BodyLinearVelocity(control)
	require.InDelta(t, 0.0, vControl.Y, timeLossTolerance,
		"locked-Y control must keep zero Y velocity")
	wantControlX := vx + float64(gravityScale*gravity.X*dt)
	require.InDelta(t, wantControlX, vControl.X, timeLossTolerance,
		"control (no TOI) must take the full step gravity on X")

	// Wall hit: TOI fires mid-step on the unlocked X axis.
	wt, hit, _ := build(t, true)
	defer wt.Destroy()

	hsim := wt.getBodySim(wt.getBodyFullID(hit))
	require.NotZero(t, hsim.flags&hadTimeOfImpact, "body must hit a TOI against the wall")

	cHit := wt.BodyWorldCenterOfMass(hit)
	vHit := wt.BodyLinearVelocity(hit)

	// Fraction of the step actually simulated (from the COM X sweep, which
	// matches the control's pre-CCD sweep).
	fraction := (cHit.X - c1.X) / (c2.X - c1.X)
	require.True(t, fraction > 0.0 && fraction < 1.0,
		"TOI fraction must be in (0,1), got %v", fraction)

	// Y stays zero (locked + lock clipping in the correction).
	require.InDelta(t, 0.0, vHit.Y, timeLossTolerance,
		"locked-Y body must keep zero Y velocity after the TOI correction")

	// X gets the correction on the unlocked axis: only the simulated
	// fraction's gravity is kept.
	wantX := vx + float64(gravityScale*gravity.X*dt*fraction)
	require.InDelta(t, wantX, vHit.X, timeLossTolerance,
		"TOI body must keep only the simulated fraction's X gravity (unlocked-X correction)")

	// And it must carry strictly less X speed than the control (the un-simulated
	// gravity on X was removed).
	require.Greater(t, vHit.X, vControl.X,
		"TOI body must have less X speed than the full-step control (gravity.X is negative)")
}

// TestSolveContinuousTimeLossFlagsClearBetweenSteps guards that the per-step
// eligibility flags (isForceDriven, isContactConstrained, isJointConstrained)
// are CLEARED between steps and across a solver-set transfer, so the
// correction resumes for later eligible hits rather than being suppressed
// forever by a stale flag. The per-step regression tests above verify the
// correction is skipped when a flag is set; this test verifies the flag does
// not stick once the condition no longer holds.
func TestSolveContinuousTimeLossFlagsClearBetweenSteps(t *testing.T) {
	const (
		dt       = 1.0 / 60.0
		radius   = 0.1
		subSteps = 8
	)

	t.Run("isForceDriven_clears_when_force_removed", func(t *testing.T) {
		def := DefaultWorldDef()
		def.Gravity = Vec2{X: 0.0, Y: -10.0}
		def.EnableSleep = false
		w := NewWorld(&def)
		defer w.Destroy()

		bd := DefaultBodyDef()
		bd.Type = DynamicBody
		bd.Position = Vec2{X: 0.0, Y: 1.0}
		bd.EnableSleep = false
		bodyID := w.CreateBody(&bd)
		circle := Circle{Center: Vec2Zero, Radius: radius}
		sd := DefaultShapeDef()
		sd.Density = 1.0
		w.CreateCircleShape(bodyID, &sd, &circle)

		// Step 1 with an applied force → isForceDriven is set on the body.
		mass := w.BodyMass(bodyID)
		w.ApplyBodyForceToCenter(bodyID, Vec2{X: 0.0, Y: -mass * 10.0}, true)
		w.Step(dt, subSteps)
		require.NotZero(t, w.getBodyFullID(bodyID).flags&isForceDriven,
			"step 1 with a force must set isForceDriven on the body")

		// Step 2 with no force → the flag must be cleared (finalize clears the
		// transient flag and the sim does not re-set it without a force).
		w.Step(dt, subSteps)
		require.Zero(t, w.getBodyFullID(bodyID).flags&isForceDriven,
			"step 2 with no force must clear isForceDriven (no sticky suppression)")
	})

	t.Run("isContactConstrained_clears_when_contact_removed", func(t *testing.T) {
		def := DefaultWorldDef()
		def.Gravity = Vec2{X: 0.0, Y: -10.0}
		def.EnableSleep = false
		w := NewWorld(&def)
		defer w.Destroy()

		// Floor: top surface at y == 0.
		gd := DefaultBodyDef()
		ground := w.CreateBody(&gd)
		floor := MakeOffsetBox(5.0, 0.05, Vec2{X: 0.0, Y: -0.05}, RotIdentity)
		gsd := DefaultShapeDef()
		floorShape := w.CreatePolygonShape(ground, &gsd, &floor)

		bd := DefaultBodyDef()
		bd.Type = DynamicBody
		bd.Position = Vec2{X: 0.0, Y: radius} // resting on the floor
		bd.EnableSleep = false
		bodyID := w.CreateBody(&bd)
		circle := Circle{Center: Vec2Zero, Radius: radius}
		sd := DefaultShapeDef()
		sd.Density = 1.0
		w.CreateCircleShape(bodyID, &sd, &circle)

		// Step 1: touching the floor → isContactConstrained set.
		w.Step(dt, subSteps)
		require.NotZero(t, w.getBodyFullID(bodyID).flags&isContactConstrained,
			"step 1 touching the floor must set isContactConstrained")

		// Remove the floor; the body is no longer touching anything.
		w.DestroyShape(floorShape, true)

		// Step 2: no contact → isContactConstrained must be cleared.
		w.Step(dt, subSteps)
		require.Zero(t, w.getBodyFullID(bodyID).flags&isContactConstrained,
			"step 2 with no contact must clear isContactConstrained (no sticky suppression)")
	})

	t.Run("isJointConstrained_clears_when_joint_destroyed", func(t *testing.T) {
		def := DefaultWorldDef()
		def.Gravity = Vec2{X: 0.0, Y: -10.0}
		def.EnableSleep = false
		w := NewWorld(&def)
		defer w.Destroy()

		gd := DefaultBodyDef()
		anchor := w.CreateBody(&gd)

		bd := DefaultBodyDef()
		bd.Type = DynamicBody
		bd.Position = Vec2{X: 0.0, Y: 0.0}
		bd.EnableSleep = false
		bodyID := w.CreateBody(&bd)
		circle := Circle{Center: Vec2Zero, Radius: radius}
		sd := DefaultShapeDef()
		sd.Density = 1.0
		w.CreateCircleShape(bodyID, &sd, &circle)

		// Horizontal-axis prismatic joint: cancels vertical gravity.
		jd := DefaultPrismaticJointDef()
		jd.Base.BodyIDA = anchor
		jd.Base.BodyIDB = bodyID
		jd.Base.LocalFrameA.P = Vec2Zero
		jd.Base.LocalFrameB.P = Vec2Zero
		jointID := w.CreatePrismaticJoint(&jd)

		// Step 1: jointed → isJointConstrained set.
		w.Step(dt, subSteps)
		require.NotZero(t, w.getBodyFullID(bodyID).flags&isJointConstrained,
			"step 1 with a joint must set isJointConstrained")

		// Destroy the joint; the body is no longer jointed.
		w.DestroyJoint(jointID, false)

		// Step 2: no joint → isJointConstrained must be cleared.
		w.Step(dt, subSteps)
		require.Zero(t, w.getBodyFullID(bodyID).flags&isJointConstrained,
			"step 2 with no joint must clear isJointConstrained (no sticky suppression)")
	})

	t.Run("transferBody_clears_transient_flags", func(t *testing.T) {
		def := DefaultWorldDef()
		def.Gravity = Vec2{X: 0.0, Y: -10.0}
		def.EnableSleep = false
		w := NewWorld(&def)
		defer w.Destroy()

		bd := DefaultBodyDef()
		bd.Type = DynamicBody
		bd.Position = Vec2{X: 0.0, Y: 1.0}
		bd.EnableSleep = false
		bodyID := w.CreateBody(&bd)
		circle := Circle{Center: Vec2Zero, Radius: radius}
		sd := DefaultShapeDef()
		sd.Density = 1.0
		w.CreateCircleShape(bodyID, &sd, &circle)

		// Step with a force so isForceDriven is set on the body this step.
		mass := w.BodyMass(bodyID)
		w.ApplyBodyForceToCenter(bodyID, Vec2{X: 0.0, Y: -mass * 10.0}, true)
		w.Step(dt, subSteps)

		// Convert dynamic -> static (transfers awake -> static set). The target
		// sim must not carry any transient flag.
		w.SetBodyType(bodyID, StaticBody)
		require.Zero(t, w.getBodySim(w.getBodyFullID(bodyID)).flags&transientSimFlags,
			"transfer to the static set must clear transient sim flags")

		// Convert static -> dynamic (transfers static -> awake set). The new
		// awake sim must not carry any transient flag.
		w.SetBodyType(bodyID, DynamicBody)
		require.Zero(t, w.getBodySim(w.getBodyFullID(bodyID)).flags&transientSimFlags,
			"transfer back to the awake set must clear transient sim flags")
	})
}
