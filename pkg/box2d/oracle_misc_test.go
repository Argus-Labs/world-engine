// Oracle tests for the remaining public surface of the port: the shape API of
// src/shape.c, the character mover queries of src/physics_world.c
// (b2World_CastMover, b2World_CollideMover) together with the
// b2CollideMoverAnd* dispatch of src/geometry.c, the explosion of
// src/physics_world.c (b2World_Explode) and the sensor overlap filtering of
// src/sensor.c.
//
// Every expected value below is derived from the C source of truth or from
// docs/character.md, never from running this Go port.
//
// UPSTREAM DRIFT: docs/character.md describes b2World_CastMover and
// b2World_CollideMover as taking a `b2Pos origin` for large-world mode, and it
// describes b2SolvePlanes / b2ClipVector as the consumers of the returned
// planes. The vendored C (v3.2.0, src/physics_world.c:2297 and :2376) has no
// origin parameter, and src/mover.c (b2SolvePlanes, b2ClipVector) is NOT part
// of this port, so those two contracts cannot be exercised here. Vendored C
// wins: the tests below use the origin-free signatures and stop at the plane
// results that b2SolvePlanes would consume.

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// ---------------------------------------------------------------------------
// src/shape.c: the per-shape-type accessor dispatch
// ---------------------------------------------------------------------------

// oracleShapeScene holds one static body carrying one shape of every type,
// each on its own body so the transforms stay independent.
type oracleShapeScene struct {
	world   *box2d.World
	circle  box2d.ShapeID
	capsule box2d.ShapeID
	polygon box2d.ShapeID
	segment box2d.ShapeID
	chain   box2d.ChainID
	chainA  box2d.ShapeID
}

// The geometry of the scene. These literals are the oracle inputs; every
// expectation in the tests is computed from them with the C formulas.
var (
	oracleCircleGeom  = box2d.Circle{Center: box2d.Vec2{X: 0.0, Y: 0.0}, Radius: 0.5}
	oracleCapsuleGeom = box2d.Capsule{
		Center1: box2d.Vec2{X: -0.5, Y: 0.0},
		Center2: box2d.Vec2{X: 0.5, Y: 0.0},
		Radius:  0.25,
	}
	oracleSegmentGeom = box2d.Segment{
		Point1: box2d.Vec2{X: -1.0, Y: 0.0},
		Point2: box2d.Vec2{X: 1.0, Y: 0.0},
	}
	// An open chain of n points yields n - 3 solid segments; the first and
	// last points are ghost vertices (src/shape.c:507-528). Five points give
	// two segments, the first running points[1] -> points[2].
	oracleChainPoints = []box2d.Vec2{
		{X: -6.0, Y: 20.0},
		{X: -2.0, Y: 20.0},
		{X: 2.0, Y: 20.0},
		{X: 6.0, Y: 20.0},
		{X: 10.0, Y: 20.0},
	}
)

func buildOracleShapeScene(t *testing.T) *oracleShapeScene {
	t.Helper()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	shapeDef := box2d.DefaultShapeDef()

	makeBody := func(position box2d.Vec2) box2d.BodyID {
		bodyDef := box2d.DefaultBodyDef()
		bodyDef.Position = position
		return world.CreateBody(&bodyDef)
	}

	circleGeom := oracleCircleGeom
	circleID := world.CreateCircleShape(makeBody(box2d.Vec2{X: 0.0, Y: 0.0}), &shapeDef, &circleGeom)

	capsuleGeom := oracleCapsuleGeom
	capsuleID := world.CreateCapsuleShape(makeBody(box2d.Vec2{X: 5.0, Y: 0.0}), &shapeDef, &capsuleGeom)

	polygonGeom := box2d.MakeBox(1.0, 2.0)
	polygonID := world.CreatePolygonShape(makeBody(box2d.Vec2{X: 10.0, Y: 0.0}), &shapeDef, &polygonGeom)

	segmentGeom := oracleSegmentGeom
	segmentID := world.CreateSegmentShape(makeBody(box2d.Vec2{X: 15.0, Y: 0.0}), &shapeDef, &segmentGeom)

	chainDef := box2d.DefaultChainDef()
	chainDef.Points = oracleChainPoints
	chainID := world.CreateChain(makeBody(box2d.Vec2{X: 0.0, Y: 0.0}), &chainDef)

	segments := make([]box2d.ShapeID, world.ChainSegmentCount(chainID))
	require.Positive(t, world.ChainSegments(chainID, segments))

	return &oracleShapeScene{
		world:   world,
		circle:  circleID,
		capsule: capsuleID,
		polygon: polygonID,
		segment: segmentID,
		chain:   chainID,
		chainA:  segments[0],
	}
}

// TestOracleShapeTypeAccessors checks the b2Shape_GetType / b2Shape_GetCircle /
// b2Shape_GetCapsule / b2Shape_GetPolygon / b2Shape_GetSegment /
// b2Shape_GetChainSegment family in src/shape.c: each getter returns a copy of
// the union member selected by shape->type, unchanged from creation.
func TestOracleShapeTypeAccessors(t *testing.T) {
	t.Parallel()

	scene := buildOracleShapeScene(t)
	w := scene.world

	assert.Equal(t, box2d.CircleShape, w.ShapeType(scene.circle))
	assert.Equal(t, box2d.CapsuleShape, w.ShapeType(scene.capsule))
	assert.Equal(t, box2d.PolygonShape, w.ShapeType(scene.polygon))
	assert.Equal(t, box2d.SegmentShape, w.ShapeType(scene.segment))
	assert.Equal(t, box2d.ChainSegmentShape, w.ShapeType(scene.chainA))

	assert.Equal(t, oracleCircleGeom, w.ShapeCircle(scene.circle))
	assert.Equal(t, oracleCapsuleGeom, w.ShapeCapsule(scene.capsule))
	assert.Equal(t, oracleSegmentGeom, w.ShapeSegment(scene.segment))

	polygon := w.ShapePolygon(scene.polygon)
	assert.Equal(t, 4, polygon.Count)
	assert.InDelta(t, 0.0, polygon.Radius, 0.0)

	// b2Shape_GetChainSegment returns the ghost-vertex carrying segment. In the
	// open-chain arm of b2CreateChain (src/shape.c:507-528) segment i is
	// { ghost1: points[i], point1: points[i+1], point2: points[i+2],
	//   ghost2: points[i+3] }.
	chainSegment := w.ShapeChainSegment(scene.chainA)
	assert.Equal(t, oracleChainPoints[0], chainSegment.Ghost1)
	assert.Equal(t, oracleChainPoints[1], chainSegment.Segment.Point1)
	assert.Equal(t, oracleChainPoints[2], chainSegment.Segment.Point2)
	assert.Equal(t, oracleChainPoints[3], chainSegment.Ghost2)

	// An open chain of n points has exactly n - 3 solid segments.
	assert.Equal(t, len(oracleChainPoints)-3, w.ChainSegmentCount(scene.chain))

	// b2Shape_GetParentChain returns the owning chain for a chain segment and
	// the null id for every other type (src/shape.c b2Shape_GetParentChain).
	assert.Equal(t, scene.chain, w.ShapeParentChain(scene.chainA))
	assert.Equal(t, box2d.ChainID{}, w.ShapeParentChain(scene.circle))
}

// TestOracleShapeComputeMassData encodes b2Shape_ComputeMassData, src/shape.c,
// which forwards to the b2Compute*Mass functions of src/geometry.c. The
// expectations are the closed forms of those C functions at the default
// density of 1:
//
//	circle  (src/geometry.c:220): m = d * pi * r^2, I = m * 0.5 * r^2
//	polygon: a 2 x 4 box has area 8, so m = 8 * d
//	segment: a segment has no area, so the mass data is all zero
func TestOracleShapeComputeMassData(t *testing.T) {
	t.Parallel()

	scene := buildOracleShapeScene(t)
	w := scene.world

	// The default shape density is 1 kg/m^2 (b2DefaultShapeDef, src/types.c).
	const density = 1.0

	circleMass := w.ShapeComputeMassData(scene.circle)
	rr := oracleCircleGeom.Radius * oracleCircleGeom.Radius
	wantMass := density * math.Pi * rr
	assert.InDelta(t, wantMass, circleMass.Mass, 1e-12)
	assert.InDelta(t, wantMass*0.5*rr, circleMass.RotationalInertia, 1e-12)
	assert.Equal(t, oracleCircleGeom.Center, circleMass.Center)

	polygonMass := w.ShapeComputeMassData(scene.polygon)
	assert.InDelta(t, 8.0*density, polygonMass.Mass, 1e-9)
	assert.InDelta(t, 0.0, polygonMass.Center.X, 1e-12)
	assert.InDelta(t, 0.0, polygonMass.Center.Y, 1e-12)

	segmentMass := w.ShapeComputeMassData(scene.segment)
	assert.InDelta(t, 0.0, segmentMass.Mass, 0.0)
	assert.InDelta(t, 0.0, segmentMass.RotationalInertia, 0.0)
}

// TestOracleShapeTestPoint encodes b2Shape_TestPoint, src/shape.c: only the
// circle, capsule and polygon arms test anything; every other shape type falls
// into the C `default: return false`.
func TestOracleShapeTestPoint(t *testing.T) {
	t.Parallel()

	scene := buildOracleShapeScene(t)
	w := scene.world

	// The circle body sits at the origin, radius 0.5.
	assert.True(t, w.ShapeTestPoint(scene.circle, box2d.Vec2{X: 0.25, Y: 0.0}))
	assert.False(t, w.ShapeTestPoint(scene.circle, box2d.Vec2{X: 0.75, Y: 0.0}))

	// The capsule body sits at (5, 0); the capsule spans x in [4.5, 5.5] with
	// radius 0.25.
	assert.True(t, w.ShapeTestPoint(scene.capsule, box2d.Vec2{X: 5.0, Y: 0.2}))
	assert.False(t, w.ShapeTestPoint(scene.capsule, box2d.Vec2{X: 5.0, Y: 0.3}))

	// The polygon body sits at (10, 0); the box spans x in [9, 11], y in
	// [-2, 2].
	assert.True(t, w.ShapeTestPoint(scene.polygon, box2d.Vec2{X: 10.5, Y: 1.5}))
	assert.False(t, w.ShapeTestPoint(scene.polygon, box2d.Vec2{X: 11.5, Y: 0.0}))

	// C default arm: a segment and a chain segment never contain a point.
	assert.False(t, w.ShapeTestPoint(scene.segment, box2d.Vec2{X: 15.0, Y: 0.0}))
	assert.False(t, w.ShapeTestPoint(scene.chainA, oracleChainPoints[1]))
}

