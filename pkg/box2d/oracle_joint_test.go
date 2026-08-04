// Oracle tests for the Go port of the Box2D v3.2.0 joints.
//
// Every expectation in this file is derived from the C source of truth that
// was ported, or from the upstream documentation — never from the Go
// implementation's own output:
//
//   - src/joint.c           b2Joint_*, b2GetJointReaction, b2DrawJoint
//   - src/distance_joint.c  b2DistanceJoint_*, b2SolveDistanceJoint, b2DrawDistanceJoint
//   - src/revolute_joint.c  b2RevoluteJoint_*, b2SolveRevoluteJoint, b2DrawRevoluteJoint
//   - src/prismatic_joint.c b2PrismaticJoint_*, b2SolvePrismaticJoint, b2DrawPrismaticJoint
//   - src/motor_joint.c     b2MotorJoint_*
//   - src/weld_joint.c      b2WeldJoint_*, b2DrawWeldJoint
//   - src/wheel_joint.c     b2WheelJoint_*, b2SolveWheelJoint, b2DrawWheelJoint
//   - include/box2d/box2d.h documented per-function contracts
//   - docs/simulation.md    "Joints" section behavioral contracts
//
// Each nontrivial expectation carries a `file:line` citation into that C
// source. Behavioral outcomes (motors reaching speed, limits stopping motion,
// rigid joints holding their rest length) get analytic bounds from the physics
// plus the C constants, never a pasted Go result.
//
// Note on preconditions: the default build compiles B2_ASSERT out
// (core_asserts_off.go debugAsserts == false), so the C `B2_ASSERT`
// preconditions are only observable as panics under the box2d_asserts build
// tag, and are not asserted here; these tests never feed assert-tripping
// inputs, so they run identically in both builds.

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

const (
	// oracleJointDT and oracleJointSubSteps are the sample-app defaults. They
	// fix context->inv_h == subStepCount / dt == 240, which world->inv_h is
	// set from: every b2*Joint_Get*Force/Torque getter scales the stored
	// impulse by inv_h, so the solver's impulse clamps translate one-to-one
	// into force and torque bounds.
	oracleJointDT       = 1.0 / 60.0
	oracleJointSubSteps = 4

	// oracleAtan2Tolerance is the documented accuracy of b2Atan2
	// (math_functions.c, "atan2 approximation with 11 bits in mantissa"),
	// which the port keeps verbatim in math_trig.go: about 0.0023 degrees.
	// b2RelativeAngle bottoms out in it, so every angle the joint API reports
	// carries at most this much error.
	oracleAtan2Tolerance = 0.0023 * math.Pi / 180.0
)

// ---------------------------------------------------------------------------
// Scene helpers
// ---------------------------------------------------------------------------

func oracleJointWorld(t *testing.T, gravity box2d.Vec2) *box2d.World {
	t.Helper()

	def := box2d.DefaultWorldDef()
	def.Gravity = gravity
	world := box2d.NewWorld(&def)
	t.Cleanup(world.Destroy)
	return world
}

// oracleJointBody creates a body carrying one 0.5x0.5 box so the broad phase
// (and therefore World.Draw, which only visits bodies it finds through the
// trees) can see it. Density is the ShapeDef default of 1, hence mass 0.25.
func oracleJointBody(t *testing.T, world *box2d.World, bodyType box2d.BodyType,
	position box2d.Vec2, rotation box2d.Rot,
) box2d.BodyID {
	t.Helper()

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = bodyType
	bodyDef.Position = position
	bodyDef.Rotation = rotation
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	box := box2d.MakeBox(0.25, 0.25)
	world.CreatePolygonShape(bodyID, &shapeDef, &box)
	return bodyID
}

func oracleJointStep(world *box2d.World, steps int) {
	for range steps {
		world.Step(oracleJointDT, oracleJointSubSteps)
	}
}

// oracleJointSlop is the position tolerance a satisfied constraint may keep:
// four times B2_LINEAR_SLOP (include/box2d/constants.h:24). It lives in a
// variable rather than an inline product because the package forbids fused
// multiply-add in compiled test code (nofma_test.go), and `x + 4.0 * slop`
// compiles to an FMA.
var oracleJointSlop = 4.0 * box2d.LinearSlop

// oracleExactRot builds a rotation from the double-precision cosine and sine
// of an angle. b2MakeRot goes through b2ComputeCosSin, a polynomial
// approximation good to roughly 1e-3 radians, so the exact form is required
// whenever a test compares a reported angle against the angle it built the
// rotation from.
func oracleExactRot(radians float64) box2d.Rot {
	return box2d.Rot{C: math.Cos(radians), S: math.Sin(radians)}
}

func oracleDistanceJoint(world *box2d.World, bodyA, bodyB box2d.BodyID,
	tune func(*box2d.DistanceJointDef),
) box2d.JointID {
	def := box2d.DefaultDistanceJointDef()
	def.Base.BodyIDA = bodyA
	def.Base.BodyIDB = bodyB
	if tune != nil {
		tune(&def)
	}
	return world.CreateDistanceJoint(&def)
}

func oracleRevoluteJoint(world *box2d.World, bodyA, bodyB box2d.BodyID,
	tune func(*box2d.RevoluteJointDef),
) box2d.JointID {
	def := box2d.DefaultRevoluteJointDef()
	def.Base.BodyIDA = bodyA
	def.Base.BodyIDB = bodyB
	if tune != nil {
		tune(&def)
	}
	return world.CreateRevoluteJoint(&def)
}

func oraclePrismaticJoint(world *box2d.World, bodyA, bodyB box2d.BodyID,
	tune func(*box2d.PrismaticJointDef),
) box2d.JointID {
	def := box2d.DefaultPrismaticJointDef()
	def.Base.BodyIDA = bodyA
	def.Base.BodyIDB = bodyB
	if tune != nil {
		tune(&def)
	}
	return world.CreatePrismaticJoint(&def)
}

func oracleWheelJoint(world *box2d.World, bodyA, bodyB box2d.BodyID,
	tune func(*box2d.WheelJointDef),
) box2d.JointID {
	def := box2d.DefaultWheelJointDef()
	def.Base.BodyIDA = bodyA
	def.Base.BodyIDB = bodyB
	if tune != nil {
		tune(&def)
	}
	return world.CreateWheelJoint(&def)
}

func oracleWeldJoint(world *box2d.World, bodyA, bodyB box2d.BodyID,
	tune func(*box2d.WeldJointDef),
) box2d.JointID {
	def := box2d.DefaultWeldJointDef()
	def.Base.BodyIDA = bodyA
	def.Base.BodyIDB = bodyB
	if tune != nil {
		tune(&def)
	}
	return world.CreateWeldJoint(&def)
}

func oracleMotorJoint(world *box2d.World, bodyA, bodyB box2d.BodyID,
	tune func(*box2d.MotorJointDef),
) box2d.JointID {
	def := box2d.DefaultMotorJointDef()
	def.Base.BodyIDA = bodyA
	def.Base.BodyIDB = bodyB
	if tune != nil {
		tune(&def)
	}
	return world.CreateMotorJoint(&def)
}

func oracleFilterJoint(world *box2d.World, bodyA, bodyB box2d.BodyID) box2d.JointID {
	def := box2d.DefaultFilterJointDef()
	def.Base.BodyIDA = bodyA
	def.Base.BodyIDB = bodyB
	return world.CreateFilterJoint(&def)
}

// oracleJointHangingDistance builds a loaded rigid distance joint: a static
// anchor at the origin and a dynamic body one metre below it, pulled down by
// gravity. The joint therefore carries a sustained tension, so the stored
// impulse is non-zero and b2Joint_GetConstraintForce reports it.
func oracleJointHangingDistance(t *testing.T) (*box2d.World, box2d.JointID) {
	t.Helper()

	world := oracleJointWorld(t, box2d.Vec2{X: 0.0, Y: -10.0})
	anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
	load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 0.0, Y: -1.0}, box2d.RotIdentity)

	jointID := oracleDistanceJoint(world, anchor, load, func(def *box2d.DistanceJointDef) {
		def.Length = 1.0
	})
	oracleJointStep(world, 20)
	return world, jointID
}

// ---------------------------------------------------------------------------
// Distance joint accessors — src/distance_joint.c
// ---------------------------------------------------------------------------

func TestOracleDistanceJointAccessors(t *testing.T) {
	t.Parallel()

	t.Run("SetLengthClampsToLinearSlopAndHuge", func(t *testing.T) {
		t.Parallel()

		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 2.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, anchor, load, nil)

		// distance_joint.c:23
		//   joint->length = b2ClampFloat( length, B2_LINEAR_SLOP, B2_HUGE );
		world.SetDistanceJointLength(jointID, -5.0)
		require.InDelta(t, box2d.LinearSlop, world.DistanceJointLength(jointID), 0)

		world.SetDistanceJointLength(jointID, 0.0)
		require.InDelta(t, box2d.LinearSlop, world.DistanceJointLength(jointID), 0)

		world.SetDistanceJointLength(jointID, 4.0*box2d.Huge)
		require.InDelta(t, box2d.Huge, world.DistanceJointLength(jointID), 0)

		// A value inside the range passes through unchanged.
		world.SetDistanceJointLength(jointID, 2.5)
		require.InDelta(t, 2.5, world.DistanceJointLength(jointID), 0)
	})

	t.Run("SetLengthZeroesConstraintImpulses", func(t *testing.T) {
		t.Parallel()

		world, jointID := oracleJointHangingDistance(t)

		// Sanity: a hanging rigid distance joint carries tension, so the
		// stored impulse is non-zero.
		require.Greater(t, box2d.Length(world.JointConstraintForce(jointID)), 0.1)

		// distance_joint.c:24-26
		//   joint->impulse = 0.0f; joint->lowerImpulse = 0.0f; joint->upperImpulse = 0.0f;
		// b2GetDistanceJointForce (distance_joint.c:209) is
		//   ( impulse + lowerImpulse - upperImpulse + motorImpulse ) * inv_h
		// and the motor is disabled here, so the reported force must be
		// exactly zero after the setter.
		world.SetDistanceJointLength(jointID, 1.0)
		force := world.JointConstraintForce(jointID)
		require.InDelta(t, 0.0, force.X, 0)
		require.InDelta(t, 0.0, force.Y, 0)

		// distance_joint.c has no b2GetDistanceJointTorque; the dispatcher
		// returns 0 for a distance joint (joint.c:995-996).
		require.InDelta(t, 0.0, world.JointConstraintTorque(jointID), 0)
	})

	t.Run("SetLengthRangeClampsThenOrders", func(t *testing.T) {
		t.Parallel()

		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 2.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, anchor, load, nil)

		// distance_joint.c:54-57
		//   minLength = b2ClampFloat( minLength, B2_LINEAR_SLOP, B2_HUGE );
		//   maxLength = b2ClampFloat( maxLength, B2_LINEAR_SLOP, B2_HUGE );
		//   joint->minLength = b2MinFloat( minLength, maxLength );
		//   joint->maxLength = b2MaxFloat( minLength, maxLength );
		world.SetDistanceJointLengthRange(jointID, 0.5, 3.0)
		require.InDelta(t, 0.5, world.DistanceJointMinLength(jointID), 0)
		require.InDelta(t, 3.0, world.DistanceJointMaxLength(jointID), 0)

		// The min/max pair is ordered after clamping, so a reversed argument
		// pair yields the same stored range.
		world.SetDistanceJointLengthRange(jointID, 3.0, 0.5)
		require.InDelta(t, 0.5, world.DistanceJointMinLength(jointID), 0)
		require.InDelta(t, 3.0, world.DistanceJointMaxLength(jointID), 0)

		// Both bounds clamp before ordering.
		world.SetDistanceJointLengthRange(jointID, -1.0, 4.0*box2d.Huge)
		require.InDelta(t, box2d.LinearSlop, world.DistanceJointMinLength(jointID), 0)
		require.InDelta(t, box2d.Huge, world.DistanceJointMaxLength(jointID), 0)
	})

	t.Run("SetLengthRangeZeroesConstraintImpulses", func(t *testing.T) {
		t.Parallel()

		world, jointID := oracleJointHangingDistance(t)
		require.Greater(t, box2d.Length(world.JointConstraintForce(jointID)), 0.1)

		// distance_joint.c:58-60 — same three impulses are cleared.
		world.SetDistanceJointLengthRange(jointID, 0.5, 2.0)
		force := world.JointConstraintForce(jointID)
		require.InDelta(t, 0.0, force.X, 0)
		require.InDelta(t, 0.0, force.Y, 0)
	})

	t.Run("EnableLimitRoundTrip", func(t *testing.T) {
		t.Parallel()

		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 2.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, anchor, load, nil)

		// distance_joint.c:36-47 — plain store/load, no impulse reset.
		require.False(t, world.IsDistanceJointLimitEnabled(jointID))
		world.EnableDistanceJointLimit(jointID, true)
		require.True(t, world.IsDistanceJointLimitEnabled(jointID))
		world.EnableDistanceJointLimit(jointID, false)
		require.False(t, world.IsDistanceJointLimitEnabled(jointID))
	})

	t.Run("EnableSpringRoundTrip", func(t *testing.T) {
		t.Parallel()

		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 2.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, anchor, load, nil)

		// distance_joint.c:98-108 — plain store/load, no impulse reset.
		require.False(t, world.IsDistanceJointSpringEnabled(jointID))
		world.EnableDistanceJointSpring(jointID, true)
		require.True(t, world.IsDistanceJointSpringEnabled(jointID))
		world.EnableDistanceJointSpring(jointID, false)
		require.False(t, world.IsDistanceJointSpringEnabled(jointID))
	})

	t.Run("SpringTuningRoundTrip", func(t *testing.T) {
		t.Parallel()

		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 2.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, anchor, load, nil)

		// distance_joint.c:125-149 — hertz and damping ratio are stored verbatim.
		world.SetDistanceJointSpringHertz(jointID, 3.5)
		world.SetDistanceJointSpringDampingRatio(jointID, 0.25)
		require.InDelta(t, 3.5, world.DistanceJointSpringHertz(jointID), 0)
		require.InDelta(t, 0.25, world.DistanceJointSpringDampingRatio(jointID), 0)

		// distance_joint.c:110-123 — the force range is stored verbatim as a
		// (lower, upper) pair and read back through two out-parameters.
		// b2DefaultDistanceJointDef seeds it with (-FLT_MAX, FLT_MAX).
		lower, upper := world.DistanceJointSpringForceRange(jointID)
		require.InDelta(t, -math.MaxFloat32, lower, 0)
		require.InDelta(t, math.MaxFloat32, upper, 0)

		world.SetDistanceJointSpringForceRange(jointID, -12.0, 34.0)
		lower, upper = world.DistanceJointSpringForceRange(jointID)
		require.InDelta(t, -12.0, lower, 0)
		require.InDelta(t, 34.0, upper, 0)
	})

	t.Run("EnableMotorZeroesMotorImpulseOnChangeOnly", func(t *testing.T) {
		t.Parallel()

		// The distance-joint motor only runs inside the soft branch of
		// b2SolveDistanceJoint (distance_joint.c:366), which needs the spring
		// enabled. Hertz stays at zero so the spring itself applies nothing
		// and the motor is the only impulse source; gravity along the joint
		// axis then keeps the motor loaded, so its impulse cannot relax to
		// zero the way an unloaded motor's would.
		world := oracleJointWorld(t, box2d.Vec2{X: 0.0, Y: -10.0})
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 0.0, Y: -1.0}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, anchor, load, func(def *box2d.DistanceJointDef) {
			def.Length = 1.0
			def.EnableSpring = true
			def.Hertz = 0.0
			def.EnableMotor = true
			def.MotorSpeed = 0.0
			def.MaxMotorForce = 50.0
		})
		require.True(t, world.IsDistanceJointMotorEnabled(jointID))

		oracleJointStep(world, 10)
		running := world.DistanceJointMotorForce(jointID)
		require.Greater(t, math.Abs(running), 0.0)

		// distance_joint.c:154 — the reset is guarded by a change of state, so
		// re-enabling an already enabled motor must leave the impulse alone.
		world.EnableDistanceJointMotor(jointID, true)
		require.InDelta(t, running, world.DistanceJointMotorForce(jointID), 0)

		// distance_joint.c:156-157
		//   joint->distanceJoint.enableMotor = enableMotor;
		//   joint->distanceJoint.motorImpulse = 0.0f;
		world.EnableDistanceJointMotor(jointID, false)
		require.False(t, world.IsDistanceJointMotorEnabled(jointID))
		require.InDelta(t, 0.0, world.DistanceJointMotorForce(jointID), 0)
	})

	t.Run("MotorParametersRoundTrip", func(t *testing.T) {
		t.Parallel()

		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, anchor, load, nil)

		// distance_joint.c:167-177 and :186-196 — plain stores.
		world.SetDistanceJointMotorSpeed(jointID, -4.25)
		require.InDelta(t, -4.25, world.DistanceJointMotorSpeed(jointID), 0)

		world.SetDistanceJointMaxMotorForce(jointID, 17.5)
		require.InDelta(t, 17.5, world.DistanceJointMaxMotorForce(jointID), 0)

		// distance_joint.c:179-184 — the getter is inv_h * motorImpulse, and
		// world->inv_h is zero until the first step, so an unstepped world
		// reports zero motor force.
		require.InDelta(t, 0.0, world.DistanceJointMotorForce(jointID), 0)
	})

	t.Run("MotorForceRespectsMaxMotorForce", func(t *testing.T) {
		t.Parallel()

		// distance_joint.c:397-399
		//   float maxImpulse = context->h * joint->maxMotorForce;
		//   joint->motorImpulse = b2ClampFloat( ..., -maxImpulse, maxImpulse );
		// and distance_joint.c:183 reports world->inv_h * motorImpulse, so
		// |motorForce| <= maxMotorForce holds to within rounding.
		const maxMotorForce = 0.5

		world := oracleJointWorld(t, box2d.Vec2{X: 0.0, Y: -10.0})
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, anchor, load, func(def *box2d.DistanceJointDef) {
			def.Length = 1.0
			def.EnableSpring = true
			def.Hertz = 0.0
			def.EnableMotor = true
			def.MotorSpeed = 25.0
			def.MaxMotorForce = maxMotorForce
		})

		for range 60 {
			world.Step(oracleJointDT, oracleJointSubSteps)
			require.LessOrEqual(t, math.Abs(world.DistanceJointMotorForce(jointID)),
				maxMotorForce*(1.0+1e-9))
		}

		// The motor is saturated against a speed it cannot reach, so it is
		// pinned at the maximum.
		require.InDelta(t, maxMotorForce, math.Abs(world.DistanceJointMotorForce(jointID)), 1e-6)
	})

	t.Run("CurrentLengthIsWorldAnchorDistance", func(t *testing.T) {
		t.Parallel()

		// distance_joint.c:88-95
		//   pA = b2TransformPoint( transformA, base->localFrameA.p );
		//   pB = b2TransformPoint( transformB, base->localFrameB.p );
		//   return b2Length( b2Sub( pB, pA ) );
		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2{X: 3.0, Y: 4.0}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, anchor, load, func(def *box2d.DistanceJointDef) {
			def.Length = 5.0
		})

		require.InDelta(t, 5.0, world.DistanceJointCurrentLength(jointID), 1e-12)

		// The local frames participate: shift frame B by (1, 0) along the
		// world x-axis and the reported length changes accordingly.
		frameB := world.JointLocalFrameB(jointID)
		frameB.P = box2d.Vec2{X: 1.0, Y: 0.0}
		world.SetJointLocalFrameB(jointID, frameB)
		require.InDelta(t, math.Hypot(4.0, 4.0), world.DistanceJointCurrentLength(jointID), 1e-12)
	})
}

