package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// requireBitEqual asserts exact (bitwise) float64 equality. The port is
// bit-deterministic, so these tests intentionally demand exact bits rather
// than an epsilon comparison.
func requireBitEqual(t *testing.T, want, got float64) {
	t.Helper()
	require.Equal(t, math.Float64bits(want), math.Float64bits(got),
		"want %v, got %v", want, got)
}

func boxPoints(halfWidth, halfHeight float64) []box2d.Vec2 {
	return []box2d.Vec2{
		{X: -halfWidth, Y: -halfHeight},
		{X: halfWidth, Y: -halfHeight},
		{X: halfWidth, Y: halfHeight},
		{X: -halfWidth, Y: halfHeight},
	}
}

func TestSegmentDistance(t *testing.T) {
	t.Parallel()

	t.Run("perpendicular", func(t *testing.T) {
		t.Parallel()
		r := box2d.SegmentDistance(
			box2d.Vec2{X: 0, Y: 0}, box2d.Vec2{X: 1, Y: 0},
			box2d.Vec2{X: 0.5, Y: 1}, box2d.Vec2{X: 0.5, Y: 2})
		requireBitEqual(t, 0.5, r.Fraction1)
		requireBitEqual(t, 0.0, r.Fraction2)
		require.Equal(t, box2d.Vec2{X: 0.5, Y: 0}, r.Closest1)
		require.Equal(t, box2d.Vec2{X: 0.5, Y: 1}, r.Closest2)
		requireBitEqual(t, 1.0, r.DistanceSquared)
	})

	t.Run("parallel", func(t *testing.T) {
		t.Parallel()
		r := box2d.SegmentDistance(
			box2d.Vec2{X: 0, Y: 0}, box2d.Vec2{X: 1, Y: 0},
			box2d.Vec2{X: 0, Y: 1}, box2d.Vec2{X: 1, Y: 1})
		requireBitEqual(t, 0.0, r.Fraction1)
		requireBitEqual(t, 0.0, r.Fraction2)
		requireBitEqual(t, 1.0, r.DistanceSquared)
	})

	t.Run("collinear separated", func(t *testing.T) {
		t.Parallel()
		r := box2d.SegmentDistance(
			box2d.Vec2{X: 0, Y: 0}, box2d.Vec2{X: 1, Y: 0},
			box2d.Vec2{X: 2, Y: 0}, box2d.Vec2{X: 3, Y: 0})
		requireBitEqual(t, 1.0, r.Fraction1)
		requireBitEqual(t, 0.0, r.Fraction2)
		require.Equal(t, box2d.Vec2{X: 1, Y: 0}, r.Closest1)
		require.Equal(t, box2d.Vec2{X: 2, Y: 0}, r.Closest2)
		requireBitEqual(t, 1.0, r.DistanceSquared)
	})

	t.Run("endpoint closest", func(t *testing.T) {
		t.Parallel()
		r := box2d.SegmentDistance(
			box2d.Vec2{X: 0, Y: 0}, box2d.Vec2{X: 1, Y: 0},
			box2d.Vec2{X: 2, Y: 1}, box2d.Vec2{X: 3, Y: 2})
		requireBitEqual(t, 1.0, r.Fraction1)
		requireBitEqual(t, 0.0, r.Fraction2)
		require.Equal(t, box2d.Vec2{X: 1, Y: 0}, r.Closest1)
		require.Equal(t, box2d.Vec2{X: 2, Y: 1}, r.Closest2)
		requireBitEqual(t, 2.0, r.DistanceSquared)
	})

	t.Run("both degenerate", func(t *testing.T) {
		t.Parallel()
		r := box2d.SegmentDistance(
			box2d.Vec2{X: 1, Y: 2}, box2d.Vec2{X: 1, Y: 2},
			box2d.Vec2{X: 4, Y: 6}, box2d.Vec2{X: 4, Y: 6})
		requireBitEqual(t, 0.0, r.Fraction1)
		requireBitEqual(t, 0.0, r.Fraction2)
		requireBitEqual(t, 25.0, r.DistanceSquared)
	})

	t.Run("segment2 degenerate", func(t *testing.T) {
		t.Parallel()
		r := box2d.SegmentDistance(
			box2d.Vec2{X: 0, Y: 0}, box2d.Vec2{X: 2, Y: 0},
			box2d.Vec2{X: 1, Y: 5}, box2d.Vec2{X: 1, Y: 5})
		requireBitEqual(t, 0.5, r.Fraction1)
		requireBitEqual(t, 0.0, r.Fraction2)
		require.Equal(t, box2d.Vec2{X: 1, Y: 0}, r.Closest1)
		requireBitEqual(t, 25.0, r.DistanceSquared)
	})
}

