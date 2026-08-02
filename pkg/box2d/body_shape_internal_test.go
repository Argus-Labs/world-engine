// Tests for the float64 port of Box2D v3.2.0 src/body.c and src/shape.c:
// body/shape lifecycle, mass properties, transforms and broad-phase proxy
// bookkeeping. Internal package tests because they inspect solver set
// membership and shape internals directly.

package box2d

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateManyBodiesMixedTypes(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	shapeDef := DefaultShapeDef()

	circle := Circle{Center: Vec2Zero, Radius: 0.5}
	box := MakeBox(0.5, 0.5)
	capsule := Capsule{Center1: Vec2{X: -0.5, Y: 0.0}, Center2: Vec2{X: 0.5, Y: 0.0}, Radius: 0.25}
	segment := Segment{Point1: Vec2{X: -1.0, Y: 0.0}, Point2: Vec2{X: 1.0, Y: 0.0}}

	ids := make([]BodyID, 0, 100)
	shapeIDs := make([]ShapeID, 0, 100)
	for i := range 100 {
		switch i % 3 {
		case 0:
			bodyDef.Type = StaticBody
		case 1:
			bodyDef.Type = KinematicBody
		default:
			bodyDef.Type = DynamicBody
		}
		bodyDef.Position = Vec2{X: float64(i), Y: 0.0}

		id := w.CreateBody(&bodyDef)
		require.True(t, w.IsBodyValid(id))
		tassert.Equal(t, bodyDef.Type, w.BodyType(id))

		var sid ShapeID
		switch i % 4 {
		case 0:
			sid = w.CreateCircleShape(id, &shapeDef, &circle)
		case 1:
			sid = w.CreatePolygonShape(id, &shapeDef, &box)
		case 2:
			sid = w.CreateCapsuleShape(id, &shapeDef, &capsule)
		default:
			sid = w.CreateSegmentShape(id, &shapeDef, &segment)
		}
		require.True(t, w.IsShapeValid(sid))
		tassert.Equal(t, id, w.ShapeBody(sid))

		ids = append(ids, id)
		shapeIDs = append(shapeIDs, sid)
	}

	counters := w.Counters()
	tassert.Equal(t, 100, counters.BodyCount)
	tassert.Equal(t, 100, counters.ShapeCount)

	// Per-set membership: static bodies in static set, dynamic/kinematic
	// awake bodies in the awake set. The set's sim points back at the body.
	for i, id := range ids {
		b := w.getBodyFullID(id)
		if i%3 == 0 {
			tassert.Equal(t, staticSet, b.setIndex, "body %d", i)
		} else {
			tassert.Equal(t, awakeSet, b.setIndex, "body %d", i)
		}
		set := &w.solverSets[b.setIndex]
		tassert.Equal(t, b.id, set.bodySims[b.localIndex].bodyID, "body %d", i)
	}

	// Awake set has one state per sim.
	awake := &w.solverSets[awakeSet]
	tassert.Len(t, awake.bodyStates, len(awake.bodySims))

	// Shape list getters.
	buffer := make([]ShapeID, 4)
	for i, id := range ids {
		tassert.Equal(t, 1, w.BodyShapeCount(id))
		n := w.BodyShapes(id, buffer)
		require.Equal(t, 1, n)
		tassert.Equal(t, shapeIDs[i], buffer[0])
	}
}

func TestBodyGenerationBumpOnRecreate(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	stale := w.CreateBody(&bodyDef)
	require.True(t, w.IsBodyValid(stale))

	w.DestroyBody(stale)
	tassert.False(t, w.IsBodyValid(stale))

	// The id pool recycles the slot LIFO, so the fresh body reuses index1
	// with a bumped generation.
	fresh := w.CreateBody(&bodyDef)
	require.True(t, w.IsBodyValid(fresh))
	tassert.Equal(t, stale.index1, fresh.index1)
	tassert.Equal(t, stale.generation+1, fresh.generation)
	tassert.False(t, w.IsBodyValid(stale))
}

