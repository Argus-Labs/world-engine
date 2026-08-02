// Oracle tests for body behavior: target transforms, velocities, mass, AABBs,
// damping, gravity scale, motion locks, sleep and disable/enable.
//
// Every expectation in this file is derived from the vendored C source of
// truth (box2d/src at v3.2.0-era vendor), upstream test_world.c, or
// docs/simulation.md — never from running the Go port. C citations are given
// as file:line next to each nontrivial assertion.

package box2d_test

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// TestOracleSetBodyTargetTransform checks b2Body_SetTargetTransform
// (body.c:825-879): linearVelocity = (target*localCenter - center)/dt
// (body.c:848-851) and angularVelocity = b2RelativeAngle(q1, q2)/dt
// (body.c:854-857), with the early-outs for disabled, static, non-positive
// time steps, sleeping bodies with wake=false, and sleepy target motion
// (body.c:830-843, 860-872).
func TestOracleSetBodyTargetTransform(t *testing.T) {
	t.Parallel()

	newSleepingFloater := func(t *testing.T, w *box2d.World) box2d.BodyID {
		t.Helper()

		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.GravityScale = 0.0
		body := w.CreateBody(&bd)
		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.4}
		sd := box2d.DefaultShapeDef()
		w.CreateCircleShape(body, &sd, &circle)
		w.SetBodyAwake(body, false)
		require.False(t, w.IsBodyAwake(body))
		return body
	}

	t.Run("KinematicFollowsTarget", func(t *testing.T) {
		t.Parallel()

		w := wboNewWorld(t)
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.KinematicBody
		bd.Position = box2d.Vec2{X: 1.0, Y: 2.0}
		body := w.CreateBody(&bd)

		target := box2d.Transform{
			P: box2d.Vec2{X: 1.5, Y: 2.25},
			Q: box2d.MakeRot(0.3),
		}
		w.SetBodyTargetTransform(body, target, wboDT, true)

		// body.c:848-851: v = (c2 - c1)/dt. Kinematic local center is the
		// origin, so v = (0.5, 0.25) * 60 = (30, 15).
		vel := w.BodyLinearVelocity(body)
		require.InDelta(t, 30.0, vel.X, 1e-9)
		require.InDelta(t, 15.0, vel.Y, 1e-9)

		// body.c:854-857: w = b2RelativeAngle(q1, q2)/dt. The exact identity
		// uses the ported b2RelativeAngle (math_functions.c b2Atan2 is a
		// polynomial approximation with ~2.3e-3 rad max error, so the result
		// is near — not exactly — 0.3 * 60 = 18).
		wantAngular := box2d.RelativeAngle(box2d.RotIdentity, target.Q) / wboDT
		require.InDelta(t, wantAngular, w.BodyAngularVelocity(body), 1e-9)
		require.InDelta(t, 18.0, w.BodyAngularVelocity(body), 0.3)

		// One step of position integration lands on the target position
		// exactly (v*dt telescopes over the sub-steps); the rotation uses the
		// first-order b2IntegrateRotation plus the approximate atan2, so it
		// is close but not exact.
		w.Step(wboDT, wboSubSteps)
		pos := w.BodyPosition(body)
		require.InDelta(t, target.P.X, pos.X, 1e-9)
		require.InDelta(t, target.P.Y, pos.Y, 1e-9)
		require.InDelta(t, 0.3, box2d.RotGetAngle(w.BodyRotation(body)), 1e-2)
	})

	t.Run("StaticIsNoOp", func(t *testing.T) {
		t.Parallel()

		w := wboNewWorld(t)
		bd := box2d.DefaultBodyDef()
		bd.Position = box2d.Vec2{X: 3.0, Y: 0.0}
		body := w.CreateBody(&bd)

		target := box2d.Transform{P: box2d.Vec2{X: 9.0, Y: 9.0}, Q: box2d.RotIdentity}
		// body.c:835-838: static bodies return early.
		w.SetBodyTargetTransform(body, target, wboDT, true)

		vel := w.BodyLinearVelocity(body)
		require.InDelta(t, 0.0, vel.X, 0.0)
		require.InDelta(t, 0.0, vel.Y, 0.0)

		w.Step(wboDT, wboSubSteps)
		pos := w.BodyPosition(body)
		require.InDelta(t, 3.0, pos.X, 0.0)
		require.InDelta(t, 0.0, pos.Y, 0.0)
	})

	t.Run("ZeroTimeStepIsNoOp", func(t *testing.T) {
		t.Parallel()

		w := wboNewWorld(t)
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.KinematicBody
		body := w.CreateBody(&bd)

		target := box2d.Transform{P: box2d.Vec2{X: 5.0, Y: 0.0}, Q: box2d.RotIdentity}
		// body.c:835-838: timeStep <= 0 returns early.
		w.SetBodyTargetTransform(body, target, 0.0, true)

		require.InDelta(t, 0.0, w.BodyLinearVelocity(body).X, 0.0)
	})

	t.Run("SleepingWithoutWakeIsNoOp", func(t *testing.T) {
		t.Parallel()

		w := wboNewWorld(t)
		body := newSleepingFloater(t, w)

		target := box2d.Transform{P: box2d.Vec2{X: 5.0, Y: 0.0}, Q: box2d.RotIdentity}
		// body.c:840-843: sleeping body with wake == false returns early.
		w.SetBodyTargetTransform(body, target, wboDT, false)
		require.False(t, w.IsBodyAwake(body))
	})

	t.Run("SleepingStaysAsleepForSleepyMotion", func(t *testing.T) {
		t.Parallel()

		w := wboNewWorld(t)
		body := newSleepingFloater(t, w)

		// body.c:860-868: maxVelocity = |v| + |w|*maxExtent = 0.0005*60 =
		// 0.03 stays below the default sleep threshold 0.05 (types.c
		// b2DefaultBodyDef), so the body stays asleep.
		target := box2d.Transform{P: box2d.Vec2{X: 0.0005, Y: 0.0}, Q: box2d.RotIdentity}
		w.SetBodyTargetTransform(body, target, wboDT, true)
		require.False(t, w.IsBodyAwake(body))
	})

	t.Run("SleepingWakesForLargeMotion", func(t *testing.T) {
		t.Parallel()

		w := wboNewWorld(t)
		body := newSleepingFloater(t, w)

		// v = 0.5*60 = 30 >> sleep threshold: body.c:871 wakes the body and
		// body.c:876-878 stores the derived velocity.
		target := box2d.Transform{P: box2d.Vec2{X: 0.5, Y: 0.0}, Q: box2d.RotIdentity}
		w.SetBodyTargetTransform(body, target, wboDT, true)

		require.True(t, w.IsBodyAwake(body))
		vel := w.BodyLinearVelocity(body)
		require.InDelta(t, 30.0, vel.X, 1e-9)
		require.InDelta(t, 0.0, vel.Y, 1e-9)
	})

	t.Run("DisabledIsNoOp", func(t *testing.T) {
		t.Parallel()

		w := wboNewWorld(t)
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		body := w.CreateBody(&bd)
		w.DisableBody(body)

		target := box2d.Transform{P: box2d.Vec2{X: 5.0, Y: 0.0}, Q: box2d.RotIdentity}
		// body.c:830-833: disabled bodies return early.
		w.SetBodyTargetTransform(body, target, wboDT, true)

		require.False(t, w.IsBodyEnabled(body))
		require.InDelta(t, 0.0, w.BodyLinearVelocity(body).X, 0.0)
	})
}