func TestShapeDistance(t *testing.T) {
	t.Parallel()

	t.Run("separated circles use radii", func(t *testing.T) {
		t.Parallel()
		input := box2d.DistanceInput{
			ProxyA:     box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0.5),
			ProxyB:     box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0.25),
			TransformA: box2d.TransformIdentity,
			TransformB: box2d.Transform{P: box2d.Vec2{X: 3, Y: 0}, Q: box2d.RotIdentity},
			UseRadii:   true,
		}
		var cache box2d.SimplexCache
		out := box2d.ShapeDistance(&input, &cache, nil)

		// distance = centerDist - r1 - r2 for point proxies with radii applied.
		requireBitEqual(t, 3.0-0.5-0.25, out.Distance)
		require.Equal(t, box2d.Vec2{X: 1, Y: 0}, out.Normal)
		require.Equal(t, box2d.Vec2{X: 0.5, Y: 0}, out.PointA)
		require.Equal(t, box2d.Vec2{X: 2.75, Y: 0}, out.PointB)
	})

	t.Run("box vs box face", func(t *testing.T) {
		t.Parallel()
		input := box2d.DistanceInput{
			ProxyA:     box2d.MakeProxy(boxPoints(0.5, 0.5), 4, 0.0),
			ProxyB:     box2d.MakeProxy(boxPoints(0.5, 0.5), 4, 0.0),
			TransformA: box2d.TransformIdentity,
			TransformB: box2d.Transform{P: box2d.Vec2{X: 2, Y: 0}, Q: box2d.RotIdentity},
			UseRadii:   false,
		}
		var cache box2d.SimplexCache
		out := box2d.ShapeDistance(&input, &cache, nil)

		requireBitEqual(t, 1.0, out.Distance)
		require.Equal(t, box2d.Vec2{X: 1, Y: 0}, out.Normal)
		requireBitEqual(t, 0.5, out.PointA.X)
		requireBitEqual(t, 1.5, out.PointB.X)
	})

	t.Run("overlapping boxes", func(t *testing.T) {
		t.Parallel()
		input := box2d.DistanceInput{
			ProxyA:     box2d.MakeProxy(boxPoints(0.5, 0.5), 4, 0.0),
			ProxyB:     box2d.MakeProxy(boxPoints(0.5, 0.5), 4, 0.0),
			TransformA: box2d.TransformIdentity,
			TransformB: box2d.Transform{P: box2d.Vec2{X: 0.5, Y: 0.25}, Q: box2d.RotIdentity},
			UseRadii:   false,
		}
		var cache box2d.SimplexCache
		out := box2d.ShapeDistance(&input, &cache, nil)

		requireBitEqual(t, 0.0, out.Distance)
	})

	t.Run("warm start is identical", func(t *testing.T) {
		t.Parallel()
		input := box2d.DistanceInput{
			ProxyA:     box2d.MakeProxy(boxPoints(0.5, 0.5), 4, 0.0),
			ProxyB:     box2d.MakeProxy(boxPoints(0.4, 0.3), 4, 0.0),
			TransformA: box2d.TransformIdentity,
			TransformB: box2d.Transform{P: box2d.Vec2{X: 1.7, Y: 0.6}, Q: box2d.MakeRot(0.3)},
			UseRadii:   false,
		}
		var cache box2d.SimplexCache
		cold := box2d.ShapeDistance(&input, &cache, nil)
		require.Positive(t, cold.Distance)

		warmCache := cache
		warm := box2d.ShapeDistance(&input, &warmCache, nil)

		requireBitEqual(t, cold.Distance, warm.Distance)
		require.Equal(t, cold.PointA, warm.PointA)
		require.Equal(t, cold.PointB, warm.PointB)
		require.Equal(t, cold.Normal, warm.Normal)
	})
}