func TestOracleDistanceJointBehavior(t *testing.T) {
	t.Parallel()

	t.Run("RigidJointHoldsRestLength", func(t *testing.T) {
		t.Parallel()

		// box2d.h:847 — "Enable/disable the distance joint spring. When
		// disabled the distance joint is rigid."
		world := oracleJointWorld(t, box2d.Vec2{X: 0.0, Y: -10.0})
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 0.0, Y: -1.0}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, anchor, load, func(def *box2d.DistanceJointDef) {
			def.Length = 1.0
			def.EnableSpring = false
		})

		oracleJointStep(world, 240)

		// A rigid constraint holds the rest length; the residual is the
		// solver's position tolerance, which is on the order of the linear
		// slop (0.005 m at the default length unit).
		require.InDelta(t, 1.0, world.DistanceJointCurrentLength(jointID), oracleJointSlop)
	})

	t.Run("LimitIsInertWhileSpringDisabled", func(t *testing.T) {
		t.Parallel()

		// box2d.h:871-872 — "Enable joint limit. The limit only works if the
		// joint spring is enabled. Otherwise the joint is rigid and the limit
		// has no effect." The gate is distance_joint.c:366:
		//   if ( joint->enableSpring && ( minLength < maxLength || !enableLimit ) )
		world := oracleJointWorld(t, box2d.Vec2{X: 0.0, Y: -10.0})
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 0.0, Y: -1.0}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, anchor, load, func(def *box2d.DistanceJointDef) {
			def.Length = 1.0
			def.EnableSpring = false
			def.EnableLimit = true
			def.MinLength = 0.1
			def.MaxLength = 0.2
		})

		oracleJointStep(world, 240)

		// The rest length wins over the (much tighter) limit range.
		require.InDelta(t, 1.0, world.DistanceJointCurrentLength(jointID), oracleJointSlop)
	})

	t.Run("SpringWithLimitStopsAtMaxLength", func(t *testing.T) {
		t.Parallel()

		// With the spring enabled the limit becomes active
		// (distance_joint.c:366, :407-470), so a load hanging on a soft joint
		// must come to rest at or just past maxLength and never below it.
		const maxLength = 2.0

		world := oracleJointWorld(t, box2d.Vec2{X: 0.0, Y: -10.0})
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 0.0, Y: -1.0}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, anchor, load, func(def *box2d.DistanceJointDef) {
			def.Length = 1.0
			def.EnableSpring = true
			def.Hertz = 0.0
			def.EnableLimit = true
			def.MinLength = 0.5
			def.MaxLength = maxLength
		})

		oracleJointStep(world, 240)

		length := world.DistanceJointCurrentLength(jointID)
		require.LessOrEqual(t, length, maxLength+oracleJointSlop)
		// Gravity pulls the load all the way onto the limit, so it rests
		// against it rather than somewhere in the middle of the range.
		require.Greater(t, length, maxLength-oracleJointSlop)
	})

	t.Run("MotorDrivesRelativeSpeed", func(t *testing.T) {
		t.Parallel()

		// docs/simulation.md "Joints" — a motor drives the joint at the
		// requested speed unless the required force exceeds the maximum.
		// distance_joint.c:393-399 solves for
		//   impulse = axialMass * ( motorSpeed - Cdot )
		// so with plenty of head room Cdot converges to motorSpeed.
		const motorSpeed = 2.0

		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, anchor, load, func(def *box2d.DistanceJointDef) {
			def.Length = 1.0
			def.EnableSpring = true
			def.Hertz = 0.0
			def.EnableMotor = true
			def.MotorSpeed = motorSpeed
			def.MaxMotorForce = 500.0
		})

		startLength := world.DistanceJointCurrentLength(jointID)
		const steps = 60
		oracleJointStep(world, steps)

		// Body A is static, so the joint's relative speed is body B's speed
		// along the joint axis (world +x here).
		require.InDelta(t, motorSpeed, world.BodyLinearVelocity(load).X, 0.05)

		// Extension over the interval is speed * time, allowing one step of
		// spin-up at the start.
		grown := world.DistanceJointCurrentLength(jointID) - startLength
		expected := motorSpeed * steps * oracleJointDT
		require.InDelta(t, expected, grown, motorSpeed*2.0*oracleJointDT)
	})
}

// ---------------------------------------------------------------------------
// Revolute joint accessors — src/revolute_joint.c
// ---------------------------------------------------------------------------

// oracleRevolutePendulum builds a hinge at the world origin with a dynamic arm
// whose centre sits one metre to the +x side, so frame B on the arm coincides
// with frame A on the static anchor.
func oracleRevolutePendulum(t *testing.T, gravity box2d.Vec2,
	tune func(*box2d.RevoluteJointDef),
) (*box2d.World, box2d.BodyID, box2d.JointID) {
	t.Helper()

	world := oracleJointWorld(t, gravity)
	anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
	arm := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)

	jointID := oracleRevoluteJoint(world, anchor, arm, func(def *box2d.RevoluteJointDef) {
		def.Base.LocalFrameB.P = box2d.Vec2{X: -1.0, Y: 0.0}
		if tune != nil {
			tune(def)
		}
	})
	return world, arm, jointID
}