// TestOracleBodyLocalPointVelocity checks b2Body_GetLocalPointVelocity:
// v + w x r with r = R*(localPoint - localCenter) (body.c), plus the zero
// result for bodies without a state (static or sleeping).
func TestOracleBodyLocalPointVelocity(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: 2.0, Y: 1.0}
	bd.Rotation = box2d.MakeRot(0.5 * math.Pi)
	bd.GravityScale = 0.0
	body := w.CreateBody(&bd)

	w.SetBodyLinearVelocity(body, box2d.Vec2{X: 1.0, Y: 2.0})
	w.SetBodyAngularVelocity(body, 3.0)

	// r = R(pi/2)*(0.5, 0) = (0, 0.5); v + w x r = (1,2) + 3*(-0.5, 0)
	// = (-0.5, 2).
	local := w.BodyLocalPointVelocity(body, box2d.Vec2{X: 0.5, Y: 0.0})
	require.InDelta(t, -0.5, local.X, 1e-9)
	require.InDelta(t, 2.0, local.Y, 1e-9)

	// The same point expressed in world space must agree
	// (body.c b2Body_GetWorldPointVelocity uses r = worldPoint - center).
	worldPoint := w.BodyWorldPoint(body, box2d.Vec2{X: 0.5, Y: 0.0})
	world := w.BodyWorldPointVelocity(body, worldPoint)
	require.InDelta(t, local.X, world.X, 1e-12)
	require.InDelta(t, local.Y, world.Y, 1e-12)

	// Static bodies have no state: zero velocity.
	sbd := box2d.DefaultBodyDef()
	staticBody := w.CreateBody(&sbd)
	v := w.BodyLocalPointVelocity(staticBody, box2d.Vec2{X: 1.0, Y: 1.0})
	require.InDelta(t, 0.0, v.X, 0.0)
	require.InDelta(t, 0.0, v.Y, 0.0)

	// Sleeping bodies have no state either.
	w.SetBodyLinearVelocity(body, box2d.Vec2Zero)
	w.SetBodyAngularVelocity(body, 0.0)
	w.SetBodyAwake(body, false)
	v = w.BodyLocalPointVelocity(body, box2d.Vec2{X: 0.5, Y: 0.0})
	require.InDelta(t, 0.0, v.X, 0.0)
	require.InDelta(t, 0.0, v.Y, 0.0)
}

