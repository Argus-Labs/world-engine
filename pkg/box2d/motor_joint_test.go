// Behavior tests for the float64 port of the Box2D v3.2.0 motor joint
// (stage E9a, src/motor_joint.c): transform tracking with the position
// springs, the maxSpringForce/maxSpringTorque and maxVelocityForce/
// maxVelocityTorque clamps, spring stiffness sanity, reaction getters,
// accessor round-trips and cross-world determinism.

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

const motorTimeStep = 1.0 / 60.0

// motorTestWorld creates a world with the given gravity and sleeping
// disabled, so a motor joint keeps tracking a target that moves between
// steps (b2Joint_SetLocalFrameA does not wake the bodies).
func motorTestWorld(gravity box2d.Vec2) *box2d.World {
	def := box2d.DefaultWorldDef()
	def.Gravity = gravity
	def.EnableSleep = false
	return box2d.NewWorld(&def)
}

// motorTestGround creates a shapeless static body at the origin.
func motorTestGround(w *box2d.World) box2d.BodyID {
	bd := box2d.DefaultBodyDef()
	return w.CreateBody(&bd)
}

// motorTestBoxBody creates a dynamic square body with the given half extent
// and density 1.
func motorTestBoxBody(w *box2d.World, pos box2d.Vec2, halfExtent float64) box2d.BodyID {
	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = pos
	bodyID := w.CreateBody(&bd)

	box := box2d.MakeBox(halfExtent, halfExtent)
	sd := box2d.DefaultShapeDef()
	sd.Density = 1.0
	w.CreatePolygonShape(bodyID, &sd, &box)
	return bodyID
}

// motorTestJoint welds a stiff position-controlling motor between a static
// ground body at the origin and a dynamic body. The target transform is the
// joint local frame on the ground, which is world space here.
func motorTestJoint(w *box2d.World, ground, body box2d.BodyID, hertz, maxForce, maxTorque float64) box2d.JointID {
	def := box2d.DefaultMotorJointDef()
	def.Base.BodyIDA = ground
	def.Base.BodyIDB = body
	def.LinearHertz = hertz
	def.LinearDampingRatio = 1.0
	def.MaxSpringForce = maxForce
	def.AngularHertz = hertz
	def.AngularDampingRatio = 1.0
	def.MaxSpringTorque = maxTorque
	return w.CreateMotorJoint(&def)
}

// motorTargetError returns the position and angle error between a body and a
// target transform.
func motorTargetError(w *box2d.World, body box2d.BodyID, target box2d.Transform) (float64, float64) {
	p := w.BodyPosition(body)
	q := w.BodyRotation(body)
	posError := box2d.Length(box2d.Sub(p, target.P))
	angleError := math.Abs(box2d.RelativeAngle(target.Q, q))
	return posError, angleError
}

func TestMotorJointTracksMovingTarget(t *testing.T) {
	t.Parallel()

	w := motorTestWorld(box2d.Vec2Zero)
	defer w.Destroy()

	ground := motorTestGround(w)
	body := motorTestBoxBody(w, box2d.Vec2Zero, 0.25)
	jointID := motorTestJoint(w, ground, body, 20.0, 1.0e4, 1.0e4)

	// The target slides and spins away from the origin for 120 steps.
	target := box2d.Transform{P: box2d.Vec2Zero, Q: box2d.RotIdentity}
	maxTrackingPosError := 0.0
	maxTrackingAngleError := 0.0

	const trackingSteps = 120
	for step := 1; step <= trackingSteps; step++ {
		k := float64(step)
		target = box2d.Transform{
			P: box2d.Vec2{X: 0.01 * k, Y: 0.005 * k},
			Q: box2d.MakeRot(0.01 * k),
		}
		w.SetJointLocalFrameA(jointID, target)
		w.Step(motorTimeStep, 4)

		posError, angleError := motorTargetError(w, body, target)
		maxTrackingPosError = math.Max(maxTrackingPosError, posError)
		maxTrackingAngleError = math.Max(maxTrackingAngleError, angleError)
	}

	t.Logf("tracking error: position %g m, angle %g rad", maxTrackingPosError, maxTrackingAngleError)
	require.Less(t, maxTrackingPosError, 0.1, "motor joint should follow the moving target closely")
	require.Less(t, maxTrackingAngleError, 0.1, "motor joint should follow the target rotation closely")

	// Hold the target still: the body must converge onto it.
	for range 60 {
		w.Step(motorTimeStep, 4)
	}

	posError, angleError := motorTargetError(w, body, target)
	t.Logf("settled error: position %g m, angle %g rad", posError, angleError)
	require.Less(t, posError, 0.01, "motor joint should converge onto a static target")
	require.Less(t, angleError, 0.01, "motor joint should converge onto a static target rotation")
}