// TestOracleShapeRayCastByType encodes b2Shape_RayCast, src/shape.c: the ray is
// pushed into shape local space, dispatched per shape type, and the hit point
// and normal are pushed back into world space. Every shape type has its own
// arm, including the chain segment arm that passes oneSided = true.
func TestOracleShapeRayCastByType(t *testing.T) {
	t.Parallel()

	scene := buildOracleShapeScene(t)
	w := scene.world

	t.Run("circle", func(t *testing.T) {
		t.Parallel()

		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -2.0, Y: 0.0},
			Translation: box2d.Vec2{X: 4.0, Y: 0.0},
			MaxFraction: 1.0,
		}
		output := w.ShapeRayCast(scene.circle, &input)
		require.True(t, output.Hit)
		// The ray enters the unit-radius-0.5 circle at x = -0.5, which is
		// 1.5 / 4.0 of the way along the translation.
		assert.InDelta(t, 1.5/4.0, output.Fraction, 1e-9)
		assert.InDelta(t, -0.5, output.Point.X, 1e-9)
		assert.InDelta(t, -1.0, output.Normal.X, 1e-9)
	})

	t.Run("capsule", func(t *testing.T) {
		t.Parallel()

		// Down the capsule at (5, 0) from above: the top surface is at
		// y = 0.25.
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 5.0, Y: 2.0},
			Translation: box2d.Vec2{X: 0.0, Y: -4.0},
			MaxFraction: 1.0,
		}
		output := w.ShapeRayCast(scene.capsule, &input)
		require.True(t, output.Hit)
		assert.InDelta(t, 0.25, output.Point.Y, 1e-9)
		assert.InDelta(t, 1.0, output.Normal.Y, 1e-9)
	})

	t.Run("polygon", func(t *testing.T) {
		t.Parallel()

		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 7.0, Y: 0.0},
			Translation: box2d.Vec2{X: 6.0, Y: 0.0},
			MaxFraction: 1.0,
		}
		output := w.ShapeRayCast(scene.polygon, &input)
		require.True(t, output.Hit)
		// The box face is at x = 9, which is 2 / 6 along the ray.
		assert.InDelta(t, 2.0/6.0, output.Fraction, 1e-9)
		assert.InDelta(t, -1.0, output.Normal.X, 1e-9)
	})

	t.Run("segment", func(t *testing.T) {
		t.Parallel()

		// The segment body is at (15, 0) and the segment spans x in [14, 16]
		// at y = 0. b2RayCastSegment with oneSided = false hits from above.
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 15.0, Y: 1.0},
			Translation: box2d.Vec2{X: 0.0, Y: -2.0},
			MaxFraction: 1.0,
		}
		output := w.ShapeRayCast(scene.segment, &input)
		require.True(t, output.Hit)
		assert.InDelta(t, 0.5, output.Fraction, 1e-9)
		assert.InDelta(t, 15.0, output.Point.X, 1e-9)
		assert.InDelta(t, 0.0, output.Point.Y, 1e-9)
	})

	t.Run("chain segment is one sided", func(t *testing.T) {
		t.Parallel()

		// b2RayCastSegment with oneSided = true skips the LEFT side of the
		// segment direction (src/geometry.c:720-729):
		//
		//	offset = b2Cross( origin - point1, point2 - point1 );
		//	if ( offset < 0 ) return the empty output;
		//
		// The first solid segment runs (-2, 20) -> (2, 20), so its direction is
		// +x and its left side is +y. A ray starting above therefore has a
		// negative offset and is rejected, while a ray starting below hits.
		fromAbove := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0.0, Y: 21.0},
			Translation: box2d.Vec2{X: 0.0, Y: -2.0},
			MaxFraction: 1.0,
		}
		assert.False(t, w.ShapeRayCast(scene.chainA, &fromAbove).Hit,
			"the one sided chain segment must reject a left-side ray")

		fromBelow := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0.0, Y: 19.0},
			Translation: box2d.Vec2{X: 0.0, Y: 2.0},
			MaxFraction: 1.0,
		}
		assert.True(t, w.ShapeRayCast(scene.chainA, &fromBelow).Hit)
	})
}

// TestOracleShapeClosestPoint encodes b2Shape_GetClosestPoint, src/shape.c: a
// b2ShapeDistance query with useRadii = true between the shape proxy and a
// single-point proxy at the target, returning pointA (the point on the shape).
// With radii enabled the result lies on the shape surface.
func TestOracleShapeClosestPoint(t *testing.T) {
	t.Parallel()

	scene := buildOracleShapeScene(t)
	w := scene.world

	// Circle of radius 0.5 at the origin, target far out on +x.
	closest := w.ShapeClosestPoint(scene.circle, box2d.Vec2{X: 10.0, Y: 0.0})
	assert.InDelta(t, 0.5, closest.X, 1e-9)
	assert.InDelta(t, 0.0, closest.Y, 1e-9)

	// Box spanning x in [9, 11], y in [-2, 2], target above the top face.
	closest = w.ShapeClosestPoint(scene.polygon, box2d.Vec2{X: 10.0, Y: 10.0})
	assert.InDelta(t, 10.0, closest.X, 1e-9)
	assert.InDelta(t, 2.0, closest.Y, 1e-9)

	// Segment from (14, 0) to (16, 0), target beyond the right end.
	closest = w.ShapeClosestPoint(scene.segment, box2d.Vec2{X: 20.0, Y: 0.0})
	assert.InDelta(t, 16.0, closest.X, 1e-9)
	assert.InDelta(t, 0.0, closest.Y, 1e-9)
}

// TestOracleShapeAABBMatchesGeometry encodes b2Shape_GetAABB, src/shape.c,
// which returns shape->aabb. b2UpdateShapeAABBs (src/shape.c) builds that AABB
// as the geometric AABB grown by B2_SPECULATIVE_DISTANCE on every side:
//
//	aabb = b2ComputeShapeAABB( shape, transform );
//	aabb.lowerBound.x -= speculativeDistance; ... (and the other three)
//	shape->aabb = aabb;
//
// The fat AABB adds the extra proxy margin on top and is not what this getter
// returns.
func TestOracleShapeAABBMatchesGeometry(t *testing.T) {
	t.Parallel()

	scene := buildOracleShapeScene(t)
	w := scene.world

	pad := box2d.SpeculativeDistance

	circleAABB := w.ShapeAABB(scene.circle)
	assert.InDelta(t, -0.5-pad, circleAABB.LowerBound.X, 1e-12)
	assert.InDelta(t, 0.5+pad, circleAABB.UpperBound.Y, 1e-12)

	// b2ComputeCapsuleAABB unions the two end caps: x in [4.25, 5.75],
	// y in [-0.25, 0.25].
	capsuleAABB := w.ShapeAABB(scene.capsule)
	assert.InDelta(t, 4.25-pad, capsuleAABB.LowerBound.X, 1e-12)
	assert.InDelta(t, 5.75+pad, capsuleAABB.UpperBound.X, 1e-12)
	assert.InDelta(t, -0.25-pad, capsuleAABB.LowerBound.Y, 1e-12)

	// b2ComputeSegmentAABB is the min/max of the two end points.
	segmentAABB := w.ShapeAABB(scene.segment)
	assert.InDelta(t, 14.0-pad, segmentAABB.LowerBound.X, 1e-12)
	assert.InDelta(t, 16.0+pad, segmentAABB.UpperBound.X, 1e-12)
}

// TestOracleSetShapeGeometry encodes the b2Shape_SetCircle / b2Shape_SetCapsule
// / b2Shape_SetSegment / b2Shape_SetPolygon family in src/shape.c. Each setter
// overwrites the union member, rewrites shape->type and rebuilds the
// broad-phase proxy; the mass properties are explicitly NOT touched.
func TestOracleSetShapeGeometry(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = box2d.DynamicBody
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	start := box2d.MakeBox(1.0, 1.0)
	shapeID := world.CreatePolygonShape(bodyID, &shapeDef, &start)

	massBefore := world.BodyMass(bodyID)

	circle := box2d.Circle{Center: box2d.Vec2{X: 0.25, Y: 0.0}, Radius: 0.75}
	world.SetShapeCircle(shapeID, &circle)
	assert.Equal(t, box2d.CircleShape, world.ShapeType(shapeID))
	assert.Equal(t, circle, world.ShapeCircle(shapeID))
	assert.InDelta(t, massBefore, world.BodyMass(bodyID), 0.0,
		"b2Shape_SetCircle does not modify the mass properties")

	capsule := box2d.Capsule{
		Center1: box2d.Vec2{X: -0.4, Y: 0.0},
		Center2: box2d.Vec2{X: 0.4, Y: 0.0},
		Radius:  0.2,
	}
	world.SetShapeCapsule(shapeID, &capsule)
	assert.Equal(t, box2d.CapsuleShape, world.ShapeType(shapeID))
	assert.Equal(t, capsule, world.ShapeCapsule(shapeID))

	segment := box2d.Segment{
		Point1: box2d.Vec2{X: -1.0, Y: -1.0},
		Point2: box2d.Vec2{X: 1.0, Y: 1.0},
	}
	world.SetShapeSegment(shapeID, &segment)
	assert.Equal(t, box2d.SegmentShape, world.ShapeType(shapeID))
	assert.Equal(t, segment, world.ShapeSegment(shapeID))

	polygon := box2d.MakeBox(2.0, 0.5)
	world.SetShapePolygon(shapeID, &polygon)
	assert.Equal(t, box2d.PolygonShape, world.ShapeType(shapeID))
	assert.Equal(t, 4, world.ShapePolygon(shapeID).Count)

	// The shape survives a step with the new geometry.
	world.Step(1.0/60.0, 4)
	assert.True(t, world.IsShapeValid(shapeID))
}

