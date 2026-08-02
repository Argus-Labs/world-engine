// Behavior tests for the float64 port of the Box2D v3.2.0 prismatic joint
// (stage E9b): axis (point-to-line) constraint, translation limits, motor,
// linear spring, translation/speed getters, accessor round-trips and
// cross-world determinism.

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

const prismaticTimeStep = 1.0 / 60.0

// prismaticRig is a vertical "elevator": a static anchor at the origin and a
// dynamic box that may only slide along the world +y axis.
type prismaticRig struct {
	world    *box2d.World
	anchor   box2d.BodyID
	elevator box2d.BodyID
	joint    box2d.JointID
}

// buildPrismaticRig creates the elevator rig. The joint local frames are
// rotated by 90 degrees so the joint x-axis points along world +y.
func buildPrismaticRig(gravity box2d.Vec2, tune func(*box2d.PrismaticJointDef)) prismaticRig {
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
	elevator := w.CreateBody(&bd)

	box := box2d.MakeBox(0.5, 0.25)
	sd := box2d.DefaultShapeDef()
	sd.Density = 1.0
	w.CreatePolygonShape(elevator, &sd, &box)

	axisRot := box2d.MakeRot(0.5 * box2d.Pi)

	jd := box2d.DefaultPrismaticJointDef()
	jd.Base.BodyIDA = anchor
	jd.Base.BodyIDB = elevator
	jd.Base.LocalFrameA.Q = axisRot
	jd.Base.LocalFrameB.Q = axisRot
	if tune != nil {
		tune(&jd)
	}
	joint := w.CreatePrismaticJoint(&jd)

	return prismaticRig{world: w, anchor: anchor, elevator: elevator, joint: joint}
}

func TestPrismaticJointAxisConstraint(t *testing.T) {
	t.Parallel()

	rig := buildPrismaticRig(box2d.Vec2{X: 0.0, Y: -10.0}, nil)
	defer rig.world.Destroy()

	// Push sideways every step: the point-to-line constraint must absorb it.
	for range 300 {
		rig.world.ApplyBodyForceToCenter(rig.elevator, box2d.Vec2{X: 50.0, Y: 0.0}, true)
		rig.world.Step(prismaticTimeStep, 4)

		p := rig.world.BodyPosition(rig.elevator)
		require.Less(t, math.Abs(p.X), 1e-3, "lateral drift off the joint axis")
	}

	angle := box2d.RotGetAngle(rig.world.BodyRotation(rig.elevator))
	require.Less(t, math.Abs(angle), 1e-3, "prismatic joint must forbid relative rotation")

	// Free fall along the axis must still happen.
	require.Less(t, rig.world.BodyPosition(rig.elevator).Y, -10.0)
}

func TestPrismaticJointLimits(t *testing.T) {
	t.Parallel()

	const lower = -1.0
	const upper = 0.25

	rig := buildPrismaticRig(box2d.Vec2{X: 0.0, Y: -10.0}, func(jd *box2d.PrismaticJointDef) {
		jd.EnableLimit = true
		jd.LowerTranslation = lower
		jd.UpperTranslation = upper
	})
	defer rig.world.Destroy()

	for range 300 {
		rig.world.Step(prismaticTimeStep, 4)

		translation := rig.world.PrismaticJointTranslation(rig.joint)
		require.GreaterOrEqual(t, translation, lower-0.05, "lower limit breached")
		require.LessOrEqual(t, translation, upper+0.05, "upper limit breached")
	}

	// Gravity must have driven the elevator down onto the lower stop.
	translation := rig.world.PrismaticJointTranslation(rig.joint)
	require.InDelta(t, lower, translation, 0.05, "elevator must rest on the lower limit")
}

