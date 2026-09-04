package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

func xf(px, py, angle float64) box2d.Transform {
	return box2d.Transform{P: box2d.Vec2{X: px, Y: py}, Q: box2d.MakeRot(angle)}
}

func requireVec(t *testing.T, want box2d.Vec2, got box2d.Vec2, tol float64) {
	t.Helper()
	require.InDelta(t, want.X, got.X, tol, "vec.X")
	require.InDelta(t, want.Y, got.Y, tol, "vec.Y")
}

func TestCollideCircles(t *testing.T) {
	t.Parallel()

	circleA := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.5}
	circleB := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.5}

	t.Run("overlapping", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollideCircles(&circleA, xf(0, 0, 0), &circleB, xf(0.9, 0, 0))
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: 1, Y: 0}, m.Normal, 1e-12)
		require.InDelta(t, -0.1, m.Points[0].Separation, 1e-12)
		require.Equal(t, uint16(0), m.Points[0].ID)
		// Contact point is at the midpoint of the two surface points.
		requireVec(t, box2d.Vec2{X: 0.45, Y: 0}, m.Points[0].AnchorA, 1e-12)
	})

	t.Run("separated beyond speculative", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollideCircles(&circleA, xf(0, 0, 0), &circleB, xf(2, 0, 0))
		require.Equal(t, 0, m.PointCount)
	})

	t.Run("speculative margin", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollideCircles(&circleA, xf(0, 0, 0), &circleB, xf(1.01, 0, 0))
		require.Equal(t, 1, m.PointCount)
		require.InDelta(t, 0.01, m.Points[0].Separation, 1e-12)
		require.Positive(t, m.Points[0].Separation)
	})
}

func TestCollideCapsuleAndCircle(t *testing.T) {
	t.Parallel()

	capsuleA := box2d.Capsule{
		Center1: box2d.Vec2{X: -0.5, Y: 0},
		Center2: box2d.Vec2{X: 0.5, Y: 0},
		Radius:  0.25,
	}
	circleB := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}

	t.Run("interior region", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollideCapsuleAndCircle(&capsuleA, xf(0, 0, 0), &circleB, xf(0, 0.45, 0))
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-12)
		require.InDelta(t, -0.05, m.Points[0].Separation, 1e-12)
	})

	t.Run("endpoint region", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollideCapsuleAndCircle(&capsuleA, xf(0, 0, 0), &circleB, xf(1.0, 0, 0))
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: 1, Y: 0}, m.Normal, 1e-12)
		require.InDelta(t, 0.0, m.Points[0].Separation, 1e-12)
	})

	t.Run("separated beyond speculative", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollideCapsuleAndCircle(&capsuleA, xf(0, 0, 0), &circleB, xf(0, 2, 0))
		require.Equal(t, 0, m.PointCount)
	})
}

func TestCollidePolygonAndCircle(t *testing.T) {
	t.Parallel()

	boxA := box2d.MakeBox(0.5, 0.5)
	circleB := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}

	t.Run("face region", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollidePolygonAndCircle(&boxA, xf(0, 0, 0), &circleB, xf(0, 0.7, 0))
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-12)
		require.InDelta(t, -0.05, m.Points[0].Separation, 1e-12)
	})

	t.Run("vertex region", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollidePolygonAndCircle(&boxA, xf(0, 0, 0), &circleB, xf(0.65, 0.65, 0))
		require.Equal(t, 1, m.PointCount)
		s := math.Sqrt(2.0) / 2.0
		requireVec(t, box2d.Vec2{X: s, Y: s}, m.Normal, 1e-12)
		require.InDelta(t, math.Sqrt(0.045)-0.25, m.Points[0].Separation, 1e-12)
	})

	t.Run("separated beyond speculative", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollidePolygonAndCircle(&boxA, xf(0, 0, 0), &circleB, xf(0, 1, 0))
		require.Equal(t, 0, m.PointCount)
	})
}