func TestShapeGenerationBumpOnRecreate(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	id := w.CreateBody(&bodyDef)

	shapeDef := DefaultShapeDef()
	circle := Circle{Center: Vec2Zero, Radius: 0.5}
	stale := w.CreateCircleShape(id, &shapeDef, &circle)
	require.True(t, w.IsShapeValid(stale))

	w.DestroyShape(stale, true)
	tassert.False(t, w.IsShapeValid(stale))

	fresh := w.CreateCircleShape(id, &shapeDef, &circle)
	require.True(t, w.IsShapeValid(fresh))
	tassert.Equal(t, stale.index1, fresh.index1)
	tassert.Equal(t, stale.generation+1, fresh.generation)
	tassert.False(t, w.IsShapeValid(stale))
}

func TestBodyMassFromShapes(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	id := w.CreateBody(&bodyDef)

	shapeDef := DefaultShapeDef()
	shapeDef.Density = 2.0

	circle := Circle{Center: Vec2Zero, Radius: 1.0}
	w.CreateCircleShape(id, &shapeDef, &circle)

	// Circle mass: density * pi * r^2.
	expectedCircleMass := 2.0 * Pi * 1.0
	tassert.InDelta(t, expectedCircleMass, w.BodyMass(id), 1e-12)

	// Add a 1x1 box: mass = density * area = 2 * 1.
	box := MakeBox(0.5, 0.5)
	w.CreatePolygonShape(id, &shapeDef, &box)
	expectedTotal := expectedCircleMass + 2.0
	tassert.InDelta(t, expectedTotal, w.BodyMass(id), 1e-12)

	// Mass equals the sum of the shape masses.
	sum := 0.0
	b := w.getBodyFullID(id)
	shapeID := b.headShapeID
	for shapeID != NullIndex {
		s := &w.shapes[shapeID]
		sum += computeShapeMass(s).Mass
		shapeID = s.nextShapeID
	}
	tassert.InDelta(t, sum, w.BodyMass(id), 1e-12)
}

func TestBodyCenterOfMassTwoOffsetShapes(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	bodyDef.Position = Vec2{X: 5.0, Y: -3.0}
	id := w.CreateBody(&bodyDef)

	shapeDef := DefaultShapeDef()
	shapeDef.Density = 1.0

	// Two unit circles at local (0,0) and (2,0): equal masses, so the local
	// center of mass is exactly (1, 0).
	c1 := Circle{Center: Vec2Zero, Radius: 1.0}
	c2 := Circle{Center: Vec2{X: 2.0, Y: 0.0}, Radius: 1.0}
	w.CreateCircleShape(id, &shapeDef, &c1)
	w.CreateCircleShape(id, &shapeDef, &c2)

	localCenter := w.BodyLocalCenterOfMass(id)
	tassert.InDelta(t, 1.0, localCenter.X, 1e-12)
	tassert.InDelta(t, 0.0, localCenter.Y, 1e-12)

	worldCenter := w.BodyWorldCenterOfMass(id)
	tassert.InDelta(t, 6.0, worldCenter.X, 1e-12)
	tassert.InDelta(t, -3.0, worldCenter.Y, 1e-12)

	// Hand-computed mass and rotational inertia about the center of mass:
	// each circle has m = pi, Ic = m * 0.5 * r^2 about its own center plus
	// the parallel axis term m * d^2 with d = 1.
	m := Pi
	expectedInertia := 2.0 * (m*0.5 + m*1.0)
	tassert.InDelta(t, 2.0*m, w.BodyMass(id), 1e-12)
	tassert.InDelta(t, expectedInertia, w.BodyRotationalInertia(id), 1e-11)
}