func TestOracleRevoluteJointAccessors(t *testing.T) {
	t.Parallel()

	t.Run("SpringTuningRoundTrip", func(t *testing.T) {
		t.Parallel()

		world, _, jointID := oracleRevolutePendulum(t, box2d.Vec2Zero, nil)

		// revolute_joint.c:34-48 — EnableSpring stores the flag (and clears
		// springImpulse only when the flag actually changes).
		require.False(t, world.IsRevoluteJointSpringEnabled(jointID))
		world.EnableRevoluteJointSpring(jointID, true)
		require.True(t, world.IsRevoluteJointSpringEnabled(jointID))
		world.EnableRevoluteJointSpring(jointID, true)
		require.True(t, world.IsRevoluteJointSpringEnabled(jointID))
		world.EnableRevoluteJointSpring(jointID, false)
		require.False(t, world.IsRevoluteJointSpringEnabled(jointID))

		// revolute_joint.c:50-72 — plain stores.
		world.SetRevoluteJointSpringHertz(jointID, 4.0)
		world.SetRevoluteJointSpringDampingRatio(jointID, 0.8)
		require.InDelta(t, 4.0, world.RevoluteJointSpringHertz(jointID), 0)
		require.InDelta(t, 0.8, world.RevoluteJointSpringDampingRatio(jointID), 0)

		// revolute_joint.c:74-84 — the target angle is stored verbatim by the
		// setter (b2CreateRevoluteJoint clamps the *definition* value to
		// [-pi, pi], the setter does not).
		world.SetRevoluteJointTargetAngle(jointID, 0.75)
		require.InDelta(t, 0.75, world.RevoluteJointTargetAngle(jointID), 0)
		world.SetRevoluteJointTargetAngle(jointID, -0.75)
		require.InDelta(t, -0.75, world.RevoluteJointTargetAngle(jointID), 0)
	})

	t.Run("AngleIsRelativeFrameRotation", func(t *testing.T) {
		t.Parallel()

		// revolute_joint.c:92-96
		//   qA = b2MulRot( transformA.q, localFrameA.q );
		//   qB = b2MulRot( transformB.q, localFrameB.q );
		//   return b2RelativeAngle( qA, qB );
		const angle = 0.5

		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		arm := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, oracleExactRot(angle))
		jointID := oracleRevoluteJoint(world, anchor, arm, nil)

		require.InDelta(t, angle, world.RevoluteJointAngle(jointID), oracleAtan2Tolerance)
	})

	t.Run("LimitsRoundTripAndDefaults", func(t *testing.T) {
		t.Parallel()

		world, _, jointID := oracleRevolutePendulum(t, box2d.Vec2Zero, nil)

		// revolute_joint.c:99-126 — flag plus the two stored angles.
		require.False(t, world.IsRevoluteJointLimitEnabled(jointID))
		require.InDelta(t, 0.0, world.RevoluteJointLowerLimit(jointID), 0)
		require.InDelta(t, 0.0, world.RevoluteJointUpperLimit(jointID), 0)

		world.EnableRevoluteJointLimit(jointID, true)
		require.True(t, world.IsRevoluteJointLimitEnabled(jointID))

		// revolute_joint.c:128-142 — b2MinFloat/b2MaxFloat of the pair.
		world.SetRevoluteJointLimits(jointID, -0.4, 0.9)
		require.InDelta(t, -0.4, world.RevoluteJointLowerLimit(jointID), 0)
		require.InDelta(t, 0.9, world.RevoluteJointUpperLimit(jointID), 0)

		world.EnableRevoluteJointLimit(jointID, false)
		require.False(t, world.IsRevoluteJointLimitEnabled(jointID))
	})

	t.Run("EnableLimitAndSetLimitsZeroTheLimitImpulses", func(t *testing.T) {
		t.Parallel()

		// A pendulum hanging against a tight limit loads lowerImpulse, which
		// b2GetRevoluteJointTorque (revolute_joint.c:236-239) reports as
		//   inv_h * ( motorImpulse + lowerImpulse - upperImpulse )
		// with the motor disabled, so the torque isolates the limit impulses.
		world, _, jointID := oracleRevolutePendulum(t, box2d.Vec2{X: 0.0, Y: -10.0},
			func(def *box2d.RevoluteJointDef) {
				def.EnableLimit = true
				def.LowerAngle = -0.05
				def.UpperAngle = 0.05
			})

		oracleJointStep(world, 60)
		require.Greater(t, math.Abs(world.JointConstraintTorque(jointID)), 0.0)

		// revolute_joint.c:135-141 — a change of limits clears both impulses.
		world.SetRevoluteJointLimits(jointID, -0.5, 0.5)
		require.InDelta(t, 0.0, world.JointConstraintTorque(jointID), 0)

		// The arm has come to rest on the old limit, so its island is asleep
		// by now. No accessor in revolute_joint.c wakes the attached bodies,
		// so the wake has to be explicit (joint.c:866-880) before the widened
		// limits can be reloaded.
		world.WakeJointBodies(jointID)
		oracleJointStep(world, 90)
		require.Greater(t, math.Abs(world.JointConstraintTorque(jointID)), 0.0)

		// revolute_joint.c:102-107 — toggling the flag clears both impulses.
		world.EnableRevoluteJointLimit(jointID, false)
		require.InDelta(t, 0.0, world.JointConstraintTorque(jointID), 0)
	})

	t.Run("SetLimitsIsANoOpWhenUnchanged", func(t *testing.T) {
		t.Parallel()

		// revolute_joint.c:135 — the reset is guarded by
		//   if ( lower != lowerAngle || upper != upperAngle )
		// so re-setting the same pair must preserve the accumulated impulses.
		world, _, jointID := oracleRevolutePendulum(t, box2d.Vec2{X: 0.0, Y: -10.0},
			func(def *box2d.RevoluteJointDef) {
				def.EnableLimit = true
				def.LowerAngle = -0.05
				def.UpperAngle = 0.05
			})

		oracleJointStep(world, 60)
		loaded := world.JointConstraintTorque(jointID)
		require.Greater(t, math.Abs(loaded), 0.0)

		world.SetRevoluteJointLimits(jointID, -0.05, 0.05)
		require.InDelta(t, loaded, world.JointConstraintTorque(jointID), 0)
	})

	t.Run("EnableMotorZeroesMotorImpulseOnChangeOnly", func(t *testing.T) {
		t.Parallel()

		world, _, jointID := oracleRevolutePendulum(t, box2d.Vec2{X: 0.0, Y: -10.0},
			func(def *box2d.RevoluteJointDef) {
				def.EnableMotor = true
				def.MotorSpeed = 0.0
				def.MaxMotorTorque = 100.0
			})
		require.True(t, world.IsRevoluteJointMotorEnabled(jointID))

		oracleJointStep(world, 30)
		running := world.RevoluteJointMotorTorque(jointID)
		require.Greater(t, math.Abs(running), 0.0)

		// revolute_joint.c:147 — the reset is guarded by a state change.
		world.EnableRevoluteJointMotor(jointID, true)
		require.InDelta(t, running, world.RevoluteJointMotorTorque(jointID), 0)

		// revolute_joint.c:149-150 — motorImpulse is cleared, and
		// revolute_joint.c:172-177 reports inv_h * motorImpulse.
		world.EnableRevoluteJointMotor(jointID, false)
		require.False(t, world.IsRevoluteJointMotorEnabled(jointID))
		require.InDelta(t, 0.0, world.RevoluteJointMotorTorque(jointID), 0)
	})

	t.Run("MotorParametersRoundTrip", func(t *testing.T) {
		t.Parallel()

		world, _, jointID := oracleRevolutePendulum(t, box2d.Vec2Zero, nil)

		// revolute_joint.c:160-170 and :179-189 — plain stores.
		world.SetRevoluteJointMotorSpeed(jointID, -3.25)
		require.InDelta(t, -3.25, world.RevoluteJointMotorSpeed(jointID), 0)

		world.SetRevoluteJointMaxMotorTorque(jointID, 22.5)
		require.InDelta(t, 22.5, world.RevoluteJointMaxMotorTorque(jointID), 0)

		// world->inv_h is zero before the first step, so the torque getter
		// reports zero regardless of the stored impulse.
		require.InDelta(t, 0.0, world.RevoluteJointMotorTorque(jointID), 0)
	})
}

func TestOracleRevoluteJointBehavior(t *testing.T) {
	t.Parallel()

	t.Run("PendulumKeepsHingeDistance", func(t *testing.T) {
		t.Parallel()

		// docs/simulation.md "Revolute Joint" — the joint forces the two
		// bodies to share an anchor point, leaving relative rotation as the
		// only degree of freedom. The arm's centre therefore stays one metre
		// from the hinge for the whole swing.
		world, arm, _ := oracleRevolutePendulum(t, box2d.Vec2{X: 0.0, Y: -10.0}, nil)

		for range 240 {
			world.Step(oracleJointDT, oracleJointSubSteps)
			require.InDelta(t, 1.0, box2d.Length(world.BodyPosition(arm)), oracleJointSlop)
		}

		// Energy is never created: with the arm released from the horizontal,
		// its centre can never rise above the hinge height.
		require.LessOrEqual(t, world.BodyPosition(arm).Y, oracleJointSlop)
	})

	t.Run("MotorReachesRequestedSpeed", func(t *testing.T) {
		t.Parallel()

		// docs/simulation.md "Revolute Joint" — "The joint motor will maintain
		// the specified speed unless the required torque exceeds the specified
		// maximum." Body A is static, so the relative speed is body B's
		// angular velocity.
		const motorSpeed = 3.0

		world, arm, jointID := oracleRevolutePendulum(t, box2d.Vec2Zero,
			func(def *box2d.RevoluteJointDef) {
				def.EnableMotor = true
				def.MotorSpeed = motorSpeed
				def.MaxMotorTorque = 1000.0
			})

		oracleJointStep(world, 60)

		require.InDelta(t, motorSpeed, world.BodyAngularVelocity(arm), 0.05)
		// One second of driving at motorSpeed advances the joint angle by
		// motorSpeed radians, wrapped into (-pi, pi] by b2RelativeAngle.
		require.InDelta(t, box2d.UnwindAngle(motorSpeed*60*oracleJointDT),
			world.RevoluteJointAngle(jointID), 0.15)
	})

	t.Run("MotorTorqueRespectsMaxMotorTorque", func(t *testing.T) {
		t.Parallel()

		// revolute_joint.c:357-358
		//   float maxImpulse = context->h * joint->maxMotorTorque;
		//   joint->motorImpulse = b2ClampFloat( ..., -maxImpulse, maxImpulse );
		// combined with the inv_h scaling in revolute_joint.c:176.
		const maxMotorTorque = 0.4

		world, _, jointID := oracleRevolutePendulum(t, box2d.Vec2{X: 0.0, Y: -10.0},
			func(def *box2d.RevoluteJointDef) {
				def.EnableMotor = true
				def.MotorSpeed = 30.0
				def.MaxMotorTorque = maxMotorTorque
			})

		for range 120 {
			world.Step(oracleJointDT, oracleJointSubSteps)
			require.LessOrEqual(t, math.Abs(world.RevoluteJointMotorTorque(jointID)),
				maxMotorTorque*(1.0+1e-9))
		}
	})

	t.Run("SpringSettlesOnTargetAngle", func(t *testing.T) {
		t.Parallel()

		// revolute_joint.c b2SolveRevoluteJoint drives the spring toward
		// b2RelativeAngle( frameA, frameB ) == targetAngle, and
		// docs/simulation.md notes a damping ratio of one is critical damping,
		// so the arm settles rather than oscillating.
		const targetAngle = 0.5

		world, _, jointID := oracleRevolutePendulum(t, box2d.Vec2Zero,
			func(def *box2d.RevoluteJointDef) {
				def.EnableSpring = true
				def.Hertz = 2.0
				def.DampingRatio = 1.0
				def.TargetAngle = targetAngle
			})

		oracleJointStep(world, 600)

		// A soft constraint approaches its target asymptotically and the
		// island freezes once the arm's speed drops under the sleep threshold
		// (0.05 m/s at a one-metre lever), so a small residual offset is
		// expected. Gravity is zero, so nothing but the spring could have
		// moved the arm off its starting angle of zero.
		require.InDelta(t, targetAngle, world.RevoluteJointAngle(jointID), 0.05)
	})

	t.Run("LimitStopsTheMotor", func(t *testing.T) {
		t.Parallel()

		// docs/simulation.md "Revolute Joint" — "A joint limit forces the
		// joint angle to remain between a lower and upper angle. The limit
		// will apply as much torque as needed to make this happen."
		const upperAngle = 0.25

		world, _, jointID := oracleRevolutePendulum(t, box2d.Vec2Zero,
			func(def *box2d.RevoluteJointDef) {
				def.EnableLimit = true
				def.LowerAngle = -upperAngle
				def.UpperAngle = upperAngle
				def.EnableMotor = true
				def.MotorSpeed = 2.0
				def.MaxMotorTorque = 5.0
			})

		// Unconstrained, one second at 2 rad/s would reach 2 radians.
		oracleJointStep(world, 60)

		angle := world.RevoluteJointAngle(jointID)
		require.LessOrEqual(t, angle, upperAngle+0.05)
		require.Greater(t, angle, upperAngle-0.05)

		// Reversing the motor drives the joint onto the lower limit instead.
		// The arm is parked on the upper limit and its island has gone to
		// sleep, and b2RevoluteJoint_SetMotorSpeed does not wake the attached
		// bodies (no b2Joint_WakeBodies call anywhere in revolute_joint.c), so
		// the wake is explicit.
		world.SetRevoluteJointMotorSpeed(jointID, -2.0)
		world.WakeJointBodies(jointID)
		oracleJointStep(world, 60)

		angle = world.RevoluteJointAngle(jointID)
		require.GreaterOrEqual(t, angle, -upperAngle-0.05)
		require.Less(t, angle, -upperAngle+0.05)
	})
}

// ---------------------------------------------------------------------------
// Prismatic joint — src/prismatic_joint.c
// ---------------------------------------------------------------------------

// oracleSlider builds a horizontal slider: a static rail body at the origin and
// a dynamic carriage that may translate along the rail's local +x axis.
func oracleSlider(t *testing.T, gravity box2d.Vec2,
	tune func(*box2d.PrismaticJointDef),
) (*box2d.World, box2d.BodyID, box2d.JointID) {
	t.Helper()

	world := oracleJointWorld(t, gravity)
	rail := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
	carriage := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2Zero, box2d.RotIdentity)

	jointID := oraclePrismaticJoint(world, rail, carriage, tune)
	return world, carriage, jointID
}

func TestOraclePrismaticJointAccessors(t *testing.T) {
	t.Parallel()

	t.Run("SetLimitsOrdersAndZeroesLimitImpulses", func(t *testing.T) {
		t.Parallel()

		world, _, jointID := oracleSlider(t, box2d.Vec2{X: -10.0, Y: 0.0},
			func(def *box2d.PrismaticJointDef) {
				def.EnableLimit = true
				def.LowerTranslation = -0.5
				def.UpperTranslation = 0.5
			})

		// prismatic_joint.c:103-104 — b2MinFloat/b2MaxFloat of the pair.
		require.InDelta(t, -0.5, world.PrismaticJointLowerLimit(jointID), 0)
		require.InDelta(t, 0.5, world.PrismaticJointUpperLimit(jointID), 0)

		oracleJointStep(world, 60)

		// b2GetPrismaticJointForce (prismatic_joint.c:211-228) splits the
		// reported force into a perpendicular part (impulse.x) and an axial
		// part inv_h * ( motorImpulse + lowerImpulse - upperImpulse ). With
		// the motor disabled the axial part is the limit impulse alone.
		axis := box2d.Vec2{X: 1.0, Y: 0.0}
		require.Greater(t, math.Abs(box2d.Dot(world.JointConstraintForce(jointID), axis)), 0.0)

		// prismatic_joint.c:99-107 — changing the limits clears both impulses.
		world.SetPrismaticJointLimits(jointID, -2.0, 2.0)
		require.InDelta(t, -2.0, world.PrismaticJointLowerLimit(jointID), 0)
		require.InDelta(t, 2.0, world.PrismaticJointUpperLimit(jointID), 0)
		require.InDelta(t, 0.0, box2d.Dot(world.JointConstraintForce(jointID), axis), 1e-12)
	})

	t.Run("EnableLimitAndSpringAndMotorRoundTrip", func(t *testing.T) {
		t.Parallel()

		world, _, jointID := oracleSlider(t, box2d.Vec2Zero, nil)

		// prismatic_joint.c:41-56 (spring), :110-126 (limit), :152-168 (motor).
		require.False(t, world.IsPrismaticJointSpringEnabled(jointID))
		require.False(t, world.IsPrismaticJointLimitEnabled(jointID))
		require.False(t, world.IsPrismaticJointMotorEnabled(jointID))

		world.EnablePrismaticJointSpring(jointID, true)
		world.EnablePrismaticJointLimit(jointID, true)
		world.EnablePrismaticJointMotor(jointID, true)
		require.True(t, world.IsPrismaticJointSpringEnabled(jointID))
		require.True(t, world.IsPrismaticJointLimitEnabled(jointID))
		require.True(t, world.IsPrismaticJointMotorEnabled(jointID))

		// prismatic_joint.c:63-93 — spring tuning and target translation.
		world.SetPrismaticJointSpringHertz(jointID, 2.5)
		world.SetPrismaticJointSpringDampingRatio(jointID, 0.4)
		world.SetPrismaticJointTargetTranslation(jointID, -1.25)
		require.InDelta(t, 2.5, world.PrismaticJointSpringHertz(jointID), 0)
		require.InDelta(t, 0.4, world.PrismaticJointSpringDampingRatio(jointID), 0)
		require.InDelta(t, -1.25, world.PrismaticJointTargetTranslation(jointID), 0)

		// prismatic_joint.c:171-197 — motor speed and maximum force.
		world.SetPrismaticJointMotorSpeed(jointID, 6.5)
		world.SetPrismaticJointMaxMotorForce(jointID, 12.0)
		require.InDelta(t, 6.5, world.PrismaticJointMotorSpeed(jointID), 0)
		require.InDelta(t, 12.0, world.PrismaticJointMaxMotorForce(jointID), 0)
	})

	t.Run("TranslationAndSpeedFollowTheAxis", func(t *testing.T) {
		t.Parallel()

		// prismatic_joint.c b2PrismaticJoint_GetTranslation:
		//   translation = b2Dot( b2Sub( pB, pA ), axisA )
		world, carriage, jointID := oracleSlider(t, box2d.Vec2Zero, nil)

		require.InDelta(t, 0.0, world.PrismaticJointTranslation(jointID), 1e-12)
		require.InDelta(t, 0.0, world.PrismaticJointSpeed(jointID), 1e-12)

		world.SetBodyTransform(carriage, box2d.Vec2{X: 1.5, Y: 0.0}, box2d.RotIdentity)
		require.InDelta(t, 1.5, world.PrismaticJointTranslation(jointID), 1e-9)

		// b2PrismaticJoint_GetSpeed projects the relative velocity onto the
		// axis, so a pure +x velocity of 2 reads back as a speed of 2.
		world.SetBodyLinearVelocity(carriage, box2d.Vec2{X: 2.0, Y: 0.0})
		require.InDelta(t, 2.0, world.PrismaticJointSpeed(jointID), 1e-9)
	})
}