func TestCollideCapsules(t *testing.T) {
	t.Parallel()

	capsuleA := box2d.Capsule{
		Center1: box2d.Vec2{X: -0.5, Y: 0},
		Center2: box2d.Vec2{X: 0.5, Y: 0},
		Radius:  0.2,
	}

	t.Run("parallel overlap gives two points", func(t *testing.T) {
		t.Parallel()
		capsuleB := capsuleA
		m := box2d.CollideCapsules(&capsuleA, xf(0, 0, 0), &capsuleB, xf(0, 0.35, 0))
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-12)
		require.InDelta(t, -0.05, m.Points[0].Separation, 1e-12)
		require.InDelta(t, -0.05, m.Points[1].Separation, 1e-12)
		// B2_MAKE_ID(0, 0) and B2_MAKE_ID(0, 1).
		require.Equal(t, uint16(0x0000), m.Points[0].ID)
		require.Equal(t, uint16(0x0001), m.Points[1].ID)
	})

	t.Run("perpendicular T gives one point", func(t *testing.T) {
		t.Parallel()
		capsuleB := box2d.Capsule{
			Center1: box2d.Vec2{X: 0, Y: -0.5},
			Center2: box2d.Vec2{X: 0, Y: 0.5},
			Radius:  0.2,
		}
		m := box2d.CollideCapsules(&capsuleA, xf(0, 0, 0), &capsuleB, xf(0, 0.9, 0))
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-12)
		require.InDelta(t, 0.0, m.Points[0].Separation, 1e-12)
		// B2_MAKE_ID(1, 0): interior of A (f1 != 0), endpoint 1 of B (f2 == 0).
		require.Equal(t, uint16(0x0100), m.Points[0].ID)
		requireVec(t, box2d.Vec2{X: 0, Y: 0.2}, m.Points[0].AnchorA, 1e-12)
	})

	t.Run("separated beyond speculative", func(t *testing.T) {
		t.Parallel()
		capsuleB := capsuleA
		m := box2d.CollideCapsules(&capsuleA, xf(0, 0, 0), &capsuleB, xf(0, 1, 0))
		require.Equal(t, 0, m.PointCount)
	})

	t.Run("id stability under perturbation", func(t *testing.T) {
		t.Parallel()
		capsuleB := capsuleA
		m1 := box2d.CollideCapsules(&capsuleA, xf(0, 0, 0), &capsuleB, xf(0, 0.35, 0))
		m2 := box2d.CollideCapsules(&capsuleA, xf(0, 0, 0), &capsuleB, xf(0.02, 0.349, 0.001))
		require.Equal(t, 2, m1.PointCount)
		require.Equal(t, 2, m2.PointCount)
		require.Equal(t, m1.Points[0].ID, m2.Points[0].ID)
		require.Equal(t, m1.Points[1].ID, m2.Points[1].ID)
	})
}

func TestCollidePolygons(t *testing.T) {
	t.Parallel()

	boxA := box2d.MakeBox(0.5, 0.5)
	boxB := box2d.MakeBox(0.5, 0.5)

	t.Run("face overlap gives two points", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollidePolygons(&boxA, xf(0, 0, 0), &boxB, xf(0, 0.98, 0))
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-12)
		require.InDelta(t, -0.02, m.Points[0].Separation, 1e-12)
		require.InDelta(t, -0.02, m.Points[1].Separation, 1e-12)
	})

	t.Run("speculative margin gives positive separation", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollidePolygons(&boxA, xf(0, 0, 0), &boxB, xf(0, 1.01, 0))
		require.Equal(t, 2, m.PointCount)
		require.InDelta(t, 0.01, m.Points[0].Separation, 1e-12)
		require.InDelta(t, 0.01, m.Points[1].Separation, 1e-12)
		require.Positive(t, m.Points[0].Separation)
		require.Positive(t, m.Points[1].Separation)
	})

	t.Run("separated beyond speculative", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollidePolygons(&boxA, xf(0, 0, 0), &boxB, xf(0, 1.05, 0))
		require.Equal(t, 0, m.PointCount)
	})

	t.Run("face ids stable across small slide", func(t *testing.T) {
		t.Parallel()
		m1 := box2d.CollidePolygons(&boxA, xf(0, 0, 0), &boxB, xf(0, 0.98, 0))
		m2 := box2d.CollidePolygons(&boxA, xf(0, 0, 0), &boxB, xf(0.05, 0.98, 0))
		m3 := box2d.CollidePolygons(&boxA, xf(0, 0, 0), &boxB, xf(0.05, 0.98, 0.001))
		require.Equal(t, 2, m1.PointCount)
		require.Equal(t, 2, m2.PointCount)
		require.Equal(t, 2, m3.PointCount)
		require.Equal(t, m1.Points[0].ID, m2.Points[0].ID)
		require.Equal(t, m1.Points[1].ID, m2.Points[1].ID)
		require.Equal(t, m1.Points[0].ID, m3.Points[0].ID)
		require.Equal(t, m1.Points[1].ID, m3.Points[1].ID)
	})

	t.Run("rounded polygons", func(t *testing.T) {
		t.Parallel()
		roundedA := box2d.MakeRoundedBox(0.5, 0.5, 0.1)
		roundedB := box2d.MakeRoundedBox(0.5, 0.5, 0.1)
		m := box2d.CollidePolygons(&roundedA, xf(0, 0, 0), &roundedB, xf(0, 1.15, 0))
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-12)
		require.InDelta(t, -0.05, m.Points[0].Separation, 1e-12)
		require.InDelta(t, -0.05, m.Points[1].Separation, 1e-12)
	})

	t.Run("vertex-vertex corner contact", func(t *testing.T) {
		t.Parallel()
		// Corner of B near the top-right corner of A along the diagonal.
		m := box2d.CollidePolygons(&boxA, xf(0, 0, 0), &boxB, xf(1.005, 1.005, 0))
		require.Equal(t, 1, m.PointCount)
		s := math.Sqrt(2.0) / 2.0
		requireVec(t, box2d.Vec2{X: s, Y: s}, m.Normal, 1e-9)
		require.InDelta(t, math.Sqrt(2.0*0.005*0.005), m.Points[0].Separation, 1e-12)
	})
}

