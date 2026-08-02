// Behavior tests for the float64 port of Box2D v3.2.0 joints (stage E8):
// CRUD across all joint types, distance joint (rigid/spring/limit/motor),
// revolute joint (pendulum/limit/motor/reactions), b2Body_SetType, sleeping
// with joints, joint events, collide-connected toggling and cross-world
// determinism.

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

const jointTimeStep = 1.0 / 60.0

// jointTestWorld creates a world with the given gravity.
func jointTestWorld(gravity box2d.Vec2) *box2d.World {
	def := box2d.DefaultWorldDef()
	def.Gravity = gravity
	return box2d.NewWorld(&def)
}

// jointTestCircleBody creates a dynamic circle body (radius 0.25, density 1).
func jointTestCircleBody(w *box2d.World, pos box2d.Vec2) box2d.BodyID {
	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = pos
	bodyID := w.CreateBody(&bd)

	circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.25}
	sd := box2d.DefaultShapeDef()
	w.CreateCircleShape(bodyID, &sd, &circle)
	return bodyID
}

// jointTestStaticBody creates a static body with no shapes.
func jointTestStaticBody(w *box2d.World, pos box2d.Vec2) box2d.BodyID {
	bd := box2d.DefaultBodyDef()
	bd.Position = pos
	return w.CreateBody(&bd)
}

func TestJointCRUDAllTypes(t *testing.T) {
	w := jointTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	bodyA := jointTestCircleBody(w, box2d.Vec2{X: 0.0, Y: 0.0})
	bodyB := jointTestCircleBody(w, box2d.Vec2{X: 1.0, Y: 0.0})

	makeBase := func(userData uint64) box2d.JointDef {
		dd := box2d.DefaultDistanceJointDef()
		base := dd.Base
		base.BodyIDA = bodyA
		base.BodyIDB = bodyB
		base.UserData = userData
		return base
	}

	var jointIDs []box2d.JointID
	var wantTypes []box2d.JointType

	{
		def := box2d.DefaultDistanceJointDef()
		def.Base = makeBase(1)
		jointIDs = append(jointIDs, w.CreateDistanceJoint(&def))
		wantTypes = append(wantTypes, box2d.DistanceJoint)
	}
	{
		def := box2d.DefaultMotorJointDef()
		def.Base = makeBase(2)
		jointIDs = append(jointIDs, w.CreateMotorJoint(&def))
		wantTypes = append(wantTypes, box2d.MotorJoint)
	}
	{
		def := box2d.DefaultFilterJointDef()
		def.Base = makeBase(3)
		jointIDs = append(jointIDs, w.CreateFilterJoint(&def))
		wantTypes = append(wantTypes, box2d.FilterJoint)
	}
	{
		def := box2d.DefaultPrismaticJointDef()
		def.Base = makeBase(4)
		jointIDs = append(jointIDs, w.CreatePrismaticJoint(&def))
		wantTypes = append(wantTypes, box2d.PrismaticJoint)
	}
	{
		def := box2d.DefaultRevoluteJointDef()
		def.Base = makeBase(5)
		jointIDs = append(jointIDs, w.CreateRevoluteJoint(&def))
		wantTypes = append(wantTypes, box2d.RevoluteJoint)
	}
	{
		def := box2d.DefaultWeldJointDef()
		def.Base = makeBase(6)
		jointIDs = append(jointIDs, w.CreateWeldJoint(&def))
		wantTypes = append(wantTypes, box2d.WeldJoint)
	}
	{
		def := box2d.DefaultWheelJointDef()
		def.Base = makeBase(7)
		jointIDs = append(jointIDs, w.CreateWheelJoint(&def))
		wantTypes = append(wantTypes, box2d.WheelJoint)
	}

	require.Equal(t, 7, w.BodyJointCount(bodyA))
	require.Equal(t, 7, w.BodyJointCount(bodyB))
	require.Equal(t, 7, w.Counters().JointCount)

	jointArray := make([]box2d.JointID, 8)
	require.Equal(t, 7, w.BodyJoints(bodyA, jointArray))

	for i, jointID := range jointIDs {
		require.True(t, w.IsJointValid(jointID))
		require.Equal(t, wantTypes[i], w.JointType(jointID))
		require.Equal(t, bodyA, w.JointBodyA(jointID))
		require.Equal(t, bodyB, w.JointBodyB(jointID))
		require.Equal(t, uint64(i+1), w.JointUserData(jointID))

		w.SetJointUserData(jointID, uint64(100+i))
		require.Equal(t, uint64(100+i), w.JointUserData(jointID))

		// Base tuning accessors work for every type.
		w.SetJointForceThreshold(jointID, 10.0)
		require.InDelta(t, 10.0, w.JointForceThreshold(jointID), 0)
		w.SetJointTorqueThreshold(jointID, 20.0)
		require.InDelta(t, 20.0, w.JointTorqueThreshold(jointID), 0)
		w.SetJointConstraintTuning(jointID, 30.0, 3.0)
		hertz, damping := w.JointConstraintTuning(jointID)
		require.InDelta(t, 30.0, hertz, 0)
		require.InDelta(t, 3.0, damping, 0)

		frame := box2d.Transform{P: box2d.Vec2{X: 0.5, Y: 0.25}, Q: box2d.RotIdentity}
		w.SetJointLocalFrameA(jointID, frame)
		require.Equal(t, frame, w.JointLocalFrameA(jointID))
		w.SetJointLocalFrameB(jointID, frame)
		require.Equal(t, frame, w.JointLocalFrameB(jointID))
	}

	// NOTE: no Step here — motor/prismatic/weld/wheel joints are E9
	// placeholders whose prepare functions panic.

	for _, jointID := range jointIDs {
		w.DestroyJoint(jointID, true)
		require.False(t, w.IsJointValid(jointID))
	}

	require.Equal(t, 0, w.BodyJointCount(bodyA))
	require.Equal(t, 0, w.BodyJointCount(bodyB))
	require.Equal(t, 0, w.Counters().JointCount)

	// The world still steps after all placeholders are gone.
	w.Step(jointTimeStep, 4)
}