func TestSetBodyMassDataOverride(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	id := w.CreateBody(&bodyDef)

	shapeDef := DefaultShapeDef()
	circle := Circle{Center: Vec2Zero, Radius: 1.0}
	w.CreateCircleShape(id, &shapeDef, &circle)

	override := MassData{Mass: 5.0, Center: Vec2{X: 0.25, Y: -0.5}, RotationalInertia: 2.0}
	w.SetBodyMassData(id, override)

	tassert.InDelta(t, 5.0, w.BodyMass(id), 0.0)
	tassert.InDelta(t, 2.0, w.BodyRotationalInertia(id), 0.0)
	tassert.Equal(t, override.Center, w.BodyLocalCenterOfMass(id))
	tassert.Equal(t, override, w.BodyMassData(id))

	b := w.getBodyFullID(id)
	bSim := w.getBodySim(b)
	tassert.InDelta(t, 1.0/5.0, bSim.invMass, 1e-15)
	tassert.InDelta(t, 1.0/2.0, bSim.invInertia, 1e-15)

	// ApplyBodyMassFromShapes restores the computed values.
	w.ApplyBodyMassFromShapes(id)
	tassert.InDelta(t, Pi, w.BodyMass(id), 1e-12)
	tassert.Equal(t, Vec2Zero, w.BodyLocalCenterOfMass(id))
}

func TestMotionLocksZeroVelocities(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	id := w.CreateBody(&bodyDef)

	w.SetBodyLinearVelocity(id, Vec2{X: 3.0, Y: 4.0})
	w.SetBodyAngularVelocity(id, 2.0)

	locks := MotionLocks{LinearX: true, LinearY: false, AngularZ: true}
	w.SetBodyMotionLocks(id, locks)
	tassert.Equal(t, locks, w.BodyMotionLocks(id))

	// Locked axes zero their velocity components.
	velocity := w.BodyLinearVelocity(id)
	tassert.InDelta(t, 0.0, velocity.X, 0.0)
	tassert.InDelta(t, 4.0, velocity.Y, 0.0)
	tassert.InDelta(t, 0.0, w.BodyAngularVelocity(id), 0.0)

	// Angular velocity writes are ignored while locked.
	w.SetBodyAngularVelocity(id, 5.0)
	tassert.InDelta(t, 0.0, w.BodyAngularVelocity(id), 0.0)
}

func TestBodyTransformRoundTrip(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	bodyDef.Position = Vec2{X: 1.0, Y: 2.0}
	bodyDef.Rotation = MakeRot(0.5)
	id := w.CreateBody(&bodyDef)

	tassert.Equal(t, Vec2{X: 1.0, Y: 2.0}, w.BodyPosition(id))
	tassert.Equal(t, MakeRot(0.5), w.BodyRotation(id))

	position := Vec2{X: -3.0, Y: 7.0}
	rotation := MakeRot(-1.25)
	w.SetBodyTransform(id, position, rotation)

	transform := w.BodyTransform(id)
	tassert.Equal(t, position, transform.P)
	tassert.Equal(t, rotation, transform.Q)

	// World/local point round trip.
	localPoint := Vec2{X: 0.5, Y: -0.25}
	worldPoint := w.BodyWorldPoint(id, localPoint)
	back := w.BodyLocalPoint(id, worldPoint)
	tassert.InDelta(t, localPoint.X, back.X, 1e-14)
	tassert.InDelta(t, localPoint.Y, back.Y, 1e-14)

	// World/local vector round trip.
	localVector := Vec2{X: 1.0, Y: 1.0}
	worldVector := w.BodyWorldVector(id, localVector)
	backVector := w.BodyLocalVector(id, worldVector)
	tassert.InDelta(t, localVector.X, backVector.X, 1e-14)
	tassert.InDelta(t, localVector.Y, backVector.Y, 1e-14)
}