func TestOraclePrismaticJointBehavior(t *testing.T) {
	t.Parallel()

	t.Run("LimitStopsTranslation", func(t *testing.T) {
		t.Parallel()

		// docs/simulation.md "Prismatic Joint" — the prismatic joint is the
		// revolute joint with translation substituted for angle, so the limit
		// bounds the translation.
		const upper = 0.75

		world, _, jointID := oracleSlider(t, box2d.Vec2{X: 10.0, Y: 0.0},
			func(def *box2d.PrismaticJointDef) {
				def.EnableLimit = true
				def.LowerTranslation = -upper
				def.UpperTranslation = upper
			})

		oracleJointStep(world, 240)

		translation := world.PrismaticJointTranslation(jointID)
		require.LessOrEqual(t, translation, upper+oracleJointSlop)
		require.Greater(t, translation, upper-oracleJointSlop)
	})

	t.Run("MotorForceRespectsMaxMotorForce", func(t *testing.T) {
		t.Parallel()

		// prismatic_joint.c:466-467
		//   float maxImpulse = context->h * joint->maxMotorForce;
		//   joint->motorImpulse = b2ClampFloat( ..., -maxImpulse, maxImpulse );
		const maxMotorForce = 0.3

		world, _, jointID := oracleSlider(t, box2d.Vec2Zero,
			func(def *box2d.PrismaticJointDef) {
				def.EnableMotor = true
				def.MotorSpeed = 40.0
				def.MaxMotorForce = maxMotorForce
			})

		for range 120 {
			world.Step(oracleJointDT, oracleJointSubSteps)
			require.LessOrEqual(t, math.Abs(world.PrismaticJointMotorForce(jointID)),
				maxMotorForce*(1.0+1e-9))
		}
	})

	t.Run("SpringSettlesOnTargetTranslation", func(t *testing.T) {
		t.Parallel()

		// docs/simulation.md — the spring-damper drives the joint to the
		// target translation; a damping ratio of one is critical damping, so
		// the carriage settles rather than oscillates.
		const target = 1.25

		world, _, jointID := oracleSlider(t, box2d.Vec2Zero,
			func(def *box2d.PrismaticJointDef) {
				def.EnableSpring = true
				def.Hertz = 3.0
				def.DampingRatio = 1.0
				def.TargetTranslation = target
			})

		oracleJointStep(world, 600)

		require.InDelta(t, target, world.PrismaticJointTranslation(jointID), oracleJointSlop)
	})

	t.Run("PerpendicularAndAngularMotionAreLocked", func(t *testing.T) {
		t.Parallel()

		// docs/simulation.md "Prismatic Joint" — "A prismatic joint allows for
		// relative translation of two bodies along a local axis. A prismatic
		// joint prevents relative rotation."
		world, carriage, _ := oracleSlider(t, box2d.Vec2{X: 0.0, Y: -10.0}, nil)

		oracleJointStep(world, 240)

		require.InDelta(t, 0.0, world.BodyPosition(carriage).Y, oracleJointSlop)
		require.InDelta(t, 0.0, box2d.RotGetAngle(world.BodyRotation(carriage)), 1e-3)
	})
}

// ---------------------------------------------------------------------------
// Wheel joint — src/wheel_joint.c
// ---------------------------------------------------------------------------

func oracleWheelRig(t *testing.T, gravity box2d.Vec2,
	tune func(*box2d.WheelJointDef),
) (*box2d.World, box2d.BodyID, box2d.JointID) {
	t.Helper()

	world := oracleJointWorld(t, gravity)
	chassis := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
	wheel := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2Zero, box2d.RotIdentity)

	jointID := oracleWheelJoint(world, chassis, wheel, tune)
	return world, wheel, jointID
}

func TestOracleWheelJointAccessors(t *testing.T) {
	t.Parallel()

	t.Run("DefaultsMatchDefaultWheelJointDef", func(t *testing.T) {
		t.Parallel()

		// joint.c b2DefaultWheelJointDef seeds enableSpring = true,
		// hertz = 1.0, dampingRatio = 0.7.
		world, _, jointID := oracleWheelRig(t, box2d.Vec2Zero, nil)

		require.True(t, world.IsWheelJointSpringEnabled(jointID))
		require.InDelta(t, 1.0, world.WheelJointSpringHertz(jointID), 0)
		require.InDelta(t, 0.7, world.WheelJointSpringDampingRatio(jointID), 0)
		require.False(t, world.IsWheelJointLimitEnabled(jointID))
		require.False(t, world.IsWheelJointMotorEnabled(jointID))
	})

	t.Run("SpringLimitAndMotorRoundTrip", func(t *testing.T) {
		t.Parallel()

		world, _, jointID := oracleWheelRig(t, box2d.Vec2Zero, nil)

		// wheel_joint.c:43-83 — spring flag and tuning.
		world.EnableWheelJointSpring(jointID, false)
		require.False(t, world.IsWheelJointSpringEnabled(jointID))
		world.SetWheelJointSpringHertz(jointID, 5.5)
		world.SetWheelJointSpringDampingRatio(jointID, 0.2)
		require.InDelta(t, 5.5, world.WheelJointSpringHertz(jointID), 0)
		require.InDelta(t, 0.2, world.WheelJointSpringDampingRatio(jointID), 0)

		// wheel_joint.c:86-98 — b2MinFloat/b2MaxFloat of the limit pair.
		world.EnableWheelJointLimit(jointID, true)
		require.True(t, world.IsWheelJointLimitEnabled(jointID))
		world.SetWheelJointLimits(jointID, -1.5, 2.5)
		require.InDelta(t, -1.5, world.WheelJointLowerLimit(jointID), 0)
		require.InDelta(t, 2.5, world.WheelJointUpperLimit(jointID), 0)

		// wheel_joint.c:120-145 — motor flag, speed and maximum torque.
		world.EnableWheelJointMotor(jointID, true)
		require.True(t, world.IsWheelJointMotorEnabled(jointID))
		world.SetWheelJointMotorSpeed(jointID, -7.5)
		world.SetWheelJointMaxMotorTorque(jointID, 9.0)
		require.InDelta(t, -7.5, world.WheelJointMotorSpeed(jointID), 0)
		require.InDelta(t, 9.0, world.WheelJointMaxMotorTorque(jointID), 0)
	})

	t.Run("EnableMotorZeroesMotorImpulseOnChangeOnly", func(t *testing.T) {
		t.Parallel()

		// The motor is asked for a speed it cannot reach inside the test
		// window, so it stays saturated at maxMotorTorque and the stored
		// motorImpulse never relaxes back to zero.
		world, _, jointID := oracleWheelRig(t, box2d.Vec2Zero,
			func(def *box2d.WheelJointDef) {
				def.EnableMotor = true
				def.MotorSpeed = 200.0
				def.MaxMotorTorque = 0.5
			})

		oracleJointStep(world, 20)
		running := world.WheelJointMotorTorque(jointID)
		require.Greater(t, math.Abs(running), 0.0)

		// wheel_joint.c b2WheelJoint_EnableMotor guards the reset behind a
		// state change, exactly like the revolute joint.
		world.EnableWheelJointMotor(jointID, true)
		require.InDelta(t, running, world.WheelJointMotorTorque(jointID), 0)

		world.EnableWheelJointMotor(jointID, false)
		require.InDelta(t, 0.0, world.WheelJointMotorTorque(jointID), 0)
	})

	t.Run("EnableLimitZeroesLimitImpulses", func(t *testing.T) {
		t.Parallel()

		// wheel_joint.c b2WheelJoint_EnableLimit clears lowerImpulse and
		// upperImpulse when the flag changes; b2GetWheelJointForce
		// (wheel_joint.c:147-163) folds them into the axial force, which is
		// the only axial term while the spring is off.
		world, _, jointID := oracleWheelRig(t, box2d.Vec2{X: 10.0, Y: 0.0},
			func(def *box2d.WheelJointDef) {
				def.EnableSpring = false
				def.EnableLimit = true
				def.LowerTranslation = -0.5
				def.UpperTranslation = 0.5
			})

		oracleJointStep(world, 60)
		axis := box2d.Vec2{X: 1.0, Y: 0.0}
		require.Greater(t, math.Abs(box2d.Dot(world.JointConstraintForce(jointID), axis)), 0.0)

		world.EnableWheelJointLimit(jointID, false)
		require.InDelta(t, 0.0, box2d.Dot(world.JointConstraintForce(jointID), axis), 1e-12)
	})
}

func TestOracleWheelJointBehavior(t *testing.T) {
	t.Parallel()

	t.Run("MotorTorqueRespectsMaxMotorTorque", func(t *testing.T) {
		t.Parallel()

		// wheel_joint.c:360-361
		//   float maxImpulse = context->h * joint->maxMotorTorque;
		//   joint->motorImpulse = b2ClampFloat( ..., -maxImpulse, maxImpulse );
		// with b2WheelJoint_GetMotorTorque scaling by inv_h.
		const maxMotorTorque = 0.2

		world, _, jointID := oracleWheelRig(t, box2d.Vec2Zero,
			func(def *box2d.WheelJointDef) {
				def.EnableMotor = true
				def.MotorSpeed = 50.0
				def.MaxMotorTorque = maxMotorTorque
			})

		for range 120 {
			world.Step(oracleJointDT, oracleJointSubSteps)
			require.LessOrEqual(t, math.Abs(world.WheelJointMotorTorque(jointID)),
				maxMotorTorque*(1.0+1e-9))
		}
	})

	t.Run("SuspensionSpringSettles", func(t *testing.T) {
		t.Parallel()

		// docs/simulation.md "Wheel Joint" — "The translation has a spring and
		// damper to simulate the vehicle suspension." With gravity along the
		// joint axis the wheel settles at a static deflection instead of
		// falling without bound.
		world, wheel, _ := oracleWheelRig(t, box2d.Vec2{X: -10.0, Y: 0.0},
			func(def *box2d.WheelJointDef) {
				def.EnableSpring = true
				def.Hertz = 4.0
				def.DampingRatio = 1.0
			})

		oracleJointStep(world, 600)

		// A settled critically damped suspension has (nearly) zero velocity
		// and a bounded deflection; a free fall over 10 s at 10 m/s^2 would be
		// 500 m and 100 m/s.
		require.Less(t, math.Abs(world.BodyLinearVelocity(wheel).X), 0.05)
		require.Less(t, math.Abs(world.BodyPosition(wheel).X), 1.0)
	})

	t.Run("LimitStopsTranslationAndRotationStaysFree", func(t *testing.T) {
		t.Parallel()

		// docs/simulation.md "Wheel Joint" — "The wheel joint restricts a
		// point on bodyB to a line on bodyA" and "The rotation allows the
		// wheel to rotate."
		const upper = 0.5

		world, wheel, _ := oracleWheelRig(t, box2d.Vec2{X: 10.0, Y: -10.0},
			func(def *box2d.WheelJointDef) {
				def.EnableSpring = false
				def.EnableLimit = true
				def.LowerTranslation = -upper
				def.UpperTranslation = upper
			})
		world.SetBodyAngularVelocity(wheel, 5.0)

		oracleJointStep(world, 240)

		position := world.BodyPosition(wheel)
		// Along the axis the limit holds.
		require.LessOrEqual(t, position.X, upper+oracleJointSlop)
		// Perpendicular to the axis the point-on-line constraint holds even
		// though gravity pulls that way.
		require.InDelta(t, 0.0, position.Y, oracleJointSlop)
		// Rotation was never constrained.
		require.Greater(t, math.Abs(world.BodyAngularVelocity(wheel)), 0.0)
	})
}

// ---------------------------------------------------------------------------
// Weld joint — src/weld_joint.c
// ---------------------------------------------------------------------------