func TestPrismaticJointMotor(t *testing.T) {
	t.Parallel()

	const motorSpeed = 2.0

	rig := buildPrismaticRig(box2d.Vec2{X: 0.0, Y: -10.0}, func(jd *box2d.PrismaticJointDef) {
		jd.EnableMotor = true
		jd.MotorSpeed = motorSpeed
		jd.MaxMotorForce = 1000.0
	})
	defer rig.world.Destroy()

	for range 120 {
		rig.world.Step(prismaticTimeStep, 4)
	}

	speed := rig.world.PrismaticJointSpeed(rig.joint)
	require.InDelta(t, motorSpeed, speed, 0.05, "motor must hold the commanded speed")

	// Two seconds of climbing at ~2 m/s.
	require.Greater(t, rig.world.PrismaticJointTranslation(rig.joint), 3.5)

	// The motor force must oppose gravity (mass 0.5 kg, g = 10 m/s^2).
	force := rig.world.PrismaticJointMotorForce(rig.joint)
	require.InDelta(t, 5.0, force, 1.0)
}

func TestPrismaticJointSpring(t *testing.T) {
	t.Parallel()

	const target = -0.75

	rig := buildPrismaticRig(box2d.Vec2Zero, func(jd *box2d.PrismaticJointDef) {
		jd.EnableSpring = true
		jd.Hertz = 2.0
		jd.DampingRatio = 0.7
		jd.TargetTranslation = target
	})
	defer rig.world.Destroy()

	for range 600 {
		rig.world.Step(prismaticTimeStep, 4)
	}

	translation := rig.world.PrismaticJointTranslation(rig.joint)
	require.InDelta(t, target, translation, 0.02, "spring must settle at the target translation")
	require.InDelta(t, 0.0, rig.world.PrismaticJointSpeed(rig.joint), 0.02, "spring must come to rest")
}

func TestPrismaticJointTranslationAndSpeedGetters(t *testing.T) {
	t.Parallel()

	rig := buildPrismaticRig(box2d.Vec2{X: 0.0, Y: -10.0}, nil)
	defer rig.world.Destroy()

	for range 60 {
		rig.world.Step(prismaticTimeStep, 4)

		anchorPos := rig.world.BodyPosition(rig.anchor)
		elevatorPos := rig.world.BodyPosition(rig.elevator)
		wantTranslation := elevatorPos.Y - anchorPos.Y
		require.InDelta(t, wantTranslation, rig.world.PrismaticJointTranslation(rig.joint), 1e-9)

		wantSpeed := rig.world.BodyLinearVelocity(rig.elevator).Y
		require.InDelta(t, wantSpeed, rig.world.PrismaticJointSpeed(rig.joint), 1e-9)
	}
}

func TestPrismaticJointOffAxisImpulse(t *testing.T) {
	t.Parallel()

	// Motor locked at zero speed: the rig cannot translate, and the angular
	// row of the block constraint must reject the off-axis impulse.
	rig := buildPrismaticRig(box2d.Vec2Zero, func(jd *box2d.PrismaticJointDef) {
		jd.EnableMotor = true
		jd.MotorSpeed = 0.0
		jd.MaxMotorForce = 1.0e5
	})
	defer rig.world.Destroy()

	// Off-center, off-axis kick.
	rig.world.ApplyBodyLinearImpulse(rig.elevator,
		box2d.Vec2{X: 2.0, Y: 1.0}, box2d.Vec2{X: 0.5, Y: 0.25}, true)

	for range 120 {
		rig.world.Step(prismaticTimeStep, 4)

		angle := box2d.RotGetAngle(rig.world.BodyRotation(rig.elevator))
		require.Less(t, math.Abs(angle), 5e-3, "locked prismatic rig must not rotate")
		require.Less(t, math.Abs(rig.world.BodyPosition(rig.elevator).X), 1e-3)
	}

	require.Less(t, math.Abs(rig.world.PrismaticJointTranslation(rig.joint)), 0.05)
}