func TestBodyVelocitySetGet(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	id := w.CreateBody(&bodyDef)

	w.SetBodyLinearVelocity(id, Vec2{X: 1.5, Y: -2.5})
	tassert.Equal(t, Vec2{X: 1.5, Y: -2.5}, w.BodyLinearVelocity(id))

	w.SetBodyAngularVelocity(id, 3.5)
	tassert.InDelta(t, 3.5, w.BodyAngularVelocity(id), 0.0)

	// Static bodies ignore velocity writes.
	staticDef := DefaultBodyDef()
	staticID := w.CreateBody(&staticDef)
	w.SetBodyLinearVelocity(staticID, Vec2{X: 1.0, Y: 1.0})
	tassert.Equal(t, Vec2Zero, w.BodyLinearVelocity(staticID))

	// Setting velocity on a sleeping body wakes it.
	w.SetBodyAwake(id, false)
	require.False(t, w.IsBodyAwake(id))
	w.SetBodyLinearVelocity(id, Vec2{X: 0.5, Y: 0.0})
	tassert.True(t, w.IsBodyAwake(id))
	tassert.Equal(t, Vec2{X: 0.5, Y: 0.0}, w.BodyLinearVelocity(id))
}

func TestBodyDampingGravityScaleSleepThreshold(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	id := w.CreateBody(&bodyDef)

	w.SetBodyLinearDamping(id, 0.5)
	tassert.InDelta(t, 0.5, w.BodyLinearDamping(id), 0.0)

	w.SetBodyAngularDamping(id, 0.75)
	tassert.InDelta(t, 0.75, w.BodyAngularDamping(id), 0.0)

	w.SetBodyGravityScale(id, 2.0)
	tassert.InDelta(t, 2.0, w.BodyGravityScale(id), 0.0)

	w.SetBodySleepThreshold(id, 0.123)
	tassert.InDelta(t, 0.123, w.BodySleepThreshold(id), 0.0)

	tassert.True(t, w.IsBodySleepEnabled(id))
	w.EnableBodySleep(id, false)
	tassert.False(t, w.IsBodySleepEnabled(id))

	w.SetBodyBullet(id, true)
	tassert.True(t, w.IsBodyBullet(id))
	w.SetBodyBullet(id, false)
	tassert.False(t, w.IsBodyBullet(id))

	w.SetBodyName(id, "crate")
	tassert.Equal(t, "crate", w.BodyName(id))

	w.SetBodyUserData(id, 42)
	tassert.Equal(t, uint64(42), w.BodyUserData(id))
}

func TestShapeProxyCounts(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	staticDef := DefaultBodyDef()
	staticID := w.CreateBody(&staticDef)

	dynamicDef := DefaultBodyDef()
	dynamicDef.Type = DynamicBody
	dynamicID := w.CreateBody(&dynamicDef)

	kinematicDef := DefaultBodyDef()
	kinematicDef.Type = KinematicBody
	kinematicID := w.CreateBody(&kinematicDef)

	shapeDef := DefaultShapeDef()
	circle := Circle{Center: Vec2Zero, Radius: 0.5}
	segment := Segment{Point1: Vec2{X: -1.0, Y: 0.0}, Point2: Vec2{X: 1.0, Y: 0.0}}

	sStatic := w.CreateSegmentShape(staticID, &shapeDef, &segment)
	sDynamic := w.CreateCircleShape(dynamicID, &shapeDef, &circle)
	w.CreateCircleShape(kinematicID, &shapeDef, &circle)

	tassert.Equal(t, 1, w.broadPhase.trees[StaticBody].GetProxyCount())
	tassert.Equal(t, 1, w.broadPhase.trees[DynamicBody].GetProxyCount())
	tassert.Equal(t, 1, w.broadPhase.trees[KinematicBody].GetProxyCount())

	w.DestroyShape(sDynamic, true)
	tassert.Equal(t, 0, w.broadPhase.trees[DynamicBody].GetProxyCount())

	w.DestroyShape(sStatic, true)
	tassert.Equal(t, 0, w.broadPhase.trees[StaticBody].GetProxyCount())

	tassert.Equal(t, 1, w.Counters().ShapeCount)
}