func TestDestroyBodyDestroysJoints(t *testing.T) {
	w := jointTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	bodyA := jointTestCircleBody(w, box2d.Vec2{X: 0.0, Y: 0.0})
	bodyB := jointTestCircleBody(w, box2d.Vec2{X: 1.0, Y: 0.0})
	bodyC := jointTestCircleBody(w, box2d.Vec2{X: 2.0, Y: 0.0})

	def1 := box2d.DefaultDistanceJointDef()
	def1.Base.BodyIDA = bodyA
	def1.Base.BodyIDB = bodyB
	joint1 := w.CreateDistanceJoint(&def1)

	def2 := box2d.DefaultDistanceJointDef()
	def2.Base.BodyIDA = bodyB
	def2.Base.BodyIDB = bodyC
	joint2 := w.CreateDistanceJoint(&def2)

	require.Equal(t, 2, w.BodyJointCount(bodyB))

	w.DestroyBody(bodyB)

	require.False(t, w.IsJointValid(joint1))
	require.False(t, w.IsJointValid(joint2))
	require.Equal(t, 0, w.BodyJointCount(bodyA))
	require.Equal(t, 0, w.BodyJointCount(bodyC))
	require.Equal(t, 0, w.Counters().JointCount)

	w.Step(jointTimeStep, 4)
}

func TestJointCollideConnectedTogglesContacts(t *testing.T) {
	w := jointTestWorld(box2d.Vec2Zero)
	defer w.Destroy()

	// Two overlapping circles.
	bodyA := jointTestCircleBody(w, box2d.Vec2{X: 0.0, Y: 0.0})
	bodyB := jointTestCircleBody(w, box2d.Vec2{X: 0.3, Y: 0.0})

	def := box2d.DefaultDistanceJointDef()
	def.Base.BodyIDA = bodyA
	def.Base.BodyIDB = bodyB
	def.Length = 0.3
	def.Base.CollideConnected = false
	jointID := w.CreateDistanceJoint(&def)
	require.False(t, w.JointCollideConnected(jointID))

	w.Step(jointTimeStep, 4)
	require.Equal(t, 0, w.BodyContactCapacity(bodyA))

	// Enabling collision buffers broad-phase moves; the next step creates the
	// contact pair between the connected bodies.
	w.SetJointCollideConnected(jointID, true)
	require.True(t, w.JointCollideConnected(jointID))
	w.Step(jointTimeStep, 4)
	require.Positive(t, w.BodyContactCapacity(bodyA))

	// Disabling collision destroys the contacts immediately.
	w.SetJointCollideConnected(jointID, false)
	require.Equal(t, 0, w.BodyContactCapacity(bodyA))
	w.Step(jointTimeStep, 4)
	require.Equal(t, 0, w.BodyContactCapacity(bodyA))
}

