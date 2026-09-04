// Tests for the float64 port of the Box2D v3.2.0 world query API
// (src/physics_world.c query section, pkg/box2d/world_query.go).

package box2d_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// The query scene is a row of 20 bodies spaced 4 m apart along the x axis, all
// centred on y = 0 with a half extent of at most 0.5 m in x. The 4 m spacing is
// wider than the largest broad-phase fat AABB (half extent 0.5 + speculative
// margin 0.02 + shape margin <= 0.71), so an AABB query clipped at a midpoint
// between two shapes can never pick up the neighbour by accident.
const (
	queryShapeCount = 20
	querySpacing    = 4.0

	// Category bits: shapes at an even index carry categoryA, odd ones
	// categoryB. Filter tests select one half of the scene with them.
	queryCategoryA uint64 = 0x0001
	queryCategoryB uint64 = 0x0002
)

// buildQueryWorld creates the shared query scene. It performs CRUD only: the
// world is never stepped, so every body keeps its creation transform.
//
// Shape mix by index modulo 4: circle (static), box (dynamic), capsule
// (static), vertical segment (static). Every shape spans y = 0 so a ray along
// the x axis crosses all of them, and no shape is collinear with that ray.
func buildQueryWorld(t *testing.T) (*box2d.World, []box2d.ShapeID) {
	t.Helper()

	worldDef := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	shapeIDs := make([]box2d.ShapeID, 0, queryShapeCount)

	for i := range queryShapeCount {
		bodyDef := box2d.DefaultBodyDef()
		bodyDef.Position = box2d.Vec2{X: querySpacing * float64(i), Y: 0.0}
		if i%4 == 1 {
			bodyDef.Type = box2d.DynamicBody
		}
		bodyID := world.CreateBody(&bodyDef)

		shapeDef := box2d.DefaultShapeDef()
		shapeDef.Filter.CategoryBits = queryCategoryA
		if i%2 == 1 {
			shapeDef.Filter.CategoryBits = queryCategoryB
		}

		var shapeID box2d.ShapeID
		switch i % 4 {
		case 0:
			circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.5}
			shapeID = world.CreateCircleShape(bodyID, &shapeDef, &circle)
		case 1:
			box := box2d.MakeSquare(0.5)
			shapeID = world.CreatePolygonShape(bodyID, &shapeDef, &box)
		case 2:
			capsule := box2d.Capsule{
				Center1: box2d.Vec2{X: 0.0, Y: -0.25},
				Center2: box2d.Vec2{X: 0.0, Y: 0.25},
				Radius:  0.5,
			}
			shapeID = world.CreateCapsuleShape(bodyID, &shapeDef, &capsule)
		default:
			segment := box2d.Segment{
				Point1: box2d.Vec2{X: 0.0, Y: -0.5},
				Point2: box2d.Vec2{X: 0.0, Y: 0.5},
			}
			shapeID = world.CreateSegmentShape(bodyID, &shapeDef, &segment)
		}

		shapeIDs = append(shapeIDs, shapeID)
	}

	return world, shapeIDs
}

// indexOfShape maps a ShapeID back to its scene index.
func indexOfShape(t *testing.T, shapeIDs []box2d.ShapeID, id box2d.ShapeID) int {
	t.Helper()

	for i, candidate := range shapeIDs {
		if candidate == id {
			return i
		}
	}

	t.Fatalf("unknown shape id %+v", id)
	return -1
}

// collector accumulates the scene indices reported by a query callback.
type collector struct {
	t         *testing.T
	shapeIDs  []box2d.ShapeID
	indices   []int
	fractions []float64
	limit     int
	ret       float64
	clip      bool
}

func (c *collector) overlap(shapeID box2d.ShapeID, _ any) bool {
	c.indices = append(c.indices, indexOfShape(c.t, c.shapeIDs, shapeID))
	return c.limit <= 0 || len(c.indices) < c.limit
}

func (c *collector) cast(shapeID box2d.ShapeID, _, _ box2d.Vec2, fraction float64, _ any) float64 {
	c.indices = append(c.indices, indexOfShape(c.t, c.shapeIDs, shapeID))
	c.fractions = append(c.fractions, fraction)
	if c.clip {
		// Clip the ray to the reported hit.
		return fraction
	}
	return c.ret
}

// ---------------------------------------------------------------------------
// OverlapAABB
// ---------------------------------------------------------------------------