func TestOracleWeldJointAccessors(t *testing.T) {
	t.Parallel()

	t.Run("StiffnessRoundTrip", func(t *testing.T) {
		t.Parallel()

		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		block := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleWeldJoint(world, anchor, block, nil)

		// joint.c b2DefaultWeldJointDef leaves every stiffness at zero, which
		// weld_joint.c treats as "maximum stiffness" (box2d.h WeldJointDef).
		require.InDelta(t, 0.0, world.WeldJointLinearHertz(jointID), 0)
		require.InDelta(t, 0.0, world.WeldJointAngularHertz(jointID), 0)

		// weld_joint.c:54-104 — four plain stores.
		world.SetWeldJointLinearHertz(jointID, 6.0)
		world.SetWeldJointLinearDampingRatio(jointID, 0.9)
		world.SetWeldJointAngularHertz(jointID, 3.0)
		world.SetWeldJointAngularDampingRatio(jointID, 0.3)
		require.InDelta(t, 6.0, world.WeldJointLinearHertz(jointID), 0)
		require.InDelta(t, 0.9, world.WeldJointLinearDampingRatio(jointID), 0)
		require.InDelta(t, 3.0, world.WeldJointAngularHertz(jointID), 0)
		require.InDelta(t, 0.3, world.WeldJointAngularDampingRatio(jointID), 0)
	})

	t.Run("RigidWeldHoldsRelativeTransform", func(t *testing.T) {
		t.Parallel()

		// docs/simulation.md "Weld Joint" — "The weld joint attempts to
		// constrain all relative motion between two bodies." weld_joint.c
		// welds the two joint *frames* together, so frame B is offset by one
		// metre to make the frames coincide at the anchor's origin.
		world := oracleJointWorld(t, box2d.Vec2{X: 0.0, Y: -10.0})
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		block := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		oracleWeldJoint(world, anchor, block, func(def *box2d.WeldJointDef) {
			def.Base.LocalFrameB.P = box2d.Vec2{X: -1.0, Y: 0.0}
		})

		oracleJointStep(world, 240)

		position := world.BodyPosition(block)
		require.InDelta(t, 1.0, position.X, oracleJointSlop)
		require.InDelta(t, 0.0, position.Y, oracleJointSlop)
		require.InDelta(t, 0.0, box2d.RotGetAngle(world.BodyRotation(block)), 1e-2)
	})

	t.Run("ConstraintForceAndTorqueScaleWithInvH", func(t *testing.T) {
		t.Parallel()

		// weld_joint.c:106-115
		//   force  = b2MulSV( world->inv_h, base->weldJoint.linearImpulse );
		//   torque = world->inv_h * base->weldJoint.angularImpulse;
		// A cantilevered block loads both, and the linear force must balance
		// the block's weight (mass 0.25 * gravity 10 = 2.5 N) once at rest.
		world := oracleJointWorld(t, box2d.Vec2{X: 0.0, Y: -10.0})
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		block := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleWeldJoint(world, anchor, block, func(def *box2d.WeldJointDef) {
			def.Base.LocalFrameB.P = box2d.Vec2{X: -1.0, Y: 0.0}
		})

		oracleJointStep(world, 240)

		force := world.JointConstraintForce(jointID)
		require.InDelta(t, 2.5, box2d.Length(force), 0.25)
		// The load hangs one metre out on the arm, so the reaction torque is
		// about weight * lever arm = 2.5 N * 1 m.
		require.InDelta(t, 2.5, math.Abs(world.JointConstraintTorque(jointID)), 0.5)

		// Before any step world->inv_h is zero, so a fresh world reports zero.
		fresh := oracleJointWorld(t, box2d.Vec2Zero)
		freshAnchor := oracleJointBody(t, fresh, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		freshBlock := oracleJointBody(t, fresh, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		freshJoint := oracleWeldJoint(fresh, freshAnchor, freshBlock, func(def *box2d.WeldJointDef) {
			def.Base.LocalFrameB.P = box2d.Vec2{X: -1.0, Y: 0.0}
		})
		require.InDelta(t, 0.0, box2d.Length(fresh.JointConstraintForce(freshJoint)), 0)
		require.InDelta(t, 0.0, fresh.JointConstraintTorque(freshJoint), 0)
	})

	t.Run("SoftWeldAllowsFlex", func(t *testing.T) {
		t.Parallel()

		// docs/simulation.md "Weld Joint" — "This constraint provides springs
		// to mimic soft-body simulation ... chains of bodies connected by weld
		// joints may flex." box2d.h documents zero hertz as maximum stiffness,
		// so the sag has to grow once the stiffness becomes finite, while the
		// bodies stay attached.
		buildSag := func(linearHertz, angularHertz float64) float64 {
			world := oracleJointWorld(t, box2d.Vec2{X: 0.0, Y: -10.0})
			anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
			block := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
			oracleWeldJoint(world, anchor, block, func(def *box2d.WeldJointDef) {
				def.Base.LocalFrameB.P = box2d.Vec2{X: -1.0, Y: 0.0}
				def.LinearHertz = linearHertz
				def.LinearDampingRatio = 1.0
				def.AngularHertz = angularHertz
				def.AngularDampingRatio = 1.0
			})

			oracleJointStep(world, 240)
			return -world.BodyPosition(block).Y
		}

		rigidSag := buildSag(0.0, 0.0)
		softSag := buildSag(1.0, 1.0)

		require.Less(t, rigidSag, oracleJointSlop)
		require.Greater(t, softSag, rigidSag)
		// Still attached: four seconds of free fall at 10 m/s^2 would be 80 m.
		require.Less(t, softSag, 5.0)
	})
}

// ---------------------------------------------------------------------------
// Motor joint — src/motor_joint.c
// ---------------------------------------------------------------------------

func TestOracleMotorJointAccessors(t *testing.T) {
	t.Parallel()

	t.Run("VelocityAndSpringParametersRoundTrip", func(t *testing.T) {
		t.Parallel()

		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		target := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleMotorJoint(world, anchor, target, nil)

		// motor_joint.c:14-60 — velocity targets and their maxima.
		world.SetMotorJointLinearVelocity(jointID, box2d.Vec2{X: 2.0, Y: -3.0})
		world.SetMotorJointAngularVelocity(jointID, 1.5)
		world.SetMotorJointMaxVelocityForce(jointID, 40.0)
		world.SetMotorJointMaxVelocityTorque(jointID, 8.0)
		require.InDelta(t, 2.0, world.MotorJointLinearVelocity(jointID).X, 0)
		require.InDelta(t, -3.0, world.MotorJointLinearVelocity(jointID).Y, 0)
		require.InDelta(t, 1.5, world.MotorJointAngularVelocity(jointID), 0)
		require.InDelta(t, 40.0, world.MotorJointMaxVelocityForce(jointID), 0)
		require.InDelta(t, 8.0, world.MotorJointMaxVelocityTorque(jointID), 0)

		// motor_joint.c:62-108 — spring tuning.
		world.SetMotorJointLinearHertz(jointID, 3.0)
		world.SetMotorJointLinearDampingRatio(jointID, 0.6)
		world.SetMotorJointAngularHertz(jointID, 2.0)
		world.SetMotorJointAngularDampingRatio(jointID, 0.4)
		require.InDelta(t, 3.0, world.MotorJointLinearHertz(jointID), 0)
		require.InDelta(t, 0.6, world.MotorJointLinearDampingRatio(jointID), 0)
		require.InDelta(t, 2.0, world.MotorJointAngularHertz(jointID), 0)
		require.InDelta(t, 0.4, world.MotorJointAngularDampingRatio(jointID), 0)
	})

	t.Run("MaxSpringForceAndTorqueClampAtZero", func(t *testing.T) {
		t.Parallel()

		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		target := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleMotorJoint(world, anchor, target, nil)

		// motor_joint.c:113
		//   joint->motorJoint.maxSpringForce = b2MaxFloat( 0.0f, maxForce );
		world.SetMotorJointMaxSpringForce(jointID, 25.0)
		require.InDelta(t, 25.0, world.MotorJointMaxSpringForce(jointID), 0)
		world.SetMotorJointMaxSpringForce(jointID, -5.0)
		require.InDelta(t, 0.0, world.MotorJointMaxSpringForce(jointID), 0)

		// motor_joint.c:125
		//   joint->motorJoint.maxSpringTorque = b2MaxFloat( 0.0f, maxTorque );
		world.SetMotorJointMaxSpringTorque(jointID, 11.0)
		require.InDelta(t, 11.0, world.MotorJointMaxSpringTorque(jointID), 0)
		world.SetMotorJointMaxSpringTorque(jointID, -0.5)
		require.InDelta(t, 0.0, world.MotorJointMaxSpringTorque(jointID), 0)
	})
}

func TestOracleMotorJointBehavior(t *testing.T) {
	t.Parallel()

	t.Run("VelocityTargetDrivesTheBody", func(t *testing.T) {
		t.Parallel()

		// docs/simulation.md "Motor Joint" — the motor joint controls the
		// motion of a body; b2MotorJoint_SetLinearVelocity sets the target
		// relative velocity, capped by the maximum velocity force. Gravity is
		// on so the motor has to keep working: a target it can hold for free
		// leaves every impulse at zero.
		world := oracleJointWorld(t, box2d.Vec2{X: 0.0, Y: -10.0})
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		target := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleMotorJoint(world, anchor, target, func(def *box2d.MotorJointDef) {
			def.LinearVelocity = box2d.Vec2{X: 1.5, Y: 0.0}
			def.MaxVelocityForce = 500.0
			def.AngularVelocity = 2.0
			def.MaxVelocityTorque = 500.0
		})

		oracleJointStep(world, 60)

		require.InDelta(t, 1.5, world.BodyLinearVelocity(target).X, 0.05)
		require.InDelta(t, 0.0, world.BodyLinearVelocity(target).Y, 0.05)
		require.InDelta(t, 2.0, world.BodyAngularVelocity(target), 0.05)

		// b2GetMotorJointForce (motor_joint.c:134-138) reports
		//   inv_h * ( linearVelocityImpulse + linearSpringImpulse )
		// which must balance the body's weight (mass 0.25 * gravity 10).
		require.InDelta(t, 2.5, box2d.Length(world.JointConstraintForce(jointID)), 0.25)
	})

	t.Run("ZeroTargetActsLikeFriction", func(t *testing.T) {
		t.Parallel()

		// docs/simulation.md "Motor Joint" — "With a velocity of zero this
		// acts like top-down friction."
		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		target := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		oracleMotorJoint(world, anchor, target, func(def *box2d.MotorJointDef) {
			def.MaxVelocityForce = 500.0
			def.MaxVelocityTorque = 500.0
		})
		world.SetBodyLinearVelocity(target, box2d.Vec2{X: 5.0, Y: 5.0})
		world.SetBodyAngularVelocity(target, 5.0)

		oracleJointStep(world, 60)

		require.InDelta(t, 0.0, box2d.Length(world.BodyLinearVelocity(target)), 0.05)
		require.InDelta(t, 0.0, world.BodyAngularVelocity(target), 0.05)
	})
}

// ---------------------------------------------------------------------------
// Shared joint accessors — src/joint.c
// ---------------------------------------------------------------------------

func TestOracleJointBaseAccessors(t *testing.T) {
	t.Parallel()

	t.Run("IdentityAndUserData", func(t *testing.T) {
		t.Parallel()

		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleRevoluteJoint(world, anchor, load, func(def *box2d.RevoluteJointDef) {
			def.Base.UserData = 42
		})

		// joint.c b2Joint_GetType / GetBodyA / GetBodyB.
		require.Equal(t, box2d.RevoluteJoint, world.JointType(jointID))
		require.Equal(t, anchor, world.JointBodyA(jointID))
		require.Equal(t, load, world.JointBodyB(jointID))
		require.True(t, world.IsJointValid(jointID))

		// joint.c:851-864 — user data is stored on the b2Joint, not the sim.
		require.Equal(t, uint64(42), world.JointUserData(jointID))
		world.SetJointUserData(jointID, 7)
		require.Equal(t, uint64(7), world.JointUserData(jointID))
	})

	t.Run("LocalFramesRoundTrip", func(t *testing.T) {
		t.Parallel()

		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleRevoluteJoint(world, anchor, load, nil)

		// joint.c b2Joint_SetLocalFrameA/B store the frame on the joint sim.
		frameA := box2d.Transform{P: box2d.Vec2{X: 0.25, Y: -0.5}, Q: box2d.MakeRot(0.3)}
		frameB := box2d.Transform{P: box2d.Vec2{X: -1.0, Y: 0.75}, Q: box2d.MakeRot(-0.2)}
		world.SetJointLocalFrameA(jointID, frameA)
		world.SetJointLocalFrameB(jointID, frameB)

		require.InDelta(t, frameA.P.X, world.JointLocalFrameA(jointID).P.X, 0)
		require.InDelta(t, frameA.P.Y, world.JointLocalFrameA(jointID).P.Y, 0)
		require.InDelta(t, frameA.Q.C, world.JointLocalFrameA(jointID).Q.C, 0)
		require.InDelta(t, frameA.Q.S, world.JointLocalFrameA(jointID).Q.S, 0)
		require.InDelta(t, frameB.P.X, world.JointLocalFrameB(jointID).P.X, 0)
		require.InDelta(t, frameB.P.Y, world.JointLocalFrameB(jointID).P.Y, 0)
		require.InDelta(t, frameB.Q.C, world.JointLocalFrameB(jointID).Q.C, 0)
		require.InDelta(t, frameB.Q.S, world.JointLocalFrameB(jointID).Q.S, 0)
	})

	t.Run("ConstraintTuningAndThresholdsRoundTrip", func(t *testing.T) {
		t.Parallel()

		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleRevoluteJoint(world, anchor, load, nil)

		// joint.c b2DefaultJointDef seeds constraintHertz 60, damping 2 and
		// FLT_MAX thresholds (the "events disabled" sentinel).
		hertz, damping := world.JointConstraintTuning(jointID)
		require.InDelta(t, 60.0, hertz, 0)
		require.InDelta(t, 2.0, damping, 0)
		require.InDelta(t, math.MaxFloat32, world.JointForceThreshold(jointID), 0)
		require.InDelta(t, math.MaxFloat32, world.JointTorqueThreshold(jointID), 0)

		// joint.c b2Joint_SetConstraintTuning / SetForceThreshold /
		// SetTorqueThreshold — plain stores on the joint sim.
		world.SetJointConstraintTuning(jointID, 30.0, 1.0)
		hertz, damping = world.JointConstraintTuning(jointID)
		require.InDelta(t, 30.0, hertz, 0)
		require.InDelta(t, 1.0, damping, 0)

		world.SetJointForceThreshold(jointID, 12.5)
		world.SetJointTorqueThreshold(jointID, 3.5)
		require.InDelta(t, 12.5, world.JointForceThreshold(jointID), 0)
		require.InDelta(t, 3.5, world.JointTorqueThreshold(jointID), 0)
	})

	t.Run("CollideConnectedToggle", func(t *testing.T) {
		t.Parallel()

		// joint.c b2Joint_SetCollideConnected: enabling re-buffers the
		// broad-phase proxies, disabling destroys the existing contacts. The
		// observable contract is the stored flag plus the resulting contact
		// count between the two bodies.
		world := oracleJointWorld(t, box2d.Vec2Zero)
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		// Overlapping boxes, so a contact exists as soon as collision is on.
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 0.1, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleRevoluteJoint(world, anchor, load, nil)

		require.False(t, world.JointCollideConnected(jointID))
		oracleJointStep(world, 4)
		require.Equal(t, 0, world.BodyContactCapacity(load))

		world.SetJointCollideConnected(jointID, true)
		require.True(t, world.JointCollideConnected(jointID))
		oracleJointStep(world, 4)
		require.Positive(t, world.BodyContactCapacity(load))

		world.SetJointCollideConnected(jointID, false)
		require.False(t, world.JointCollideConnected(jointID))
		require.Equal(t, 0, world.BodyContactCapacity(load))

		// Setting the same value again is an early-out.
		world.SetJointCollideConnected(jointID, false)
		require.False(t, world.JointCollideConnected(jointID))
	})

	t.Run("OnlyWakeBodiesWakesTheJoint", func(t *testing.T) {
		t.Parallel()

		// No accessor in distance_joint.c, revolute_joint.c,
		// prismatic_joint.c, motor_joint.c, weld_joint.c or wheel_joint.c
		// calls b2Joint_WakeBodies or b2WakeBody: in v3.2 the per-joint
		// setters never wake the attached bodies. The only two wake sites in
		// joint.c are b2Joint_WakeBodies (joint.c:866-880) and the
		// wakeBodies branch of b2DestroyJointInternal (joint.c:713-717).
		world := oracleJointWorld(t, box2d.Vec2Zero)
		bodyA := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2Zero, box2d.RotIdentity)
		bodyB := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 2.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleRevoluteJoint(world, bodyA, bodyB, func(def *box2d.RevoluteJointDef) {
			// Coincident anchors, so the joint is already satisfied and the
			// pair can go to sleep.
			def.Base.LocalFrameB.P = box2d.Vec2{X: -2.0, Y: 0.0}
		})

		// The island sleeps after b2_timeToSleep (0.5 s) below the sleep
		// threshold; 120 steps is two seconds.
		oracleJointStep(world, 120)
		require.False(t, world.IsBodyAwake(bodyA))
		require.False(t, world.IsBodyAwake(bodyB))

		world.SetRevoluteJointMotorSpeed(jointID, 5.0)
		world.EnableRevoluteJointMotor(jointID, true)
		world.SetRevoluteJointMaxMotorTorque(jointID, 100.0)
		world.SetRevoluteJointLimits(jointID, -1.0, 1.0)
		world.EnableRevoluteJointLimit(jointID, true)
		world.SetJointUserData(jointID, 9)
		require.False(t, world.IsBodyAwake(bodyA))
		require.False(t, world.IsBodyAwake(bodyB))

		// joint.c:876-879 — b2Joint_WakeBodies wakes both attached bodies.
		world.WakeJointBodies(jointID)
		require.True(t, world.IsBodyAwake(bodyA))
		require.True(t, world.IsBodyAwake(bodyB))
	})

	t.Run("DestroyJointWakesAttachedBodiesOnDemand", func(t *testing.T) {
		t.Parallel()

		// joint.c:713-717 — b2DestroyJointInternal wakes bodyA and bodyB only
		// when the caller asks for it.
		world := oracleJointWorld(t, box2d.Vec2Zero)
		bodyA := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2Zero, box2d.RotIdentity)
		bodyB := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 2.0, Y: 0.0}, box2d.RotIdentity)
		keep := oracleRevoluteJoint(world, bodyA, bodyB, func(def *box2d.RevoluteJointDef) {
			def.Base.LocalFrameB.P = box2d.Vec2{X: -2.0, Y: 0.0}
		})
		drop := oracleRevoluteJoint(world, bodyA, bodyB, func(def *box2d.RevoluteJointDef) {
			def.Base.LocalFrameB.P = box2d.Vec2{X: -2.0, Y: 0.0}
		})

		oracleJointStep(world, 120)
		require.False(t, world.IsBodyAwake(bodyA))

		world.DestroyJoint(drop, false)
		require.False(t, world.IsJointValid(drop))
		require.False(t, world.IsBodyAwake(bodyA))
		require.False(t, world.IsBodyAwake(bodyB))

		world.DestroyJoint(keep, true)
		require.False(t, world.IsJointValid(keep))
		require.True(t, world.IsBodyAwake(bodyA))
		require.True(t, world.IsBodyAwake(bodyB))
	})
}