func TestDistanceJointRigid(t *testing.T) {
	w := jointTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	bodyA := jointTestCircleBody(w, box2d.Vec2{X: 0.0, Y: 0.0})
	bodyB := jointTestCircleBody(w, box2d.Vec2{X: 1.0, Y: 0.0})

	// Small relative velocity so the constraint actually works.
	w.SetBodyLinearVelocity(bodyB, box2d.Vec2{X: 0.0, Y: 1.0})

	def := box2d.DefaultDistanceJointDef()
	def.Base.BodyIDA = bodyA
	def.Base.BodyIDB = bodyB
	def.Length = 1.0
	jointID := w.CreateDistanceJoint(&def)

	require.InDelta(t, 1.0, w.DistanceJointCurrentLength(jointID), 1e-12)
	require.InDelta(t, 1.0, w.DistanceJointLength(jointID), 0)

	for range 120 {
		w.Step(jointTimeStep, 4)
	}

	pA := w.BodyPosition(bodyA)
	pB := w.BodyPosition(bodyB)
	dist := box2d.Distance(pA, pB)
	require.InDelta(t, 1.0, dist, 1e-3)
	require.InDelta(t, dist, w.DistanceJointCurrentLength(jointID), 1e-12)
}

func TestDistanceJointSpring(t *testing.T) {
	w := jointTestWorld(box2d.Vec2Zero)
	defer w.Destroy()

	anchor := jointTestStaticBody(w, box2d.Vec2{X: 0.0, Y: 0.0})
	body := jointTestCircleBody(w, box2d.Vec2{X: 1.5, Y: 0.0})

	def := box2d.DefaultDistanceJointDef()
	def.Base.BodyIDA = anchor
	def.Base.BodyIDB = body
	def.Length = 1.0
	def.EnableSpring = true
	def.Hertz = 2.0
	def.DampingRatio = 0.1
	jointID := w.CreateDistanceJoint(&def)
	require.True(t, w.IsDistanceJointSpringEnabled(jointID))
	require.InDelta(t, 2.0, w.DistanceJointSpringHertz(jointID), 0)
	require.InDelta(t, 0.1, w.DistanceJointSpringDampingRatio(jointID), 0)

	// The spring must overshoot below the rest length (oscillation) ...
	minLength := 1.5
	for range 120 {
		w.Step(jointTimeStep, 4)
		minLength = math.Min(minLength, w.DistanceJointCurrentLength(jointID))
	}
	require.Less(t, minLength, 0.99)

	// ... and settle near the rest length.
	for range 600 {
		w.Step(jointTimeStep, 4)
	}
	require.InDelta(t, 1.0, w.DistanceJointCurrentLength(jointID), 0.02)
	require.Less(t, box2d.Length(w.BodyLinearVelocity(body)), 0.05)
}