func TestOverlapAABB(t *testing.T) {
	t.Parallel()

	t.Run("hand picked hit set", func(t *testing.T) {
		t.Parallel()
		world, shapeIDs := buildQueryWorld(t)

		// Clipped between the shapes at x = 8 and x = 12.
		aabb := box2d.AABB{
			LowerBound: box2d.Vec2{X: -1.0, Y: -1.0},
			UpperBound: box2d.Vec2{X: 9.0, Y: 1.0},
		}
		c := &collector{t: t, shapeIDs: shapeIDs}
		stats := world.OverlapAABB(aabb, box2d.DefaultQueryFilter(), c.overlap, nil)

		assert.ElementsMatch(t, []int{0, 1, 2}, c.indices)
		assert.Positive(t, stats.LeafVisits)
	})

	t.Run("filter respected", func(t *testing.T) {
		t.Parallel()
		world, shapeIDs := buildQueryWorld(t)

		aabb := box2d.AABB{
			LowerBound: box2d.Vec2{X: -1.0, Y: -1.0},
			UpperBound: box2d.Vec2{X: 9.0, Y: 1.0},
		}
		filter := box2d.DefaultQueryFilter()
		filter.MaskBits = queryCategoryA
		c := &collector{t: t, shapeIDs: shapeIDs}
		world.OverlapAABB(aabb, filter, c.overlap, nil)

		// Only the even indices carry queryCategoryA.
		assert.ElementsMatch(t, []int{0, 2}, c.indices)
	})

	t.Run("early termination", func(t *testing.T) {
		t.Parallel()
		world, shapeIDs := buildQueryWorld(t)

		aabb := box2d.AABB{
			LowerBound: box2d.Vec2{X: -1.0, Y: -1.0},
			UpperBound: box2d.Vec2{X: 100.0, Y: 1.0},
		}
		full := &collector{t: t, shapeIDs: shapeIDs}
		world.OverlapAABB(aabb, box2d.DefaultQueryFilter(), full.overlap, nil)
		require.Len(t, full.indices, queryShapeCount)

		// Upstream b2World_OverlapAABB does not break out of the body-type
		// tree loop when the callback returns false: it stops the current
		// tree traversal only. The scene has one non-empty static tree and one
		// non-empty dynamic tree, so a limit of one yields two reports.
		stopped := &collector{t: t, shapeIDs: shapeIDs, limit: 1}
		world.OverlapAABB(aabb, box2d.DefaultQueryFilter(), stopped.overlap, nil)
		assert.Len(t, stopped.indices, 2)
		assert.Less(t, len(stopped.indices), len(full.indices))
	})

	t.Run("deterministic order", func(t *testing.T) {
		t.Parallel()

		aabb := box2d.AABB{
			LowerBound: box2d.Vec2{X: -1.0, Y: -1.0},
			UpperBound: box2d.Vec2{X: 100.0, Y: 1.0},
		}

		worldA, shapesA := buildQueryWorld(t)
		first := &collector{t: t, shapeIDs: shapesA}
		worldA.OverlapAABB(aabb, box2d.DefaultQueryFilter(), first.overlap, nil)

		worldB, shapesB := buildQueryWorld(t)
		second := &collector{t: t, shapeIDs: shapesB}
		worldB.OverlapAABB(aabb, box2d.DefaultQueryFilter(), second.overlap, nil)

		assert.Equal(t, first.indices, second.indices)
	})
}

// ---------------------------------------------------------------------------
// OverlapShape
// ---------------------------------------------------------------------------

func TestOverlapShape(t *testing.T) {
	t.Parallel()
	world, shapeIDs := buildQueryWorld(t)

	// A circle proxy sitting on the shape at x = 4. Its nearest neighbours are
	// 4 m away, far beyond the 0.6 m radius.
	proxy := box2d.MakeProxy([]box2d.Vec2{{X: querySpacing, Y: 0.0}}, 1, 0.6)
	c := &collector{t: t, shapeIDs: shapeIDs}
	world.OverlapShape(&proxy, box2d.DefaultQueryFilter(), c.overlap, nil)

	assert.Equal(t, []int{1}, c.indices)

	// Move the proxy into the gap: nothing is within its radius.
	gap := box2d.MakeProxy([]box2d.Vec2{{X: 2.0, Y: 0.0}}, 1, 0.6)
	empty := &collector{t: t, shapeIDs: shapeIDs}
	world.OverlapShape(&gap, box2d.DefaultQueryFilter(), empty.overlap, nil)

	assert.Empty(t, empty.indices)
}

// ---------------------------------------------------------------------------
// CastRay / CastRayClosest
// ---------------------------------------------------------------------------