// TestOracleWakeBodyTouching checks b2Body_WakeTouching (body.c:1528): waking
// the bodies touching a body via contacts. Waking a body in a sleeping set
// wakes the whole set, so the entire sleeping stack wakes.
func TestOracleWakeBodyTouching(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)
	wboGround(t, w)
	bottom, _ := wboDynamicBox(t, w, box2d.Vec2{X: 0.0, Y: 0.5})
	top, _ := wboDynamicBox(t, w, box2d.Vec2{X: 0.0, Y: 1.5})

	asleep := false
	for range 600 {
		w.Step(wboDT, wboSubSteps)
		if !w.IsBodyAwake(bottom) && !w.IsBodyAwake(top) {
			asleep = true
			break
		}
	}
	require.True(t, asleep, "stack must fall asleep first")

	// The top box only touches the bottom box; waking the bottom box wakes
	// its sleeping solver set, which holds the whole island.
	w.WakeBodyTouching(top)
	require.True(t, w.IsBodyAwake(bottom))
	require.True(t, w.IsBodyAwake(top))

	// Walk the contact list from the other body too: the bottom box holds
	// contact edges with both orientations (ground contact and top contact),
	// and waking awake bodies is a no-op (body.c b2WakeBody).
	w.WakeBodyTouching(bottom)
	require.True(t, w.IsBodyAwake(bottom))
	require.True(t, w.IsBodyAwake(top))
}

// TestOracleMaxLinearSpeedClampsImpulse checks b2LimitVelocity (body.c:28-35)
// invoked from b2Body_ApplyLinearImpulse and
// b2Body_ApplyLinearImpulseToCenter (body.c:1017, 1044): the post-impulse
// speed is clamped to the world maximum linear speed.
func TestOracleMaxLinearSpeedClampsImpulse(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)
	w.SetMaximumLinearSpeed(10.0)

	newBall := func(x float64) box2d.BodyID {
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: x, Y: 10.0}
		bd.GravityScale = 0.0
		body := w.CreateBody(&bd)
		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
		sd := box2d.DefaultShapeDef()
		w.CreateCircleShape(body, &sd, &circle)
		return body
	}

	center := newBall(0.0)
	w.ApplyBodyLinearImpulseToCenter(center, box2d.Vec2{X: 1000.0, Y: 0.0}, true)
	vel := w.BodyLinearVelocity(center)
	require.InDelta(t, 10.0, vel.X, 1e-9)
	require.InDelta(t, 0.0, vel.Y, 1e-9)

	offCenter := newBall(5.0)
	point := w.BodyWorldPoint(offCenter, box2d.Vec2{X: 0.0, Y: 0.4})
	w.ApplyBodyLinearImpulse(offCenter, box2d.Vec2{X: 1000.0, Y: 0.0}, point, true)
	vel = w.BodyLinearVelocity(offCenter)
	speed := math.Hypot(vel.X, vel.Y)
	require.InDelta(t, 10.0, speed, 1e-9)
	require.NotEqual(t, 0.0, w.BodyAngularVelocity(offCenter),
		"off-center impulse must add spin")
}

// TestOracleBodyNameTruncation: the body name buffer is
// char[B2_NAME_LENGTH=32] and the copy loop keeps at most 31 characters
// (body.c:270-284 at creation, body.c:1306-1320 in b2Body_SetName).
func TestOracleBodyNameTruncation(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)
	longName := strings.Repeat("a", 40)
	want := strings.Repeat("a", 31)

	bd := box2d.DefaultBodyDef()
	bd.Name = longName
	body := w.CreateBody(&bd)
	require.Equal(t, want, w.BodyName(body))

	w.SetBodyName(body, longName+"b")
	require.Equal(t, want, w.BodyName(body))

	w.SetBodyName(body, "short")
	require.Equal(t, "short", w.BodyName(body))
}

// TestOracleComputeBodyAABB checks b2Body_ComputeAABB (body.c:483): with no
// shapes the AABB is a point at the body origin; otherwise it is the union of
// the shape AABBs. Shape AABBs carry the speculative margin
// (shape.c:90-101 b2UpdateShapeAABBs adds B2_SPECULATIVE_DISTANCE).
func TestOracleComputeBodyAABB(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)

	bd := box2d.DefaultBodyDef()
	bd.Position = box2d.Vec2{X: 2.0, Y: 3.0}
	body := w.CreateBody(&bd)

	aabb := w.ComputeBodyAABB(body)
	require.InDelta(t, 2.0, aabb.LowerBound.X, 0.0)
	require.InDelta(t, 3.0, aabb.LowerBound.Y, 0.0)
	require.InDelta(t, 2.0, aabb.UpperBound.X, 0.0)
	require.InDelta(t, 3.0, aabb.UpperBound.Y, 0.0)

	sd := box2d.DefaultShapeDef()
	c1 := box2d.Circle{Center: box2d.Vec2{X: 1.0, Y: 0.0}, Radius: 0.5}
	w.CreateCircleShape(body, &sd, &c1)
	c2 := box2d.Circle{Center: box2d.Vec2{X: -1.0, Y: 0.0}, Radius: 0.25}
	w.CreateCircleShape(body, &sd, &c2)

	// Union of world circle AABBs (geometry.c b2ComputeCircleAABB: p +- r)
	// inflated by the speculative distance (shape.c:97-100).
	spec := box2d.SpeculativeDistance
	aabb = w.ComputeBodyAABB(body)
	require.InDelta(t, 0.75-spec, aabb.LowerBound.X, 1e-12)
	require.InDelta(t, 2.5-spec, aabb.LowerBound.Y, 1e-12)
	require.InDelta(t, 3.5+spec, aabb.UpperBound.X, 1e-12)
	require.InDelta(t, 3.5+spec, aabb.UpperBound.Y, 1e-12)
}