func TestDistanceJointLimit(t *testing.T) {
	w := jointTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	anchor := jointTestStaticBody(w, box2d.Vec2{X: 0.0, Y: 0.0})
	body := jointTestCircleBody(w, box2d.Vec2{X: 0.0, Y: -1.0})

	// Weak spring so gravity stretches the joint into the limit.
	def := box2d.DefaultDistanceJointDef()
	def.Base.BodyIDA = anchor
	def.Base.BodyIDB = body
	def.Length = 1.0
	def.EnableSpring = true
	def.Hertz = 0.5
	def.DampingRatio = 0.5
	def.EnableLimit = true
	def.MinLength = 0.5
	def.MaxLength = 1.2
	jointID := w.CreateDistanceJoint(&def)
	require.True(t, w.IsDistanceJointLimitEnabled(jointID))
	require.InDelta(t, 0.5, w.DistanceJointMinLength(jointID), 0)
	require.InDelta(t, 1.2, w.DistanceJointMaxLength(jointID), 0)

	maxLength := 0.0
	for range 240 {
		w.Step(jointTimeStep, 4)
		maxLength = math.Max(maxLength, w.DistanceJointCurrentLength(jointID))
	}

	// The limit clamps the stretch (small tolerance for the soft solver).
	require.Less(t, maxLength, 1.2+0.01)
	require.InDelta(t, 1.2, w.DistanceJointCurrentLength(jointID), 0.01)
}

func TestDistanceJointMotor(t *testing.T) {
	w := jointTestWorld(box2d.Vec2Zero)
	defer w.Destroy()

	anchor := jointTestStaticBody(w, box2d.Vec2{X: 0.0, Y: 0.0})
	body := jointTestCircleBody(w, box2d.Vec2{X: 1.0, Y: 0.0})

	def := box2d.DefaultDistanceJointDef()
	def.Base.BodyIDA = anchor
	def.Base.BodyIDB = body
	def.Length = 1.0
	// The motor only runs on the soft path: spring enabled with zero hertz.
	def.EnableSpring = true
	def.Hertz = 0.0
	def.EnableMotor = true
	def.MotorSpeed = 0.5
	def.MaxMotorForce = 1000.0
	jointID := w.CreateDistanceJoint(&def)
	require.True(t, w.IsDistanceJointMotorEnabled(jointID))
	require.InDelta(t, 0.5, w.DistanceJointMotorSpeed(jointID), 0)
	require.InDelta(t, 1000.0, w.DistanceJointMaxMotorForce(jointID), 0)

	for range 60 {
		w.Step(jointTimeStep, 4)
	}

	// The motor drives the length change to motorSpeed within a band.
	speed := box2d.Length(w.BodyLinearVelocity(body))
	require.InDelta(t, 0.5, speed, 0.05)
	require.InDelta(t, 1.5, w.DistanceJointCurrentLength(jointID), 0.1)
	require.True(t, box2d.IsValidFloat(w.DistanceJointMotorForce(jointID)))
}

// jointTestPendulum builds a revolute pendulum: a dynamic circle attached to
// a static anchor at the origin with arm length 1.
func jointTestPendulum(w *box2d.World, def *box2d.RevoluteJointDef, bobPos box2d.Vec2) (box2d.BodyID, box2d.JointID) {
	anchor := jointTestStaticBody(w, box2d.Vec2Zero)
	bob := jointTestCircleBody(w, bobPos)

	def.Base.BodyIDA = anchor
	def.Base.BodyIDB = bob
	def.Base.LocalFrameA.P = box2d.Vec2Zero
	def.Base.LocalFrameB.P = box2d.Vec2{X: -bobPos.X, Y: -bobPos.Y}
	jointID := w.CreateRevoluteJoint(def)
	return bob, jointID
}

func TestRevoluteJointPendulum(t *testing.T) {
	w := jointTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	def := box2d.DefaultRevoluteJointDef()
	bob, jointID := jointTestPendulum(w, &def, box2d.Vec2{X: 1.0, Y: 0.0})

	// Energy sanity: starting horizontal with arm length 1, the speed can
	// never exceed sqrt(2*g*h) = sqrt(20) plus solver slack.
	maxSpeed := math.Sqrt(20.0) + 0.5

	swungDown := false
	for range 300 {
		w.Step(jointTimeStep, 4)

		p := w.BodyPosition(bob)
		require.InDelta(t, 1.0, box2d.Length(p), 0.02, "pendulum bob left the circle")
		require.Less(t, box2d.Length(w.BodyLinearVelocity(bob)), maxSpeed, "pendulum gained energy")
		if p.Y < -0.5 {
			swungDown = true
		}
	}

	require.True(t, swungDown, "pendulum did not swing")
	require.Less(t, w.JointLinearSeparation(jointID), 0.01)
}