func TestShapeCast(t *testing.T) {
	t.Parallel()

	t.Run("circle proxy into box", func(t *testing.T) {
		t.Parallel()
		input := box2d.ShapeCastPairInput{
			ProxyA:       box2d.MakeProxy(boxPoints(0.5, 0.5), 4, 0.0),
			ProxyB:       box2d.MakeProxy([]box2d.Vec2{{X: 2, Y: 0}}, 1, 0.25),
			TransformA:   box2d.TransformIdentity,
			TransformB:   box2d.TransformIdentity,
			TranslationB: box2d.Vec2{X: -2, Y: 0},
			MaxFraction:  1.0,
			CanEncroach:  false,
		}
		out := box2d.ShapeCast(&input)

		require.True(t, out.Hit)
		// Conservative advancement stops when the hull distance reaches
		// target = max(linearSlop, totalRadius - linearSlop) = 0.245.
		target := 0.25 - box2d.LinearSlop
		expected := (1.5 - target) / 2.0
		require.InDelta(t, expected, out.Fraction, 1e-9)
		require.Equal(t, box2d.Vec2{X: 1, Y: 0}, out.Normal)
		require.InDelta(t, 0.5, out.Point.X, 1e-12)
		require.InDelta(t, 0.0, out.Point.Y, 1e-12)
	})

	t.Run("miss receding", func(t *testing.T) {
		t.Parallel()
		input := box2d.ShapeCastPairInput{
			ProxyA:       box2d.MakeProxy(boxPoints(0.5, 0.5), 4, 0.0),
			ProxyB:       box2d.MakeProxy([]box2d.Vec2{{X: 2, Y: 0}}, 1, 0.25),
			TransformA:   box2d.TransformIdentity,
			TransformB:   box2d.TransformIdentity,
			TranslationB: box2d.Vec2{X: 2, Y: 0},
			MaxFraction:  1.0,
			CanEncroach:  false,
		}
		out := box2d.ShapeCast(&input)
		require.False(t, out.Hit)
	})

	t.Run("miss beyond max fraction", func(t *testing.T) {
		t.Parallel()
		input := box2d.ShapeCastPairInput{
			ProxyA:       box2d.MakeProxy(boxPoints(0.5, 0.5), 4, 0.0),
			ProxyB:       box2d.MakeProxy([]box2d.Vec2{{X: 5, Y: 0}}, 1, 0.25),
			TransformA:   box2d.TransformIdentity,
			TransformB:   box2d.TransformIdentity,
			TranslationB: box2d.Vec2{X: -2, Y: 0},
			MaxFraction:  1.0,
			CanEncroach:  false,
		}
		out := box2d.ShapeCast(&input)
		require.False(t, out.Hit)
	})

	t.Run("initial overlap", func(t *testing.T) {
		t.Parallel()
		input := box2d.ShapeCastPairInput{
			ProxyA:       box2d.MakeProxy(boxPoints(0.5, 0.5), 4, 0.0),
			ProxyB:       box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0.25),
			TransformA:   box2d.TransformIdentity,
			TransformB:   box2d.TransformIdentity,
			TranslationB: box2d.Vec2{X: -2, Y: 0},
			MaxFraction:  1.0,
			CanEncroach:  false,
		}
		out := box2d.ShapeCast(&input)
		require.True(t, out.Hit)
		requireBitEqual(t, 0.0, out.Fraction)
	})

	t.Run("can encroach", func(t *testing.T) {
		t.Parallel()
		input := box2d.ShapeCastPairInput{
			ProxyA:       box2d.MakeProxy(boxPoints(0.5, 0.5), 4, 0.0),
			ProxyB:       box2d.MakeProxy([]box2d.Vec2{{X: 0.74, Y: 0}}, 1, 0.25),
			TransformA:   box2d.TransformIdentity,
			TransformB:   box2d.TransformIdentity,
			TranslationB: box2d.Vec2{X: -2, Y: 0},
			MaxFraction:  1.0,
			CanEncroach:  true,
		}
		out := box2d.ShapeCast(&input)
		require.True(t, out.Hit)
		// Encroachment reduces the target to distance - linearSlop, so the cast
		// advances by linearSlop of separation: fraction ~= 0.005 / 2.
		require.InDelta(t, box2d.LinearSlop/2.0, out.Fraction, 1e-9)
	})
}