// ---------------------------------------------------------------------------
// b2Joint_GetLinearSeparation / b2Joint_GetAngularSeparation — joint.c:1029-1200
// ---------------------------------------------------------------------------

// oracleSeparationWorld builds two static bodies so the transforms never move,
// letting the separation getters be checked as pure geometry. Body A sits at
// the origin with identity rotation, body B wherever the caller asks.
func oracleSeparationWorld(t *testing.T, positionB box2d.Vec2, rotationB box2d.Rot) (
	*box2d.World, box2d.BodyID, box2d.BodyID,
) {
	t.Helper()

	world := oracleJointWorld(t, box2d.Vec2Zero)
	bodyA := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
	bodyB := oracleJointBody(t, world, box2d.StaticBody, positionB, rotationB)
	return world, bodyA, bodyB
}

func TestOracleJointLinearSeparation(t *testing.T) {
	t.Parallel()

	// Body B sits three metres below body A, so with identity local frames
	// dp = pB - pA = (0, -3) and |dp| = 3.
	const dropY = -3.0
	const dropLength = 3.0

	t.Run("DistanceRigidUsesRestLengthError", func(t *testing.T) {
		t.Parallel()

		// joint.c:1069 — `return b2AbsFloat( length - distanceJoint->length );`
		world, bodyA, bodyB := oracleSeparationWorld(t, box2d.Vec2{X: 0.0, Y: dropY}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, bodyA, bodyB, func(def *box2d.DistanceJointDef) {
			def.Length = 1.0
			def.EnableSpring = false
		})

		require.InDelta(t, dropLength-1.0, world.JointLinearSeparation(jointID), 1e-12)

		// joint.c:1275 — a distance joint has no angular separation.
		require.InDelta(t, 0.0, world.JointAngularSeparation(jointID), 0)
	})

	t.Run("DistanceSpringWithoutLimitIsAdmissible", func(t *testing.T) {
		t.Parallel()

		// joint.c:1064-1066 — a soft distance joint with no limit reports no
		// separation at all: the spring travel is admissible movement.
		world, bodyA, bodyB := oracleSeparationWorld(t, box2d.Vec2{X: 0.0, Y: dropY}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, bodyA, bodyB, func(def *box2d.DistanceJointDef) {
			def.Length = 1.0
			def.EnableSpring = true
			def.EnableLimit = false
		})

		require.InDelta(t, 0.0, world.JointLinearSeparation(jointID), 0)
	})

	t.Run("DistanceSpringWithLimitReportsLimitViolation", func(t *testing.T) {
		t.Parallel()

		// joint.c:1041-1057
		//   if ( length < minLength ) return minLength - length;
		//   if ( length > maxLength ) return length - maxLength;
		//   return 0.0f;
		world, bodyA, bodyB := oracleSeparationWorld(t, box2d.Vec2{X: 0.0, Y: dropY}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, bodyA, bodyB, func(def *box2d.DistanceJointDef) {
			def.Length = 1.0
			def.EnableSpring = true
			def.EnableLimit = true
			def.MinLength = 1.0
			def.MaxLength = 2.0
		})

		// Over the maximum by 1.
		require.InDelta(t, dropLength-2.0, world.JointLinearSeparation(jointID), 1e-12)

		// Under the minimum by 1.
		world.SetDistanceJointLengthRange(jointID, 4.0, 5.0)
		require.InDelta(t, 4.0-dropLength, world.JointLinearSeparation(jointID), 1e-12)

		// Inside the range: no separation.
		world.SetDistanceJointLengthRange(jointID, 1.0, 5.0)
		require.InDelta(t, 0.0, world.JointLinearSeparation(jointID), 0)
	})

	t.Run("RevoluteIsAnchorDistance", func(t *testing.T) {
		t.Parallel()

		// joint.c:1101 — `case b2_revoluteJoint: return b2Length( dp );`
		world, bodyA, bodyB := oracleSeparationWorld(t, box2d.Vec2{X: 0.0, Y: dropY}, box2d.RotIdentity)
		jointID := oracleRevoluteJoint(world, bodyA, bodyB, nil)

		require.InDelta(t, dropLength, world.JointLinearSeparation(jointID), 1e-12)
	})

	t.Run("WeldDependsOnLinearHertz", func(t *testing.T) {
		t.Parallel()

		// joint.c:1105-1112
		//   if ( weldJoint->linearHertz == 0.0f ) return b2Length( dp );
		//   return 0.0f;
		world, bodyA, bodyB := oracleSeparationWorld(t, box2d.Vec2{X: 0.0, Y: dropY}, box2d.RotIdentity)
		jointID := oracleWeldJoint(world, bodyA, bodyB, nil)

		require.InDelta(t, dropLength, world.JointLinearSeparation(jointID), 1e-12)

		world.SetWeldJointLinearHertz(jointID, 3.0)
		require.InDelta(t, 0.0, world.JointLinearSeparation(jointID), 0)
	})

	t.Run("MotorAndFilterHaveNoSeparation", func(t *testing.T) {
		t.Parallel()

		// joint.c:1071-1075 — both cases return zero.
		world, bodyA, bodyB := oracleSeparationWorld(t, box2d.Vec2{X: 0.0, Y: dropY}, box2d.RotIdentity)
		motorID := oracleMotorJoint(world, bodyA, bodyB, nil)
		filterID := oracleFilterJoint(world, bodyA, bodyB)

		require.InDelta(t, 0.0, world.JointLinearSeparation(motorID), 0)
		require.InDelta(t, 0.0, world.JointLinearSeparation(filterID), 0)
		require.InDelta(t, 0.0, world.JointAngularSeparation(motorID), 0)
		require.InDelta(t, 0.0, world.JointAngularSeparation(filterID), 0)

		// joint.c:1001-1002 and :1034-1035 — the filter joint has no solve
		// work at all, so its constraint reaction is identically zero.
		filterForce := world.JointConstraintForce(filterID)
		require.InDelta(t, 0.0, filterForce.X, 0)
		require.InDelta(t, 0.0, filterForce.Y, 0)
		require.InDelta(t, 0.0, world.JointConstraintTorque(filterID), 0)
	})

	t.Run("PrismaticCombinesPerpendicularAndLimitError", func(t *testing.T) {
		t.Parallel()

		// joint.c:1078-1098
		//   axisA = b2RotateVector( xfA.q, {1,0} ); perpA = b2LeftPerp( axisA );
		//   perpendicularSeparation = |dot( perpA, dp )|
		//   limitSeparation from the translation vs the limits
		//   return sqrtf( perp^2 + limit^2 )
		// With xfA identity, axisA = (1,0) and perpA = (0,1).
		world, bodyA, bodyB := oracleSeparationWorld(t, box2d.Vec2{X: 5.0, Y: dropY}, box2d.RotIdentity)
		jointID := oraclePrismaticJoint(world, bodyA, bodyB, nil)

		// No limit: only the perpendicular error, |dot((0,1), (5,-3))| = 3.
		require.InDelta(t, dropLength, world.JointLinearSeparation(jointID), 1e-12)

		// Limit enabled and violated on the upper side: translation is
		// dot((1,0), (5,-3)) = 5, so the limit error is 5 - 1 = 4 and the
		// total is sqrt(3^2 + 4^2) = 5.
		world.EnablePrismaticJointLimit(jointID, true)
		world.SetPrismaticJointLimits(jointID, -1.0, 1.0)
		require.InDelta(t, 5.0, world.JointLinearSeparation(jointID), 1e-12)

		// Inside the limits the axial term drops out again.
		world.SetPrismaticJointLimits(jointID, -10.0, 10.0)
		require.InDelta(t, dropLength, world.JointLinearSeparation(jointID), 1e-12)
	})

	t.Run("PrismaticLowerLimitBranch", func(t *testing.T) {
		t.Parallel()

		// joint.c:1085-1088 — the lower branch mirrors the upper one.
		world, bodyA, bodyB := oracleSeparationWorld(t, box2d.Vec2{X: -5.0, Y: dropY}, box2d.RotIdentity)
		jointID := oraclePrismaticJoint(world, bodyA, bodyB, func(def *box2d.PrismaticJointDef) {
			def.EnableLimit = true
			def.LowerTranslation = -1.0
			def.UpperTranslation = 1.0
		})

		// translation = -5, so the limit error is -1 - (-5) = 4, and the
		// perpendicular error is |dot((0,1), (-5,-3))| = 3.
		require.InDelta(t, 5.0, world.JointLinearSeparation(jointID), 1e-12)
	})

	t.Run("WheelCombinesPerpendicularAndLimitError", func(t *testing.T) {
		t.Parallel()

		// joint.c:1114-1136 — identical construction to the prismatic case.
		world, bodyA, bodyB := oracleSeparationWorld(t, box2d.Vec2{X: 5.0, Y: dropY}, box2d.RotIdentity)
		jointID := oracleWheelJoint(world, bodyA, bodyB, nil)

		require.InDelta(t, dropLength, world.JointLinearSeparation(jointID), 1e-12)

		world.EnableWheelJointLimit(jointID, true)
		world.SetWheelJointLimits(jointID, -1.0, 1.0)
		require.InDelta(t, 5.0, world.JointLinearSeparation(jointID), 1e-12)

		world.SetWheelJointLimits(jointID, -10.0, 10.0)
		require.InDelta(t, dropLength, world.JointLinearSeparation(jointID), 1e-12)
	})

	t.Run("WheelLowerLimitBranch", func(t *testing.T) {
		t.Parallel()

		world, bodyA, bodyB := oracleSeparationWorld(t, box2d.Vec2{X: -5.0, Y: dropY}, box2d.RotIdentity)
		jointID := oracleWheelJoint(world, bodyA, bodyB, func(def *box2d.WheelJointDef) {
			def.EnableLimit = true
			def.LowerTranslation = -1.0
			def.UpperTranslation = 1.0
		})

		require.InDelta(t, 5.0, world.JointLinearSeparation(jointID), 1e-12)
		// joint.c:1138 — a wheel joint has no angular separation.
		require.InDelta(t, 0.0, world.JointAngularSeparation(jointID), 0)
	})
}