// TestOracleSetShapeCapsuleRejectsDegenerate encodes the guard at the head of
// b2Shape_SetCapsule, src/shape.c:
//
//	float lengthSqr = b2DistanceSquared( capsule->center1, capsule->center2 );
//	if ( lengthSqr <= B2_LINEAR_SLOP * B2_LINEAR_SLOP ) { return; }
//
// A degenerate capsule leaves the shape untouched, keeping the previous type.
func TestOracleSetShapeCapsuleRejectsDegenerate(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	box := box2d.MakeBox(1.0, 1.0)
	shapeID := world.CreatePolygonShape(bodyID, &shapeDef, &box)

	// Half a linear slop apart, well inside the rejection threshold.
	degenerate := box2d.Capsule{
		Center1: box2d.Vec2{X: 0.0, Y: 0.0},
		Center2: box2d.Vec2{X: 0.5 * box2d.LinearSlop, Y: 0.0},
		Radius:  0.25,
	}
	world.SetShapeCapsule(shapeID, &degenerate)

	assert.Equal(t, box2d.PolygonShape, world.ShapeType(shapeID),
		"a degenerate capsule must be rejected before the type is rewritten")
}

// TestOracleShapeEventFlags encodes the b2Shape_Enable*Events /
// b2Shape_Are*EventsEnabled pairs in src/shape.c. Each pair is an independent
// boolean on the shape; toggling one must not disturb the others.
func TestOracleShapeEventFlags(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = box2d.DynamicBody
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	circle := box2d.Circle{Radius: 0.5}
	shapeID := world.CreateCircleShape(bodyID, &shapeDef, &circle)

	// b2DefaultShapeDef (src/types.c) leaves every event flag off; the caller
	// opts in per shape.
	require.False(t, world.AreShapeSensorEventsEnabled(shapeID))
	require.False(t, world.AreShapeContactEventsEnabled(shapeID))
	require.False(t, world.AreShapePreSolveEventsEnabled(shapeID))
	require.False(t, world.AreShapeHitEventsEnabled(shapeID))

	// Each setter writes exactly one field.
	world.EnableShapeSensorEvents(shapeID, true)
	assert.True(t, world.AreShapeSensorEventsEnabled(shapeID))
	assert.False(t, world.AreShapeContactEventsEnabled(shapeID))
	assert.False(t, world.AreShapePreSolveEventsEnabled(shapeID))
	assert.False(t, world.AreShapeHitEventsEnabled(shapeID))

	world.EnableShapeContactEvents(shapeID, true)
	assert.True(t, world.AreShapeContactEventsEnabled(shapeID))

	world.EnableShapePreSolveEvents(shapeID, true)
	assert.True(t, world.AreShapePreSolveEventsEnabled(shapeID))

	world.EnableShapeHitEvents(shapeID, true)
	assert.True(t, world.AreShapeHitEventsEnabled(shapeID))

	// Flipping everything back restores the original state.
	world.EnableShapeSensorEvents(shapeID, false)
	world.EnableShapeContactEvents(shapeID, false)
	world.EnableShapePreSolveEvents(shapeID, false)
	world.EnableShapeHitEvents(shapeID, false)
	assert.False(t, world.AreShapeSensorEventsEnabled(shapeID))
	assert.False(t, world.AreShapeContactEventsEnabled(shapeID))
	assert.False(t, world.AreShapePreSolveEventsEnabled(shapeID))
	assert.False(t, world.AreShapeHitEventsEnabled(shapeID))
}

// TestOracleShapeSurfaceMaterial encodes b2Shape_GetSurfaceMaterial /
// b2Shape_SetSurfaceMaterial and b2Shape_SetUserMaterial /
// b2Shape_GetUserMaterial, src/shape.c: the setter replaces the whole
// b2SurfaceMaterial struct and the getter returns a copy of it.
func TestOracleShapeSurfaceMaterial(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	circle := box2d.Circle{Radius: 0.5}
	shapeID := world.CreateCircleShape(bodyID, &shapeDef, &circle)

	assert.Equal(t, shapeDef.Material, world.ShapeSurfaceMaterial(shapeID))

	material := box2d.DefaultSurfaceMaterial()
	material.Friction = 0.42
	material.Restitution = 0.25
	material.CustomColor = uint32(box2d.ColorAqua)
	material.UserMaterialID = 7
	world.SetShapeSurfaceMaterial(shapeID, material)

	assert.Equal(t, material, world.ShapeSurfaceMaterial(shapeID))
	// The individual getters read the same struct.
	assert.InDelta(t, 0.42, world.ShapeFriction(shapeID), 0.0)
	assert.InDelta(t, 0.25, world.ShapeRestitution(shapeID), 0.0)
	assert.Equal(t, uint64(7), world.ShapeUserMaterial(shapeID))

	world.SetShapeUserMaterial(shapeID, 99)
	assert.Equal(t, uint64(99), world.ShapeUserMaterial(shapeID))
	// b2Shape_SetUserMaterial only writes the id field.
	assert.InDelta(t, 0.42, world.ShapeFriction(shapeID), 0.0)
}

// TestOracleChainSurfaceMaterials encodes b2Chain_GetSurfaceMaterialCount,
// b2Chain_GetSurfaceMaterial and b2Chain_SetSurfaceMaterial, src/shape.c.
//
// The C code allows either one material for the whole chain or one per point.
// With a per-segment material array, b2Chain_SetSurfaceMaterial writes only the
// segment at materialIndex; with a single material it fans the value out to
// every segment.
//
// A LOOP chain is used here because it is the only shape where the C keeps
// materialCount == segmentCount (src/shape.c:455-505 makes n segments for n
// points), so material index i maps to segment i.
func TestOracleChainSurfaceMaterials(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyID := world.CreateBody(&bodyDef)

	points := []box2d.Vec2{
		{X: -3.0, Y: -3.0},
		{X: 3.0, Y: -3.0},
		{X: 3.0, Y: 3.0},
		{X: -3.0, Y: 3.0},
	}

	materials := make([]box2d.SurfaceMaterial, len(points))
	for i := range materials {
		materials[i] = box2d.DefaultSurfaceMaterial()
		materials[i].Friction = 0.1 * float64(i+1)
	}

	chainDef := box2d.DefaultChainDef()
	chainDef.Points = points
	chainDef.Materials = materials
	chainDef.IsLoop = true
	chainID := world.CreateChain(bodyID, &chainDef)

	assert.Equal(t, len(points), world.ChainSurfaceMaterialCount(chainID))
	for i := range points {
		assert.InDelta(t, 0.1*float64(i+1), world.ChainSurfaceMaterial(chainID, i).Friction, 1e-12)
	}

	// A loop chain of n points has n segments.
	segmentCount := world.ChainSegmentCount(chainID)
	require.Equal(t, len(points), segmentCount)

	segments := make([]box2d.ShapeID, segmentCount)
	require.Equal(t, segmentCount, world.ChainSegments(chainID, segments))

	// Per-segment materials: only index 1 changes.
	replacement := box2d.DefaultSurfaceMaterial()
	replacement.Friction = 0.95
	world.SetChainSurfaceMaterial(chainID, replacement, 1)

	assert.InDelta(t, 0.95, world.ChainSurfaceMaterial(chainID, 1).Friction, 1e-12)
	assert.InDelta(t, 0.95, world.ShapeFriction(segments[1]), 1e-12)
	assert.InDelta(t, 0.1, world.ShapeFriction(segments[0]), 1e-12)

	// b2Chain_GetSegments stops at the caller's array length.
	short := make([]box2d.ShapeID, 1)
	assert.Equal(t, 1, world.ChainSegments(chainID, short))
}

// TestOracleChainSingleMaterialFansOut checks the other arm of
// b2Chain_SetSurfaceMaterial (src/shape.c): with exactly one material the C
// code loops over every segment and assigns the same value.
func TestOracleChainSingleMaterialFansOut(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyID := world.CreateBody(&bodyDef)

	chainDef := box2d.DefaultChainDef()
	chainDef.Points = []box2d.Vec2{
		{X: -6.0, Y: 0.0},
		{X: -2.0, Y: 0.0},
		{X: 2.0, Y: 0.0},
		{X: 6.0, Y: 0.0},
		{X: 10.0, Y: 0.0},
	}
	chainID := world.CreateChain(bodyID, &chainDef)

	assert.Equal(t, 1, world.ChainSurfaceMaterialCount(chainID),
		"b2DefaultChainDef leaves a single default material")

	material := box2d.DefaultSurfaceMaterial()
	material.Restitution = 0.7
	world.SetChainSurfaceMaterial(chainID, material, 0)

	segmentCount := world.ChainSegmentCount(chainID)
	segments := make([]box2d.ShapeID, segmentCount)
	require.Equal(t, segmentCount, world.ChainSegments(chainID, segments))
	for _, segment := range segments {
		assert.InDelta(t, 0.7, world.ShapeRestitution(segment), 1e-12)
	}
	assert.InDelta(t, 0.7, world.ChainSurfaceMaterial(chainID, 0).Restitution, 1e-12)
}

// TestOracleApplyShapeWind encodes the circle arm of b2Shape_ApplyWind,
// src/shape.c:
//
//	airDensity     = 1.2250 / volumeUnits
//	projectedArea  = 2 * radius
//	relativeVel    = wind - drag * shapeVelocity
//	force          = 0.5 * airDensity * projectedArea * speed^2 * direction
//
// For a body at rest the shape velocity is zero, so the relative velocity is
// the wind itself. The centroid equals the local centre for a single centred
// circle, so the lever, and therefore the torque, is zero.
//
// The force is observed through one gravity-free step: b2IntegrateVelocities
// (src/solver.c) gives v = h * invMass * force with no damping.
func TestOracleApplyShapeWind(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = box2d.DynamicBody
	bodyID := world.CreateBody(&bodyDef)

	const radius = 0.5
	shapeDef := box2d.DefaultShapeDef()
	circle := box2d.Circle{Radius: radius}
	shapeID := world.CreateCircleShape(bodyID, &shapeDef, &circle)

	const windSpeed = 10.0
	world.ApplyShapeWind(shapeID, box2d.Vec2{X: windSpeed, Y: 0.0}, 1.0, 0.0, true)

	const timeStep = 1.0 / 60.0
	world.Step(timeStep, 4)

	// C constants, restated here rather than read from the port.
	const airDensity = 1.2250
	const projectedArea = 2.0 * radius
	wantForce := 0.5 * airDensity * projectedArea * windSpeed * windSpeed

	mass := world.BodyMass(bodyID)
	require.Positive(t, mass)
	wantSpeed := timeStep * wantForce / mass

	velocity := world.BodyLinearVelocity(bodyID)
	assert.InDelta(t, wantSpeed, velocity.X, 1e-9)
	assert.InDelta(t, 0.0, velocity.Y, 1e-12)
	assert.InDelta(t, 0.0, world.BodyAngularVelocity(bodyID), 1e-12,
		"a centred circle has a zero lever, so the wind applies no torque")
}

