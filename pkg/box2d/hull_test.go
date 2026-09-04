// Tests for the float64 port of Box2D v3.2.0 src/hull.c.

package box2d_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// signedArea returns twice the signed area of the hull polygon. It is positive
// for counter-clockwise winding.
func signedArea(h box2d.Hull) float64 {
	total := 0.0
	for i := 0; i < h.Count; i++ {
		j := (i + 1) % h.Count
		total += box2d.Cross(h.Points[i], h.Points[j])
	}
	return total
}

func TestHullSquare(t *testing.T) {
	t.Parallel()

	points := []box2d.Vec2{{X: -1, Y: -1}, {X: 1, Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1}}
	hull := box2d.ComputeHull(points)

	require.Equal(t, 4, hull.Count)
	// The first hull point is the point furthest from the AABB center; for this
	// input the algorithm keeps the original order.
	assert.Equal(t, points, hull.Points[:hull.Count])
	assert.Positive(t, signedArea(hull), "hull must wind counter-clockwise")
	assert.True(t, box2d.ValidateHull(&hull))
}

func TestHullTriangle(t *testing.T) {
	t.Parallel()

	points := []box2d.Vec2{{X: 0, Y: 0}, {X: 2, Y: 0}, {X: 0, Y: 2}}
	hull := box2d.ComputeHull(points)

	require.Equal(t, 3, hull.Count)
	assert.Positive(t, signedArea(hull))
	assert.True(t, box2d.ValidateHull(&hull))
}

func TestHullDropsInteriorPoints(t *testing.T) {
	t.Parallel()

	// A square plus a point strictly inside it.
	points := []box2d.Vec2{
		{X: -1, Y: -1}, {X: 1, Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1},
		{X: 0.1, Y: -0.2},
	}
	hull := box2d.ComputeHull(points)

	require.Equal(t, 4, hull.Count)
	assert.Positive(t, signedArea(hull))
	assert.True(t, box2d.ValidateHull(&hull))
}

func TestHullWeldsDuplicatePoints(t *testing.T) {
	t.Parallel()

	// The weld tolerance is 16 * linearSlop^2 in squared distance, i.e. points
	// closer than 4 * linearSlop = 0.02 are merged.
	points := []box2d.Vec2{
		{X: -1, Y: -1},
		{X: -1, Y: -1},        // exact duplicate
		{X: -1 + 1e-3, Y: -1}, // within the weld tolerance
		{X: 1, Y: -1},
		{X: 1, Y: 1},
		{X: -1, Y: 1},
	}
	hull := box2d.ComputeHull(points)

	require.Equal(t, 4, hull.Count)
	assert.Positive(t, signedArea(hull))
	assert.True(t, box2d.ValidateHull(&hull))
}

func TestHullDropsCollinearPoints(t *testing.T) {
	t.Parallel()

	// Extra points sitting exactly on the bottom and right edges.
	points := []box2d.Vec2{
		{X: -1, Y: -1}, {X: 0, Y: -1}, {X: 1, Y: -1},
		{X: 1, Y: 0}, {X: 1, Y: 1}, {X: -1, Y: 1},
	}
	hull := box2d.ComputeHull(points)

	require.Equal(t, 4, hull.Count)
	assert.Positive(t, signedArea(hull))
	assert.True(t, box2d.ValidateHull(&hull))
}

func TestHullAllCollinear(t *testing.T) {
	t.Parallel()

	points := []box2d.Vec2{{X: -1, Y: 0}, {X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0}}
	hull := box2d.ComputeHull(points)

	assert.Zero(t, hull.Count, "collinear input must produce an empty hull")
}

func TestHullDegenerateInput(t *testing.T) {
	t.Parallel()

	t.Run("no points", func(t *testing.T) {
		t.Parallel()
		hull := box2d.ComputeHull(nil)
		assert.Zero(t, hull.Count)
	})

	t.Run("two points", func(t *testing.T) {
		t.Parallel()
		hull := box2d.ComputeHull([]box2d.Vec2{{X: 0, Y: 0}, {X: 1, Y: 1}})
		assert.Zero(t, hull.Count)
	})

	t.Run("too many points", func(t *testing.T) {
		t.Parallel()
		points := make([]box2d.Vec2, box2d.MaxPolygonVertices+1)
		for i := range points {
			points[i] = box2d.Vec2{X: float64(i), Y: float64(i * i)}
		}
		hull := box2d.ComputeHull(points)
		assert.Zero(t, hull.Count)
	})

	t.Run("all points welded together", func(t *testing.T) {
		t.Parallel()
		hull := box2d.ComputeHull([]box2d.Vec2{
			{X: 0, Y: 0}, {X: 1e-4, Y: 0}, {X: 0, Y: 1e-4},
		})
		assert.Zero(t, hull.Count)
	})
}

func TestHullValidate(t *testing.T) {
	t.Parallel()

	t.Run("good hull", func(t *testing.T) {
		t.Parallel()
		hull := box2d.ComputeHull([]box2d.Vec2{
			{X: -1, Y: -1}, {X: 1, Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1},
		})
		assert.True(t, box2d.ValidateHull(&hull))
	})

	t.Run("too few points", func(t *testing.T) {
		t.Parallel()
		hull := box2d.Hull{Count: 2}
		hull.Points[0] = box2d.Vec2{X: 0, Y: 0}
		hull.Points[1] = box2d.Vec2{X: 1, Y: 0}
		assert.False(t, box2d.ValidateHull(&hull))
	})

	t.Run("too many points", func(t *testing.T) {
		t.Parallel()
		hull := box2d.Hull{Count: box2d.MaxPolygonVertices + 1}
		assert.False(t, box2d.ValidateHull(&hull))
	})

	t.Run("clockwise winding", func(t *testing.T) {
		t.Parallel()
		hull := box2d.Hull{Count: 4}
		hull.Points[0] = box2d.Vec2{X: -1, Y: -1}
		hull.Points[1] = box2d.Vec2{X: -1, Y: 1}
		hull.Points[2] = box2d.Vec2{X: 1, Y: 1}
		hull.Points[3] = box2d.Vec2{X: 1, Y: -1}
		assert.False(t, box2d.ValidateHull(&hull), "clockwise hull is not convex-left")
	})

	t.Run("collinear points", func(t *testing.T) {
		t.Parallel()
		hull := box2d.Hull{Count: 4}
		hull.Points[0] = box2d.Vec2{X: -1, Y: -1}
		hull.Points[1] = box2d.Vec2{X: 0, Y: -1}
		hull.Points[2] = box2d.Vec2{X: 1, Y: -1}
		hull.Points[3] = box2d.Vec2{X: 0, Y: 1}
		assert.False(t, box2d.ValidateHull(&hull))
	})

	t.Run("non-convex", func(t *testing.T) {
		t.Parallel()
		hull := box2d.Hull{Count: 4}
		hull.Points[0] = box2d.Vec2{X: -1, Y: -1}
		hull.Points[1] = box2d.Vec2{X: 1, Y: -1}
		hull.Points[2] = box2d.Vec2{X: 0, Y: 0} // reflex vertex
		hull.Points[3] = box2d.Vec2{X: 0, Y: 1}
		assert.False(t, box2d.ValidateHull(&hull))
	})
}