func TestRevoluteJointLimit(t *testing.T) {
	w := jointTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	def := box2d.DefaultRevoluteJointDef()
	def.EnableLimit = true
	def.LowerAngle = -0.25
	def.UpperAngle = 0.25
	_, jointID := jointTestPendulum(w, &def, box2d.Vec2{X: 1.0, Y: 0.0})

	require.True(t, w.IsRevoluteJointLimitEnabled(jointID))
	require.InDelta(t, -0.25, w.RevoluteJointLowerLimit(jointID), 0)
	require.InDelta(t, 0.25, w.RevoluteJointUpperLimit(jointID), 0)

	for range 300 {
		w.Step(jointTimeStep, 4)
		angle := w.RevoluteJointAngle(jointID)
		require.GreaterOrEqual(t, angle, -0.25-0.02)
		require.LessOrEqual(t, angle, 0.25+0.02)
	}

	// Gravity holds the pendulum against the lower limit at steady state.
	require.InDelta(t, -0.25, w.RevoluteJointAngle(jointID), 0.03)
}

func TestRevoluteJointMotor(t *testing.T) {
	w := jointTestWorld(box2d.Vec2Zero)
	defer w.Destroy()

	def := box2d.DefaultRevoluteJointDef()
	def.EnableMotor = true
	def.MotorSpeed = 2.0
	def.MaxMotorTorque = 100.0
	bob, jointID := jointTestPendulum(w, &def, box2d.Vec2{X: 1.0, Y: 0.0})

	require.True(t, w.IsRevoluteJointMotorEnabled(jointID))
	require.InDelta(t, 2.0, w.RevoluteJointMotorSpeed(jointID), 0)
	require.InDelta(t, 100.0, w.RevoluteJointMaxMotorTorque(jointID), 0)

	for range 120 {
		w.Step(jointTimeStep, 4)
	}

	// The motor reaches the target speed band.
	require.InDelta(t, 2.0, w.BodyAngularVelocity(bob), 0.1)
	require.True(t, box2d.IsValidFloat(w.RevoluteJointMotorTorque(jointID)))
}

func TestRevoluteJointReactions(t *testing.T) {
	w := jointTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	// Bob hanging straight down at rest: the constraint force carries the
	// weight m*g.
	def := box2d.DefaultRevoluteJointDef()
	bob, jointID := jointTestPendulum(w, &def, box2d.Vec2{X: 0.0, Y: -1.0})

	for range 120 {
		w.Step(jointTimeStep, 4)
	}

	mass := w.BodyMass(bob)
	require.Positive(t, mass)

	force := w.JointConstraintForce(jointID)
	torque := w.JointConstraintTorque(jointID)
	require.True(t, box2d.IsValidVec2(force))
	require.True(t, box2d.IsValidFloat(torque))

	weight := 10.0 * mass
	require.InDelta(t, weight, box2d.Length(force), 0.25*weight)
	require.InDelta(t, 0.0, w.JointAngularSeparation(jointID), 1e-9)
}

func TestJointEvents(t *testing.T) {
	w := jointTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	anchor := jointTestStaticBody(w, box2d.Vec2Zero)
	body := jointTestCircleBody(w, box2d.Vec2{X: 0.0, Y: -1.0})

	def := box2d.DefaultDistanceJointDef()
	def.Base.BodyIDA = anchor
	def.Base.BodyIDB = body
	def.Length = 1.0
	def.Base.UserData = 4242
	def.Base.ForceThreshold = 0.5 // well below the hanging weight
	jointID := w.CreateDistanceJoint(&def)

	for range 10 {
		w.Step(jointTimeStep, 4)
	}

	events := w.JointEvents()
	require.NotEmpty(t, events.JointEvents)
	require.Equal(t, jointID, events.JointEvents[0].JointID)
	require.Equal(t, uint64(4242), events.JointEvents[0].UserData)

	// Raising the threshold silences the event.
	w.SetJointForceThreshold(jointID, 1.0e6)
	w.Step(jointTimeStep, 4)
	require.Empty(t, w.JointEvents().JointEvents)
}