func TestCollideSegmentAndCircle(t *testing.T) {
	t.Parallel()

	segmentA := box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}}
	circleB := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}

	t.Run("overlapping", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollideSegmentAndCircle(&segmentA, xf(0, 0, 0), &circleB, xf(0, 0.2, 0))
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-12)
		require.InDelta(t, -0.05, m.Points[0].Separation, 1e-12)
	})

	t.Run("separated beyond speculative", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollideSegmentAndCircle(&segmentA, xf(0, 0, 0), &circleB, xf(0, 1, 0))
		require.Equal(t, 0, m.PointCount)
	})
}

func TestCollideSegmentAndCapsule(t *testing.T) {
	t.Parallel()

	segmentA := box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}}
	capsuleB := box2d.Capsule{
		Center1: box2d.Vec2{X: -0.3, Y: 0},
		Center2: box2d.Vec2{X: 0.3, Y: 0},
		Radius:  0.2,
	}

	t.Run("parallel overlap gives two points", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollideSegmentAndCapsule(&segmentA, xf(0, 0, 0), &capsuleB, xf(0, 0.15, 0))
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-12)
		require.InDelta(t, -0.05, m.Points[0].Separation, 1e-12)
		require.InDelta(t, -0.05, m.Points[1].Separation, 1e-12)
	})

	t.Run("separated beyond speculative", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollideSegmentAndCapsule(&segmentA, xf(0, 0, 0), &capsuleB, xf(0, 1, 0))
		require.Equal(t, 0, m.PointCount)
	})
}

func TestCollidePolygonAndCapsule(t *testing.T) {
	t.Parallel()

	boxA := box2d.MakeBox(0.5, 0.5)
	capsuleB := box2d.Capsule{
		Center1: box2d.Vec2{X: -0.3, Y: 0},
		Center2: box2d.Vec2{X: 0.3, Y: 0},
		Radius:  0.2,
	}

	m := box2d.CollidePolygonAndCapsule(&boxA, xf(0, 0, 0), &capsuleB, xf(0, 0.65, 0))
	require.Equal(t, 2, m.PointCount)
	requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-12)
	require.InDelta(t, -0.05, m.Points[0].Separation, 1e-12)
	require.InDelta(t, -0.05, m.Points[1].Separation, 1e-12)
}

func TestCollideSegmentAndPolygon(t *testing.T) {
	t.Parallel()

	segmentA := box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}}
	boxB := box2d.MakeBox(0.5, 0.5)

	t.Run("overlapping", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollideSegmentAndPolygon(&segmentA, xf(0, 0, 0), &boxB, xf(0, 0.45, 0))
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-12)
		require.InDelta(t, -0.05, m.Points[0].Separation, 1e-12)
		require.InDelta(t, -0.05, m.Points[1].Separation, 1e-12)
	})

	t.Run("separated beyond speculative", func(t *testing.T) {
		t.Parallel()
		m := box2d.CollideSegmentAndPolygon(&segmentA, xf(0, 0, 0), &boxB, xf(0, 1, 0))
		require.Equal(t, 0, m.PointCount)
	})
}

// Chain segments collide only on their right side. A segment from (-1,0) to
// (1,0) has edge direction (1,0), so the collision side is y < 0.
func TestCollideChainSegmentAndCircle(t *testing.T) {
	t.Parallel()

	collinear := box2d.ChainSegment{
		Ghost1:  box2d.Vec2{X: -2, Y: 0},
		Segment: box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}},
		Ghost2:  box2d.Vec2{X: 2, Y: 0},
	}

	t.Run("interior contact on collision side", func(t *testing.T) {
		t.Parallel()
		circleB := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}
		m := box2d.CollideChainSegmentAndCircle(&collinear, xf(0, 0, 0), &circleB, xf(0, -0.2, 0))
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: -1}, m.Normal, 1e-12)
		require.InDelta(t, -0.05, m.Points[0].Separation, 1e-12)
	})

	t.Run("wrong side is rejected", func(t *testing.T) {
		t.Parallel()
		circleB := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}
		m := box2d.CollideChainSegmentAndCircle(&collinear, xf(0, 0, 0), &circleB, xf(0, 0.2, 0))
		require.Equal(t, 0, m.PointCount)
	})

	t.Run("ghost voronoi region rejects past tail", func(t *testing.T) {
		t.Parallel()
		// Circle behind point1: with a collinear ghost the previous edge owns
		// the region, so the collision is rejected.
		circleB := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.35}
		m := box2d.CollideChainSegmentAndCircle(&collinear, xf(0, 0, 0), &circleB, xf(-1.1, -0.3, 0))
		require.Equal(t, 0, m.PointCount)
	})

	t.Run("convex ghost vertex admits past tail", func(t *testing.T) {
		t.Parallel()
		// Same circle position, but ghost1 makes a convex corner at point1
		// (relative to the collision side), so the vertex collision is kept.
		convex := collinear
		convex.Ghost1 = box2d.Vec2{X: -2, Y: 1}
		circleB := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.35}
		m := box2d.CollideChainSegmentAndCircle(&convex, xf(0, 0, 0), &circleB, xf(-1.1, -0.3, 0))
		require.Equal(t, 1, m.PointCount)
		require.InDelta(t, math.Sqrt(0.1)-0.35, m.Points[0].Separation, 1e-12)
	})
}