// TestOracleApplyShapeWindIgnoresNonSolidShapes encodes the early return of
// b2Shape_ApplyWind, src/shape.c:
//
//	if ( type != b2_circleShape && type != b2_capsuleShape &&
//	     type != b2_polygonShape ) return;
//	if ( body->type != b2_dynamicBody ) return;
//
// Neither a segment shape nor a static body may pick up any wind force.
func TestOracleApplyShapeWindIgnoresNonSolidShapes(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	shapeDef := box2d.DefaultShapeDef()

	// A segment on a dynamic body: rejected by the shape type test.
	segmentBodyDef := box2d.DefaultBodyDef()
	segmentBodyDef.Type = box2d.DynamicBody
	segmentBodyID := world.CreateBody(&segmentBodyDef)
	segment := box2d.Segment{Point1: box2d.Vec2{X: -1.0}, Point2: box2d.Vec2{X: 1.0}}
	segmentShapeID := world.CreateSegmentShape(segmentBodyID, &shapeDef, &segment)

	// A polygon on a static body: rejected by the body type test.
	staticBodyDef := box2d.DefaultBodyDef()
	staticBodyDef.Position = box2d.Vec2{X: 10.0, Y: 0.0}
	staticBodyID := world.CreateBody(&staticBodyDef)
	box := box2d.MakeBox(1.0, 1.0)
	staticShapeID := world.CreatePolygonShape(staticBodyID, &shapeDef, &box)

	wind := box2d.Vec2{X: 50.0, Y: 0.0}
	world.ApplyShapeWind(segmentShapeID, wind, 1.0, 0.0, true)
	world.ApplyShapeWind(staticShapeID, wind, 1.0, 0.0, true)

	world.Step(1.0/60.0, 4)

	assert.InDelta(t, 0.0, world.BodyLinearVelocity(segmentBodyID).X, 1e-12)
	assert.InDelta(t, 0.0, world.BodyLinearVelocity(staticBodyID).X, 1e-12)
}

// TestOracleApplyShapeWindOnCapsuleAndPolygon exercises the two remaining arms
// of b2Shape_ApplyWind (capsule and polygon). The C code only ever adds to
// sim->force, so with a positive wind along +x and a symmetric shape the body
// must accelerate along +x. The polygon arm additionally skips back-facing
// edges (`if projectedArea <= 0 continue`), which the symmetric box exercises
// on two of its four edges.
func TestOracleApplyShapeWindOnCapsuleAndPolygon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func(w *box2d.World, bodyID box2d.BodyID) box2d.ShapeID
	}{
		{
			name: "capsule",
			build: func(w *box2d.World, bodyID box2d.BodyID) box2d.ShapeID {
				shapeDef := box2d.DefaultShapeDef()
				capsule := box2d.Capsule{
					Center1: box2d.Vec2{X: 0.0, Y: -0.5},
					Center2: box2d.Vec2{X: 0.0, Y: 0.5},
					Radius:  0.2,
				}
				return w.CreateCapsuleShape(bodyID, &shapeDef, &capsule)
			},
		},
		{
			name: "polygon",
			build: func(w *box2d.World, bodyID box2d.BodyID) box2d.ShapeID {
				shapeDef := box2d.DefaultShapeDef()
				box := box2d.MakeBox(0.5, 1.0)
				return w.CreatePolygonShape(bodyID, &shapeDef, &box)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			worldDef := box2d.DefaultWorldDef()
			worldDef.Gravity = box2d.Vec2{}
			world := box2d.NewWorld(&worldDef)
			t.Cleanup(world.Destroy)

			bodyDef := box2d.DefaultBodyDef()
			bodyDef.Type = box2d.DynamicBody
			bodyID := world.CreateBody(&bodyDef)

			shapeID := test.build(world, bodyID)

			world.ApplyShapeWind(shapeID, box2d.Vec2{X: 20.0, Y: 0.0}, 1.0, 0.0, true)
			world.Step(1.0/60.0, 4)

			assert.Positive(t, world.BodyLinearVelocity(bodyID).X,
				"the wind force must push the body downwind")
		})
	}
}

// ---------------------------------------------------------------------------
// The character mover: b2World_CollideMover and b2World_CastMover
// ---------------------------------------------------------------------------

// oracleCollectPlanes runs b2World_CollideMover and returns every plane result
// the callback saw, keyed by shape id order of arrival.
func oracleCollectPlanes(w *box2d.World, mover *box2d.Capsule) []box2d.PlaneResult {
	var planes []box2d.PlaneResult
	w.CollideMover(mover, box2d.DefaultQueryFilter(), func(_ box2d.ShapeID, plane *box2d.PlaneResult, _ any) bool {
		planes = append(planes, *plane)
		return true
	}, nil)
	return planes
}

// TestOracleCollideMoverPlanes encodes the four b2CollideMoverAnd* functions of
// src/geometry.c, reached through b2CollideMover (src/shape.c) and
// b2World_CollideMover (src/physics_world.c:2376).
//
// Every one of them runs a b2ShapeDistance with useRadii = false between the
// bare shape and the mover's inner segment carrying the mover radius, then:
//
//	if ( distance <= totalRadius )
//	    plane  = { distanceOutput.normal, totalRadius - distance }
//	    point  = distanceOutput.pointA
//	    hit    = true
//
// The only difference between them is totalRadius:
//
//	circle  (geometry.c:962): mover->radius + shape->radius
//	capsule (geometry.c:988): mover->radius + shape->radius
//	polygon (geometry.c:1016): mover->radius + shape->radius
//	segment (geometry.c:1043): mover->radius          (a segment has no radius)
//
// The mover below is a vertical capsule, so the closest feature of every shape
// is on the -x side and the plane normal, which points from the shape to the
// mover, is exactly +x.
func TestOracleCollideMoverPlanes(t *testing.T) {
	t.Parallel()

	const moverRadius = 0.5

	tests := []struct {
		name string
		// build creates the static shape at the world origin.
		build func(w *box2d.World, bodyID box2d.BodyID)
		// moverX is the x position of the mover's inner segment.
		moverX float64
		// wantOffset is totalRadius - distance, computed by hand from the C.
		wantOffset float64
		// wantPointX is distanceOutput.pointA.x on the shape.
		wantPointX float64
	}{
		{
			// A circle of radius 1 at the origin. The distance proxy for the
			// circle is its bare centre, so the distance to a mover segment at
			// x = 1.2 is 1.2 and totalRadius is 0.5 + 1.0 = 1.5.
			name: "circle",
			build: func(w *box2d.World, bodyID box2d.BodyID) {
				shapeDef := box2d.DefaultShapeDef()
				circle := box2d.Circle{Radius: 1.0}
				w.CreateCircleShape(bodyID, &shapeDef, &circle)
			},
			moverX:     1.2,
			wantOffset: 1.5 - 1.2,
			wantPointX: 0.0,
		},
		{
			// A vertical capsule from (0,-1) to (0,1) with radius 0.3. Its
			// proxy is the bare inner segment, so the distance to a mover
			// segment at x = 0.5 is 0.5 and totalRadius is 0.5 + 0.3 = 0.8.
			name: "capsule",
			build: func(w *box2d.World, bodyID box2d.BodyID) {
				shapeDef := box2d.DefaultShapeDef()
				capsule := box2d.Capsule{
					Center1: box2d.Vec2{X: 0.0, Y: -1.0},
					Center2: box2d.Vec2{X: 0.0, Y: 1.0},
					Radius:  0.3,
				}
				w.CreateCapsuleShape(bodyID, &shapeDef, &capsule)
			},
			moverX:     0.5,
			wantOffset: 0.8 - 0.5,
			wantPointX: 0.0,
		},
		{
			// A 2 x 2 box at the origin: its right face is at x = 1, so the
			// distance to a mover segment at x = 1.3 is 0.3 and totalRadius is
			// 0.5 + 0 = 0.5.
			name: "polygon",
			build: func(w *box2d.World, bodyID box2d.BodyID) {
				shapeDef := box2d.DefaultShapeDef()
				box := box2d.MakeBox(1.0, 1.0)
				w.CreatePolygonShape(bodyID, &shapeDef, &box)
			},
			moverX:     1.3,
			wantOffset: 0.5 - 0.3,
			wantPointX: 1.0,
		},
		{
			// A vertical segment from (0,-2) to (0,2). The C uses
			// totalRadius = mover->radius only, so a mover segment at x = 0.3
			// gives 0.5 - 0.3.
			name: "segment",
			build: func(w *box2d.World, bodyID box2d.BodyID) {
				shapeDef := box2d.DefaultShapeDef()
				segment := box2d.Segment{
					Point1: box2d.Vec2{X: 0.0, Y: -2.0},
					Point2: box2d.Vec2{X: 0.0, Y: 2.0},
				}
				w.CreateSegmentShape(bodyID, &shapeDef, &segment)
			},
			moverX:     0.3,
			wantOffset: 0.5 - 0.3,
			wantPointX: 0.0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			worldDef := box2d.DefaultWorldDef()
			worldDef.Gravity = box2d.Vec2{}
			world := box2d.NewWorld(&worldDef)
			t.Cleanup(world.Destroy)

			bodyDef := box2d.DefaultBodyDef()
			bodyID := world.CreateBody(&bodyDef)
			test.build(world, bodyID)

			mover := box2d.Capsule{
				Center1: box2d.Vec2{X: test.moverX, Y: -0.25},
				Center2: box2d.Vec2{X: test.moverX, Y: 0.25},
				Radius:  moverRadius,
			}

			planes := oracleCollectPlanes(world, &mover)
			require.Len(t, planes, 1)

			plane := planes[0]
			assert.True(t, plane.Hit)
			assert.InDelta(t, 1.0, plane.Plane.Normal.X, 1e-9,
				"the plane normal points from the shape to the mover")
			assert.InDelta(t, 0.0, plane.Plane.Normal.Y, 1e-9)
			assert.InDelta(t, test.wantOffset, plane.Plane.Offset, 1e-9)
			assert.InDelta(t, test.wantPointX, plane.Point.X, 1e-9)
		})
	}
}

