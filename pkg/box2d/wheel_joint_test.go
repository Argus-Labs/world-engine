// Behavior tests for the float64 port of the Box2D v3.2.0 wheel joint (stage
// E9b): suspension spring sag, rotational motor, translation limits, the
// point-to-line constraint, accessor round-trips and cross-world determinism.

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

const wheelTimeStep = 1.0 / 60.0

// wheelRig is a single suspension corner: a static chassis anchor and a
// dynamic wheel that may slide along the world +y axis and spin freely.
type wheelRig struct {
	world  *box2d.World
	anchor box2d.BodyID
	wheel  box2d.BodyID
	joint  box2d.JointID
}

// buildWheelRig creates the suspension rig. The joint local frames are rotated
// by 90 degrees so the joint x-axis (the suspension travel) points along world
// +y.
func buildWheelRig(gravity box2d.Vec2, tune func(*box2d.WheelJointDef)) wheelRig {
	wd := box2d.DefaultWorldDef()
	wd.Gravity = gravity
	w := box2d.NewWorld(&wd)

	ad := box2d.DefaultBodyDef()
	ad.Position = box2d.Vec2{X: 0.0, Y: 0.0}
	anchor := w.CreateBody(&ad)

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: 0.0, Y: 0.0}
	bd.EnableSleep = false
	wheel := w.CreateBody(&bd)

	circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.35}
	sd := box2d.DefaultShapeDef()
	sd.Density = 1.0
	w.CreateCircleShape(wheel, &sd, &circle)

	axisRot := box2d.MakeRot(0.5 * box2d.Pi)

	jd := box2d.DefaultWheelJointDef()
	jd.Base.BodyIDA = anchor
	jd.Base.BodyIDB = wheel
	jd.Base.LocalFrameA.Q = axisRot
	jd.Base.LocalFrameB.Q = axisRot
	if tune != nil {
		tune(&jd)
	}
	joint := w.CreateWheelJoint(&jd)

	return wheelRig{world: w, anchor: anchor, wheel: wheel, joint: joint}
}

// wheelTranslation measures the suspension travel directly from the body
// positions (the wheel joint has no upstream translation getter).
func wheelTranslation(rig wheelRig) float64 {
	return rig.world.BodyPosition(rig.wheel).Y - rig.world.BodyPosition(rig.anchor).Y
}

func TestWheelJointSuspensionSag(t *testing.T) {
	t.Parallel()

	const hertz = 2.0
	const gravity = -10.0

	rig := buildWheelRig(box2d.Vec2{X: 0.0, Y: gravity}, func(jd *box2d.WheelJointDef) {
		jd.EnableSpring = true
		jd.Hertz = hertz
		jd.DampingRatio = 0.15
	})
	defer rig.world.Destroy()

	// The spring stiffness of a soft constraint at f Hz is k = m * (2*pi*f)^2,
	// so the static sag is m*g/k = g / (2*pi*f)^2 and is mass independent.
	omega := 2.0 * box2d.Pi * hertz
	wantSag := -gravity / (omega * omega)

	// The rig must oscillate before it settles: track the extremes.
	minTranslation := 0.0
	for range 120 {
		rig.world.Step(wheelTimeStep, 4)
		minTranslation = math.Min(minTranslation, wheelTranslation(rig))
	}
	require.Less(t, minTranslation, -wantSag*1.2, "under-damped spring must overshoot the static sag")

	for range 900 {
		rig.world.Step(wheelTimeStep, 4)
	}

	translation := wheelTranslation(rig)
	require.InDelta(t, -wantSag, translation, 0.25*wantSag, "suspension must settle at m*g/k")
	require.InDelta(t, 0.0, rig.world.BodyLinearVelocity(rig.wheel).Y, 1e-3, "suspension must come to rest")

	// A stiffer spring must sag less.
	stiff := buildWheelRig(box2d.Vec2{X: 0.0, Y: gravity}, func(jd *box2d.WheelJointDef) {
		jd.EnableSpring = true
		jd.Hertz = 2.0 * hertz
		jd.DampingRatio = 0.15
	})
	defer stiff.world.Destroy()

	for range 1020 {
		stiff.world.Step(wheelTimeStep, 4)
	}
	require.Greater(t, wheelTranslation(stiff), translation, "doubling the hertz must reduce the sag")
}