// TestOracleBodyContactCapacityAndData checks b2Body_GetContactCapacity and
// b2Body_GetContactData (body.c:426+): the capacity bounds the touching
// contacts and the returned data carries the touching manifold and shape ids.
func TestOracleBodyContactCapacityAndData(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)
	_, groundShape := wboGround(t, w)
	body, boxShape := wboDynamicBox(t, w, box2d.Vec2{X: 0.0, Y: 0.5})

	for range 2 {
		w.Step(wboDT, wboSubSteps)
	}

	capacity := w.BodyContactCapacity(body)
	require.GreaterOrEqual(t, capacity, 1)

	data := make([]box2d.ContactData, capacity)
	n := w.BodyContactData(body, data)
	require.Equal(t, 1, n)

	// A box resting on the ground has a two-point manifold.
	require.Equal(t, 2, data[0].Manifold.PointCount)

	got := map[box2d.ShapeID]bool{data[0].ShapeIDA: true, data[0].ShapeIDB: true}
	require.True(t, got[groundShape], "contact data must reference the ground shape")
	require.True(t, got[boxShape], "contact data must reference the box shape")

	// b2Contact_IsValid (physics_world.c): a live contact id validates, the
	// zero id does not, and destroying a body frees its contacts.
	contactID := data[0].ContactID
	require.True(t, w.IsContactValid(contactID))
	require.False(t, w.IsContactValid(box2d.ContactID{}))

	w.DestroyBody(body)
	require.False(t, w.IsContactValid(contactID))
}

// TestOracleApplyMassFromShapes checks b2Body_ApplyMassFromShapes and
// b2UpdateBodyMassData (body.c): deferred mass with updateBodyMass=false
// (upstream DeferredMassFlagSyncTest, test_world.c:537), the analytic box
// mass/inertia, zero-density shapes, and the kinematic zero-mass path.
func TestOracleApplyMassFromShapes(t *testing.T) {
	t.Parallel()

	t.Run("DeferredMassThenApply", func(t *testing.T) {
		t.Parallel()

		w := wboNewWorld(t)
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		body := w.CreateBody(&bd)

		box := box2d.MakeBox(0.5, 0.5)
		sd := box2d.DefaultShapeDef()
		sd.UpdateBodyMass = false
		w.CreatePolygonShape(body, &sd, &box)

		// No automatic mass update: the shapeless-body mass (zero) remains.
		require.InDelta(t, 0.0, w.BodyMass(body), 0.0)

		w.ApplyBodyMassFromShapes(body)

		// geometry.c b2ComputePolygonMass for a centered 1x1 box, density 1:
		// m = 1, I = m*(w^2 + h^2)/12 = 1/6 about the centroid.
		require.InDelta(t, 1.0, w.BodyMass(body), 1e-9)
		require.InDelta(t, 1.0/6.0, w.BodyRotationalInertia(body), 1e-9)
		center := w.BodyLocalCenterOfMass(body)
		require.InDelta(t, 0.0, center.X, 1e-12)
		require.InDelta(t, 0.0, center.Y, 1e-12)

		w.Step(wboDT, wboSubSteps)
	})

	t.Run("ZeroDensityShapeIsSkipped", func(t *testing.T) {
		t.Parallel()

		w := wboNewWorld(t)
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		body := w.CreateBody(&bd)

		box := box2d.MakeBox(0.5, 0.5)
		sd := box2d.DefaultShapeDef()
		w.CreatePolygonShape(body, &sd, &box)

		// body.c b2UpdateBodyMassData skips shapes with zero density.
		zeroSd := box2d.DefaultShapeDef()
		zeroSd.Density = 0.0
		circle := box2d.Circle{Center: box2d.Vec2{X: 2.0, Y: 0.0}, Radius: 0.5}
		w.CreateCircleShape(body, &zeroSd, &circle)

		require.InDelta(t, 1.0, w.BodyMass(body), 1e-9)
		center := w.BodyLocalCenterOfMass(body)
		require.InDelta(t, 0.0, center.X, 1e-12)
	})

	t.Run("KinematicHasZeroMass", func(t *testing.T) {
		t.Parallel()

		w := wboNewWorld(t)
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.KinematicBody
		body := w.CreateBody(&bd)

		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
		sd := box2d.DefaultShapeDef()
		w.CreateCircleShape(body, &sd, &circle)

		// body.c:716-736: non-dynamic bodies keep zero mass and inertia.
		w.ApplyBodyMassFromShapes(body)
		require.InDelta(t, 0.0, w.BodyMass(body), 0.0)
		require.InDelta(t, 0.0, w.BodyRotationalInertia(body), 0.0)
	})
}