func TestCastRay(t *testing.T) {
	t.Parallel()

	origin := box2d.Vec2{X: -2.0, Y: 0.0}
	translation := box2d.Vec2{X: 100.0, Y: 0.0}

	t.Run("no clipping reports every shape", func(t *testing.T) {
		t.Parallel()
		world, shapeIDs := buildQueryWorld(t)

		c := &collector{t: t, shapeIDs: shapeIDs, ret: 1.0}
		stats := world.CastRay(origin, translation, box2d.DefaultQueryFilter(), c.cast, nil)

		assert.Len(t, c.indices, queryShapeCount)
		assert.Positive(t, stats.LeafVisits)
	})

	t.Run("skip fraction keeps enumerating", func(t *testing.T) {
		t.Parallel()
		world, shapeIDs := buildQueryWorld(t)

		// Returning -1 tells the cast to ignore the shape and continue.
		c := &collector{t: t, shapeIDs: shapeIDs, ret: -1.0}
		world.CastRay(origin, translation, box2d.DefaultQueryFilter(), c.cast, nil)

		assert.Len(t, c.indices, queryShapeCount)
	})

	t.Run("zero fraction terminates", func(t *testing.T) {
		t.Parallel()
		world, shapeIDs := buildQueryWorld(t)

		c := &collector{t: t, shapeIDs: shapeIDs, ret: 0.0}
		world.CastRay(origin, translation, box2d.DefaultQueryFilter(), c.cast, nil)

		assert.Len(t, c.indices, 1)
	})

	t.Run("clipping ends on the closest hit", func(t *testing.T) {
		t.Parallel()
		world, shapeIDs := buildQueryWorld(t)

		// The collector returns the reported fraction, so the cast is clipped
		// after every hit and the last report is the closest one.
		c := &collector{t: t, shapeIDs: shapeIDs, clip: true}
		world.CastRay(origin, translation, box2d.DefaultQueryFilter(), c.cast, nil)

		require.NotEmpty(t, c.indices)
		last := len(c.indices) - 1
		assert.Equal(t, 0, c.indices[last])
		// The circle at the origin has radius 0.5, so the ray travels
		// 1.5 m of its 100 m translation.
		assert.InDelta(t, 0.015, c.fractions[last], 1e-12)

		for i := 1; i < len(c.fractions); i++ {
			assert.LessOrEqual(t, c.fractions[i], c.fractions[i-1])
		}
	})
}

func TestCastRayClosest(t *testing.T) {
	t.Parallel()

	translation := box2d.Vec2{X: 100.0, Y: 0.0}

	t.Run("nearest shape", func(t *testing.T) {
		t.Parallel()
		world, shapeIDs := buildQueryWorld(t)

		result := world.CastRayClosest(box2d.Vec2{X: -2.0, Y: 0.0}, translation, box2d.DefaultQueryFilter())

		require.True(t, result.Hit)
		assert.Equal(t, 0, indexOfShape(t, shapeIDs, result.ShapeID))
		assert.InDelta(t, 0.015, result.Fraction, 1e-12)
		assert.InDelta(t, -0.5, result.Point.X, 1e-12)
		assert.InDelta(t, 0.0, result.Point.Y, 1e-12)
		assert.InDelta(t, -1.0, result.Normal.X, 1e-12)
		assert.InDelta(t, 0.0, result.Normal.Y, 1e-12)
		assert.Positive(t, result.LeafVisits)
	})

	t.Run("miss", func(t *testing.T) {
		t.Parallel()
		world, _ := buildQueryWorld(t)

		result := world.CastRayClosest(box2d.Vec2{X: -2.0, Y: 10.0}, translation, box2d.DefaultQueryFilter())

		assert.False(t, result.Hit)
	})

	t.Run("filter respected", func(t *testing.T) {
		t.Parallel()
		world, shapeIDs := buildQueryWorld(t)

		filter := box2d.DefaultQueryFilter()
		filter.MaskBits = queryCategoryB
		result := world.CastRayClosest(box2d.Vec2{X: -2.0, Y: 0.0}, translation, filter)

		require.True(t, result.Hit)
		// The first odd-index shape is the 1x1 box centred at x = 4.
		assert.Equal(t, 1, indexOfShape(t, shapeIDs, result.ShapeID))
		assert.InDelta(t, 0.055, result.Fraction, 1e-12)
		assert.InDelta(t, 3.5, result.Point.X, 1e-12)
	})

	t.Run("initial overlap ignored", func(t *testing.T) {
		t.Parallel()
		world, shapeIDs := buildQueryWorld(t)

		// The origin sits inside the first circle. Upstream b2RayCastClosestFcn
		// returns -1 for a zero fraction, so that shape is skipped.
		result := world.CastRayClosest(box2d.Vec2{}, translation, box2d.DefaultQueryFilter())

		require.True(t, result.Hit)
		assert.Equal(t, 1, indexOfShape(t, shapeIDs, result.ShapeID))
		assert.Positive(t, result.Fraction)
		assert.InDelta(t, 0.035, result.Fraction, 1e-12)
	})
}

// ---------------------------------------------------------------------------
// CastShape
// ---------------------------------------------------------------------------