func TestMotorJointMaxSpringForceClamp(t *testing.T) {
	t.Parallel()

	// A body of mass 1 needs 10 N to hold station against this gravity.
	const strongForce = 500.0
	const weakForce = 1.0

	drop := func(maxForce float64) float64 {
		w := motorTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
		defer w.Destroy()

		ground := motorTestGround(w)
		body := motorTestBoxBody(w, box2d.Vec2Zero, 0.5)
		require.InDelta(t, 1.0, w.BodyMass(body), 1.0e-9)
		motorTestJoint(w, ground, body, 20.0, maxForce, 1.0e4)

		for range 120 {
			w.Step(motorTimeStep, 4)
		}
		return w.BodyPosition(body).Y
	}

	strongY := drop(strongForce)
	weakY := drop(weakForce)
	t.Logf("held y %g m, weak y %g m", strongY, weakY)

	require.Less(t, math.Abs(strongY), 0.05, "a strong motor should hold the body at the target")
	require.Less(t, weakY, -1.0, "a motor clamped below the weight should let the body fall")
}

func TestMotorJointMaxSpringTorqueClamp(t *testing.T) {
	t.Parallel()

	// The body starts rotated away from the target angle. A torque clamp of
	// zero disables the angular spring entirely (upstream gates on
	// maxSpringTorque > 0).
	spin := func(maxTorque float64) float64 {
		w := motorTestWorld(box2d.Vec2Zero)
		defer w.Destroy()

		ground := motorTestGround(w)

		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Rotation = box2d.MakeRot(1.0)
		body := w.CreateBody(&bd)
		box := box2d.MakeBox(0.5, 0.5)
		sd := box2d.DefaultShapeDef()
		sd.Density = 1.0
		w.CreatePolygonShape(body, &sd, &box)

		motorTestJoint(w, ground, body, 20.0, 1.0e4, maxTorque)

		for range 120 {
			w.Step(motorTimeStep, 4)
		}
		return math.Abs(box2d.RotGetAngle(w.BodyRotation(body)))
	}

	// The start angle is measured through the package trig, which is the
	// approximate b2MakeRot/b2Rot_GetAngle pair, not exactly 1 radian.
	startAngle := math.Abs(box2d.RotGetAngle(box2d.MakeRot(1.0)))
	strongAngle := spin(1.0e4)
	disabledAngle := spin(0.0)
	t.Logf("start angle %g rad, strong angle %g rad, disabled angle %g rad", startAngle, strongAngle, disabledAngle)

	require.Less(t, strongAngle, 0.01, "a strong angular spring should reach the target angle")
	require.InDelta(t, startAngle, disabledAngle, 1.0e-12, "a zero torque clamp should leave the rotation alone")
}

func TestMotorJointVelocityMotorClamp(t *testing.T) {
	t.Parallel()

	// Pure velocity control: no springs, so the joint acts as a top-down
	// drive toward the requested relative velocity.
	w := motorTestWorld(box2d.Vec2Zero)
	defer w.Destroy()

	ground := motorTestGround(w)
	body := motorTestBoxBody(w, box2d.Vec2Zero, 0.5)

	def := box2d.DefaultMotorJointDef()
	def.Base.BodyIDA = ground
	def.Base.BodyIDB = body
	def.LinearVelocity = box2d.Vec2{X: 2.0, Y: 0.0}
	def.MaxVelocityForce = 500.0
	def.AngularVelocity = 3.0
	def.MaxVelocityTorque = 500.0
	jointID := w.CreateMotorJoint(&def)

	for range 60 {
		w.Step(motorTimeStep, 4)
	}

	v := w.BodyLinearVelocity(body)
	require.InDelta(t, 2.0, v.X, 0.05, "velocity motor should reach the requested linear velocity")
	require.InDelta(t, 0.0, v.Y, 0.05)
	require.InDelta(t, 3.0, w.BodyAngularVelocity(body), 0.05, "velocity motor should reach the requested angular velocity")

	// Clamping the motor to zero force and torque frees the body.
	w.SetMotorJointMaxVelocityForce(jointID, 0.0)
	w.SetMotorJointMaxVelocityTorque(jointID, 0.0)
	w.SetBodyLinearVelocity(body, box2d.Vec2{X: 5.0, Y: 0.0})
	w.SetBodyAngularVelocity(body, 5.0)

	for range 30 {
		w.Step(motorTimeStep, 4)
	}

	require.InDelta(t, 5.0, w.BodyLinearVelocity(body).X, 1.0e-9, "a disabled velocity motor must not act")
	require.InDelta(t, 5.0, w.BodyAngularVelocity(body), 1.0e-9, "a disabled velocity motor must not act")
}