func TestOracleJointAngularSeparation(t *testing.T) {
	t.Parallel()

	// joint.c:1148 — relativeAngle = b2RelativeAngle( xfA.q, xfB.q ), and with
	// body A at identity this is body B's rotation angle.
	const relativeAngle = 0.5

	t.Run("PrismaticReportsRelativeAngle", func(t *testing.T) {
		t.Parallel()

		// joint.c:1163-1166 — the prismatic case returns the relative angle
		// unmodified.
		world, bodyA, bodyB := oracleSeparationWorld(t, box2d.Vec2{X: 1.0, Y: 0.0},
			oracleExactRot(relativeAngle))
		jointID := oraclePrismaticJoint(world, bodyA, bodyB, nil)

		require.InDelta(t, relativeAngle, world.JointAngularSeparation(jointID), oracleAtan2Tolerance)
	})

	t.Run("RevoluteReportsLimitViolationOnly", func(t *testing.T) {
		t.Parallel()

		// joint.c:1168-1186
		//   if ( enableLimit ) { if ( angle < lowerAngle ) return lowerAngle - angle;
		//                        if ( upperAngle < angle ) return angle - upperAngle; }
		//   return 0.0f;
		world, bodyA, bodyB := oracleSeparationWorld(t, box2d.Vec2Zero, oracleExactRot(relativeAngle))
		jointID := oracleRevoluteJoint(world, bodyA, bodyB, nil)

		// Limit disabled: the free rotation is admissible.
		require.InDelta(t, 0.0, world.JointAngularSeparation(jointID), 0)

		// Over the upper limit by 0.4.
		world.EnableRevoluteJointLimit(jointID, true)
		world.SetRevoluteJointLimits(jointID, -0.1, 0.1)
		require.InDelta(t, relativeAngle-0.1, world.JointAngularSeparation(jointID), oracleAtan2Tolerance)

		// Inside the limits.
		world.SetRevoluteJointLimits(jointID, -1.0, 1.0)
		require.InDelta(t, 0.0, world.JointAngularSeparation(jointID), 0)

		// Under the lower limit by 0.4.
		world.SetRevoluteJointLimits(jointID, 0.9, 1.0)
		require.InDelta(t, 0.9-relativeAngle, world.JointAngularSeparation(jointID), oracleAtan2Tolerance)

		// The same branch reached from the other side: a negative relative
		// angle below a symmetric limit range.
		mirrored, mirroredA, mirroredB := oracleSeparationWorld(t, box2d.Vec2Zero,
			oracleExactRot(-relativeAngle))
		mirroredJoint := oracleRevoluteJoint(mirrored, mirroredA, mirroredB,
			func(def *box2d.RevoluteJointDef) {
				def.EnableLimit = true
				def.LowerAngle = -0.1
				def.UpperAngle = 0.1
			})
		require.InDelta(t, relativeAngle-0.1, mirrored.JointAngularSeparation(mirroredJoint),
			oracleAtan2Tolerance)
	})

	t.Run("WeldDependsOnAngularHertz", func(t *testing.T) {
		t.Parallel()

		// joint.c:1188-1196
		//   if ( weldJoint->angularHertz == 0.0f ) return relativeAngle;
		//   return 0.0f;
		world, bodyA, bodyB := oracleSeparationWorld(t, box2d.Vec2{X: 1.0, Y: 0.0},
			oracleExactRot(relativeAngle))
		jointID := oracleWeldJoint(world, bodyA, bodyB, nil)

		require.InDelta(t, relativeAngle, world.JointAngularSeparation(jointID), oracleAtan2Tolerance)

		world.SetWeldJointAngularHertz(jointID, 2.0)
		require.InDelta(t, 0.0, world.JointAngularSeparation(jointID), 0)
	})
}

// ---------------------------------------------------------------------------
// b2GetJointReaction — joint.c:882-948, consumed by the joint events
// ---------------------------------------------------------------------------

func TestOracleJointReactionEvents(t *testing.T) {
	t.Parallel()

	// solver.c:196-207 — a joint is reported when
	//   ( forceThreshold < FLT_MAX || torqueThreshold < FLT_MAX ) &&
	//   ( force >= forceThreshold || torque >= torqueThreshold )
	// with force/torque coming from b2GetJointReaction. A zero threshold
	// therefore reports every awake joint, which exercises the reaction
	// formula for every joint type.
	world := oracleJointWorld(t, box2d.Vec2{X: 0.0, Y: -10.0})

	jointIDs := make([]box2d.JointID, 0, 7)
	pair := func(row float64) (box2d.BodyID, box2d.BodyID) {
		anchor := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2{X: 0.0, Y: row}, box2d.RotIdentity)
		load := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 1.0, Y: row}, box2d.RotIdentity)
		return anchor, load
	}

	anchor, load := pair(0.0)
	jointIDs = append(jointIDs, oracleDistanceJoint(world, anchor, load, func(def *box2d.DistanceJointDef) {
		def.Length = 1.0
		def.EnableSpring = true
		def.Hertz = 2.0
		def.DampingRatio = 0.5
		def.EnableMotor = true
		def.MotorSpeed = 0.5
		def.MaxMotorForce = 10.0
		def.EnableLimit = true
		def.MinLength = 0.5
		def.MaxLength = 1.5
	}))

	anchor, load = pair(20.0)
	jointIDs = append(jointIDs, oracleRevoluteJoint(world, anchor, load, func(def *box2d.RevoluteJointDef) {
		def.Base.LocalFrameB.P = box2d.Vec2{X: -1.0, Y: 0.0}
		def.EnableMotor = true
		def.MotorSpeed = 1.0
		def.MaxMotorTorque = 10.0
		def.EnableLimit = true
		def.LowerAngle = -0.5
		def.UpperAngle = 0.5
	}))

	anchor, load = pair(40.0)
	jointIDs = append(jointIDs, oraclePrismaticJoint(world, anchor, load, func(def *box2d.PrismaticJointDef) {
		def.EnableMotor = true
		def.MotorSpeed = 1.0
		def.MaxMotorForce = 10.0
		def.EnableLimit = true
		def.LowerTranslation = -1.0
		def.UpperTranslation = 2.0
	}))

	anchor, load = pair(60.0)
	jointIDs = append(jointIDs, oracleWheelJoint(world, anchor, load, func(def *box2d.WheelJointDef) {
		def.EnableMotor = true
		def.MotorSpeed = 1.0
		def.MaxMotorTorque = 10.0
		def.EnableLimit = true
		def.LowerTranslation = -1.0
		def.UpperTranslation = 2.0
	}))

	anchor, load = pair(80.0)
	jointIDs = append(jointIDs, oracleWeldJoint(world, anchor, load, nil))

	anchor, load = pair(100.0)
	jointIDs = append(jointIDs, oracleMotorJoint(world, anchor, load, func(def *box2d.MotorJointDef) {
		def.LinearVelocity = box2d.Vec2{X: 1.0, Y: 0.0}
		def.MaxVelocityForce = 20.0
		def.AngularVelocity = 1.0
		def.MaxVelocityTorque = 20.0
		def.LinearHertz = 2.0
		def.LinearDampingRatio = 0.5
		def.MaxSpringForce = 20.0
		def.AngularHertz = 2.0
		def.AngularDampingRatio = 0.5
		def.MaxSpringTorque = 20.0
	}))

	anchor, load = pair(120.0)
	jointIDs = append(jointIDs, oracleFilterJoint(world, anchor, load))

	for _, jointID := range jointIDs {
		world.SetJointForceThreshold(jointID, 0.0)
		world.SetJointTorqueThreshold(jointID, 0.0)
	}

	// Two steps: the first one populates the impulses, the second reports on
	// joints that are unambiguously awake.
	oracleJointStep(world, 2)

	events := world.JointEvents().JointEvents
	require.Len(t, events, len(jointIDs))

	reported := make(map[box2d.JointID]bool, len(events))
	for _, event := range events {
		reported[event.JointID] = true
		require.Equal(t, world.JointUserData(event.JointID), event.UserData)
	}
	for _, jointID := range jointIDs {
		require.True(t, reported[jointID], "joint type %v produced no reaction event", world.JointType(jointID))
	}

	// Raising every threshold above the achievable reaction silences the
	// events again (solver.c:203).
	for _, jointID := range jointIDs {
		world.SetJointForceThreshold(jointID, 1.0e6)
		world.SetJointTorqueThreshold(jointID, 1.0e6)
	}
	oracleJointStep(world, 1)
	require.Empty(t, world.JointEvents().JointEvents)
}

// ---------------------------------------------------------------------------
// b2DrawJoint and the per-type draw routines — joint.c:1440-1520 and friends
// ---------------------------------------------------------------------------

type oracleDrawCall struct {
	vertices []box2d.Vec2
	text     string
	p1       box2d.Vec2
	p2       box2d.Vec2
	radius   float64
	size     float64
	color    box2d.HexColor
}

type oracleDrawRecorder struct {
	polygons []oracleDrawCall
	circles  []oracleDrawCall
	lines    []oracleDrawCall
	points   []oracleDrawCall
	strings  []oracleDrawCall
}

// debugDraw returns a DebugDraw that records only what the joint routines emit:
// shapes, masses, names, contacts and islands are all switched off.
func (r *oracleDrawRecorder) debugDraw() box2d.DebugDraw {
	draw := box2d.DefaultDebugDraw()
	draw.DrawShapes = false
	draw.DrawJoints = true

	draw.DrawPolygonFcn = func(vertices []box2d.Vec2, color box2d.HexColor, _ any) {
		r.polygons = append(r.polygons, oracleDrawCall{
			vertices: append([]box2d.Vec2(nil), vertices...),
			color:    color,
		})
	}
	draw.DrawSolidPolygonFcn = func(_ box2d.Transform, _ []box2d.Vec2, _ float64, _ box2d.HexColor, _ any) {}
	draw.DrawCircleFcn = func(center box2d.Vec2, radius float64, color box2d.HexColor, _ any) {
		r.circles = append(r.circles, oracleDrawCall{p1: center, radius: radius, color: color})
	}
	draw.DrawSolidCircleFcn = func(_ box2d.Transform, _ float64, _ box2d.HexColor, _ any) {}
	draw.DrawSolidCapsuleFcn = func(_, _ box2d.Vec2, _ float64, _ box2d.HexColor, _ any) {}
	draw.DrawLineFcn = func(p1, p2 box2d.Vec2, color box2d.HexColor, _ any) {
		r.lines = append(r.lines, oracleDrawCall{p1: p1, p2: p2, color: color})
	}
	draw.DrawTransformFcn = func(_ box2d.Transform, _ any) {}
	draw.DrawPointFcn = func(p box2d.Vec2, size float64, color box2d.HexColor, _ any) {
		r.points = append(r.points, oracleDrawCall{p1: p, size: size, color: color})
	}
	draw.DrawStringFcn = func(p box2d.Vec2, s string, color box2d.HexColor, _ any) {
		r.strings = append(r.strings, oracleDrawCall{p1: p, text: s, color: color})
	}

	return draw
}

func oracleCountColor(calls []oracleDrawCall, color box2d.HexColor) int {
	count := 0
	for _, call := range calls {
		if call.color == color {
			count++
		}
	}
	return count
}