func TestCollideChainSegmentAndCapsule(t *testing.T) {
	t.Parallel()

	chain := box2d.ChainSegment{
		Ghost1:  box2d.Vec2{X: -2, Y: 0},
		Segment: box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}},
		Ghost2:  box2d.Vec2{X: 2, Y: 0},
	}
	capsuleB := box2d.Capsule{
		Center1: box2d.Vec2{X: -0.3, Y: 0},
		Center2: box2d.Vec2{X: 0.3, Y: 0},
		Radius:  0.2,
	}

	t.Run("parallel overlap below", func(t *testing.T) {
		t.Parallel()
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndCapsule(&chain, xf(0, 0, 0), &capsuleB, xf(0, -0.15, 0), &cache)
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: -1}, m.Normal, 1e-12)
		require.InDelta(t, -0.05, m.Points[0].Separation, 1e-12)
		require.InDelta(t, -0.05, m.Points[1].Separation, 1e-12)
	})

	t.Run("wrong side is rejected", func(t *testing.T) {
		t.Parallel()
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndCapsule(&chain, xf(0, 0, 0), &capsuleB, xf(0, 0.5, 0), &cache)
		require.Equal(t, 0, m.PointCount)
	})
}

func TestCollideChainSegmentAndPolygon(t *testing.T) {
	t.Parallel()

	chain := box2d.ChainSegment{
		Ghost1:  box2d.Vec2{X: -2, Y: 0},
		Segment: box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}},
		Ghost2:  box2d.Vec2{X: 2, Y: 0},
	}
	boxB := box2d.MakeBox(0.5, 0.5)

	t.Run("face contact below", func(t *testing.T) {
		t.Parallel()
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndPolygon(&chain, xf(0, 0, 0), &boxB, xf(0, -0.4, 0), &cache)
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: -1}, m.Normal, 1e-12)
		require.InDelta(t, -0.1, m.Points[0].Separation, 1e-12)
		require.InDelta(t, -0.1, m.Points[1].Separation, 1e-12)
	})

	t.Run("one-sided rejection above", func(t *testing.T) {
		t.Parallel()
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndPolygon(&chain, xf(0, 0, 0), &boxB, xf(0, 0.4, 0), &cache)
		require.Equal(t, 0, m.PointCount)
	})

	t.Run("speculative margin below", func(t *testing.T) {
		t.Parallel()
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndPolygon(&chain, xf(0, 0, 0), &boxB, xf(0, -0.51, 0), &cache)
		require.Equal(t, 2, m.PointCount)
		require.InDelta(t, 0.01, m.Points[0].Separation, 1e-12)
		require.InDelta(t, 0.01, m.Points[1].Separation, 1e-12)
		require.Positive(t, m.Points[0].Separation)
		require.Positive(t, m.Points[1].Separation)
	})

	t.Run("face ids stable across small slide", func(t *testing.T) {
		t.Parallel()
		var cache1, cache2 box2d.SimplexCache
		m1 := box2d.CollideChainSegmentAndPolygon(&chain, xf(0, 0, 0), &boxB, xf(0, -0.4, 0), &cache1)
		m2 := box2d.CollideChainSegmentAndPolygon(&chain, xf(0, 0, 0), &boxB, xf(0.05, -0.4, 0), &cache2)
		require.Equal(t, 2, m1.PointCount)
		require.Equal(t, 2, m2.PointCount)
		require.Equal(t, m1.Points[0].ID, m2.Points[0].ID)
		require.Equal(t, m1.Points[1].ID, m2.Points[1].ID)
	})

	t.Run("separated beyond speculative", func(t *testing.T) {
		t.Parallel()
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndPolygon(&chain, xf(0, 0, 0), &boxB, xf(0, -1.0, 0), &cache)
		require.Equal(t, 0, m.PointCount)
	})
}