// TestOracleCollideMoverMisses checks the other arm of the C:
//
//	return (b2PlaneResult){ 0 };
//
// A mover further away than totalRadius produces no hit, and the world-level
// callback filter in TreeCollideCallback (src/physics_world.c:2344) only
// forwards `result.hit && b2IsNormalized( result.plane.normal )`, so the
// callback must not fire at all.
func TestOracleCollideMoverMisses(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyID := world.CreateBody(&bodyDef)
	shapeDef := box2d.DefaultShapeDef()
	circle := box2d.Circle{Radius: 1.0}
	world.CreateCircleShape(bodyID, &shapeDef, &circle)

	// totalRadius is 1.5; put the mover segment at x = 3.
	far := box2d.Capsule{
		Center1: box2d.Vec2{X: 3.0, Y: -0.25},
		Center2: box2d.Vec2{X: 3.0, Y: 0.25},
		Radius:  0.5,
	}
	assert.Empty(t, oracleCollectPlanes(world, &far))

	// Just inside the threshold the callback fires again.
	near := far
	near.Center1.X = 1.4
	near.Center2.X = 1.4
	assert.Len(t, oracleCollectPlanes(world, &near), 1)
}

// TestOracleCollideMoverRespectsFilter encodes the filter test at the head of
// TreeCollideCallback, src/physics_world.c:2344:
//
//	if ( b2ShouldQueryCollide( shape->filter, worldContext->filter ) == false )
//	    return true;
//
// b2ShouldQueryCollide (src/shape.c) requires both directions to pass:
// shape.categoryBits & query.maskBits and shape.maskBits & query.categoryBits.
func TestOracleCollideMoverRespectsFilter(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	shapeDef.Filter.CategoryBits = 0x2
	shapeDef.Filter.MaskBits = 0xFFFF
	circle := box2d.Circle{Radius: 1.0}
	world.CreateCircleShape(bodyID, &shapeDef, &circle)

	mover := box2d.Capsule{
		Center1: box2d.Vec2{X: 1.2, Y: -0.25},
		Center2: box2d.Vec2{X: 1.2, Y: 0.25},
		Radius:  0.5,
	}

	count := func(filter box2d.QueryFilter) int {
		hits := 0
		world.CollideMover(&mover, filter, func(_ box2d.ShapeID, _ *box2d.PlaneResult, _ any) bool {
			hits++
			return true
		}, nil)
		return hits
	}

	matching := box2d.DefaultQueryFilter()
	matching.MaskBits = 0x2
	assert.Equal(t, 1, count(matching))

	// The query mask no longer contains the shape category.
	rejecting := box2d.DefaultQueryFilter()
	rejecting.MaskBits = 0x4
	assert.Equal(t, 0, count(rejecting))
}

// TestOracleCollideMoverEarlyOut encodes the callback contract of
// b2World_CollideMover: the b2PlaneResultFcn return value is forwarded to the
// dynamic tree query, so returning false stops the traversal.
func TestOracleCollideMoverEarlyOut(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	// Three overlapping circles all touching the mover.
	shapeDef := box2d.DefaultShapeDef()
	for i := range 3 {
		bodyDef := box2d.DefaultBodyDef()
		bodyDef.Position = box2d.Vec2{X: 0.0, Y: float64(i) * 0.2}
		bodyID := world.CreateBody(&bodyDef)
		circle := box2d.Circle{Radius: 1.0}
		world.CreateCircleShape(bodyID, &shapeDef, &circle)
	}

	mover := box2d.Capsule{
		Center1: box2d.Vec2{X: 1.2, Y: -0.25},
		Center2: box2d.Vec2{X: 1.2, Y: 0.25},
		Radius:  0.5,
	}

	require.Len(t, oracleCollectPlanes(world, &mover), 3)

	stopped := 0
	world.CollideMover(&mover, box2d.DefaultQueryFilter(),
		func(_ box2d.ShapeID, _ *box2d.PlaneResult, _ any) bool {
			stopped++
			return false
		}, nil)
	assert.Equal(t, 1, stopped)
}

// TestOracleCastMover encodes b2World_CastMover, src/physics_world.c:2297 and
// its MoverCastCallback (src/physics_world.c:2268), plus the encroachment
// contract of docs/character.md:
//
//   - the fraction starts at 1 and only ever shrinks;
//   - a shape the mover already overlaps reports fraction 0 from
//     b2ShapeCastShape and is explicitly ignored ("Ignore overlapping
//     shapes"), so it must not stop the mover;
//   - with nothing in the way the cast returns 1.
func TestOracleCastMover(t *testing.T) {
	t.Parallel()

	buildWorld := func(t *testing.T) *box2d.World {
		t.Helper()

		worldDef := box2d.DefaultWorldDef()
		worldDef.Gravity = box2d.Vec2{}
		world := box2d.NewWorld(&worldDef)
		t.Cleanup(world.Destroy)
		return world
	}

	addWall := func(w *box2d.World, x float64) {
		bodyDef := box2d.DefaultBodyDef()
		bodyDef.Position = box2d.Vec2{X: x, Y: 0.0}
		bodyID := w.CreateBody(&bodyDef)
		shapeDef := box2d.DefaultShapeDef()
		wall := box2d.MakeBox(0.5, 5.0)
		w.CreatePolygonShape(bodyID, &shapeDef, &wall)
	}

	mover := box2d.Capsule{
		Center1: box2d.Vec2{X: 0.0, Y: -0.5},
		Center2: box2d.Vec2{X: 0.0, Y: 0.5},
		Radius:  0.5,
	}
	translation := box2d.Vec2{X: 10.0, Y: 0.0}

	t.Run("empty world returns one", func(t *testing.T) {
		t.Parallel()

		world := buildWorld(t)
		assert.InDelta(t, 1.0, world.CastMover(&mover, translation, box2d.DefaultQueryFilter()), 0.0)
	})

	t.Run("a wall shortens the cast", func(t *testing.T) {
		t.Parallel()

		world := buildWorld(t)
		addWall(world, 5.0)

		fraction := world.CastMover(&mover, translation, box2d.DefaultQueryFilter())
		assert.Less(t, fraction, 1.0)
		assert.Positive(t, fraction)

		// The wall's near face is at x = 4.5 and the mover surface starts at
		// x = 0.5, so the mover travels about 4 of its 10 unit translation
		// before touching. docs/character.md allows a small extra amount:
		// encroachment lets the capsule move slightly into a surface it is
		// sliding along, bounded by the linear slop.
		assert.LessOrEqual(t, fraction, 0.4+box2d.LinearSlop)
		assert.GreaterOrEqual(t, fraction, 0.4-box2d.LinearSlop)
	})

	t.Run("the nearest wall wins", func(t *testing.T) {
		t.Parallel()

		near := buildWorld(t)
		addWall(near, 3.0)
		nearFraction := near.CastMover(&mover, translation, box2d.DefaultQueryFilter())

		far := buildWorld(t)
		addWall(far, 6.0)
		farFraction := far.CastMover(&mover, translation, box2d.DefaultQueryFilter())

		assert.Less(t, nearFraction, farFraction)
	})

	t.Run("overlapping shapes are ignored", func(t *testing.T) {
		t.Parallel()

		world := buildWorld(t)
		// A wall the mover already sits inside: b2ShapeCastShape returns
		// fraction 0 and MoverCastCallback drops it.
		addWall(world, 0.0)
		addWall(world, 5.0)

		fraction := world.CastMover(&mover, translation, box2d.DefaultQueryFilter())
		assert.Positive(t, fraction,
			"an already overlapping shape must not stop the mover (encroachment)")
	})

	t.Run("the query filter gates the cast", func(t *testing.T) {
		t.Parallel()

		world := buildWorld(t)

		bodyDef := box2d.DefaultBodyDef()
		bodyDef.Position = box2d.Vec2{X: 5.0, Y: 0.0}
		bodyID := world.CreateBody(&bodyDef)
		shapeDef := box2d.DefaultShapeDef()
		shapeDef.Filter.CategoryBits = 0x2
		wall := box2d.MakeBox(0.5, 5.0)
		world.CreatePolygonShape(bodyID, &shapeDef, &wall)

		matching := box2d.DefaultQueryFilter()
		matching.MaskBits = 0x2
		assert.Less(t, world.CastMover(&mover, translation, matching), 1.0)

		rejecting := box2d.DefaultQueryFilter()
		rejecting.MaskBits = 0x4
		assert.InDelta(t, 1.0, world.CastMover(&mover, translation, rejecting), 0.0)
	})
}

// ---------------------------------------------------------------------------
// b2World_Explode
// ---------------------------------------------------------------------------

// TestOracleExplodeAtShapeCentroid encodes the two degenerate arms of
// ExplosionCallback, src/physics_world.c:
//
//	if ( output.distance == 0.0f ) closestPoint = b2TransformPoint( transform,
//	                                                b2GetShapeCentroid( shape ) );
//	...
//	if ( b2LengthSquared( direction ) > 100.0f * FLT_EPSILON * FLT_EPSILON )
//	    direction = b2Normalize( direction );
//	else
//	    direction = (b2Vec2){ 1.0f, 0.0f };
//
// An explosion placed exactly on the centroid of a symmetric shape hits both:
// the distance is zero, the closest point is the centroid, the direction
// degenerates and the C falls back to +x.
func TestOracleExplodeAtShapeCentroid(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = box2d.DynamicBody
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	box := box2d.MakeBox(1.0, 1.0)
	world.CreatePolygonShape(bodyID, &shapeDef, &box)

	explosionDef := box2d.DefaultExplosionDef()
	explosionDef.Position = box2d.Vec2{}
	explosionDef.Radius = 2.0
	explosionDef.Falloff = 0.0
	explosionDef.ImpulsePerLength = 10.0
	world.Explode(&explosionDef)

	velocity := world.BodyLinearVelocity(bodyID)
	assert.Positive(t, velocity.X, "the degenerate direction falls back to +x")
	assert.InDelta(t, 0.0, velocity.Y, 1e-9)

	// The centroid of a centred box is the body centre, so the impulse line of
	// action passes through it and no spin is produced.
	assert.InDelta(t, 0.0, world.BodyAngularVelocity(bodyID), 1e-9)
}