func TestTimeOfImpact(t *testing.T) {
	t.Parallel()

	staticSweep := box2d.Sweep{
		LocalCenter: box2d.Vec2{},
		C1:          box2d.Vec2{},
		C2:          box2d.Vec2{},
		Q1:          box2d.RotIdentity,
		Q2:          box2d.RotIdentity,
	}

	t.Run("head-on bullet vs static box", func(t *testing.T) {
		t.Parallel()
		input := box2d.TOIInput{
			ProxyA: box2d.MakeProxy(boxPoints(0.5, 0.5), 4, 0.0),
			ProxyB: box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0.0),
			SweepA: staticSweep,
			SweepB: box2d.Sweep{
				C1: box2d.Vec2{X: 2, Y: 0},
				C2: box2d.Vec2{X: -2, Y: 0},
				Q1: box2d.RotIdentity,
				Q2: box2d.RotIdentity,
			},
			MaxFraction: 1.0,
		}
		out := box2d.TimeOfImpact(&input)

		require.Equal(t, box2d.TOIStateHit, out.State)
		// Hit when the point reaches target = linearSlop from the face at
		// x = 0.5: t = (2 - 0.5 - 0.005) / 4. The root finder converges to a
		// bracket of tolerance = 0.25 * linearSlop in separation, i.e.
		// +-0.0003125 in time, so the analytic value cannot be matched
		// tighter than that.
		expected := (2.0 - 0.5 - box2d.LinearSlop) / 4.0
		require.InDelta(t, expected, out.Fraction, 1e-3)
	})

	t.Run("rotating translating boxes", func(t *testing.T) {
		t.Parallel()
		input := box2d.TOIInput{
			ProxyA: box2d.MakeProxy(boxPoints(2.0, 0.1), 4, 0.0),
			ProxyB: box2d.MakeProxy(boxPoints(0.25, 0.05), 4, 0.0),
			SweepA: staticSweep,
			SweepB: box2d.Sweep{
				C1: box2d.Vec2{X: -0.5, Y: 1.0},
				C2: box2d.Vec2{X: 0.5, Y: 0.1},
				Q1: box2d.RotIdentity,
				Q2: box2d.MakeRot(1.5),
			},
			MaxFraction: 1.0,
		}
		out := box2d.TimeOfImpact(&input)

		require.Equal(t, box2d.TOIStateHit, out.State)
		require.Greater(t, out.Fraction, 0.0)
		require.Less(t, out.Fraction, 1.0)
	})

	t.Run("no collision", func(t *testing.T) {
		t.Parallel()
		input := box2d.TOIInput{
			ProxyA: box2d.MakeProxy(boxPoints(0.5, 0.5), 4, 0.0),
			ProxyB: box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0.0),
			SweepA: staticSweep,
			SweepB: box2d.Sweep{
				C1: box2d.Vec2{X: 2, Y: 2},
				C2: box2d.Vec2{X: -2, Y: 2},
				Q1: box2d.RotIdentity,
				Q2: box2d.RotIdentity,
			},
			MaxFraction: 1.0,
		}
		out := box2d.TimeOfImpact(&input)

		require.Equal(t, box2d.TOIStateSeparated, out.State)
		requireBitEqual(t, 1.0, out.Fraction)
	})

	t.Run("touching start", func(t *testing.T) {
		t.Parallel()
		input := box2d.TOIInput{
			ProxyA: box2d.MakeProxy(boxPoints(0.5, 0.5), 4, 0.0),
			ProxyB: box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0.0),
			SweepA: staticSweep,
			SweepB: box2d.Sweep{
				C1: box2d.Vec2{X: 0.504, Y: 0},
				C2: box2d.Vec2{X: -2, Y: 0},
				Q1: box2d.RotIdentity,
				Q2: box2d.RotIdentity,
			},
			MaxFraction: 1.0,
		}
		out := box2d.TimeOfImpact(&input)

		require.Equal(t, box2d.TOIStateHit, out.State)
		requireBitEqual(t, 0.0, out.Fraction)
	})

	t.Run("overlapped start", func(t *testing.T) {
		t.Parallel()
		input := box2d.TOIInput{
			ProxyA: box2d.MakeProxy(boxPoints(0.5, 0.5), 4, 0.0),
			ProxyB: box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0.0),
			SweepA: staticSweep,
			SweepB: box2d.Sweep{
				C1: box2d.Vec2{X: 0, Y: 0},
				C2: box2d.Vec2{X: -2, Y: 0},
				Q1: box2d.RotIdentity,
				Q2: box2d.RotIdentity,
			},
			MaxFraction: 1.0,
		}
		out := box2d.TimeOfImpact(&input)

		require.Equal(t, box2d.TOIStateOverlapped, out.State)
		requireBitEqual(t, 0.0, out.Fraction)
	})
}