// TestOracleSetBodyMassDataOverride checks b2Body_SetMassData (body.c): the
// override replaces mass, inertia and local center, and the world center of
// mass moves to transform * center (docs/simulation.md, mass override rules).
func TestOracleSetBodyMassDataOverride(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)
	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: 1.0, Y: 2.0}
	body := w.CreateBody(&bd)
	box := box2d.MakeBox(0.5, 0.5)
	sd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(body, &sd, &box)

	override := box2d.MassData{
		Mass:              5.0,
		Center:            box2d.Vec2{X: 0.5, Y: 0.0},
		RotationalInertia: 2.0,
	}
	w.SetBodyMassData(body, override)

	got := w.BodyMassData(body)
	require.InDelta(t, 5.0, got.Mass, 0.0)
	require.InDelta(t, 2.0, got.RotationalInertia, 0.0)
	require.InDelta(t, 0.5, got.Center.X, 0.0)

	require.InDelta(t, 5.0, w.BodyMass(body), 0.0)
	require.InDelta(t, 2.0, w.BodyRotationalInertia(body), 0.0)

	worldCenter := w.BodyWorldCenterOfMass(body)
	require.InDelta(t, 1.5, worldCenter.X, 1e-12)
	require.InDelta(t, 2.0, worldCenter.Y, 1e-12)
}

// TestOracleKinematicAndStaticSemantics encodes the docs body-type contracts
// (docs/simulation.md "Body types"): static and kinematic bodies store zero
// mass, static bodies never gain velocity, and kinematic bodies move by their
// velocity while ignoring gravity, forces, torques and impulses
// (solver.c:99: gravity needs invMass > 0; body.c force/impulse setters
// early-out for non-dynamic bodies).
func TestOracleKinematicAndStaticSemantics(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)

	sbd := box2d.DefaultBodyDef()
	staticBody := w.CreateBody(&sbd)
	require.InDelta(t, 0.0, w.BodyMass(staticBody), 0.0)
	w.SetBodyLinearVelocity(staticBody, box2d.Vec2{X: 1.0, Y: 1.0})
	require.InDelta(t, 0.0, w.BodyLinearVelocity(staticBody).X, 0.0)

	kbd := box2d.DefaultBodyDef()
	kbd.Type = box2d.KinematicBody
	kbd.Position = box2d.Vec2{X: 0.0, Y: 5.0}
	kin := w.CreateBody(&kbd)
	circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
	sd := box2d.DefaultShapeDef()
	w.CreateCircleShape(kin, &sd, &circle)

	require.InDelta(t, 0.0, w.BodyMass(kin), 0.0)
	require.InDelta(t, 0.0, w.BodyRotationalInertia(kin), 0.0)

	w.SetBodyLinearVelocity(kin, box2d.Vec2{X: 2.0, Y: 0.0})
	w.ApplyBodyForceToCenter(kin, box2d.Vec2{X: 100.0, Y: 100.0}, true)
	w.ApplyBodyTorque(kin, 50.0, true)
	w.ApplyBodyLinearImpulseToCenter(kin, box2d.Vec2{X: 100.0, Y: 100.0}, true)
	w.ApplyBodyAngularImpulse(kin, 50.0, true)

	for range 60 {
		w.Step(wboDT, wboSubSteps)
	}

	// Velocity is untouched by gravity/forces; x advances by v*t = 2.
	vel := w.BodyLinearVelocity(kin)
	require.InDelta(t, 2.0, vel.X, 1e-12)
	require.InDelta(t, 0.0, vel.Y, 1e-12)
	require.InDelta(t, 0.0, w.BodyAngularVelocity(kin), 1e-12)
	pos := w.BodyPosition(kin)
	require.InDelta(t, 2.0, pos.X, 1e-9)
	require.InDelta(t, 5.0, pos.Y, 1e-9)
}