// TestOracleExplodeFalloffScale encodes the falloff ramp of ExplosionCallback,
// src/physics_world.c:
//
//	if ( output.distance > radius && falloff > 0.0f )
//	    scale = b2ClampFloat( ( radius + falloff - output.distance ) / falloff,
//	                          0.0f, 1.0f );
//
// A shape whose distance is exactly halfway across the falloff band gets half
// the impulse of the same shape sitting inside the radius. The impulse is
// linear in the scale, so the ratio of the resulting speeds is exactly 1/2.
func TestOracleExplodeFalloffScale(t *testing.T) {
	t.Parallel()

	// speedAt builds a world with one circle at the given distance from the
	// explosion and returns the speed the explosion imparts.
	speedAt := func(t *testing.T, centerX, radius, falloff float64) float64 {
		t.Helper()

		worldDef := box2d.DefaultWorldDef()
		worldDef.Gravity = box2d.Vec2{}
		world := box2d.NewWorld(&worldDef)
		t.Cleanup(world.Destroy)

		bodyDef := box2d.DefaultBodyDef()
		bodyDef.Type = box2d.DynamicBody
		bodyDef.Position = box2d.Vec2{X: centerX, Y: 0.0}
		bodyID := world.CreateBody(&bodyDef)

		shapeDef := box2d.DefaultShapeDef()
		circle := box2d.Circle{Radius: 0.5}
		world.CreateCircleShape(bodyID, &shapeDef, &circle)

		explosionDef := box2d.DefaultExplosionDef()
		explosionDef.Position = box2d.Vec2{}
		explosionDef.Radius = radius
		explosionDef.Falloff = falloff
		explosionDef.ImpulsePerLength = 10.0
		world.Explode(&explosionDef)

		return box2d.Length(world.BodyLinearVelocity(bodyID))
	}

	const explosionRadius = 2.0
	const falloff = 2.0

	// Surface distance 1.5, inside the radius: scale is 1.
	inside := speedAt(t, 2.0, explosionRadius, falloff)
	// Surface distance 3.0, exactly halfway across [2, 4]: scale is 0.5.
	half := speedAt(t, 3.5, explosionRadius, falloff)
	// Surface distance 4.5, past radius + falloff: the callback returns early.
	outside := speedAt(t, 5.0, explosionRadius, falloff)

	require.Positive(t, inside)
	assert.InDelta(t, 0.5*inside, half, 1e-9)
	assert.InDelta(t, 0.0, outside, 0.0)
}

// ---------------------------------------------------------------------------
// src/sensor.c
// ---------------------------------------------------------------------------

// TestOracleSensorCustomFiltering encodes the custom filter block of
// b2SensorQueryCallback, src/sensor.c:
//
//	if ( sensorShape->enableCustomFiltering || otherShape->enableCustomFiltering )
//	{
//	    b2CustomFilterFcn* customFilterFcn = world->customFilterFcn;
//	    if ( customFilterFcn != NULL )
//	    {
//	        bool shouldCollide = customFilterFcn( idA, idB, world->customFilterContext );
//	        if ( shouldCollide == false ) return true;
//	    }
//	}
//
// With the flag off the callback is never consulted; with the flag on a false
// return suppresses the overlap entirely.
func TestOracleSensorCustomFiltering(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T, customFiltering, allow bool) (*box2d.World, *int) {
		t.Helper()

		worldDef := box2d.DefaultWorldDef()
		worldDef.Gravity = box2d.Vec2{}
		world := box2d.NewWorld(&worldDef)
		t.Cleanup(world.Destroy)

		calls := 0
		world.SetCustomFilterCallback(func(_, _ box2d.ShapeID, _ any) bool {
			calls++
			return allow
		}, nil)

		sensorBodyDef := box2d.DefaultBodyDef()
		sensorBodyID := world.CreateBody(&sensorBodyDef)
		sensorShapeDef := box2d.DefaultShapeDef()
		sensorShapeDef.IsSensor = true
		sensorShapeDef.EnableSensorEvents = true
		sensorShapeDef.EnableCustomFiltering = customFiltering
		sensorBox := box2d.MakeBox(2.0, 2.0)
		world.CreatePolygonShape(sensorBodyID, &sensorShapeDef, &sensorBox)

		visitorBodyDef := box2d.DefaultBodyDef()
		visitorBodyDef.Type = box2d.DynamicBody
		visitorBodyID := world.CreateBody(&visitorBodyDef)
		visitorShapeDef := box2d.DefaultShapeDef()
		visitorShapeDef.EnableSensorEvents = true
		visitorCircle := box2d.Circle{Radius: 0.5}
		world.CreateCircleShape(visitorBodyID, &visitorShapeDef, &visitorCircle)

		return world, &calls
	}

	t.Run("flag off never calls the filter", func(t *testing.T) {
		t.Parallel()

		world, calls := build(t, false, false)
		world.Step(1.0/60.0, 4)

		assert.Zero(t, *calls)
		assert.Len(t, world.SensorEvents().BeginEvents, 1,
			"without custom filtering the overlap is reported")
	})

	t.Run("flag on and filter rejects", func(t *testing.T) {
		t.Parallel()

		world, calls := build(t, true, false)
		world.Step(1.0/60.0, 4)

		assert.Positive(t, *calls, "the custom filter must be consulted")
		assert.Empty(t, world.SensorEvents().BeginEvents,
			"a false custom filter suppresses the sensor overlap")
	})

	t.Run("flag on and filter accepts", func(t *testing.T) {
		t.Parallel()

		world, calls := build(t, true, true)
		world.Step(1.0/60.0, 4)

		assert.Positive(t, *calls)
		assert.Len(t, world.SensorEvents().BeginEvents, 1)
	})
}

// ---------------------------------------------------------------------------
// World query early-outs (src/physics_world.c query section)
// ---------------------------------------------------------------------------

// TestOracleWorldQueryEarlyOuts encodes the callback contract shared by
// b2World_OverlapAABB and b2World_OverlapShape (src/physics_world.c): the
// b2OverlapResultFcn return value is forwarded to the dynamic tree, so
// returning false stops the traversal after the first result.
func TestOracleWorldQueryEarlyOuts(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	shapeDef := box2d.DefaultShapeDef()
	for i := range 5 {
		bodyDef := box2d.DefaultBodyDef()
		bodyDef.Position = box2d.Vec2{X: float64(i) * 0.1, Y: 0.0}
		bodyID := world.CreateBody(&bodyDef)
		circle := box2d.Circle{Radius: 0.5}
		world.CreateCircleShape(bodyID, &shapeDef, &circle)
	}

	everything := box2d.AABB{
		LowerBound: box2d.Vec2{X: -10.0, Y: -10.0},
		UpperBound: box2d.Vec2{X: 10.0, Y: 10.0},
	}

	all := 0
	world.OverlapAABB(everything, box2d.DefaultQueryFilter(), func(_ box2d.ShapeID, _ any) bool {
		all++
		return true
	}, nil)
	require.Equal(t, 5, all)

	stopped := 0
	world.OverlapAABB(everything, box2d.DefaultQueryFilter(), func(_ box2d.ShapeID, _ any) bool {
		stopped++
		return false
	}, nil)
	assert.Equal(t, 1, stopped)

	proxy := box2d.MakeProxy([]box2d.Vec2{{X: 0.0, Y: 0.0}}, 1, 1.0)

	shapeAll := 0
	world.OverlapShape(&proxy, box2d.DefaultQueryFilter(), func(_ box2d.ShapeID, _ any) bool {
		shapeAll++
		return true
	}, nil)
	require.Equal(t, 5, shapeAll)

	shapeStopped := 0
	world.OverlapShape(&proxy, box2d.DefaultQueryFilter(), func(_ box2d.ShapeID, _ any) bool {
		shapeStopped++
		return false
	}, nil)
	assert.Equal(t, 1, shapeStopped)
}

// TestOracleCastRayZeroFractionStops encodes the RayCastCallback contract of
// b2World_CastRay (src/physics_world.c): whatever the b2CastResultFcn returns
// becomes the new maxFraction, so returning 1 keeps the full ray and every
// shape on the line is reported, while returning 0 terminates the ray at the
// first hit.
func TestOracleCastRayZeroFractionStops(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	shapeDef := box2d.DefaultShapeDef()
	for i := range 4 {
		bodyDef := box2d.DefaultBodyDef()
		bodyDef.Position = box2d.Vec2{X: 2.0 * float64(i+1), Y: 0.0}
		bodyID := world.CreateBody(&bodyDef)
		circle := box2d.Circle{Radius: 0.5}
		world.CreateCircleShape(bodyID, &shapeDef, &circle)
	}

	origin := box2d.Vec2{X: -1.0, Y: 0.0}
	translation := box2d.Vec2{X: 20.0, Y: 0.0}

	all := 0
	world.CastRay(origin, translation, box2d.DefaultQueryFilter(),
		func(_ box2d.ShapeID, _ box2d.Vec2, _ box2d.Vec2, _ float64, _ any) float64 {
			all++
			return 1.0
		}, nil)
	assert.Equal(t, 4, all)

	stopped := 0
	world.CastRay(origin, translation, box2d.DefaultQueryFilter(),
		func(_ box2d.ShapeID, _ box2d.Vec2, _ box2d.Vec2, _ float64, _ any) float64 {
			stopped++
			return 0.0
		}, nil)
	assert.Equal(t, 1, stopped)

	// b2World_CastRayClosest keeps the nearest hit only. The nearest circle
	// surface is at x = 1.5, which is 2.5 / 20 along the ray.
	result := world.CastRayClosest(origin, translation, box2d.DefaultQueryFilter())
	require.True(t, result.Hit)
	assert.InDelta(t, 2.5/20.0, result.Fraction, 1e-9)
	assert.InDelta(t, 1.5, result.Point.X, 1e-9)
	assert.InDelta(t, -1.0, result.Normal.X, 1e-9)
}