func TestWheelJointAxisConstraint(t *testing.T) {
	t.Parallel()

	rig := buildWheelRig(box2d.Vec2{X: 0.0, Y: -10.0}, nil)
	defer rig.world.Destroy()

	for range 300 {
		rig.world.ApplyBodyForceToCenter(rig.wheel, box2d.Vec2{X: 30.0, Y: 0.0}, true)
		rig.world.Step(wheelTimeStep, 4)

		require.Less(t, math.Abs(rig.world.BodyPosition(rig.wheel).X), 1e-3,
			"point-to-line constraint must hold the wheel on the axis")
	}
}

func TestWheelJointMotor(t *testing.T) {
	t.Parallel()

	const motorSpeed = 5.0

	rig := buildWheelRig(box2d.Vec2Zero, func(jd *box2d.WheelJointDef) {
		jd.EnableSpring = false
		jd.EnableMotor = true
		jd.MotorSpeed = motorSpeed
		jd.MaxMotorTorque = 100.0
	})
	defer rig.world.Destroy()

	for range 120 {
		rig.world.Step(wheelTimeStep, 4)
	}

	require.InDelta(t, motorSpeed, rig.world.BodyAngularVelocity(rig.wheel), 1e-3,
		"motor must spin the wheel up to the commanded speed")

	// Reverse the motor and check it comes back the other way.
	rig.world.SetWheelJointMotorSpeed(rig.joint, -motorSpeed)
	for range 120 {
		rig.world.Step(wheelTimeStep, 4)
	}
	require.InDelta(t, -motorSpeed, rig.world.BodyAngularVelocity(rig.wheel), 1e-3)

	// A saturated motor cannot exceed the torque budget.
	rig.world.SetWheelJointMaxMotorTorque(rig.joint, 0.0)
	for range 30 {
		rig.world.Step(wheelTimeStep, 4)
	}
	require.InDelta(t, 0.0, rig.world.WheelJointMotorTorque(rig.joint), 1e-9)
}

func TestWheelJointLimits(t *testing.T) {
	t.Parallel()

	const lower = -0.2
	const upper = 0.4

	rig := buildWheelRig(box2d.Vec2{X: 0.0, Y: -10.0}, func(jd *box2d.WheelJointDef) {
		jd.EnableSpring = false
		jd.EnableLimit = true
		jd.LowerTranslation = lower
		jd.UpperTranslation = upper
	})
	defer rig.world.Destroy()

	for range 300 {
		rig.world.Step(wheelTimeStep, 4)

		translation := wheelTranslation(rig)
		require.GreaterOrEqual(t, translation, lower-0.02, "lower limit breached")
		require.LessOrEqual(t, translation, upper+0.02, "upper limit breached")
	}

	require.InDelta(t, lower, wheelTranslation(rig), 0.02, "wheel must rest on the lower stop")

	// Kick it upward hard: the upper stop must catch it.
	rig.world.ApplyBodyLinearImpulseToCenter(rig.wheel, box2d.Vec2{X: 0.0, Y: 8.0}, true)
	for range 120 {
		rig.world.Step(wheelTimeStep, 4)
		require.LessOrEqual(t, wheelTranslation(rig), upper+0.05, "upper limit breached after impulse")
	}
}