// TestOracleGravityScale checks the gravity-scale contract
// (docs/simulation.md "Gravity Scale"; solver.c:99-102: lvd includes
// h * gravityScale * gravity): after one step the velocity is
// gravityScale * g * dt, and a zero-scale body floats.
func TestOracleGravityScale(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)

	newBall := func(x, scale float64) box2d.BodyID {
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: x, Y: 0.0}
		bd.GravityScale = scale
		body := w.CreateBody(&bd)
		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
		sd := box2d.DefaultShapeDef()
		w.CreateCircleShape(body, &sd, &circle)
		return body
	}

	floater := newBall(0.0, 0.0)
	normal := newBall(5.0, 1.0)
	heavy := newBall(10.0, 2.0)

	require.InDelta(t, 0.0, w.BodyGravityScale(floater), 0.0)
	require.InDelta(t, 2.0, w.BodyGravityScale(heavy), 0.0)

	w.Step(wboDT, wboSubSteps)

	// v_y = -g * scale * dt accumulated over the sub-steps.
	require.InDelta(t, 0.0, w.BodyLinearVelocity(floater).Y, 0.0)
	require.InDelta(t, -10.0*wboDT, w.BodyLinearVelocity(normal).Y, 1e-12)
	require.InDelta(t, -20.0*wboDT, w.BodyLinearVelocity(heavy).Y, 1e-12)

	for range 59 {
		w.Step(wboDT, wboSubSteps)
	}

	// The zero-scale body floats in place (docs: "this body will float").
	pos := w.BodyPosition(floater)
	require.InDelta(t, 0.0, pos.X, 0.0)
	require.InDelta(t, 0.0, pos.Y, 0.0)

	// SetBodyGravityScale round-trips.
	w.SetBodyGravityScale(floater, 3.0)
	require.InDelta(t, 3.0, w.BodyGravityScale(floater), 0.0)
}

// TestOracleDamping checks the Pade damping update v2 = v1 / (1 + h*c)
// applied per sub-step (solver.c:88-96; docs/simulation.md "Damping"):
// after one step of n sub-steps the velocity is v0 * (1/(1+h*c))^n.
func TestOracleDamping(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.GravityScale = 0.0
	bd.LinearDamping = 0.5
	bd.AngularDamping = 1.0
	bd.LinearVelocity = box2d.Vec2{X: 3.0, Y: 4.0}
	bd.AngularVelocity = 2.0
	body := w.CreateBody(&bd)
	circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
	sd := box2d.DefaultShapeDef()
	w.CreateCircleShape(body, &sd, &circle)

	require.InDelta(t, 0.5, w.BodyLinearDamping(body), 0.0)
	require.InDelta(t, 1.0, w.BodyAngularDamping(body), 0.0)

	h := wboDT / float64(wboSubSteps)
	linFactor := math.Pow(1.0/(1.0+h*0.5), wboSubSteps)
	angFactor := math.Pow(1.0/(1.0+h*1.0), wboSubSteps)

	w.Step(wboDT, wboSubSteps)

	vel := w.BodyLinearVelocity(body)
	require.InDelta(t, 3.0*linFactor, vel.X, 1e-10)
	require.InDelta(t, 4.0*linFactor, vel.Y, 1e-10)
	require.InDelta(t, 2.0*angFactor, w.BodyAngularVelocity(body), 1e-10)

	// Setter round-trip (body.c b2Body_SetLinearDamping/SetAngularDamping).
	w.SetBodyLinearDamping(body, 0.25)
	require.InDelta(t, 0.25, w.BodyLinearDamping(body), 0.0)
	w.SetBodyAngularDamping(body, 0.75)
	require.InDelta(t, 0.75, w.BodyAngularDamping(body), 0.0)
}

// TestOracleMotionLocks checks the motion locks: locked velocity components
// are zeroed during velocity integration (solver.c:124-137) and
// b2Body_SetAngularVelocity is ignored under an angular lock (body.c).
func TestOracleMotionLocks(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)

	newLockedBody := func(x float64, locks box2d.MotionLocks, v box2d.Vec2) box2d.BodyID {
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: x, Y: 10.0}
		bd.MotionLocks = locks
		bd.LinearVelocity = v
		body := w.CreateBody(&bd)
		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
		sd := box2d.DefaultShapeDef()
		w.CreateCircleShape(body, &sd, &circle)
		return body
	}

	lockX := newLockedBody(0.0, box2d.MotionLocks{LinearX: true}, box2d.Vec2{X: 3.0, Y: 0.0})
	lockY := newLockedBody(5.0, box2d.MotionLocks{LinearY: true}, box2d.Vec2{X: 1.0, Y: 0.0})
	lockAng := newLockedBody(10.0, box2d.MotionLocks{AngularZ: true}, box2d.Vec2Zero)

	require.True(t, w.BodyMotionLocks(lockX).LinearX)

	// body.c b2Body_SetAngularVelocity: ignored under lockAngularZ.
	w.SetBodyAngularVelocity(lockAng, 5.0)
	require.InDelta(t, 0.0, w.BodyAngularVelocity(lockAng), 0.0)
	w.ApplyBodyTorque(lockAng, 50.0, true)

	w.Step(wboDT, wboSubSteps)

	// solver.c:124-127: locked x velocity is zeroed; gravity still applies.
	require.InDelta(t, 0.0, w.BodyLinearVelocity(lockX).X, 0.0)
	require.InDelta(t, 0.0, w.BodyPosition(lockX).X, 0.0)
	require.InDelta(t, -10.0*wboDT, w.BodyLinearVelocity(lockX).Y, 1e-12)

	// solver.c:129-132: locked y velocity is zeroed; x keeps sliding.
	require.InDelta(t, 0.0, w.BodyLinearVelocity(lockY).Y, 0.0)
	require.InDelta(t, 10.0, w.BodyPosition(lockY).Y, 0.0)
	require.InDelta(t, 1.0, w.BodyLinearVelocity(lockY).X, 1e-12)

	// solver.c:134-137: locked angular velocity is zeroed despite torque.
	require.InDelta(t, 0.0, w.BodyAngularVelocity(lockAng), 0.0)
}