// TestOracleShapeAPIIsInertWhileTheWorldIsLocked encodes the
// `B2_ASSERT( world->locked == false ); if ( world->locked ) return;` guard
// that opens most of the mutating shape API in src/shape.c
// (b2Shape_SetFilter, b2Shape_Enable*Events, b2Shape_Set{Circle,Capsule,
// Segment,Polygon}, b2Shape_GetContactCapacity, b2Shape_GetSensorCapacity,
// b2Chain_GetSegmentCount, b2Chain_GetSegments, b2Chain_SetSurfaceMaterial,
// b2DestroyShape, b2DestroyChain).
//
// The world is locked for the whole of b2World_Step, and the pre-solve
// callback runs inside the narrow phase, so a call made from there must be a
// no-op and the queries must return zero.
func TestOracleShapeAPIIsInertWhileTheWorldIsLocked(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	groundDef := box2d.DefaultBodyDef()
	groundID := world.CreateBody(&groundDef)
	groundShapeDef := box2d.DefaultShapeDef()
	groundShapeDef.EnablePreSolveEvents = true
	ground := box2d.MakeBox(10.0, 0.5)
	world.CreatePolygonShape(groundID, &groundShapeDef, &ground)

	boxDef := box2d.DefaultBodyDef()
	boxDef.Type = box2d.DynamicBody
	boxDef.Position = box2d.Vec2{X: 0.0, Y: 0.9}
	boxID := world.CreateBody(&boxDef)
	boxShapeDef := box2d.DefaultShapeDef()
	boxShapeDef.EnablePreSolveEvents = true
	box := box2d.MakeBox(0.5, 0.5)
	shapeID := world.CreatePolygonShape(boxID, &boxShapeDef, &box)

	chainDef := box2d.DefaultChainDef()
	chainDef.Points = []box2d.Vec2{
		{X: -20.0, Y: -5.0},
		{X: -10.0, Y: -5.0},
		{X: 10.0, Y: -5.0},
		{X: 20.0, Y: -5.0},
		{X: 30.0, Y: -5.0},
	}
	chainID := world.CreateChain(groundID, &chainDef)

	// The state the locked calls must fail to change.
	filterBefore := world.ShapeFilter(shapeID)
	frictionBefore := world.ShapeFriction(shapeID)
	typeBefore := world.ShapeType(shapeID)

	calls := 0
	world.SetPreSolveCallback(func(_, _ box2d.ShapeID, _, _ box2d.Vec2, _ any) bool {
		calls++

		// Setters: every one of these returns before touching the shape.
		world.EnableShapeSensorEvents(shapeID, true)
		world.EnableShapeContactEvents(shapeID, true)
		world.EnableShapePreSolveEvents(shapeID, false)
		world.EnableShapeHitEvents(shapeID, true)

		newFilter := box2d.DefaultFilter()
		newFilter.CategoryBits = 0x40
		world.SetShapeFilter(shapeID, newFilter)

		world.SetShapeDensity(shapeID, 5.0, false)
		world.SetShapeFriction(shapeID, 0.99)
		world.SetShapeRestitution(shapeID, 0.99)
		world.SetShapeUserMaterial(shapeID, 1234)

		circle := box2d.Circle{Radius: 0.25}
		world.SetShapeCircle(shapeID, &circle)
		capsule := box2d.Capsule{
			Center1: box2d.Vec2{X: -0.4, Y: 0.0},
			Center2: box2d.Vec2{X: 0.4, Y: 0.0},
			Radius:  0.2,
		}
		world.SetShapeCapsule(shapeID, &capsule)
		segment := box2d.Segment{Point1: box2d.Vec2{X: -1.0}, Point2: box2d.Vec2{X: 1.0}}
		world.SetShapeSegment(shapeID, &segment)
		other := box2d.MakeBox(2.0, 2.0)
		world.SetShapePolygon(shapeID, &other)

		material := box2d.DefaultSurfaceMaterial()
		material.Friction = 0.01
		world.SetChainSurfaceMaterial(chainID, material, 0)

		// Destroyers: also guarded.
		world.DestroyShape(shapeID, true)
		world.DestroyChain(chainID)

		// Queries: guarded ones report zero while locked.
		assert.Equal(t, 0, world.ShapeContactCapacity(shapeID))
		assert.Equal(t, 0, world.ShapeContactData(shapeID, make([]box2d.ContactData, 4)))
		assert.Equal(t, 0, world.ShapeSensorCapacity(shapeID))
		assert.Equal(t, 0, world.ShapeSensorData(shapeID, make([]box2d.ShapeID, 4)))
		assert.Equal(t, 0, world.ChainSegmentCount(chainID))
		assert.Equal(t, 0, world.ChainSegments(chainID, make([]box2d.ShapeID, 4)))

		return true
	}, nil)

	for range 30 {
		world.Step(1.0/60.0, 4)
		if calls > 0 {
			break
		}
	}
	require.Positive(t, calls, "the scene must produce a pre-solve callback")

	// Nothing changed, and both the shape and the chain survived.
	assert.True(t, world.IsShapeValid(shapeID))
	assert.True(t, world.IsChainValid(chainID))
	assert.Equal(t, typeBefore, world.ShapeType(shapeID))
	assert.Equal(t, filterBefore, world.ShapeFilter(shapeID))
	assert.InDelta(t, frictionBefore, world.ShapeFriction(shapeID), 0.0)
	assert.True(t, world.AreShapePreSolveEventsEnabled(shapeID))
	assert.False(t, world.AreShapeSensorEventsEnabled(shapeID))
}

// TestOracleWorldQueryIsInertWhileTheWorldIsLocked encodes the same
// `if ( world->locked ) return;` guard for the world query and explosion API
// of src/physics_world.c: b2World_OverlapAABB, b2World_OverlapShape,
// b2World_CastRay, b2World_CastRayClosest, b2World_CastShape,
// b2World_CastMover, b2World_CollideMover and b2World_Explode.
//
// The C returns a zeroed b2TreeStats / b2RayResult, a fraction of 1 for the
// mover cast, and never invokes the user callback.
func TestOracleWorldQueryIsInertWhileTheWorldIsLocked(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	groundDef := box2d.DefaultBodyDef()
	groundID := world.CreateBody(&groundDef)
	groundShapeDef := box2d.DefaultShapeDef()
	groundShapeDef.EnablePreSolveEvents = true
	ground := box2d.MakeBox(10.0, 0.5)
	world.CreatePolygonShape(groundID, &groundShapeDef, &ground)

	boxDef := box2d.DefaultBodyDef()
	boxDef.Type = box2d.DynamicBody
	boxDef.Position = box2d.Vec2{X: 0.0, Y: 0.9}
	boxID := world.CreateBody(&boxDef)
	boxShapeDef := box2d.DefaultShapeDef()
	boxShapeDef.EnablePreSolveEvents = true
	box := box2d.MakeBox(0.5, 0.5)
	world.CreatePolygonShape(boxID, &boxShapeDef, &box)

	everything := box2d.AABB{
		LowerBound: box2d.Vec2{X: -100.0, Y: -100.0},
		UpperBound: box2d.Vec2{X: 100.0, Y: 100.0},
	}
	proxy := box2d.MakeProxy([]box2d.Vec2{{X: 0.0, Y: 0.0}}, 1, 1.0)
	mover := box2d.Capsule{
		Center1: box2d.Vec2{X: 0.0, Y: 5.0},
		Center2: box2d.Vec2{X: 0.0, Y: 6.0},
		Radius:  0.5,
	}

	calls := 0
	world.SetPreSolveCallback(func(_, _ box2d.ShapeID, _, _ box2d.Vec2, _ any) bool {
		calls++

		visits := 0
		overlapFcn := func(_ box2d.ShapeID, _ any) bool {
			visits++
			return true
		}

		stats := world.OverlapAABB(everything, box2d.DefaultQueryFilter(), overlapFcn, nil)
		assert.Equal(t, box2d.TreeStats{}, stats)

		stats = world.OverlapShape(&proxy, box2d.DefaultQueryFilter(), overlapFcn, nil)
		assert.Equal(t, box2d.TreeStats{}, stats)

		castFcn := func(_ box2d.ShapeID, _, _ box2d.Vec2, _ float64, _ any) float64 {
			visits++
			return 1.0
		}
		origin := box2d.Vec2{X: -20.0, Y: 0.0}
		translation := box2d.Vec2{X: 40.0, Y: 0.0}

		stats = world.CastRay(origin, translation, box2d.DefaultQueryFilter(), castFcn, nil)
		assert.Equal(t, box2d.TreeStats{}, stats)

		result := world.CastRayClosest(origin, translation, box2d.DefaultQueryFilter())
		assert.False(t, result.Hit)

		stats = world.CastShape(&proxy, translation, box2d.DefaultQueryFilter(), castFcn, nil)
		assert.Equal(t, box2d.TreeStats{}, stats)

		assert.InDelta(t, 1.0, world.CastMover(&mover, translation, box2d.DefaultQueryFilter()), 0.0)

		world.CollideMover(&mover, box2d.DefaultQueryFilter(),
			func(_ box2d.ShapeID, _ *box2d.PlaneResult, _ any) bool {
				visits++
				return true
			}, nil)

		explosionDef := box2d.DefaultExplosionDef()
		explosionDef.Position = box2d.Vec2{}
		explosionDef.Radius = 50.0
		explosionDef.ImpulsePerLength = 100.0
		world.Explode(&explosionDef)

		assert.Zero(t, visits, "a locked world must not invoke any query callback")

		return true
	}, nil)

	for range 30 {
		world.Step(1.0/60.0, 4)
		if calls > 0 {
			break
		}
	}
	require.Positive(t, calls, "the scene must produce a pre-solve callback")
}