func TestShapeAABBMatchesComputed(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	bodyDef.Position = Vec2{X: 2.0, Y: 3.0}
	bodyDef.Rotation = MakeRot(0.7)
	id := w.CreateBody(&bodyDef)

	shapeDef := DefaultShapeDef()
	circle := Circle{Center: Vec2{X: 0.5, Y: -0.5}, Radius: 0.5}
	sid := w.CreateCircleShape(id, &shapeDef, &circle)

	transform := w.BodyTransform(id)
	expected := ComputeCircleAABB(&circle, transform)
	expected.LowerBound.X -= SpeculativeDistance
	expected.LowerBound.Y -= SpeculativeDistance
	expected.UpperBound.X += SpeculativeDistance
	expected.UpperBound.Y += SpeculativeDistance

	tassert.Equal(t, expected, w.ShapeAABB(sid))

	// SetBodyTransform refreshes the shape AABB the same way.
	w.SetBodyTransform(id, Vec2{X: -4.0, Y: 1.0}, MakeRot(-0.3))
	transform = w.BodyTransform(id)
	expected = ComputeCircleAABB(&circle, transform)
	expected.LowerBound.X -= SpeculativeDistance
	expected.LowerBound.Y -= SpeculativeDistance
	expected.UpperBound.X += SpeculativeDistance
	expected.UpperBound.Y += SpeculativeDistance

	tassert.Equal(t, expected, w.ShapeAABB(sid))

	// ComputeBodyAABB unions the shape AABBs (single shape here).
	tassert.Equal(t, w.ShapeAABB(sid), w.ComputeBodyAABB(id))
}

func TestDestroyBodyDestroysShapes(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	id := w.CreateBody(&bodyDef)

	shapeDef := DefaultShapeDef()
	circle := Circle{Center: Vec2Zero, Radius: 0.5}
	box := MakeBox(0.5, 0.5)
	capsule := Capsule{Center1: Vec2{X: -0.5, Y: 0.0}, Center2: Vec2{X: 0.5, Y: 0.0}, Radius: 0.25}

	s1 := w.CreateCircleShape(id, &shapeDef, &circle)
	s2 := w.CreatePolygonShape(id, &shapeDef, &box)
	s3 := w.CreateCapsuleShape(id, &shapeDef, &capsule)

	require.Equal(t, 3, w.BodyShapeCount(id))
	require.Equal(t, 3, w.Counters().ShapeCount)
	require.Equal(t, 3, w.broadPhase.trees[DynamicBody].GetProxyCount())

	w.DestroyBody(id)

	tassert.False(t, w.IsBodyValid(id))
	tassert.False(t, w.IsShapeValid(s1))
	tassert.False(t, w.IsShapeValid(s2))
	tassert.False(t, w.IsShapeValid(s3))
	tassert.Equal(t, 0, w.Counters().ShapeCount)
	tassert.Equal(t, 0, w.Counters().BodyCount)
	tassert.Equal(t, 0, w.Counters().IslandCount)
	tassert.Equal(t, 0, w.broadPhase.trees[DynamicBody].GetProxyCount())
}