func TestPrismaticJointAccessorRoundTrip(t *testing.T) {
	t.Parallel()

	rig := buildPrismaticRig(box2d.Vec2{X: 0.0, Y: -10.0}, nil)
	defer rig.world.Destroy()

	w := rig.world
	id := rig.joint

	require.False(t, w.IsPrismaticJointSpringEnabled(id))
	w.EnablePrismaticJointSpring(id, true)
	require.True(t, w.IsPrismaticJointSpringEnabled(id))

	w.SetPrismaticJointSpringHertz(id, 3.5)
	require.InDelta(t, 3.5, w.PrismaticJointSpringHertz(id), 0.0)

	w.SetPrismaticJointSpringDampingRatio(id, 0.4)
	require.InDelta(t, 0.4, w.PrismaticJointSpringDampingRatio(id), 0.0)

	w.SetPrismaticJointTargetTranslation(id, -1.25)
	require.InDelta(t, -1.25, w.PrismaticJointTargetTranslation(id), 0.0)

	require.False(t, w.IsPrismaticJointLimitEnabled(id))
	w.EnablePrismaticJointLimit(id, true)
	require.True(t, w.IsPrismaticJointLimitEnabled(id))

	w.SetPrismaticJointLimits(id, -2.0, 3.0)
	require.InDelta(t, -2.0, w.PrismaticJointLowerLimit(id), 0.0)
	require.InDelta(t, 3.0, w.PrismaticJointUpperLimit(id), 0.0)

	require.False(t, w.IsPrismaticJointMotorEnabled(id))
	w.EnablePrismaticJointMotor(id, true)
	require.True(t, w.IsPrismaticJointMotorEnabled(id))

	w.SetPrismaticJointMotorSpeed(id, -1.5)
	require.InDelta(t, -1.5, w.PrismaticJointMotorSpeed(id), 0.0)

	w.SetPrismaticJointMaxMotorForce(id, 250.0)
	require.InDelta(t, 250.0, w.PrismaticJointMaxMotorForce(id), 0.0)

	// Toggling the motor off clears the accumulated impulse, so the reported
	// motor force drops to zero.
	w.Step(prismaticTimeStep, 4)
	w.EnablePrismaticJointMotor(id, false)
	require.InDelta(t, 0.0, w.PrismaticJointMotorForce(id), 0.0)
}

func TestPrismaticJointReactions(t *testing.T) {
	t.Parallel()

	rig := buildPrismaticRig(box2d.Vec2{X: 0.0, Y: -10.0}, func(jd *box2d.PrismaticJointDef) {
		jd.EnableLimit = true
		jd.LowerTranslation = -0.5
		jd.UpperTranslation = 0.5
	})
	defer rig.world.Destroy()

	for range 240 {
		rig.world.ApplyBodyForceToCenter(rig.elevator, box2d.Vec2{X: 20.0, Y: 0.0}, true)
		rig.world.Step(prismaticTimeStep, 4)
	}

	// Resting on the lower stop while pushed sideways: the constraint force
	// carries both the sideways push and the weight.
	force := rig.world.JointConstraintForce(rig.joint)
	require.Greater(t, math.Abs(force.X), 10.0)
	require.Greater(t, math.Abs(force.Y), 1.0)
	require.True(t, box2d.IsValidFloat(rig.world.JointConstraintTorque(rig.joint)))
}

func TestPrismaticJointDeterminism(t *testing.T) {
	t.Parallel()

	run := func() []float64 {
		rig := buildPrismaticRig(box2d.Vec2{X: 0.0, Y: -10.0}, func(jd *box2d.PrismaticJointDef) {
			jd.EnableLimit = true
			jd.LowerTranslation = -1.5
			jd.UpperTranslation = 1.5
			jd.EnableSpring = true
			jd.Hertz = 2.5
			jd.DampingRatio = 0.3
		})
		defer rig.world.Destroy()

		var samples []float64
		for range 200 {
			rig.world.ApplyBodyForceToCenter(rig.elevator, box2d.Vec2{X: 7.0, Y: 0.0}, true)
			rig.world.Step(prismaticTimeStep, 4)

			p := rig.world.BodyPosition(rig.elevator)
			v := rig.world.BodyLinearVelocity(rig.elevator)
			samples = append(samples, p.X, p.Y, v.X, v.Y,
				rig.world.BodyAngularVelocity(rig.elevator),
				rig.world.PrismaticJointTranslation(rig.joint),
				rig.world.PrismaticJointSpeed(rig.joint))
		}
		return samples
	}

	first := run()
	second := run()

	require.Len(t, second, len(first))
	for i := range first {
		require.Equal(t, math.Float64bits(first[i]), math.Float64bits(second[i]),
			"prismatic joint scene is not bit-identical at sample %d", i)
	}
}