func TestMotorJointSpringHertzStrength(t *testing.T) {
	t.Parallel()

	// Sanity check on the position correction strength: a stiffer spring
	// pulls the body back to the target faster. Upstream v3.0 exposed a
	// correctionFactor; v3.2 replaces it with the hertz/damping softness.
	settle := func(hertz float64) float64 {
		w := motorTestWorld(box2d.Vec2Zero)
		defer w.Destroy()

		ground := motorTestGround(w)
		body := motorTestBoxBody(w, box2d.Vec2{X: 2.0, Y: 0.0}, 0.5)
		motorTestJoint(w, ground, body, hertz, 1.0e4, 1.0e4)

		for range 20 {
			w.Step(motorTimeStep, 4)
		}
		return math.Abs(w.BodyPosition(body).X)
	}

	soft := settle(1.0)
	stiff := settle(10.0)
	t.Logf("soft residual %g m, stiff residual %g m", soft, stiff)

	require.Less(t, stiff, soft, "a stiffer motor spring must correct the offset faster")
	require.Less(t, stiff, 0.5, "a stiff motor spring should mostly close a 2 m offset in 20 steps")
}

func TestMotorJointReactionGetters(t *testing.T) {
	t.Parallel()

	w := motorTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	ground := motorTestGround(w)
	// Anchor the motor away from the center of mass so it must also carry a
	// torque.
	body := motorTestBoxBody(w, box2d.Vec2Zero, 0.5)

	def := box2d.DefaultMotorJointDef()
	def.Base.BodyIDA = ground
	def.Base.BodyIDB = body
	def.Base.LocalFrameA.P = box2d.Vec2{X: 1.0, Y: 0.0}
	def.Base.LocalFrameB.P = box2d.Vec2{X: 1.0, Y: 0.0}
	def.LinearHertz = 20.0
	def.LinearDampingRatio = 1.0
	def.MaxSpringForce = 1.0e4
	def.AngularHertz = 20.0
	def.AngularDampingRatio = 1.0
	def.MaxSpringTorque = 1.0e4
	jointID := w.CreateMotorJoint(&def)

	for range 180 {
		w.Step(motorTimeStep, 4)
	}

	force := w.JointConstraintForce(jointID)
	torque := w.JointConstraintTorque(jointID)
	t.Logf("force (%g, %g) N, torque %g N*m", force.X, force.Y, torque)

	require.True(t, box2d.IsValidVec2(force))
	require.True(t, box2d.IsValidFloat(torque))
	// The motor carries the full weight of the 1 kg body.
	require.InDelta(t, 10.0, box2d.Length(force), 1.0, "motor force should balance gravity")
	require.Less(t, math.Abs(torque), 1.0e4, "motor torque must respect the spring torque clamp")
}