func TestJointedPairSleepsAndWakes(t *testing.T) {
	w := jointTestWorld(box2d.Vec2Zero)
	defer w.Destroy()

	bodyA := jointTestCircleBody(w, box2d.Vec2{X: 0.0, Y: 0.0})
	bodyB := jointTestCircleBody(w, box2d.Vec2{X: 1.0, Y: 0.0})

	def := box2d.DefaultDistanceJointDef()
	def.Base.BodyIDA = bodyA
	def.Base.BodyIDB = bodyB
	def.Length = 1.0
	jointID := w.CreateDistanceJoint(&def)

	for range 60 {
		w.Step(jointTimeStep, 4)
	}

	// The joined pair sleeps together.
	require.False(t, w.IsBodyAwake(bodyA))
	require.False(t, w.IsBodyAwake(bodyB))

	// Wake via the joint wake API.
	w.WakeJointBodies(jointID)
	require.True(t, w.IsBodyAwake(bodyA))
	require.True(t, w.IsBodyAwake(bodyB))

	// Solving still works after the sleep/wake round trip.
	for range 30 {
		w.Step(jointTimeStep, 4)
	}
	require.InDelta(t, 1.0, w.DistanceJointCurrentLength(jointID), 1e-3)
}

func TestSetBodyTypeDynamicToStaticMidContact(t *testing.T) {
	w := jointTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	// Ground box.
	gd := box2d.DefaultBodyDef()
	gd.Position = box2d.Vec2{X: 0.0, Y: -1.0}
	ground := w.CreateBody(&gd)
	groundBox := box2d.MakeBox(10.0, 1.0)
	gsd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	box := jointTestCircleBody(w, box2d.Vec2{X: 0.0, Y: 1.0})

	// Let the circle land and rest on the ground (mid-contact).
	for range 120 {
		w.Step(jointTimeStep, 4)
	}
	require.Positive(t, w.BodyContactCapacity(box))

	w.SetBodyType(box, box2d.StaticBody)
	require.Equal(t, box2d.StaticBody, w.BodyType(box))

	// Contacts destroyed by the type change.
	require.Equal(t, 0, w.BodyContactCapacity(box))
	require.InDelta(t, 0.0, w.BodyMass(box), 0)

	// The now-static circle does not move and another body rests on it
	// (proves the proxy moved to the static tree).
	posBefore := w.BodyPosition(box)
	ball := jointTestCircleBody(w, box2d.Vec2{X: 0.0, Y: 1.5})
	for range 180 {
		w.Step(jointTimeStep, 4)
	}
	require.Equal(t, posBefore, w.BodyPosition(box))
	require.Greater(t, w.BodyPosition(ball).Y, posBefore.Y+0.4)
}

func TestSetBodyTypeStaticToDynamicFalls(t *testing.T) {
	w := jointTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	bd := box2d.DefaultBodyDef()
	bd.Position = box2d.Vec2{X: 0.0, Y: 5.0}
	body := w.CreateBody(&bd)
	circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.25}
	sd := box2d.DefaultShapeDef()
	w.CreateCircleShape(body, &sd, &circle)

	for range 30 {
		w.Step(jointTimeStep, 4)
	}
	require.InDelta(t, 5.0, w.BodyPosition(body).Y, 0)

	w.SetBodyType(body, box2d.DynamicBody)
	require.Equal(t, box2d.DynamicBody, w.BodyType(body))
	require.Positive(t, w.BodyMass(body))

	for range 30 {
		w.Step(jointTimeStep, 4)
	}
	require.Less(t, w.BodyPosition(body).Y, 4.0)
}