func TestShapeAccessors(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	id := w.CreateBody(&bodyDef)

	shapeDef := DefaultShapeDef()
	circle := Circle{Center: Vec2{X: 0.1, Y: 0.2}, Radius: 0.5}
	sid := w.CreateCircleShape(id, &shapeDef, &circle)

	tassert.Equal(t, CircleShape, w.ShapeType(sid))
	tassert.Equal(t, circle, w.ShapeCircle(sid))
	tassert.False(t, w.IsShapeSensor(sid))
	tassert.InDelta(t, 1.0, w.ShapeDensity(sid), 0.0)
	tassert.InDelta(t, 0.6, w.ShapeFriction(sid), 1e-15)

	w.SetShapeFriction(sid, 0.9)
	tassert.InDelta(t, 0.9, w.ShapeFriction(sid), 0.0)

	w.SetShapeRestitution(sid, 0.4)
	tassert.InDelta(t, 0.4, w.ShapeRestitution(sid), 0.0)

	w.SetShapeUserMaterial(sid, 33)
	tassert.Equal(t, uint64(33), w.ShapeUserMaterial(sid))

	w.SetShapeUserData(sid, 99)
	tassert.Equal(t, uint64(99), w.ShapeUserData(sid))

	// Density change recomputes mass.
	w.SetShapeDensity(sid, 2.0, true)
	tassert.InDelta(t, 2.0, w.ShapeDensity(sid), 0.0)
	tassert.InDelta(t, 2.0*Pi*0.25, w.BodyMass(id), 1e-12)

	// Point tests in world space.
	center := w.BodyWorldPoint(id, circle.Center)
	tassert.True(t, w.ShapeTestPoint(sid, center))
	tassert.False(t, w.ShapeTestPoint(sid, Add(center, Vec2{X: 10.0, Y: 0.0})))

	// Closest point of an internal target is the target itself.
	closest := w.ShapeClosestPoint(sid, center)
	tassert.InDelta(t, center.X, closest.X, 1e-12)
	tassert.InDelta(t, center.Y, closest.Y, 1e-12)

	// Ray cast through the shape center hits the surface.
	origin := Add(center, Vec2{X: -5.0, Y: 0.0})
	input := RayCastInput{Origin: origin, Translation: Vec2{X: 10.0, Y: 0.0}, MaxFraction: 1.0}
	output := w.ShapeRayCast(sid, &input)
	require.True(t, output.Hit)
	tassert.InDelta(t, center.X-circle.Radius, output.Point.X, 1e-12)

	// Event flags.
	w.EnableShapeSensorEvents(sid, true)
	tassert.True(t, w.AreShapeSensorEventsEnabled(sid))
	w.EnableShapeContactEvents(sid, true)
	tassert.True(t, w.AreShapeContactEventsEnabled(sid))
	w.EnableShapeHitEvents(sid, true)
	tassert.True(t, w.AreShapeHitEventsEnabled(sid))
	w.EnableShapePreSolveEvents(sid, true)
	tassert.True(t, w.AreShapePreSolveEventsEnabled(sid))
}

func TestSetShapeFilterKeepsProxy(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	id := w.CreateBody(&bodyDef)

	shapeDef := DefaultShapeDef()
	circle := Circle{Center: Vec2Zero, Radius: 0.5}
	sid := w.CreateCircleShape(id, &shapeDef, &circle)

	filter := w.ShapeFilter(sid)
	tassert.Equal(t, DefaultCategoryBits, filter.CategoryBits)

	// Changing only mask bits keeps the proxy (no category change).
	oldProxy := w.shapes[int(sid.index1)-1].proxyKey
	filter.MaskBits = 0x0F
	w.SetShapeFilter(sid, filter)
	tassert.Equal(t, filter, w.ShapeFilter(sid))
	tassert.Equal(t, oldProxy, w.shapes[int(sid.index1)-1].proxyKey)

	// Changing category bits destroys and recreates the proxy.
	filter.CategoryBits = 0x02
	w.SetShapeFilter(sid, filter)
	tassert.Equal(t, filter, w.ShapeFilter(sid))
	tassert.Equal(t, 1, w.broadPhase.trees[DynamicBody].GetProxyCount())
	tassert.Equal(t, filter.CategoryBits,
		w.broadPhase.trees[DynamicBody].GetCategoryBits(proxyKeyID(w.shapes[int(sid.index1)-1].proxyKey)))
}

func TestSensorShapeBookkeeping(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	id := w.CreateBody(&bodyDef)

	shapeDef := DefaultShapeDef()
	shapeDef.IsSensor = true
	circle := Circle{Center: Vec2Zero, Radius: 0.5}
	sid1 := w.CreateCircleShape(id, &shapeDef, &circle)
	sid2 := w.CreateCircleShape(id, &shapeDef, &circle)

	require.True(t, w.IsShapeSensor(sid1))
	require.True(t, w.IsShapeSensor(sid2))
	require.Len(t, w.sensors, 2)
	tassert.Equal(t, 0, w.ShapeSensorCapacity(sid1))

	// Destroying the first sensor swaps the second into its slot and fixes
	// the back reference.
	w.DestroyShape(sid1, true)
	require.Len(t, w.sensors, 1)
	s2 := w.getShape(sid2)
	tassert.Equal(t, 0, s2.sensorIndex)
	tassert.Equal(t, s2.id, w.sensors[0].shapeID)
}