// TestOracleDisableEnableBody checks b2Body_Disable/b2Body_Enable
// (body.c:1605, 1675) and the docs disabling contract (docs/simulation.md
// "Disabling"): a disabled body freezes, loses its contacts, and its joints
// move to the disabled set; enabling restores simulation. Joints whose other
// body is still disabled stay disabled.
func TestOracleDisableEnableBody(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)
	ground, _ := wboGround(t, w)
	bodyA, _ := wboDynamicBox(t, w, box2d.Vec2{X: 0.0, Y: 0.5})
	bodyB, _ := wboDynamicBox(t, w, box2d.Vec2{X: 2.0, Y: 0.5})

	// CollideConnected keeps the ground-A contact alive: with the default
	// (false) the joint suppresses collision between its bodies
	// (joint.c: destroyContactsBetweenBodies + collide filtering).
	jd1 := box2d.DefaultDistanceJointDef()
	jd1.Base.BodyIDA = ground
	jd1.Base.BodyIDB = bodyA
	jd1.Base.CollideConnected = true
	jd1.Length = 10.5
	jointGA := w.CreateDistanceJoint(&jd1)

	jd2 := box2d.DefaultDistanceJointDef()
	jd2.Base.BodyIDA = bodyA
	jd2.Base.BodyIDB = bodyB
	jd2.Length = 2.0
	jointAB := w.CreateDistanceJoint(&jd2)

	for range 10 {
		w.Step(wboDT, wboSubSteps)
	}
	require.GreaterOrEqual(t, w.BodyContactCapacity(bodyA), 1)

	// Disable A: contacts destroyed, body frozen, joints to the disabled set.
	w.DisableBody(bodyA)
	require.False(t, w.IsBodyEnabled(bodyA))
	require.Equal(t, 0, w.BodyContactCapacity(bodyA))
	require.True(t, w.IsJointValid(jointGA))
	require.True(t, w.IsJointValid(jointAB))

	// Disabling twice is a no-op (body.c:1611-1614 early return).
	w.DisableBody(bodyA)
	require.False(t, w.IsBodyEnabled(bodyA))

	posA := w.BodyPosition(bodyA)
	for range 30 {
		w.Step(wboDT, wboSubSteps)
	}
	after := w.BodyPosition(bodyA)
	require.InDelta(t, posA.X, after.X, 0.0)
	require.InDelta(t, posA.Y, after.Y, 0.0)

	// Disable B as well: the A-B joint is already in the disabled set
	// (body.c disable loop skips joints already disabled by the other body).
	w.DisableBody(bodyB)
	require.False(t, w.IsBodyEnabled(bodyB))

	// Enable A: the ground joint transfers back, the A-B joint stays
	// disabled because B is still disabled (body.c:1685+ enable loop).
	w.EnableBody(bodyA)
	require.True(t, w.IsBodyEnabled(bodyA))
	require.True(t, w.IsBodyAwake(bodyA))

	// Enable B: the A-B joint transfers back too.
	w.EnableBody(bodyB)
	require.True(t, w.IsBodyEnabled(bodyB))

	// Enabling an enabled body is a no-op.
	w.EnableBody(bodyB)
	require.True(t, w.IsBodyEnabled(bodyB))

	for range 30 {
		w.Step(wboDT, wboSubSteps)
	}
	require.True(t, w.IsJointValid(jointGA))
	require.True(t, w.IsJointValid(jointAB))

	// Static bodies can be disabled and enabled as well; the joint between
	// two static bodies lives in the static set (body.c enable: static path).
	sbd := box2d.DefaultBodyDef()
	sbd.Position = box2d.Vec2{X: 30.0, Y: 0.0}
	static2 := w.CreateBody(&sbd)
	sBox := box2d.MakeBox(0.5, 0.5)
	sSd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(static2, &sSd, &sBox)

	jd3 := box2d.DefaultDistanceJointDef()
	jd3.Base.BodyIDA = ground
	jd3.Base.BodyIDB = static2
	jd3.Length = 30.0
	jointSS := w.CreateDistanceJoint(&jd3)

	w.DisableBody(static2)
	require.False(t, w.IsBodyEnabled(static2))
	w.EnableBody(static2)
	require.True(t, w.IsBodyEnabled(static2))
	require.True(t, w.IsJointValid(jointSS))

	w.Step(wboDT, wboSubSteps)
}