func TestSetBodyTypeKinematic(t *testing.T) {
	w := jointTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	body := jointTestCircleBody(w, box2d.Vec2{X: 0.0, Y: 2.0})

	w.SetBodyType(body, box2d.KinematicBody)
	require.Equal(t, box2d.KinematicBody, w.BodyType(body))
	w.SetBodyLinearVelocity(body, box2d.Vec2{X: 1.0, Y: 0.0})

	for range 60 {
		w.Step(jointTimeStep, 4)
	}

	// Kinematic bodies ignore gravity and move with their velocity.
	p := w.BodyPosition(body)
	require.InDelta(t, 1.0, p.X, 1e-9)
	require.InDelta(t, 2.0, p.Y, 1e-9)

	// Back to dynamic: gravity applies again.
	w.SetBodyType(body, box2d.DynamicBody)
	for range 30 {
		w.Step(jointTimeStep, 4)
	}
	require.Less(t, w.BodyPosition(body).Y, 2.0)
}

func TestSetBodyTypeWithJointBothDirections(t *testing.T) {
	w := jointTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w.Destroy()

	// The anchor gets a shape so it has mass once it becomes dynamic.
	abd := box2d.DefaultBodyDef()
	abd.Position = box2d.Vec2Zero
	anchor := w.CreateBody(&abd)
	anchorCircle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.25}
	asd := box2d.DefaultShapeDef()
	w.CreateCircleShape(anchor, &asd, &anchorCircle)

	bob := jointTestCircleBody(w, box2d.Vec2{X: 0.0, Y: -1.0})

	def := box2d.DefaultRevoluteJointDef()
	def.Base.BodyIDA = anchor
	def.Base.BodyIDB = bob
	def.Base.LocalFrameB.P = box2d.Vec2{X: 0.0, Y: 1.0}
	jointID := w.CreateRevoluteJoint(&def)

	for range 30 {
		w.Step(jointTimeStep, 4)
	}

	// static -> dynamic: both bodies fall, the joint keeps holding.
	w.SetBodyType(anchor, box2d.DynamicBody)
	require.True(t, w.IsJointValid(jointID))
	for range 60 {
		w.Step(jointTimeStep, 4)
	}
	require.Less(t, w.BodyPosition(anchor).Y, -1.0)
	require.Less(t, w.JointLinearSeparation(jointID), 0.01)
	dist := box2d.Distance(w.BodyPosition(anchor), w.BodyPosition(bob))
	require.InDelta(t, 1.0, dist, 0.01)

	// dynamic -> static: the anchor freezes and the pendulum still solves.
	w.SetBodyType(anchor, box2d.StaticBody)
	require.True(t, w.IsJointValid(jointID))
	frozen := w.BodyPosition(anchor)
	for range 60 {
		w.Step(jointTimeStep, 4)
	}
	require.Equal(t, frozen, w.BodyPosition(anchor))
	require.Less(t, w.JointLinearSeparation(jointID), 0.01)
}

// jointTestMixedScene builds a scene with a ground, a revolute pendulum, a
// distance-jointed pair and a loose box, then runs a scripted SetBodyType
// sequence while stepping.
func jointTestMixedScene(w *box2d.World, steps int) []box2d.BodyID {
	gd := box2d.DefaultBodyDef()
	gd.Position = box2d.Vec2{X: 0.0, Y: -2.0}
	ground := w.CreateBody(&gd)
	groundBox := box2d.MakeBox(20.0, 1.0)
	gsd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	anchor := jointTestStaticBody(w, box2d.Vec2{X: -3.0, Y: 2.0})
	bob := jointTestCircleBody(w, box2d.Vec2{X: -2.0, Y: 2.0})
	rd := box2d.DefaultRevoluteJointDef()
	rd.Base.BodyIDA = anchor
	rd.Base.BodyIDB = bob
	rd.Base.LocalFrameB.P = box2d.Vec2{X: -1.0, Y: 0.0}
	w.CreateRevoluteJoint(&rd)

	pairA := jointTestCircleBody(w, box2d.Vec2{X: 2.0, Y: 1.0})
	pairB := jointTestCircleBody(w, box2d.Vec2{X: 3.0, Y: 1.0})
	dd := box2d.DefaultDistanceJointDef()
	dd.Base.BodyIDA = pairA
	dd.Base.BodyIDB = pairB
	dd.Length = 1.0
	w.CreateDistanceJoint(&dd)

	loose := jointTestCircleBody(w, box2d.Vec2{X: 0.0, Y: 3.0})

	bodies := []box2d.BodyID{anchor, bob, pairA, pairB, loose}

	for step := 1; step <= steps; step++ {
		switch step {
		case 20:
			w.SetBodyType(loose, box2d.StaticBody)
		case 40:
			w.SetBodyType(anchor, box2d.DynamicBody)
		case 60:
			w.SetBodyType(loose, box2d.DynamicBody)
		case 80:
			w.SetBodyType(pairA, box2d.KinematicBody)
		case 100:
			w.SetBodyType(pairA, box2d.DynamicBody)
		default:
		}
		w.Step(jointTimeStep, 4)
	}

	return bodies
}