func TestWheelJointAccessorRoundTrip(t *testing.T) {
	t.Parallel()

	rig := buildWheelRig(box2d.Vec2{X: 0.0, Y: -10.0}, nil)
	defer rig.world.Destroy()

	w := rig.world
	id := rig.joint

	// DefaultWheelJointDef enables the spring at 1 Hz / 0.7 damping.
	require.True(t, w.IsWheelJointSpringEnabled(id))
	require.InDelta(t, 1.0, w.WheelJointSpringHertz(id), 0.0)
	require.InDelta(t, 0.7, w.WheelJointSpringDampingRatio(id), 0.0)

	w.EnableWheelJointSpring(id, false)
	require.False(t, w.IsWheelJointSpringEnabled(id))

	w.SetWheelJointSpringHertz(id, 5.5)
	require.InDelta(t, 5.5, w.WheelJointSpringHertz(id), 0.0)

	w.SetWheelJointSpringDampingRatio(id, 0.25)
	require.InDelta(t, 0.25, w.WheelJointSpringDampingRatio(id), 0.0)

	require.False(t, w.IsWheelJointLimitEnabled(id))
	w.EnableWheelJointLimit(id, true)
	require.True(t, w.IsWheelJointLimitEnabled(id))

	w.SetWheelJointLimits(id, -0.75, 1.25)
	require.InDelta(t, -0.75, w.WheelJointLowerLimit(id), 0.0)
	require.InDelta(t, 1.25, w.WheelJointUpperLimit(id), 0.0)

	require.False(t, w.IsWheelJointMotorEnabled(id))
	w.EnableWheelJointMotor(id, true)
	require.True(t, w.IsWheelJointMotorEnabled(id))

	w.SetWheelJointMotorSpeed(id, -3.25)
	require.InDelta(t, -3.25, w.WheelJointMotorSpeed(id), 0.0)

	w.SetWheelJointMaxMotorTorque(id, 42.0)
	require.InDelta(t, 42.0, w.WheelJointMaxMotorTorque(id), 0.0)

	// Toggling the motor off clears the accumulated impulse.
	w.Step(wheelTimeStep, 4)
	w.EnableWheelJointMotor(id, false)
	require.InDelta(t, 0.0, w.WheelJointMotorTorque(id), 0.0)

	// Reactions stay finite.
	w.Step(wheelTimeStep, 4)
	force := w.JointConstraintForce(id)
	require.True(t, box2d.IsValidVec2(force))
	require.True(t, box2d.IsValidFloat(w.JointConstraintTorque(id)))
}

func TestWheelJointDeterminism(t *testing.T) {
	t.Parallel()

	run := func() []float64 {
		rig := buildWheelRig(box2d.Vec2{X: 0.0, Y: -10.0}, func(jd *box2d.WheelJointDef) {
			jd.Hertz = 3.0
			jd.DampingRatio = 0.2
			jd.EnableLimit = true
			jd.LowerTranslation = -0.75
			jd.UpperTranslation = 0.75
			jd.EnableMotor = true
			jd.MotorSpeed = 6.0
			jd.MaxMotorTorque = 20.0
		})
		defer rig.world.Destroy()

		var samples []float64
		for range 200 {
			rig.world.ApplyBodyForceToCenter(rig.wheel, box2d.Vec2{X: 4.0, Y: 0.0}, true)
			rig.world.Step(wheelTimeStep, 4)

			p := rig.world.BodyPosition(rig.wheel)
			v := rig.world.BodyLinearVelocity(rig.wheel)
			f := rig.world.JointConstraintForce(rig.joint)
			samples = append(samples, p.X, p.Y, v.X, v.Y,
				rig.world.BodyAngularVelocity(rig.wheel),
				rig.world.WheelJointMotorTorque(rig.joint),
				f.X, f.Y)
		}
		return samples
	}

	first := run()
	second := run()

	require.Len(t, second, len(first))
	for i := range first {
		require.Equal(t, math.Float64bits(first[i]), math.Float64bits(second[i]),
			"wheel joint scene is not bit-identical at sample %d", i)
	}
}