func TestOracleDrawJoint(t *testing.T) {
	t.Parallel()

	t.Run("WeldJointDrawsTwoBoxes", func(t *testing.T) {
		t.Parallel()

		// weld_joint.c:465-488
		//   box = b2MakeBox( 0.25f * drawScale, 0.125f * drawScale );
		//   points[i] = b2TransformPoint( frameA, box.vertices[i] );
		//   DrawPolygonFcn( points, 4, b2_colorDarkOrange );
		//   points[i] = b2TransformPoint( frameB, box.vertices[i] );
		//   DrawPolygonFcn( points, 4, b2_colorDarkCyan );
		// joint.c:1461 — drawScale = b2MaxFloat( 0.0001f, draw->jointScale *
		// joint->drawScale ), which is 1 * 1 here.
		world := oracleJointWorld(t, box2d.Vec2Zero)
		bodyA := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		bodyB := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		oracleWeldJoint(world, bodyA, bodyB, nil)

		var recorder oracleDrawRecorder
		draw := recorder.debugDraw()
		world.Draw(&draw)

		require.Len(t, recorder.polygons, 2)
		require.Equal(t, box2d.ColorDarkOrange, recorder.polygons[0].color)
		require.Equal(t, box2d.ColorDarkCyan, recorder.polygons[1].color)
		require.Len(t, recorder.polygons[0].vertices, 4)
		require.Len(t, recorder.polygons[1].vertices, 4)

		// Frame A is the identity transform on a body at the origin, so the
		// first polygon is the raw box from the C formula.
		box := box2d.MakeBox(0.25, 0.125)
		for i, vertex := range recorder.polygons[0].vertices {
			require.InDelta(t, box.Vertices[i].X, vertex.X, 1e-12)
			require.InDelta(t, box.Vertices[i].Y, vertex.Y, 1e-12)
		}
		// Frame B rides on the body at (1, 0), so the second polygon is the
		// same box translated by one metre.
		for i, vertex := range recorder.polygons[1].vertices {
			require.InDelta(t, box.Vertices[i].X+1.0, vertex.X, 1e-12)
			require.InDelta(t, box.Vertices[i].Y, vertex.Y, 1e-12)
		}
	})

	t.Run("DistanceJointDrawsLimitsAndSpringRest", func(t *testing.T) {
		t.Parallel()

		// distance_joint.c b2DrawDistanceJoint:
		//   min<max && enableLimit  -> LightGreen tick (minLength > slop),
		//                              Red tick (maxLength < B2_HUGE),
		//                              Gray span between them
		//   always                  -> White line pA..pB, White points at pA, pB
		//   hertz > 0 && spring     -> Blue point at the rest length
		world := oracleJointWorld(t, box2d.Vec2Zero)
		bodyA := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		bodyB := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2{X: 3.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleDistanceJoint(world, bodyA, bodyB, func(def *box2d.DistanceJointDef) {
			def.Length = 2.0
			def.EnableSpring = true
			def.Hertz = 2.0
			def.EnableLimit = true
			def.MinLength = 1.0
			def.MaxLength = 4.0
		})

		var recorder oracleDrawRecorder
		draw := recorder.debugDraw()
		world.Draw(&draw)

		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorLightGreen))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorRed))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorGray))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorWhite))
		require.Equal(t, 2, oracleCountColor(recorder.points, box2d.ColorWhite))
		require.Equal(t, 1, oracleCountColor(recorder.points, box2d.ColorBlue))

		// Turning the spring off drops the rest-length marker; turning the
		// limit off drops all three limit lines.
		world.EnableDistanceJointSpring(jointID, false)
		world.EnableDistanceJointLimit(jointID, false)

		recorder = oracleDrawRecorder{}
		draw = recorder.debugDraw()
		world.Draw(&draw)

		require.Equal(t, 0, oracleCountColor(recorder.lines, box2d.ColorLightGreen))
		require.Equal(t, 0, oracleCountColor(recorder.lines, box2d.ColorRed))
		require.Equal(t, 0, oracleCountColor(recorder.lines, box2d.ColorGray))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorWhite))
		require.Equal(t, 0, oracleCountColor(recorder.points, box2d.ColorBlue))
	})

	t.Run("DistanceJointSkipsDegenerateLimitTicks", func(t *testing.T) {
		t.Parallel()

		// distance_joint.c guards each tick separately:
		//   minLength > B2_LINEAR_SLOP  -> LightGreen
		//   maxLength < B2_HUGE         -> Red
		//   both                        -> Gray span
		// b2DefaultDistanceJointDef leaves maxLength at B2_HUGE and minLength
		// at zero, so an enabled limit draws none of the three.
		world := oracleJointWorld(t, box2d.Vec2Zero)
		bodyA := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		bodyB := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2{X: 3.0, Y: 0.0}, box2d.RotIdentity)
		oracleDistanceJoint(world, bodyA, bodyB, func(def *box2d.DistanceJointDef) {
			def.Length = 2.0
			def.EnableLimit = true
		})

		var recorder oracleDrawRecorder
		draw := recorder.debugDraw()
		world.Draw(&draw)

		require.Equal(t, 0, oracleCountColor(recorder.lines, box2d.ColorLightGreen))
		require.Equal(t, 0, oracleCountColor(recorder.lines, box2d.ColorRed))
		require.Equal(t, 0, oracleCountColor(recorder.lines, box2d.ColorGray))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorWhite))
	})

	t.Run("PrismaticJointDrawsLimitsAndSpringTarget", func(t *testing.T) {
		t.Parallel()

		// prismatic_joint.c b2DrawPrismaticJoint:
		//   DimGray line frameA..frameB
		//   enableLimit -> Gray span, Green tick at lower, Red tick at upper
		//   else        -> a single Gray axis line
		//   enableSpring-> Violet point (size 8) at the target translation
		//   always      -> Gray point (size 5) at frameA, Blue point at frameB
		world := oracleJointWorld(t, box2d.Vec2Zero)
		bodyA := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		bodyB := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oraclePrismaticJoint(world, bodyA, bodyB, func(def *box2d.PrismaticJointDef) {
			def.EnableLimit = true
			def.LowerTranslation = -1.0
			def.UpperTranslation = 2.0
			def.EnableSpring = true
			def.TargetTranslation = 0.5
		})

		var recorder oracleDrawRecorder
		draw := recorder.debugDraw()
		world.Draw(&draw)

		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorDimGray))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorGray))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorGreen))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorRed))
		require.Equal(t, 1, oracleCountColor(recorder.points, box2d.ColorViolet))
		require.Equal(t, 1, oracleCountColor(recorder.points, box2d.ColorGray))
		require.Equal(t, 1, oracleCountColor(recorder.points, box2d.ColorBlue))

		// The violet marker sits at frameA.p + targetTranslation * axisA,
		// which is (0.5, 0) here, and is drawn at size 8.
		for _, point := range recorder.points {
			if point.color == box2d.ColorViolet {
				require.InDelta(t, 0.5, point.p1.X, 1e-12)
				require.InDelta(t, 0.0, point.p1.Y, 1e-12)
				require.InDelta(t, 8.0, point.size, 0)
			}
		}

		// Without the limit the three limit lines collapse into one Gray axis
		// line, and without the spring the violet marker disappears.
		world.EnablePrismaticJointLimit(jointID, false)
		world.EnablePrismaticJointSpring(jointID, false)

		recorder = oracleDrawRecorder{}
		draw = recorder.debugDraw()
		world.Draw(&draw)

		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorDimGray))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorGray))
		require.Equal(t, 0, oracleCountColor(recorder.lines, box2d.ColorGreen))
		require.Equal(t, 0, oracleCountColor(recorder.lines, box2d.ColorRed))
		require.Equal(t, 0, oracleCountColor(recorder.points, box2d.ColorViolet))
	})

	t.Run("WheelJointDrawsLimits", func(t *testing.T) {
		t.Parallel()

		// wheel_joint.c b2DrawWheelJoint:
		//   Blue line frameA..frameB
		//   enableLimit -> Gray span, Green tick, Red tick (offset 0.1*scale)
		//   else        -> a single Gray axis line
		//   always      -> Gray point (size 5) at frameA, DimGray point at frameB
		world := oracleJointWorld(t, box2d.Vec2Zero)
		bodyA := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		bodyB := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleWheelJoint(world, bodyA, bodyB, func(def *box2d.WheelJointDef) {
			def.EnableLimit = true
			def.LowerTranslation = -1.0
			def.UpperTranslation = 2.0
		})

		var recorder oracleDrawRecorder
		draw := recorder.debugDraw()
		world.Draw(&draw)

		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorBlue))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorGray))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorGreen))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorRed))
		require.Equal(t, 1, oracleCountColor(recorder.points, box2d.ColorGray))
		require.Equal(t, 1, oracleCountColor(recorder.points, box2d.ColorDimGray))

		world.EnableWheelJointLimit(jointID, false)

		recorder = oracleDrawRecorder{}
		draw = recorder.debugDraw()
		world.Draw(&draw)

		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorBlue))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorGray))
		require.Equal(t, 0, oracleCountColor(recorder.lines, box2d.ColorGreen))
		require.Equal(t, 0, oracleCountColor(recorder.lines, box2d.ColorRed))
	})

	t.Run("RevoluteJointDrawsHingeLimitsAndSpring", func(t *testing.T) {
		t.Parallel()

		// revolute_joint.c b2DrawRevoluteJoint:
		//   Gray circle of radius 0.25 * drawScale at frameB.p
		//   Gray radius line on frameA, Blue radius line on frameB
		//   enableLimit  -> Green line at the lower angle, Red at the upper
		//   enableSpring -> Violet line at the target angle
		//   always       -> three Gold connector lines
		world := oracleJointWorld(t, box2d.Vec2Zero)
		bodyA := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		bodyB := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		jointID := oracleRevoluteJoint(world, bodyA, bodyB, func(def *box2d.RevoluteJointDef) {
			def.EnableLimit = true
			def.LowerAngle = -0.5
			def.UpperAngle = 0.5
			def.EnableSpring = true
			def.TargetAngle = 0.25
		})

		var recorder oracleDrawRecorder
		draw := recorder.debugDraw()
		world.Draw(&draw)

		require.Len(t, recorder.circles, 1)
		require.Equal(t, box2d.ColorGray, recorder.circles[0].color)
		require.InDelta(t, 0.25, recorder.circles[0].radius, 0)
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorGray))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorBlue))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorGreen))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorRed))
		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorViolet))
		require.Equal(t, 3, oracleCountColor(recorder.lines, box2d.ColorGold))

		world.EnableRevoluteJointLimit(jointID, false)
		world.EnableRevoluteJointSpring(jointID, false)

		recorder = oracleDrawRecorder{}
		draw = recorder.debugDraw()
		world.Draw(&draw)

		require.Equal(t, 0, oracleCountColor(recorder.lines, box2d.ColorGreen))
		require.Equal(t, 0, oracleCountColor(recorder.lines, box2d.ColorRed))
		require.Equal(t, 0, oracleCountColor(recorder.lines, box2d.ColorViolet))
		require.Equal(t, 3, oracleCountColor(recorder.lines, box2d.ColorGold))
	})

	t.Run("MotorJointDrawsTwoAnchorPoints", func(t *testing.T) {
		t.Parallel()

		// joint.c:1450-1454
		//   DrawPointFcn( pA, 8.0f, b2_colorYellowGreen );
		//   DrawPointFcn( pB, 8.0f, b2_colorPlum );
		//   DrawLineFcn( pA, pB, b2_colorLightGray );
		world := oracleJointWorld(t, box2d.Vec2Zero)
		bodyA := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		bodyB := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		oracleMotorJoint(world, bodyA, bodyB, nil)

		var recorder oracleDrawRecorder
		draw := recorder.debugDraw()
		world.Draw(&draw)

		require.Len(t, recorder.points, 2)
		require.Equal(t, box2d.ColorYellowGreen, recorder.points[0].color)
		require.InDelta(t, 8.0, recorder.points[0].size, 0)
		require.Equal(t, box2d.ColorPlum, recorder.points[1].color)
		require.InDelta(t, 8.0, recorder.points[1].size, 0)
		require.Len(t, recorder.lines, 1)
		require.Equal(t, box2d.ColorLightGray, recorder.lines[0].color)
		require.InDelta(t, 0.0, recorder.lines[0].p1.X, 1e-12)
		require.InDelta(t, 1.0, recorder.lines[0].p2.X, 1e-12)
	})

	t.Run("FilterJointDrawsOneGoldLine", func(t *testing.T) {
		t.Parallel()

		// joint.c:1446-1448 — `DrawLineFcn( pA, pB, b2_colorGold )`.
		world := oracleJointWorld(t, box2d.Vec2Zero)
		bodyA := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		bodyB := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2{X: 1.0, Y: 0.0}, box2d.RotIdentity)
		oracleFilterJoint(world, bodyA, bodyB)

		var recorder oracleDrawRecorder
		draw := recorder.debugDraw()
		world.Draw(&draw)

		require.Len(t, recorder.lines, 1)
		require.Equal(t, box2d.ColorGold, recorder.lines[0].color)
		require.Empty(t, recorder.points)
	})

	t.Run("JointExtrasDrawForceVectorAndLabel", func(t *testing.T) {
		t.Parallel()

		// joint.c:1500-1512
		//   p = b2Lerp( pA, pB, 0.5f );
		//   DrawLineFcn( p, b2MulAdd( p, 0.001f, force ), b2_colorAzure );
		//   snprintf( buffer, 64, "f = [%g, %g], t = %g", ... );
		//   DrawStringFcn( p, buffer, b2_colorAzure );
		world := oracleJointWorld(t, box2d.Vec2Zero)
		bodyA := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		bodyB := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2{X: 2.0, Y: 0.0}, box2d.RotIdentity)
		oracleWeldJoint(world, bodyA, bodyB, nil)

		var recorder oracleDrawRecorder
		draw := recorder.debugDraw()
		draw.DrawJointExtras = true
		world.Draw(&draw)

		require.Equal(t, 1, oracleCountColor(recorder.lines, box2d.ColorAzure))
		require.Len(t, recorder.strings, 1)
		require.Equal(t, box2d.ColorAzure, recorder.strings[0].color)
		require.Contains(t, recorder.strings[0].text, "f = [")
		require.Contains(t, recorder.strings[0].text, "], t = ")
		// The midpoint of the two anchors.
		require.InDelta(t, 1.0, recorder.strings[0].p1.X, 1e-12)
		require.InDelta(t, 0.0, recorder.strings[0].p1.Y, 1e-12)
	})

	t.Run("RevoluteJointExtrasAddAngleLabel", func(t *testing.T) {
		t.Parallel()

		// revolute_joint.c b2DrawRevoluteJoint emits its own degree label when
		// draw->drawJointExtras is set, on top of the shared force label from
		// joint.c, so two strings arrive for one revolute joint.
		world := oracleJointWorld(t, box2d.Vec2Zero)
		bodyA := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		bodyB := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.MakeRot(0.5))
		oracleRevoluteJoint(world, bodyA, bodyB, nil)

		var recorder oracleDrawRecorder
		draw := recorder.debugDraw()
		draw.DrawJointExtras = true
		world.Draw(&draw)

		require.Len(t, recorder.strings, 2)
		require.Contains(t, recorder.strings[0].text, "deg")
		require.Equal(t, box2d.ColorWhite, recorder.strings[0].color)
		require.Contains(t, recorder.strings[1].text, "f = [")
	})

	t.Run("GraphColorsMarkTheJointMidpoint", func(t *testing.T) {
		t.Parallel()

		// joint.c:1490-1498 — an awake joint carries a graph colour index, and
		// the marker is a size-5 point at the midpoint of the two anchors.
		world := oracleJointWorld(t, box2d.Vec2Zero)
		bodyA := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		bodyB := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 2.0, Y: 0.0}, box2d.RotIdentity)
		oracleWeldJoint(world, bodyA, bodyB, nil)

		var recorder oracleDrawRecorder
		draw := recorder.debugDraw()
		draw.DrawGraphColors = true
		world.Draw(&draw)

		require.Len(t, recorder.points, 1)
		require.InDelta(t, 5.0, recorder.points[0].size, 0)
		require.InDelta(t, 1.0, recorder.points[0].p1.X, 1e-12)
		require.InDelta(t, 0.0, recorder.points[0].p1.Y, 1e-12)
	})

	t.Run("DisabledBodySuppressesTheJoint", func(t *testing.T) {
		t.Parallel()

		// joint.c b2DrawJoint returns early when either attached body is in
		// the disabled solver set.
		world := oracleJointWorld(t, box2d.Vec2Zero)
		bodyA := oracleJointBody(t, world, box2d.StaticBody, box2d.Vec2Zero, box2d.RotIdentity)
		bodyB := oracleJointBody(t, world, box2d.DynamicBody, box2d.Vec2{X: 2.0, Y: 0.0}, box2d.RotIdentity)
		oracleWeldJoint(world, bodyA, bodyB, nil)

		var recorder oracleDrawRecorder
		draw := recorder.debugDraw()
		world.Draw(&draw)
		require.Len(t, recorder.polygons, 2)

		world.DisableBody(bodyB)

		recorder = oracleDrawRecorder{}
		draw = recorder.debugDraw()
		world.Draw(&draw)
		require.Empty(t, recorder.polygons)
	})
}