func TestMotorJointAccessorRoundTrip(t *testing.T) {
	t.Parallel()

	w := motorTestWorld(box2d.Vec2Zero)
	defer w.Destroy()

	ground := motorTestGround(w)
	body := motorTestBoxBody(w, box2d.Vec2Zero, 0.5)

	def := box2d.DefaultMotorJointDef()
	def.Base.BodyIDA = ground
	def.Base.BodyIDB = body
	jointID := w.CreateMotorJoint(&def)

	w.SetMotorJointLinearVelocity(jointID, box2d.Vec2{X: 1.5, Y: -2.5})
	require.Equal(t, box2d.Vec2{X: 1.5, Y: -2.5}, w.MotorJointLinearVelocity(jointID))

	w.SetMotorJointAngularVelocity(jointID, 3.25)
	require.InDelta(t, 3.25, w.MotorJointAngularVelocity(jointID), 0.0)

	w.SetMotorJointMaxVelocityForce(jointID, 12.5)
	require.InDelta(t, 12.5, w.MotorJointMaxVelocityForce(jointID), 0.0)

	w.SetMotorJointMaxVelocityTorque(jointID, 7.5)
	require.InDelta(t, 7.5, w.MotorJointMaxVelocityTorque(jointID), 0.0)

	w.SetMotorJointLinearHertz(jointID, 4.5)
	require.InDelta(t, 4.5, w.MotorJointLinearHertz(jointID), 0.0)

	w.SetMotorJointLinearDampingRatio(jointID, 0.75)
	require.InDelta(t, 0.75, w.MotorJointLinearDampingRatio(jointID), 0.0)

	w.SetMotorJointAngularHertz(jointID, 6.5)
	require.InDelta(t, 6.5, w.MotorJointAngularHertz(jointID), 0.0)

	w.SetMotorJointAngularDampingRatio(jointID, 0.25)
	require.InDelta(t, 0.25, w.MotorJointAngularDampingRatio(jointID), 0.0)

	w.SetMotorJointMaxSpringForce(jointID, 33.0)
	require.InDelta(t, 33.0, w.MotorJointMaxSpringForce(jointID), 0.0)

	w.SetMotorJointMaxSpringTorque(jointID, 44.0)
	require.InDelta(t, 44.0, w.MotorJointMaxSpringTorque(jointID), 0.0)

	// The spring clamps are floored at zero upstream.
	w.SetMotorJointMaxSpringForce(jointID, -1.0)
	require.InDelta(t, 0.0, w.MotorJointMaxSpringForce(jointID), 0.0)

	w.SetMotorJointMaxSpringTorque(jointID, -1.0)
	require.InDelta(t, 0.0, w.MotorJointMaxSpringTorque(jointID), 0.0)
}

func TestMotorJointDestroyMidSimulation(t *testing.T) {
	t.Parallel()

	w := motorTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	ground := motorTestGround(w)
	body := motorTestBoxBody(w, box2d.Vec2Zero, 0.5)
	jointID := motorTestJoint(w, ground, body, 20.0, 1.0e4, 1.0e4)

	for range 30 {
		w.Step(motorTimeStep, 4)
	}
	held := w.BodyPosition(body).Y
	require.Less(t, math.Abs(held), 0.05, "motor should hold the body before the joint is destroyed")

	w.DestroyJoint(jointID, true)

	for range 60 {
		w.Step(motorTimeStep, 4)
	}
	require.Less(t, w.BodyPosition(body).Y, held-0.4, "the body must fall once the motor is gone")
}

// buildMotorDeterminismScene builds a motor-driven body plus a free box that
// it pushes around.
func buildMotorDeterminismScene(w *box2d.World) []box2d.BodyID {
	ground := motorTestGround(w)

	driver := motorTestBoxBody(w, box2d.Vec2{X: -2.0, Y: 0.5}, 0.5)
	cargo := motorTestBoxBody(w, box2d.Vec2{X: 1.0, Y: 0.5}, 0.5)

	def := box2d.DefaultMotorJointDef()
	def.Base.BodyIDA = ground
	def.Base.BodyIDB = driver
	def.Base.LocalFrameA.P = box2d.Vec2{X: -2.0, Y: 0.5}
	def.LinearVelocity = box2d.Vec2{X: 1.5, Y: 0.0}
	def.MaxVelocityForce = 200.0
	def.AngularVelocity = 0.0
	def.MaxVelocityTorque = 50.0
	def.LinearHertz = 2.0
	def.LinearDampingRatio = 0.5
	def.MaxSpringForce = 100.0
	def.AngularHertz = 2.0
	def.AngularDampingRatio = 0.5
	def.MaxSpringTorque = 50.0
	w.CreateMotorJoint(&def)

	return []box2d.BodyID{driver, cargo}
}

func TestMotorJointDeterminism(t *testing.T) {
	t.Parallel()

	run := func() []uint64 {
		w := motorTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
		defer w.Destroy()

		bodies := buildMotorDeterminismScene(w)
		hashes := make([]uint64, 0, 200)
		for range 200 {
			w.Step(motorTimeStep, 4)
			hashes = append(hashes, hashWorldState(w, bodies))
		}
		return hashes
	}

	first := run()
	second := run()
	require.Equal(t, first, second, "motor joint stepping must be bit-identical across worlds")
}