func TestCastShape(t *testing.T) {
	t.Parallel()
	world, shapeIDs := buildQueryWorld(t)

	origin := box2d.Vec2{X: -2.0, Y: 0.0}
	translation := box2d.Vec2{X: 100.0, Y: 0.0}

	// A circle of radius 0.25 cast along the same line as the ray above.
	proxy := box2d.MakeProxy([]box2d.Vec2{origin}, 1, 0.25)
	c := &collector{t: t, shapeIDs: shapeIDs, clip: true}
	stats := world.CastShape(&proxy, translation, box2d.DefaultQueryFilter(), c.cast, nil)

	require.NotEmpty(t, c.indices)
	last := len(c.indices) - 1
	assert.Equal(t, 0, c.indices[last])
	assert.Positive(t, stats.LeafVisits)

	// Contact when the centres are 0.5 + 0.25 apart, i.e. after 1.25 m.
	shapeFraction := c.fractions[last]
	assert.InDelta(t, 0.0125, shapeFraction, 1e-3)

	// Sanity bound against the equivalent ray cast: a fattened cast must stop
	// earlier than the zero radius ray, but by no more than the extra radius.
	ray := world.CastRayClosest(origin, translation, box2d.DefaultQueryFilter())
	require.True(t, ray.Hit)
	assert.Less(t, shapeFraction, ray.Fraction)
	assert.Greater(t, shapeFraction, ray.Fraction-0.25/100.0-1e-9)
}

// ---------------------------------------------------------------------------
// CastMover / CollideMover
// ---------------------------------------------------------------------------

func TestCastMover(t *testing.T) {
	t.Parallel()
	world, _ := buildQueryWorld(t)

	// A vertical capsule mover to the left of the first circle. Its spine spans
	// y = [-0.4, 0.4] so the closest point to the circle centre is on the
	// spine, and contact happens when the centres are 0.5 + 0.3 apart.
	mover := box2d.Capsule{
		Center1: box2d.Vec2{X: -2.0, Y: -0.4},
		Center2: box2d.Vec2{X: -2.0, Y: 0.4},
		Radius:  0.3,
	}
	fraction := world.CastMover(&mover, box2d.Vec2{X: 100.0, Y: 0.0}, box2d.DefaultQueryFilter())

	// Travel of 1.2 m out of a 100 m translation.
	assert.InDelta(t, 0.012, fraction, 1e-3)

	// A mover aimed above the scene travels the whole way.
	clearOfScene := box2d.Capsule{
		Center1: box2d.Vec2{X: -2.0, Y: 9.6},
		Center2: box2d.Vec2{X: -2.0, Y: 10.4},
		Radius:  0.3,
	}
	assert.InDelta(t, 1.0, world.CastMover(&clearOfScene, box2d.Vec2{X: 100.0, Y: 0.0}, box2d.DefaultQueryFilter()), 1e-12)
}

func TestWorldQueryCollideMover(t *testing.T) {
	t.Parallel()
	world, shapeIDs := buildQueryWorld(t)

	// Overlap the first circle by 0.1 m: centres 0.7 apart, radii sum 0.8.
	mover := box2d.Capsule{
		Center1: box2d.Vec2{X: -0.7, Y: -0.4},
		Center2: box2d.Vec2{X: -0.7, Y: 0.4},
		Radius:  0.3,
	}

	planes := make([]box2d.PlaneResult, 0, 4)
	hitIndices := make([]int, 0, 4)
	fcn := func(shapeID box2d.ShapeID, plane *box2d.PlaneResult, _ any) bool {
		hitIndices = append(hitIndices, indexOfShape(t, shapeIDs, shapeID))
		planes = append(planes, *plane)
		return true
	}

	world.CollideMover(&mover, box2d.DefaultQueryFilter(), fcn, nil)

	require.Len(t, planes, 1)
	assert.Equal(t, []int{0}, hitIndices)
	assert.True(t, planes[0].Hit)
	assert.InDelta(t, -1.0, planes[0].Plane.Normal.X, 1e-9)
	assert.InDelta(t, 0.0, planes[0].Plane.Normal.Y, 1e-9)
	assert.InDelta(t, 0.1, planes[0].Plane.Offset, 1e-9)

	// A mover in the gap between shapes gathers no planes.
	far := box2d.Capsule{
		Center1: box2d.Vec2{X: 2.0, Y: -0.4},
		Center2: box2d.Vec2{X: 2.0, Y: 0.4},
		Radius:  0.3,
	}
	count := 0
	world.CollideMover(&far, box2d.DefaultQueryFilter(), func(box2d.ShapeID, *box2d.PlaneResult, any) bool {
		count++
		return true
	}, nil)
	assert.Equal(t, 0, count)
}