func TestChainCreateDestroy(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	id := w.CreateBody(&bodyDef)

	chainDef := DefaultChainDef()
	chainDef.Points = []Vec2{
		{X: -2.0, Y: 0.0},
		{X: -1.0, Y: 0.5},
		{X: 1.0, Y: 0.5},
		{X: 2.0, Y: 0.0},
		{X: 0.0, Y: -1.0},
	}
	chainDef.IsLoop = true

	chainID := w.CreateChain(id, &chainDef)
	require.True(t, w.IsChainValid(chainID))

	// A loop of n points makes n segments.
	tassert.Equal(t, 5, w.ChainSegmentCount(chainID))
	tassert.Equal(t, 5, w.Counters().ShapeCount)

	segments := make([]ShapeID, 8)
	n := w.ChainSegments(chainID, segments)
	require.Equal(t, 5, n)
	for i := range n {
		require.True(t, w.IsShapeValid(segments[i]))
		tassert.Equal(t, ChainSegmentShape, w.ShapeType(segments[i]))
		tassert.Equal(t, chainID, w.ShapeParentChain(segments[i]))
	}

	// Chain materials fan out to all segments when there is one material.
	material := DefaultSurfaceMaterial()
	material.Friction = 0.9
	w.SetChainSurfaceMaterial(chainID, material, 0)
	for i := range n {
		tassert.InDelta(t, 0.9, w.ShapeFriction(segments[i]), 0.0)
	}

	w.DestroyChain(chainID)
	tassert.False(t, w.IsChainValid(chainID))
	tassert.Equal(t, 0, w.Counters().ShapeCount)
	for i := range n {
		tassert.False(t, w.IsShapeValid(segments[i]))
	}
}

func TestApplyForcesAndImpulses(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	id := w.CreateBody(&bodyDef)

	shapeDef := DefaultShapeDef()
	circle := Circle{Center: Vec2Zero, Radius: 1.0}
	w.CreateCircleShape(id, &shapeDef, &circle)

	// Impulse to center: dv = J / m.
	mass := w.BodyMass(id)
	w.ApplyBodyLinearImpulseToCenter(id, Vec2{X: mass, Y: 0.0}, true)
	velocity := w.BodyLinearVelocity(id)
	tassert.InDelta(t, 1.0, velocity.X, 1e-12)
	tassert.InDelta(t, 0.0, velocity.Y, 1e-12)

	// Angular impulse: dw = L / I.
	inertia := w.BodyRotationalInertia(id)
	w.ApplyBodyAngularImpulse(id, inertia, true)
	tassert.InDelta(t, 1.0, w.BodyAngularVelocity(id), 1e-12)

	// Forces accumulate on the body sim until the solver runs.
	w.ApplyBodyForce(id, Vec2{X: 10.0, Y: 0.0}, w.BodyWorldCenterOfMass(id), true)
	w.ApplyBodyTorque(id, 3.0, true)
	b := w.getBodyFullID(id)
	bSim := w.getBodySim(b)
	tassert.Equal(t, Vec2{X: 10.0, Y: 0.0}, bSim.force)
	tassert.InDelta(t, 3.0, bSim.torque, 0.0)

	w.ClearBodyForces(id)
	tassert.Equal(t, Vec2Zero, bSim.force)
	tassert.InDelta(t, 0.0, bSim.torque, 0.0)

	// Point velocity of a rotating body: v = w x r.
	w.SetBodyLinearVelocity(id, Vec2Zero)
	w.SetBodyAngularVelocity(id, 2.0)
	pv := w.BodyWorldPointVelocity(id, Add(w.BodyWorldCenterOfMass(id), Vec2{X: 1.0, Y: 0.0}))
	tassert.InDelta(t, 0.0, pv.X, 1e-12)
	tassert.InDelta(t, 2.0, pv.Y, 1e-12)
}