func TestSetBodyTypeDeterminism(t *testing.T) {
	const steps = 200

	w1 := jointTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w1.Destroy()
	w2 := jointTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w2.Destroy()

	bodies1 := jointTestMixedScene(w1, steps)
	bodies2 := jointTestMixedScene(w2, steps)

	require.Equal(t, hashWorldState(w1, bodies1), hashWorldState(w2, bodies2))
	for i := range bodies1 {
		p1 := w1.BodyPosition(bodies1[i])
		p2 := w2.BodyPosition(bodies2[i])
		require.Equal(t, math.Float64bits(p1.X), math.Float64bits(p2.X))
		require.Equal(t, math.Float64bits(p1.Y), math.Float64bits(p2.Y))
	}
}

// jointTestDeterminismScene builds a distance+revolute scene for the
// two-world determinism check.
func jointTestDeterminismScene(w *box2d.World) []box2d.BodyID {
	gd := box2d.DefaultBodyDef()
	gd.Position = box2d.Vec2{X: 0.0, Y: -2.0}
	ground := w.CreateBody(&gd)
	groundBox := box2d.MakeBox(20.0, 1.0)
	gsd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	anchor := jointTestStaticBody(w, box2d.Vec2{X: 0.0, Y: 3.0})
	bob := jointTestCircleBody(w, box2d.Vec2{X: 1.0, Y: 3.0})
	rd := box2d.DefaultRevoluteJointDef()
	rd.Base.BodyIDA = anchor
	rd.Base.BodyIDB = bob
	rd.Base.LocalFrameB.P = box2d.Vec2{X: -1.0, Y: 0.0}
	w.CreateRevoluteJoint(&rd)

	swingA := jointTestCircleBody(w, box2d.Vec2{X: 4.0, Y: 2.0})
	swingB := jointTestCircleBody(w, box2d.Vec2{X: 5.0, Y: 2.0})
	w.SetBodyLinearVelocity(swingB, box2d.Vec2{X: 0.0, Y: 2.0})
	dd := box2d.DefaultDistanceJointDef()
	dd.Base.BodyIDA = swingA
	dd.Base.BodyIDB = swingB
	dd.Length = 1.0
	w.CreateDistanceJoint(&dd)

	springA := jointTestStaticBody(w, box2d.Vec2{X: -4.0, Y: 3.0})
	springB := jointTestCircleBody(w, box2d.Vec2{X: -4.0, Y: 1.0})
	sd := box2d.DefaultDistanceJointDef()
	sd.Base.BodyIDA = springA
	sd.Base.BodyIDB = springB
	sd.Length = 1.0
	sd.EnableSpring = true
	sd.Hertz = 3.0
	sd.DampingRatio = 0.2
	w.CreateDistanceJoint(&sd)

	return []box2d.BodyID{bob, swingA, swingB, springB}
}

func TestJointDeterminism(t *testing.T) {
	w1 := jointTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w1.Destroy()
	w2 := jointTestWorld(box2d.Vec2{X: 0.0, Y: -10.0})
	defer w2.Destroy()

	bodies1 := jointTestDeterminismScene(w1)
	bodies2 := jointTestDeterminismScene(w2)

	for step := 1; step <= 200; step++ {
		w1.Step(jointTimeStep, 4)
		w2.Step(jointTimeStep, 4)

		if step%50 == 0 {
			require.Equal(t, hashWorldState(w1, bodies1), hashWorldState(w2, bodies2),
				"worlds diverged at step %d", step)
		}
	}
}