func TestGetSweepTransform(t *testing.T) {
	t.Parallel()

	sweep := box2d.Sweep{
		LocalCenter: box2d.Vec2{X: 0.5, Y: 0},
		C1:          box2d.Vec2{X: 1, Y: 1},
		C2:          box2d.Vec2{X: 3, Y: 5},
		Q1:          box2d.RotIdentity,
		Q2:          box2d.MakeRot(box2d.Pi / 2.0),
	}

	t.Run("alpha 0", func(t *testing.T) {
		t.Parallel()
		xf := box2d.GetSweepTransform(&sweep, 0.0)
		require.Equal(t, box2d.RotIdentity, xf.Q)
		require.Equal(t, box2d.Vec2{X: 0.5, Y: 1}, xf.P)
	})

	t.Run("alpha 1", func(t *testing.T) {
		t.Parallel()
		xf := box2d.GetSweepTransform(&sweep, 1.0)
		// q2 rotates localCenter (0.5, 0) to approximately (0, 0.5).
		require.InDelta(t, 0.0, box2d.RelativeAngle(sweep.Q2, xf.Q), 1e-6)
		require.InDelta(t, 3.0, xf.P.X, 1e-6)
		require.InDelta(t, 4.5, xf.P.Y, 1e-6)
	})

	t.Run("alpha 0.5", func(t *testing.T) {
		t.Parallel()
		xf := box2d.GetSweepTransform(&sweep, 0.5)
		require.True(t, box2d.IsNormalizedRot(xf.Q))
		// Midpoint rotation is 45 degrees; center lerp is (2, 3).
		const cos45 = 0.7071067811865476
		require.InDelta(t, 2.0-cos45*0.5, xf.P.X, 1e-6)
		require.InDelta(t, 3.0-cos45*0.5, xf.P.Y, 1e-6)
	})
}

func TestPointInPolygon(t *testing.T) {
	t.Parallel()

	square := box2d.MakeSquare(0.5)
	rounded := box2d.MakeRoundedBox(0.5, 0.5, 0.1)

	t.Run("sharp box", func(t *testing.T) {
		t.Parallel()
		require.True(t, box2d.PointInPolygon(&square, box2d.Vec2{X: 0, Y: 0}))
		require.True(t, box2d.PointInPolygon(&square, box2d.Vec2{X: 0.49, Y: 0.49}))
		// Exactly on the edge: distance zero.
		require.True(t, box2d.PointInPolygon(&square, box2d.Vec2{X: 0.5, Y: 0}))
		require.False(t, box2d.PointInPolygon(&square, box2d.Vec2{X: 0.51, Y: 0}))
		require.False(t, box2d.PointInPolygon(&square, box2d.Vec2{X: 0, Y: 2}))
	})

	t.Run("rounded box", func(t *testing.T) {
		t.Parallel()
		require.True(t, box2d.PointInPolygon(&rounded, box2d.Vec2{X: 0, Y: 0}))
		// Inside the radius band around the core hull.
		require.True(t, box2d.PointInPolygon(&rounded, box2d.Vec2{X: 0.55, Y: 0}))
		require.False(t, box2d.PointInPolygon(&rounded, box2d.Vec2{X: 0.65, Y: 0}))
	})
}