// TestOracleBulletFlagSurvivesMotionLocks ports SetBulletDriftTest
// (test_world.c:611-654): b2Body_SetMotionLocks must not clobber the bullet
// flag in either direction.
func TestOracleBulletFlagSurvivesMotionLocks(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)

	{
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.IsBullet = false
		body := w.CreateBody(&bd)

		require.False(t, w.IsBodyBullet(body))
		w.SetBodyBullet(body, true)
		require.True(t, w.IsBodyBullet(body))

		w.SetBodyMotionLocks(body, box2d.MotionLocks{LinearX: true})
		require.True(t, w.IsBodyBullet(body))
	}

	{
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.IsBullet = true
		body := w.CreateBody(&bd)

		require.True(t, w.IsBodyBullet(body))
		w.SetBodyBullet(body, false)
		require.False(t, w.IsBodyBullet(body))

		w.SetBodyMotionLocks(body, box2d.MotionLocks{LinearX: true})
		require.False(t, w.IsBodyBullet(body))
	}
}

// TestOracleSleepEnableFlag ports EnableSleepFlagSyncTest
// (test_world.c:560-579) and encodes the docs sleep contract
// (docs/simulation.md "Sleep Parameters"): a body with sleep disabled never
// falls asleep; enabling sleep lets it sleep after settling.
func TestOracleSleepEnableFlag(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)
	wboGround(t, w)

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: 0.0, Y: 0.5}
	bd.EnableSleep = false
	body := w.CreateBody(&bd)
	box := box2d.MakeBox(0.5, 0.5)
	sd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(body, &sd, &box)

	require.False(t, w.IsBodySleepEnabled(body))

	// B2_TIME_TO_SLEEP is 0.5s (solver.c:773); 240 steps = 4s of rest is far
	// beyond it, so staying awake proves enableSleep=false blocks sleep.
	for range 240 {
		w.Step(wboDT, wboSubSteps)
		require.True(t, w.IsBodyAwake(body), "sleep-disabled body must stay awake")
	}

	// Sleep threshold round-trip (body.c b2Body_SetSleepThreshold).
	w.SetBodySleepThreshold(body, 0.1)
	require.InDelta(t, 0.1, w.BodySleepThreshold(body), 0.0)

	w.EnableBodySleep(body, true)
	require.True(t, w.IsBodySleepEnabled(body))

	asleep := false
	for range 600 {
		w.Step(wboDT, wboSubSteps)
		if !w.IsBodyAwake(body) {
			asleep = true
			break
		}
	}
	require.True(t, asleep, "body must sleep once sleep is re-enabled")

	// EnableBodySleep(false) wakes a sleeping body (body.c b2Body_EnableSleep).
	w.EnableBodySleep(body, false)
	require.True(t, w.IsBodyAwake(body))
}

// TestOracleContactAndHitEventToggles checks b2Body_EnableContactEvents and
// b2Body_EnableHitEvents (body.c): contact begin events follow the shape
// flags (contact.c: events are enabled when either shape opts in) and hit
// events fire when the approach speed exceeds the world threshold
// (solver.c:1948-2001).
func TestOracleContactAndHitEventToggles(t *testing.T) {
	t.Parallel()

	w := wboNewWorld(t)
	wboGround(t, w) // ground shape has contact events off (default)

	newEventBox := func(x float64) (box2d.BodyID, box2d.ShapeID) {
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: x, Y: 2.0}
		body := w.CreateBody(&bd)
		box := box2d.MakeBox(0.5, 0.5)
		sd := box2d.DefaultShapeDef()
		sd.EnableContactEvents = true
		shape := w.CreatePolygonShape(body, &sd, &box)
		return body, shape
	}

	silentBody, silentShape := newEventBox(0.0)
	loudBody, loudShape := newEventBox(5.0)

	// Toggle before the contacts are created: the contact picks up the shape
	// flags at creation time.
	w.EnableBodyContactEvents(silentBody, false)
	w.EnableBodyHitEvents(loudBody, true)

	sawSilentBegin := false
	sawLoudBegin := false
	sawHit := false
	var hitSpeed float64

	for range 240 {
		w.Step(wboDT, wboSubSteps)
		events := w.ContactEvents()
		for i := range events.BeginEvents {
			e := &events.BeginEvents[i]
			if e.ShapeIDA == silentShape || e.ShapeIDB == silentShape {
				sawSilentBegin = true
			}
			if e.ShapeIDA == loudShape || e.ShapeIDB == loudShape {
				sawLoudBegin = true
			}
		}
		for i := range events.HitEvents {
			e := &events.HitEvents[i]
			if e.ShapeIDA == loudShape || e.ShapeIDB == loudShape {
				sawHit = true
				hitSpeed = e.ApproachSpeed
			}
		}
	}

	require.False(t, sawSilentBegin, "disabled contact events must suppress begin events")
	require.True(t, sawLoudBegin, "enabled contact events must produce begin events")
	require.True(t, sawHit, "enabled hit events must produce a hit on impact")

	// solver.c:1964,1974: only approach speeds above the threshold are kept.
	require.Greater(t, hitSpeed, w.HitEventThreshold())

	// _ = silentBody keeps parity with upstream naming.
	require.True(t, w.IsBodyValid(silentBody))
	require.True(t, w.IsBodyValid(loudBody))
}