// TestOracleCastShapeByShapeType drives b2ShapeCastShape (src/shape.c) through
// b2World_CastShape for every shape type. The C pushes the cast proxy into the
// shape's local frame, dispatches per type, and pushes the hit back into world
// space. The chain segment arm additionally rejects a cast whose proxy
// centroid starts behind the segment:
//
//	if ( b2Cross( r, edge ) < 0.0f ) return output;   // starts behind
func TestOracleCastShapeByShapeType(t *testing.T) {
	t.Parallel()

	scene := buildOracleShapeScene(t)
	w := scene.world

	castAt := func(origin, translation box2d.Vec2) map[box2d.ShapeID]float64 {
		proxy := box2d.MakeProxy([]box2d.Vec2{origin}, 1, 0.1)
		hits := map[box2d.ShapeID]float64{}
		w.CastShape(&proxy, translation, box2d.DefaultQueryFilter(),
			func(shapeID box2d.ShapeID, _, _ box2d.Vec2, fraction float64, _ any) float64 {
				hits[shapeID] = fraction
				return 1.0
			}, nil)
		return hits
	}

	// A horizontal sweep along y = 0 crosses the circle, the capsule, the
	// polygon and the segment bodies, which sit at x = 0, 5, 10 and 15.
	alongRow := castAt(box2d.Vec2{X: -5.0, Y: 0.0}, box2d.Vec2{X: 30.0, Y: 0.0})
	assert.Contains(t, alongRow, scene.circle)
	assert.Contains(t, alongRow, scene.capsule)
	assert.Contains(t, alongRow, scene.polygon)
	assert.Contains(t, alongRow, scene.segment)

	// The chain sits at y = 20. Its solid segments run left to right, so their
	// front side is -y: a cast coming up from below hits, one coming down from
	// above starts behind and is rejected.
	fromBelow := castAt(box2d.Vec2{X: 0.0, Y: 17.0}, box2d.Vec2{X: 0.0, Y: 6.0})
	assert.Contains(t, fromBelow, scene.chainA)

	fromAbove := castAt(box2d.Vec2{X: 0.0, Y: 23.0}, box2d.Vec2{X: 0.0, Y: -6.0})
	assert.NotContains(t, fromAbove, scene.chainA,
		"a chain segment rejects a shape cast that starts behind it")
}

// TestOracleShapeClosestPointCapsuleAndChain extends the b2Shape_GetClosestPoint
// coverage to the remaining b2MakeShapeDistanceProxy arms (capsule and chain
// segment), whose proxies are the two-point inner segments.
func TestOracleShapeClosestPointCapsuleAndChain(t *testing.T) {
	t.Parallel()

	scene := buildOracleShapeScene(t)
	w := scene.world

	// The capsule body sits at (5, 0), spanning x in [4.5, 5.5] with radius
	// 0.25, so the closest point to a target far above is (5, 0.25).
	closest := w.ShapeClosestPoint(scene.capsule, box2d.Vec2{X: 5.0, Y: 10.0})
	assert.InDelta(t, 5.0, closest.X, 1e-9)
	assert.InDelta(t, 0.25, closest.Y, 1e-9)

	// The first chain segment runs (-2, 20) -> (2, 20) with zero radius.
	closest = w.ShapeClosestPoint(scene.chainA, box2d.Vec2{X: 0.0, Y: 30.0})
	assert.InDelta(t, 0.0, closest.X, 1e-9)
	assert.InDelta(t, 20.0, closest.Y, 1e-9)
}

// TestOracleShapeContactAndSensorData encodes b2Shape_GetContactCapacity,
// b2Shape_GetContactData, b2Shape_GetSensorCapacity and b2Shape_GetSensorData
// in src/shape.c.
//
// The contact pair reports its two shape ids and its manifold; a sensor shape
// reports zero contacts (the C early-returns on `shape->sensorIndex !=
// B2_NULL_INDEX`) and reports its overlaps through the sensor accessors
// instead. A non-sensor shape reports zero sensor capacity.
func TestOracleShapeContactAndSensorData(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.EnableSleep = false
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	groundDef := box2d.DefaultBodyDef()
	groundID := world.CreateBody(&groundDef)
	groundShapeDef := box2d.DefaultShapeDef()
	ground := box2d.MakeBox(10.0, 0.5)
	groundShapeID := world.CreatePolygonShape(groundID, &groundShapeDef, &ground)

	boxDef := box2d.DefaultBodyDef()
	boxDef.Type = box2d.DynamicBody
	boxDef.Position = box2d.Vec2{X: 0.0, Y: 1.0}
	boxID := world.CreateBody(&boxDef)
	boxShapeDef := box2d.DefaultShapeDef()
	boxShapeDef.EnableSensorEvents = true
	box := box2d.MakeBox(0.5, 0.5)
	boxShapeID := world.CreatePolygonShape(boxID, &boxShapeDef, &box)

	sensorBodyDef := box2d.DefaultBodyDef()
	sensorBodyDef.Position = box2d.Vec2{X: 0.0, Y: 1.0}
	sensorBodyID := world.CreateBody(&sensorBodyDef)
	sensorShapeDef := box2d.DefaultShapeDef()
	sensorShapeDef.IsSensor = true
	sensorShapeDef.EnableSensorEvents = true
	sensorBox := box2d.MakeBox(2.0, 2.0)
	sensorShapeID := world.CreatePolygonShape(sensorBodyID, &sensorShapeDef, &sensorBox)

	for range 60 {
		world.Step(1.0/60.0, 4)
	}

	capacity := world.ShapeContactCapacity(boxShapeID)
	require.Positive(t, capacity)

	contacts := make([]box2d.ContactData, capacity)
	count := world.ShapeContactData(boxShapeID, contacts)
	require.Positive(t, count)

	found := false
	for _, data := range contacts[:count] {
		if (data.ShapeIDA == boxShapeID && data.ShapeIDB == groundShapeID) ||
			(data.ShapeIDA == groundShapeID && data.ShapeIDB == boxShapeID) {
			found = true
			assert.Positive(t, data.Manifold.PointCount)
		}
	}
	assert.True(t, found, "the resting box must report its contact with the ground")

	// A short output array truncates rather than overflowing.
	assert.Equal(t, 1, world.ShapeContactData(boxShapeID, make([]box2d.ContactData, 1)))

	// A sensor shape reports no contacts at all.
	assert.Equal(t, 0, world.ShapeContactCapacity(sensorShapeID))
	assert.Equal(t, 0, world.ShapeContactData(sensorShapeID, make([]box2d.ContactData, 4)))

	// The sensor accessors are the mirror image.
	sensorCapacity := world.ShapeSensorCapacity(sensorShapeID)
	require.Positive(t, sensorCapacity)
	visitors := make([]box2d.ShapeID, sensorCapacity)
	visitorCount := world.ShapeSensorData(sensorShapeID, visitors)
	require.Positive(t, visitorCount)
	assert.Contains(t, visitors[:visitorCount], boxShapeID)

	// A short output array truncates here too.
	assert.Equal(t, 1, world.ShapeSensorData(sensorShapeID, make([]box2d.ShapeID, 1)))

	// A non-sensor shape reports zero sensor capacity.
	assert.Equal(t, 0, world.ShapeSensorCapacity(boxShapeID))
	assert.Equal(t, 0, world.ShapeSensorData(boxShapeID, make([]box2d.ShapeID, 4)))
}

// TestOracleCollideMoverChainSegment drives the b2_chainSegmentShape arm of
// b2CollideMover, src/shape.c:
//
//	case b2_chainSegmentShape:
//	    result = b2CollideMoverAndSegment( &localMover, &shape->chainSegment.segment );
//
// which is the same b2CollideMoverAndSegment used for a plain segment, so
// totalRadius is the mover radius alone (src/geometry.c:1043).
func TestOracleCollideMoverChainSegment(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyID := world.CreateBody(&bodyDef)

	chainDef := box2d.DefaultChainDef()
	// Five points, one solid segment from (0, 0) to (10, 0).
	chainDef.Points = []box2d.Vec2{
		{X: -3.0, Y: 0.0},
		{X: 0.0, Y: 0.0},
		{X: 10.0, Y: 0.0},
		{X: 13.0, Y: 0.0},
	}
	world.CreateChain(bodyID, &chainDef)

	// A horizontal mover 0.3 above the segment with radius 0.5: the distance
	// between the bare segment and the mover's inner segment is 0.3 and
	// totalRadius is 0.5, so the plane offset is 0.2 and the normal points up.
	mover := box2d.Capsule{
		Center1: box2d.Vec2{X: 4.0, Y: 0.3},
		Center2: box2d.Vec2{X: 6.0, Y: 0.3},
		Radius:  0.5,
	}

	planes := oracleCollectPlanes(world, &mover)
	require.Len(t, planes, 1)

	plane := planes[0]
	assert.True(t, plane.Hit)
	assert.InDelta(t, 0.0, plane.Plane.Normal.X, 1e-9)
	assert.InDelta(t, 1.0, plane.Plane.Normal.Y, 1e-9)
	assert.InDelta(t, 0.5-0.3, plane.Plane.Offset, 1e-9)
	assert.InDelta(t, 0.0, plane.Point.Y, 1e-9)

	// Beyond the mover radius there is no hit at all.
	far := mover
	far.Center1.Y = 0.8
	far.Center2.Y = 0.8
	assert.Empty(t, oracleCollectPlanes(world, &far))
}

// TestOracleCollideMoverOnRotatedBody encodes the frame conversion at the head
// and tail of b2CollideMover, src/shape.c:
//
//	localMover.center1 = b2InvTransformPoint( transform, mover->center1 ); ...
//	...
//	result.plane.normal = b2RotateVector( transform.q, result.plane.normal );
//
// A shape on a rotated body must therefore report a plane normal in world
// space. A box rotated by a quarter turn presents its (local) +y face along
// world -x, so a mover placed on world -x gets a world normal of (-1, 0).
//
// Note what the C does NOT do: `result.point` is left exactly as
// b2ShapeDistance produced it, in the shape's LOCAL frame. Only the normal is
// rotated back (src/shape.c:974-980). The point assertion below pins that
// asymmetry down.
func TestOracleCollideMoverOnRotatedBody(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Rotation = box2d.MakeRot(0.5 * box2d.Pi)
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	// A 2 x 2 box is rotation invariant in extent, so the geometry stays the
	// same and only the frame conversion is under test.
	box := box2d.MakeBox(1.0, 1.0)
	world.CreatePolygonShape(bodyID, &shapeDef, &box)

	mover := box2d.Capsule{
		Center1: box2d.Vec2{X: -1.3, Y: -0.25},
		Center2: box2d.Vec2{X: -1.3, Y: 0.25},
		Radius:  0.5,
	}

	planes := oracleCollectPlanes(world, &mover)
	require.Len(t, planes, 1)

	plane := planes[0]
	assert.True(t, plane.Hit)
	assert.InDelta(t, -1.0, plane.Plane.Normal.X, 1e-9)
	assert.InDelta(t, 0.0, plane.Plane.Normal.Y, 1e-9)
	assert.InDelta(t, 0.5-0.3, plane.Plane.Offset, 1e-9)

	// In the box's local frame the mover sits above the +y face, so the point
	// b2ShapeDistance returns is on that local face at y = 1.
	assert.InDelta(t, 1.0, plane.Point.Y, 1e-9)
}