func TestShapeCastPolygonRayConsistency(t *testing.T) {
	t.Parallel()

	t.Run("rounded polygon matches ray cast exactly", func(t *testing.T) {
		t.Parallel()
		rounded := box2d.MakeRoundedBox(0.5, 0.5, 0.25)
		ray := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 2, Y: 0.1},
			Translation: box2d.Vec2{X: -2, Y: 0},
			MaxFraction: 1.0,
		}
		rc := box2d.RayCastPolygon(&rounded, &ray)

		scInput := box2d.ShapeCastInput{
			Proxy:       box2d.MakeProxy([]box2d.Vec2{ray.Origin}, 1, 0.0),
			Translation: ray.Translation,
			MaxFraction: ray.MaxFraction,
			CanEncroach: false,
		}
		sc := box2d.ShapeCastPolygon(&rounded, &scInput)

		// The rounded ray-cast branch delegates to the same shape cast, so
		// the outputs must be bit-identical.
		require.Equal(t, sc, rc)
		require.True(t, rc.Hit)
	})

	t.Run("zero radius point cast fraction", func(t *testing.T) {
		t.Parallel()
		square := box2d.MakeSquare(0.5)
		ray := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 2, Y: 0},
			Translation: box2d.Vec2{X: -2, Y: 0},
			MaxFraction: 1.0,
		}
		rc := box2d.RayCastPolygon(&square, &ray)
		require.True(t, rc.Hit)
		requireBitEqual(t, 0.75, rc.Fraction)

		scInput := box2d.ShapeCastInput{
			Proxy:       box2d.MakeProxy([]box2d.Vec2{ray.Origin}, 1, 0.0),
			Translation: ray.Translation,
			MaxFraction: ray.MaxFraction,
			CanEncroach: false,
		}
		sc := box2d.ShapeCastPolygon(&square, &scInput)
		require.True(t, sc.Hit)
		// The shape cast stops linearSlop short of the surface:
		// fraction = rayFraction - linearSlop / |translation|.
		require.InDelta(t, rc.Fraction-float64(box2d.LinearSlop/2.0), sc.Fraction, 1e-9)
	})

	t.Run("rounded ray cast regression", func(t *testing.T) {
		t.Parallel()
		// Regression for the removed E2 assert+miss fallback: a rounded
		// polygon ray cast must hit via the shape-cast path.
		rounded := box2d.MakeRoundedBox(0.5, 0.5, 0.25)
		ray := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 2, Y: 0},
			Translation: box2d.Vec2{X: -2, Y: 0},
			MaxFraction: 1.0,
		}
		out := box2d.RayCastPolygon(&rounded, &ray)

		require.True(t, out.Hit)
		target := 0.25 - box2d.LinearSlop
		expected := (1.5 - target) / 2.0
		require.InDelta(t, expected, out.Fraction, 1e-9)
		require.Equal(t, box2d.Vec2{X: 1, Y: 0}, out.Normal)
	})
}

func TestShapeCastShapesAgainstProxy(t *testing.T) {
	t.Parallel()

	proxy := box2d.MakeProxy([]box2d.Vec2{{X: 2, Y: 0}}, 1, 0.0)
	input := box2d.ShapeCastInput{
		Proxy:       proxy,
		Translation: box2d.Vec2{X: -2, Y: 0},
		MaxFraction: 1.0,
		CanEncroach: false,
	}

	t.Run("circle", func(t *testing.T) {
		t.Parallel()
		circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.5}
		out := box2d.ShapeCastCircle(&circle, &input)
		require.True(t, out.Hit)
		// Hull distance 2.0, target = 0.5 - linearSlop.
		require.InDelta(t, (2.0-(0.5-box2d.LinearSlop))/2.0, out.Fraction, 1e-9)
	})

	t.Run("capsule", func(t *testing.T) {
		t.Parallel()
		capsule := box2d.Capsule{
			Center1: box2d.Vec2{X: 0, Y: -0.5},
			Center2: box2d.Vec2{X: 0, Y: 0.5},
			Radius:  0.5,
		}
		out := box2d.ShapeCastCapsule(&capsule, &input)
		require.True(t, out.Hit)
		require.InDelta(t, (2.0-(0.5-box2d.LinearSlop))/2.0, out.Fraction, 1e-9)
	})

	t.Run("segment", func(t *testing.T) {
		t.Parallel()
		segment := box2d.Segment{
			Point1: box2d.Vec2{X: 0, Y: -1},
			Point2: box2d.Vec2{X: 0, Y: 1},
		}
		out := box2d.ShapeCastSegment(&segment, &input)
		require.True(t, out.Hit)
		require.InDelta(t, (2.0-box2d.LinearSlop)/2.0, out.Fraction, 1e-9)
	})

	t.Run("polygon", func(t *testing.T) {
		t.Parallel()
		square := box2d.MakeSquare(0.5)
		out := box2d.ShapeCastPolygon(&square, &input)
		require.True(t, out.Hit)
		require.InDelta(t, (1.5-box2d.LinearSlop)/2.0, out.Fraction, 1e-9)
	})
}

func TestCollideMover(t *testing.T) {
	t.Parallel()

	mover := box2d.Capsule{
		Center1: box2d.Vec2{X: 0, Y: 0.3},
		Center2: box2d.Vec2{X: 0, Y: 0.9},
		Radius:  0.3,
	}

	t.Run("circle hit", func(t *testing.T) {
		t.Parallel()
		circle := box2d.Circle{Center: box2d.Vec2{X: 0, Y: -0.1}, Radius: 0.2}
		result := box2d.CollideMoverAndCircle(&mover, &circle)
		require.True(t, result.Hit)
		// Hull distance = 0.4, total radius = 0.5: separation deficit 0.1.
		require.InDelta(t, 0.1, result.Plane.Offset, 1e-12)
	})

	t.Run("circle miss", func(t *testing.T) {
		t.Parallel()
		circle := box2d.Circle{Center: box2d.Vec2{X: 0, Y: -1.0}, Radius: 0.2}
		result := box2d.CollideMoverAndCircle(&mover, &circle)
		require.False(t, result.Hit)
	})

	t.Run("capsule hit", func(t *testing.T) {
		t.Parallel()
		capsule := box2d.Capsule{
			Center1: box2d.Vec2{X: -1, Y: -0.1},
			Center2: box2d.Vec2{X: 1, Y: -0.1},
			Radius:  0.2,
		}
		result := box2d.CollideMoverAndCapsule(&mover, &capsule)
		require.True(t, result.Hit)
		require.InDelta(t, 0.1, result.Plane.Offset, 1e-12)
	})

	t.Run("polygon hit", func(t *testing.T) {
		t.Parallel()
		// Floor top edge at y = 0.05; mover bottom hull point at y = 0.3.
		// Hull distance 0.25 versus mover radius 0.3: deficit 0.05.
		floor := box2d.MakeOffsetBox(2.0, 0.1, box2d.Vec2{X: 0, Y: -0.05}, box2d.RotIdentity)
		result := box2d.CollideMoverAndPolygon(&mover, &floor)
		require.True(t, result.Hit)
		require.InDelta(t, 0.05, result.Plane.Offset, 1e-12)
	})

	t.Run("polygon miss", func(t *testing.T) {
		t.Parallel()
		floor := box2d.MakeOffsetBox(2.0, 0.1, box2d.Vec2{X: 0, Y: -0.5}, box2d.RotIdentity)
		result := box2d.CollideMoverAndPolygon(&mover, &floor)
		require.False(t, result.Hit)
	})

	t.Run("segment hit", func(t *testing.T) {
		t.Parallel()
		segment := box2d.Segment{
			Point1: box2d.Vec2{X: -1, Y: 0.1},
			Point2: box2d.Vec2{X: 1, Y: 0.1},
		}
		result := box2d.CollideMoverAndSegment(&mover, &segment)
		require.True(t, result.Hit)
		// Hull distance 0.2 versus mover radius 0.3.
		require.InDelta(t, 0.1, result.Plane.Offset, 1e-12)
	})
}

// TestDistanceDeterminism runs the golden pseudo-fixed input table twice and
// requires bitwise-identical outputs (see golden_distance_test.go for the
// cross-architecture golden file).
func TestDistanceDeterminism(t *testing.T) {
	t.Parallel()

	first := computeGoldenDistance()
	second := computeGoldenDistance()
	require.Equal(t, first, second)
}
