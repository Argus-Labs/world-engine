// Oracle-based tests for the collision layer (aabb.go, geometry.go, hull.go,
// distance.go, manifold.go, collision.go and the chain-segment contact
// registers in contact.go).
//
// Every expected value is derived ONLY from the vendored C source of truth
// (scratchpad vendored-c/box2d/src/{aabb,geometry,hull,distance,manifold}.c)
// or ported from the upstream unit tests (test_collision.c, test_distance.c,
// test_shape.c). No expectation was produced by executing the Go code: values
// are hand-computed by following the C algorithm in exact arithmetic, and each
// nontrivial expectation cites the C function (file:line) or upstream test it
// comes from.
//
// Tolerances: the C code runs in float32 while this port runs in float64.
// Upstream ENSURE_SMALL(x, FLT_EPSILON) assertions are ported with 1e-6
// (looser than FLT_EPSILON ~ 1.19e-7 to absorb float32->float64 differences);
// hand-derived exact-arithmetic expectations use 1e-9..1e-12 because the
// float64 port evaluates the same expressions with at most a few ULP of noise.
//
// Vendored-vs-upstream drift observed while porting (vendored C wins):
//   - test_collision.c LargeWorldManifoldTest / LargeWorldAABBTest use the
//     upstream-main b2WorldTransform / b2LocalManifold / b2ComputeFatShapeAABB
//     API and a BOX2D_DOUBLE_PRECISION build flag; none of these exist in the
//     vendored v3.2.0 sources. The origin-frame manifold assertions and the
//     rounded-box AABB extents are ported against the vendored b2Transform
//     API; the double-precision-only blocks are ported as far-from-origin
//     consistency checks, which the float64 port satisfies by construction.
package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// invSqrt2 is 1/sqrt(2), used by several hand-derived diagonal normals.
const invSqrt2 = 0.70710678118654752

// ---------------------------------------------------------------------------
// aabb.go
// ---------------------------------------------------------------------------

// TestOracleAABBValidity ports AABBTest from upstream test_collision.c
// (b2IsValidAABB, b2AABB_Overlaps, b2AABB_Contains; vendored aabb.c:11 and
// math_functions.h helpers).
func TestOracleAABBValidity(t *testing.T) {
	t.Parallel()

	a := box2d.AABB{
		LowerBound: box2d.Vec2{X: -1, Y: -1},
		UpperBound: box2d.Vec2{X: -2, Y: -2},
	}
	require.False(t, box2d.IsValidAABB(a))

	a.UpperBound = box2d.Vec2{X: 1, Y: 1}
	require.True(t, box2d.IsValidAABB(a))

	b := box2d.AABB{
		LowerBound: box2d.Vec2{X: 2, Y: 2},
		UpperBound: box2d.Vec2{X: 4, Y: 4},
	}
	require.False(t, box2d.AABBOverlaps(a, b))
	require.False(t, box2d.AABBContains(a, b))

	// NaN bounds are invalid (vendored aabb.c:11 checks b2IsValidVec2).
	nan := math.NaN()
	require.False(t, box2d.IsValidAABB(box2d.AABB{
		LowerBound: box2d.Vec2{X: nan, Y: 0},
		UpperBound: box2d.Vec2{X: 1, Y: 1},
	}))
}

// TestOracleAABBRayCast ports the 14 cases of AABBRayCastTest from upstream
// test_collision.c against vendored aabb.c b2AABB_RayCast (aabb.c:19). All
// fractions are exact geometry: e.g. a ray from x=-3 to x=3 crosses x=-1 at
// t = 2/6 = 1/3.
func TestOracleAABBRayCast(t *testing.T) {
	t.Parallel()

	aabb := box2d.AABB{
		LowerBound: box2d.Vec2{X: -1, Y: -1},
		UpperBound: box2d.Vec2{X: 1, Y: 1},
	}

	cases := []struct {
		name     string
		box      box2d.AABB
		p1, p2   box2d.Vec2
		hit      bool
		fraction float64
		normal   box2d.Vec2
		point    box2d.Vec2
		hasPoint bool
	}{
		{
			name: "hit from left", box: aabb,
			p1: box2d.Vec2{X: -3, Y: 0}, p2: box2d.Vec2{X: 3, Y: 0},
			hit: true, fraction: 1.0 / 3.0, normal: box2d.Vec2{X: -1, Y: 0},
			point: box2d.Vec2{X: -1, Y: 0}, hasPoint: true,
		},
		{
			name: "hit from right", box: aabb,
			p1: box2d.Vec2{X: 3, Y: 0}, p2: box2d.Vec2{X: -3, Y: 0},
			hit: true, fraction: 1.0 / 3.0, normal: box2d.Vec2{X: 1, Y: 0},
			point: box2d.Vec2{X: 1, Y: 0}, hasPoint: true,
		},
		{
			name: "hit from bottom", box: aabb,
			p1: box2d.Vec2{X: 0, Y: -3}, p2: box2d.Vec2{X: 0, Y: 3},
			hit: true, fraction: 1.0 / 3.0, normal: box2d.Vec2{X: 0, Y: -1},
			point: box2d.Vec2{X: 0, Y: -1}, hasPoint: true,
		},
		{
			name: "hit from top", box: aabb,
			p1: box2d.Vec2{X: 0, Y: 3}, p2: box2d.Vec2{X: 0, Y: -3},
			hit: true, fraction: 1.0 / 3.0, normal: box2d.Vec2{X: 0, Y: 1},
			point: box2d.Vec2{X: 0, Y: 1}, hasPoint: true,
		},
		{
			name: "miss parallel to x axis", box: aabb,
			p1: box2d.Vec2{X: -3, Y: 2}, p2: box2d.Vec2{X: 3, Y: 2},
		},
		{
			name: "miss parallel to y axis", box: aabb,
			p1: box2d.Vec2{X: 2, Y: -3}, p2: box2d.Vec2{X: 2, Y: 3},
		},
		{
			name: "start inside is a miss", box: aabb,
			p1: box2d.Vec2{X: 0, Y: 0}, p2: box2d.Vec2{X: 2, Y: 0},
		},
		{
			name: "diagonal corner hit", box: aabb,
			p1: box2d.Vec2{X: -2, Y: -2}, p2: box2d.Vec2{X: 2, Y: 2},
			hit: true, fraction: 0.25,
			// Normal is (-1,0) or (0,-1); the vendored loop processes x
			// first and y overwrites on strict >, so the tie keeps x.
			normal: box2d.Vec2{X: -1, Y: 0},
		},
		{
			name: "parallel outside edge misses", box: aabb,
			p1: box2d.Vec2{X: -2, Y: 1.5}, p2: box2d.Vec2{X: 2, Y: 1.5},
		},
		{
			name: "parallel exactly on boundary hits", box: aabb,
			p1: box2d.Vec2{X: -2, Y: 1}, p2: box2d.Vec2{X: 2, Y: 1},
			hit: true, fraction: 0.25, normal: box2d.Vec2{X: -1, Y: 0},
		},
		{
			name: "short ray misses", box: aabb,
			p1: box2d.Vec2{X: -3, Y: 0}, p2: box2d.Vec2{X: -2.5, Y: 0},
		},
		{
			name: "zero length ray misses", box: aabb,
			p1: box2d.Vec2{X: 0, Y: 0}, p2: box2d.Vec2{X: 0, Y: 0},
		},
		{
			name: "hit exactly at t equals one", box: aabb,
			p1: box2d.Vec2{X: -2, Y: 0}, p2: box2d.Vec2{X: -1, Y: 0},
			hit: true, fraction: 1.0, normal: box2d.Vec2{X: -1, Y: 0},
		},
		{
			// Enters the x slab at t=1/3 and leaves it at t=2/3, but only
			// reaches y=1 at t=5/6: tmin > tmax in the y block (aabb.c:19).
			name: "diagonal slab miss", box: aabb,
			p1: box2d.Vec2{X: -3, Y: 6}, p2: box2d.Vec2{X: 3, Y: 0},
		},
		{
			name: "offset aabb",
			box: box2d.AABB{
				LowerBound: box2d.Vec2{X: 2, Y: 3},
				UpperBound: box2d.Vec2{X: 4, Y: 5},
			},
			p1: box2d.Vec2{X: 0, Y: 4}, p2: box2d.Vec2{X: 6, Y: 4},
			hit: true, fraction: 1.0 / 3.0, normal: box2d.Vec2{X: -1, Y: 0},
			point: box2d.Vec2{X: 2, Y: 4}, hasPoint: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := box2d.AABBRayCast(tc.box, tc.p1, tc.p2)
			require.Equal(t, tc.hit, out.Hit)
			if !tc.hit {
				return
			}
			require.InDelta(t, tc.fraction, out.Fraction, 1e-6)
			requireVec(t, tc.normal, out.Normal, 1e-6)
			if tc.hasPoint {
				requireVec(t, tc.point, out.Point, 1e-6)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// geometry.go: mass properties (upstream ShapeMassTest in test_shape.c,
// vendored geometry.c:220/234/273)
// ---------------------------------------------------------------------------

func TestOracleShapeMass(t *testing.T) {
	t.Parallel()

	t.Run("circle", func(t *testing.T) {
		t.Parallel()
		// Upstream ShapeMassTest: circle {(1,0), r=1}, density 1:
		// mass = rho*pi*r^2 = pi, center = (1,0), I = mass*0.5*r^2 = pi/2
		// (geometry.c:220).
		circle := box2d.Circle{Center: box2d.Vec2{X: 1, Y: 0}, Radius: 1}
		md := box2d.ComputeCircleMass(&circle, 1.0)
		require.InDelta(t, math.Pi, md.Mass, 1e-9)
		requireVec(t, box2d.Vec2{X: 1, Y: 0}, md.Center, 0)
		require.InDelta(t, 0.5*math.Pi, md.RotationalInertia, 1e-9)
	})

	t.Run("capsule analytic", func(t *testing.T) {
		t.Parallel()
		// Capsule {(-1,0),(1,0), r=1}, density 1. Per geometry.c:234:
		//   circleMass = pi*r^2 = pi, boxMass = 2*r*len = 4
		//   lc = 4r/(3pi), h = len/2 = 1
		//   circleInertia = circleMass*(0.5 r^2 + h^2 + 2 h lc)
		//                 = pi*(1.5 + 8/(3pi)) = 1.5pi + 8/3
		//   boxInertia = boxMass*(4 r^2 + len^2)/12 = 4*8/12 = 8/3
		//   I = 1.5pi + 16/3
		capsule := box2d.Capsule{
			Center1: box2d.Vec2{X: -1, Y: 0},
			Center2: box2d.Vec2{X: 1, Y: 0},
			Radius:  1,
		}
		md := box2d.ComputeCapsuleMass(&capsule, 1.0)
		require.InDelta(t, math.Pi+4.0, md.Mass, 1e-9)
		requireVec(t, box2d.Vec2{}, md.Center, 1e-12)
		require.InDelta(t, 1.5*math.Pi+16.0/3.0, md.RotationalInertia, 1e-9)
	})

	t.Run("capsule bounded by hull and box", func(t *testing.T) {
		t.Parallel()
		// Upstream ShapeMassTest middle block: the capsule mass and inertia
		// are strictly between an inscribed 8-gon hull approximation and the
		// circumscribing box. This is a mathematical (strict) inequality of
		// the C formulas, not a numeric constant.
		capsule := box2d.Capsule{
			Center1: box2d.Vec2{X: -1, Y: 0},
			Center2: box2d.Vec2{X: 1, Y: 0},
			Radius:  1,
		}
		radius := capsule.Radius
		length := 2.0
		md := box2d.ComputeCapsuleMass(&capsule, 1.0)

		r := box2d.MakeBox(radius+0.5*length, radius)
		mdUpper := box2d.ComputePolygonMass(&r, 1.0)

		const n = 4
		var points []box2d.Vec2
		d := math.Pi / (n - 1.0)
		angle := -0.5 * math.Pi
		for range n {
			// float64(...) forces product rounding so the package's no-FMA
			// guard (nofma_test.go) stays clean in this test binary too.
			points = append(points, box2d.Vec2{
				X: 1.0 + float64(radius*math.Cos(angle)),
				Y: radius * math.Sin(angle),
			})
			angle += d
		}
		angle = 0.5 * math.Pi
		for range n {
			points = append(points, box2d.Vec2{
				X: -1.0 + float64(radius*math.Cos(angle)),
				Y: radius * math.Sin(angle),
			})
			angle += d
		}
		hull := box2d.ComputeHull(points)
		ac := box2d.MakePolygon(&hull, 0.0)
		mdLower := box2d.ComputePolygonMass(&ac, 1.0)

		require.Less(t, mdLower.Mass, md.Mass)
		require.Less(t, md.Mass, mdUpper.Mass)
		require.Less(t, mdLower.RotationalInertia, md.RotationalInertia)
		require.Less(t, md.RotationalInertia, mdUpper.RotationalInertia)
	})

	t.Run("box", func(t *testing.T) {
		t.Parallel()
		// Upstream ShapeMassTest: 2x2 box, density 1: mass 4, center (0,0),
		// I = m*(w^2+h^2)/12 = 4*8/12 = 8/3 (geometry.c:273).
		b := box2d.MakeBox(1, 1)
		md := box2d.ComputePolygonMass(&b, 1.0)
		require.InDelta(t, 4.0, md.Mass, 1e-9)
		requireVec(t, box2d.Vec2{}, md.Center, 1e-12)
		require.InDelta(t, 8.0/3.0, md.RotationalInertia, 1e-9)
	})

	t.Run("offset box matches centered box", func(t *testing.T) {
		t.Parallel()
		// Upstream ShapeMassTest last block: translating a box moves the
		// centroid but not the mass or the about-centroid inertia.
		offset := box2d.Vec2{X: 0.4, Y: -0.7}
		b1 := box2d.MakeBox(0.25, 0.5)
		b2 := box2d.MakeOffsetBox(0.25, 0.5, offset, box2d.RotIdentity)

		m1 := box2d.ComputePolygonMass(&b1, 1.0)
		m2 := box2d.ComputePolygonMass(&b2, 1.0)
		require.InDelta(t, m1.Mass, m2.Mass, 1e-9)
		require.InDelta(t, m1.RotationalInertia, m2.RotationalInertia, 1e-9)
		requireVec(t, offset, m2.Center, 1e-12)
	})

	t.Run("rounded box pushes vertices out by sqrt2 radius", func(t *testing.T) {
		t.Parallel()
		// geometry.c:273 (b2ComputePolygonMass, radius > 0 branch): each
		// vertex moves by sqrt2*radius along the normalized corner-normal
		// sum, with the literal constant sqrt2 = 1.412 (geometry.c:325).
		// For a square of half width w the corner mid-normal is
		// (+-1/sqrt2, +-1/sqrt2), so the result is a square of half width
		//   a = w + 1.412*radius/sqrt(2)
		// giving mass = 4a^2 and I = mass*8a^2/12 = (2/3)*mass*a^2.
		// The C constant is 1.412f (~1.41199994); using 1.412 exactly is
		// within the 1e-6 relative tolerance below.
		const w, radius = 0.5, 0.25
		rb := box2d.MakeRoundedBox(w, w, radius)
		md := box2d.ComputePolygonMass(&rb, 1.0)

		a := w + 1.412*radius*invSqrt2
		wantMass := 4.0 * a * a
		wantInertia := wantMass * (2.0 / 3.0) * a * a
		require.InDelta(t, wantMass, md.Mass, 1e-6*wantMass)
		requireVec(t, box2d.Vec2{}, md.Center, 1e-12)
		require.InDelta(t, wantInertia, md.RotationalInertia, 1e-6*wantInertia)
	})

	t.Run("density scales linearly", func(t *testing.T) {
		t.Parallel()
		// Every mass formula in geometry.c is linear in rho.
		b := box2d.MakeBox(1, 1)
		md := box2d.ComputePolygonMass(&b, 2.5)
		require.InDelta(t, 10.0, md.Mass, 1e-9)
		require.InDelta(t, 2.5*8.0/3.0, md.RotationalInertia, 1e-9)
	})
}

// ---------------------------------------------------------------------------
// geometry.go: AABBs (upstream ShapeAABBTest in test_shape.c)
// ---------------------------------------------------------------------------

func TestOracleShapeAABBs(t *testing.T) {
	t.Parallel()

	t.Run("circle identity", func(t *testing.T) {
		t.Parallel()
		// Upstream ShapeAABBTest: circle {(1,0), r=1} -> [0,-1]x[2,1].
		circle := box2d.Circle{Center: box2d.Vec2{X: 1, Y: 0}, Radius: 1}
		b := box2d.ComputeCircleAABB(&circle, box2d.TransformIdentity)
		requireVec(t, box2d.Vec2{X: 0, Y: -1}, b.LowerBound, 1e-12)
		requireVec(t, box2d.Vec2{X: 2, Y: 1}, b.UpperBound, 1e-12)
	})

	t.Run("circle transformed", func(t *testing.T) {
		t.Parallel()
		// geometry.c b2ComputeCircleAABB: center maps through the transform,
		// bounds are center +- r. Rot 90deg maps (1,0)->(0,1); plus (2,3)
		// gives center (2,4) and bounds [1,3]x[3,5].
		circle := box2d.Circle{Center: box2d.Vec2{X: 1, Y: 0}, Radius: 1}
		b := box2d.ComputeCircleAABB(&circle, xf(2, 3, 0.5*math.Pi))
		requireVec(t, box2d.Vec2{X: 1, Y: 3}, b.LowerBound, 1e-12)
		requireVec(t, box2d.Vec2{X: 3, Y: 5}, b.UpperBound, 1e-12)
	})

	t.Run("capsule", func(t *testing.T) {
		t.Parallel()
		// geometry.c b2ComputeCapsuleAABB: min/max of the two centers
		// extended by r: capsule {(-1,0),(1,0), r=1} -> [-2,-1]x[2,1].
		capsule := box2d.Capsule{
			Center1: box2d.Vec2{X: -1, Y: 0},
			Center2: box2d.Vec2{X: 1, Y: 0},
			Radius:  1,
		}
		b := box2d.ComputeCapsuleAABB(&capsule, box2d.TransformIdentity)
		requireVec(t, box2d.Vec2{X: -2, Y: -1}, b.LowerBound, 1e-12)
		requireVec(t, box2d.Vec2{X: 2, Y: 1}, b.UpperBound, 1e-12)
	})

	t.Run("polygon", func(t *testing.T) {
		t.Parallel()
		// Upstream ShapeAABBTest: 2x2 box -> [-1,-1]x[1,1].
		b := box2d.MakeBox(1, 1)
		got := box2d.ComputePolygonAABB(&b, box2d.TransformIdentity)
		requireVec(t, box2d.Vec2{X: -1, Y: -1}, got.LowerBound, 1e-12)
		requireVec(t, box2d.Vec2{X: 1, Y: 1}, got.UpperBound, 1e-12)
	})

	t.Run("segment", func(t *testing.T) {
		t.Parallel()
		// Upstream ShapeAABBTest: segment {(0,1),(0,-1)} -> [0,-1]x[0,1].
		segment := box2d.Segment{
			Point1: box2d.Vec2{X: 0, Y: 1},
			Point2: box2d.Vec2{X: 0, Y: -1},
		}
		got := box2d.ComputeSegmentAABB(&segment, box2d.TransformIdentity)
		requireVec(t, box2d.Vec2{X: 0, Y: -1}, got.LowerBound, 1e-12)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, got.UpperBound, 1e-12)
	})

	t.Run("rounded box extents", func(t *testing.T) {
		t.Parallel()
		// Upstream LargeWorldAABBTest (origin part): rounded box with half
		// extents 0.5 and radius 0.1 has tight extent 0.6 each way.
		// NOTE upstream drift: the far-from-origin half of that test needs
		// the upstream-main BOX2D_DOUBLE_PRECISION build and
		// b2ComputeFatShapeAABB, which the vendored v3.2.0 C does not have.
		// This float64 port keeps the extents exactly at 1e7 as well, which
		// is what the C algorithm produces in exact arithmetic.
		rb := box2d.MakeRoundedBox(0.5, 0.5, 0.1)
		got := box2d.ComputePolygonAABB(&rb, box2d.TransformIdentity)
		requireVec(t, box2d.Vec2{X: -0.6, Y: -0.6}, got.LowerBound, 1e-6)
		requireVec(t, box2d.Vec2{X: 0.6, Y: 0.6}, got.UpperBound, 1e-6)

		const d = 1.0e7
		far := box2d.ComputePolygonAABB(&rb, xf(d, d, 0))
		require.LessOrEqual(t, far.LowerBound.X, d-0.6+1e-6)
		require.LessOrEqual(t, far.LowerBound.Y, d-0.6+1e-6)
		require.GreaterOrEqual(t, far.UpperBound.X, d+0.6-1e-6)
		require.GreaterOrEqual(t, far.UpperBound.Y, d+0.6-1e-6)
	})
}

// ---------------------------------------------------------------------------
// geometry.go: point containment (upstream PointInShapeTest in test_shape.c)
// ---------------------------------------------------------------------------

func TestOraclePointInShapes(t *testing.T) {
	t.Parallel()

	p1 := box2d.Vec2{X: 0.5, Y: 0.5}
	p2 := box2d.Vec2{X: 4, Y: -4}

	t.Run("circle", func(t *testing.T) {
		t.Parallel()
		circle := box2d.Circle{Center: box2d.Vec2{X: 1, Y: 0}, Radius: 1}
		require.True(t, box2d.PointInCircle(&circle, p1))
		require.False(t, box2d.PointInCircle(&circle, p2))
	})

	t.Run("polygon", func(t *testing.T) {
		t.Parallel()
		b := box2d.MakeBox(1, 1)
		require.True(t, box2d.PointInPolygon(&b, p1))
		require.False(t, box2d.PointInPolygon(&b, p2))
	})

	t.Run("capsule interior and caps", func(t *testing.T) {
		t.Parallel()
		// geometry.c b2PointInCapsule: distance from the point to the
		// clamped segment closest point vs radius. (0,0.4) is 0.4 from the
		// axis (inside, r=0.5); (1.6,0) is 0.6 from center2 (outside).
		capsule := box2d.Capsule{
			Center1: box2d.Vec2{X: -1, Y: 0},
			Center2: box2d.Vec2{X: 1, Y: 0},
			Radius:  0.5,
		}
		require.True(t, box2d.PointInCapsule(&capsule, box2d.Vec2{X: 0, Y: 0.4}))
		require.True(t, box2d.PointInCapsule(&capsule, box2d.Vec2{X: 1.4, Y: 0}))
		require.False(t, box2d.PointInCapsule(&capsule, box2d.Vec2{X: 1.6, Y: 0}))
		require.False(t, box2d.PointInCapsule(&capsule, box2d.Vec2{X: 0, Y: 0.6}))
	})

	t.Run("degenerate capsule is a circle", func(t *testing.T) {
		t.Parallel()
		// geometry.c b2PointInCapsule dd == 0 branch.
		capsule := box2d.Capsule{
			Center1: box2d.Vec2{X: 1, Y: 2},
			Center2: box2d.Vec2{X: 1, Y: 2},
			Radius:  0.5,
		}
		require.True(t, box2d.PointInCapsule(&capsule, box2d.Vec2{X: 1.3, Y: 2}))
		require.False(t, box2d.PointInCapsule(&capsule, box2d.Vec2{X: 2, Y: 2}))
	})
}

// ---------------------------------------------------------------------------
// geometry.go: ray casts (upstream RayCastShapeTest in test_shape.c plus
// hand-derived branch cases from vendored geometry.c:506/582/718/799)
// ---------------------------------------------------------------------------

func TestOracleRayCastCircleBranches(t *testing.T) {
	t.Parallel()

	circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 1}

	t.Run("upstream straight hit", func(t *testing.T) {
		t.Parallel()
		// Upstream RayCastShapeTest with the circle recentered at the
		// origin: ray (-4,0)+(8,0) hits x=-1 at fraction 3/8... for the
		// upstream circle at (1,0) the hit is x=0 at fraction 0.5. Keep the
		// upstream shape.
		c := box2d.Circle{Center: box2d.Vec2{X: 1, Y: 0}, Radius: 1}
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -4, Y: 0},
			Translation: box2d.Vec2{X: 8, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastCircle(&c, &input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.5, out.Fraction, 1e-6)
		requireVec(t, box2d.Vec2{X: -1, Y: 0}, out.Normal, 1e-6)
	})

	t.Run("zero length ray overlapping", func(t *testing.T) {
		t.Parallel()
		// geometry.c:506 length == 0 branch with initial overlap: hit at the
		// origin with zero fraction and normal.
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0.5, Y: 0},
			Translation: box2d.Vec2{},
			MaxFraction: 1,
		}
		out := box2d.RayCastCircle(&circle, &input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.0, out.Fraction, 0)
		requireVec(t, input.Origin, out.Point, 0)
	})

	t.Run("zero length ray outside misses", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 3, Y: 0},
			Translation: box2d.Vec2{},
			MaxFraction: 1,
		}
		out := box2d.RayCastCircle(&circle, &input)
		require.False(t, out.Hit)
	})

	t.Run("closest point outside radius misses", func(t *testing.T) {
		t.Parallel()
		// Line y=2 stays 2 > r away from the center: cc > rr branch.
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -3, Y: 2},
			Translation: box2d.Vec2{X: 6, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastCircle(&circle, &input)
		require.False(t, out.Hit)
	})

	t.Run("intersection beyond ray range misses", func(t *testing.T) {
		t.Parallel()
		// Hit would be at distance 2 along a unit-length ray:
		// fraction 2 > maxFraction*length = 1 (geometry.c:506 range check).
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 3, Y: 0},
			Translation: box2d.Vec2{X: -1, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastCircle(&circle, &input)
		require.False(t, out.Hit)
	})

	t.Run("ray leaving from inside reports overlap", func(t *testing.T) {
		t.Parallel()
		// Origin inside, moving away: fraction = t-h < 0, but the initial
		// overlap check hits at the origin.
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0.5, Y: 0},
			Translation: box2d.Vec2{X: 5, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastCircle(&circle, &input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.0, out.Fraction, 0)
		requireVec(t, input.Origin, out.Point, 0)
	})
}

func TestOracleRayCastCapsuleBranches(t *testing.T) {
	t.Parallel()

	capsule := box2d.Capsule{
		Center1: box2d.Vec2{X: -1, Y: 0},
		Center2: box2d.Vec2{X: 1, Y: 0},
		Radius:  0.5,
	}

	t.Run("side hit from above", func(t *testing.T) {
		t.Parallel()
		// geometry.c:582 side-hit branch. Ray (0,2)->(0,-2): top surface is
		// y = 0.5, so travel 1.5 of length 4: fraction 0.375, normal (0,1),
		// point (0,0.5).
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0, Y: 2},
			Translation: box2d.Vec2{X: 0, Y: -4},
			MaxFraction: 1,
		}
		out := box2d.RayCastCapsule(&capsule, &input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.375, out.Fraction, 1e-9)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, out.Normal, 1e-9)
		requireVec(t, box2d.Vec2{X: 0, Y: 0.5}, out.Point, 1e-9)
	})

	t.Run("cap hit ahead of segment", func(t *testing.T) {
		t.Parallel()
		// Ray starts inside the infinite slab ahead of center2 (qa >
		// capsuleLength) and defers to the circle at (1,0): hit at x=1.5,
		// fraction (3-1.5)/4 = 0.375, normal (1,0).
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 3, Y: 0},
			Translation: box2d.Vec2{X: -4, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastCapsule(&capsule, &input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.375, out.Fraction, 1e-9)
		requireVec(t, box2d.Vec2{X: 1, Y: 0}, out.Normal, 1e-9)
		requireVec(t, box2d.Vec2{X: 1.5, Y: 0}, out.Point, 1e-9)
	})

	t.Run("cap hit behind segment", func(t *testing.T) {
		t.Parallel()
		// qa < 0 branch: circle at center1.
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -3, Y: 0},
			Translation: box2d.Vec2{X: 4, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastCapsule(&capsule, &input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.375, out.Fraction, 1e-9)
		requireVec(t, box2d.Vec2{X: -1, Y: 0}, out.Normal, 1e-9)
		requireVec(t, box2d.Vec2{X: -1.5, Y: 0}, out.Point, 1e-9)
	})

	t.Run("start inside capsule", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0, Y: 0},
			Translation: box2d.Vec2{X: 2, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastCapsule(&capsule, &input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.0, out.Fraction, 0)
		requireVec(t, input.Origin, out.Point, 0)
	})

	t.Run("parallel ray outside misses", func(t *testing.T) {
		t.Parallel()
		// den ~ 0 branch (geometry.c:582 Cramer denominator).
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -3, Y: 1},
			Translation: box2d.Vec2{X: 6, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastCapsule(&capsule, &input)
		require.False(t, out.Hit)
	})

	t.Run("passes behind segment hits cap sphere", func(t *testing.T) {
		t.Parallel()
		// s1 < 0 branch, then the circle at center1 resolves the hit. Ray
		// x = -1.3 downward: offset from center1 is 0.3, so the hit is at
		// y = sqrt(0.25-0.09) = 0.4: fraction (2-0.4)/4 = 0.4 and normal
		// (-0.3,0.4)/0.5 = (-0.6,0.8).
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -1.3, Y: 2},
			Translation: box2d.Vec2{X: 0, Y: -4},
			MaxFraction: 1,
		}
		out := box2d.RayCastCapsule(&capsule, &input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.4, out.Fraction, 1e-9)
		requireVec(t, box2d.Vec2{X: -0.6, Y: 0.8}, out.Normal, 1e-9)
		requireVec(t, box2d.Vec2{X: -1.3, Y: 0.4}, out.Point, 1e-9)
	})

	t.Run("passes behind segment and misses cap", func(t *testing.T) {
		t.Parallel()
		// s1 < 0, circle at center1 misses: line x=-2 is 1 > r from (-1,0).
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -2, Y: 2},
			Translation: box2d.Vec2{X: 0, Y: -4},
			MaxFraction: 1,
		}
		out := box2d.RayCastCapsule(&capsule, &input)
		require.False(t, out.Hit)
	})

	t.Run("passes ahead of segment hits cap sphere", func(t *testing.T) {
		t.Parallel()
		// s1 > capsuleLength branch: circle at center2.
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 1.3, Y: 2},
			Translation: box2d.Vec2{X: 0, Y: -4},
			MaxFraction: 1,
		}
		out := box2d.RayCastCapsule(&capsule, &input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.4, out.Fraction, 1e-9)
		requireVec(t, box2d.Vec2{X: 0.6, Y: 0.8}, out.Normal, 1e-9)
	})

	t.Run("side approach out of range misses", func(t *testing.T) {
		t.Parallel()
		// s2 beyond maxFraction*rayLength (geometry.c:582 range check):
		// surface at y=0.5 needs travel 1.5 but the ray only goes 1.
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0, Y: 2},
			Translation: box2d.Vec2{X: 0, Y: -1},
			MaxFraction: 1,
		}
		out := box2d.RayCastCapsule(&capsule, &input)
		require.False(t, out.Hit)
	})

	t.Run("degenerate capsule is a circle", func(t *testing.T) {
		t.Parallel()
		// capsuleLength < epsilon branch.
		point := box2d.Capsule{
			Center1: box2d.Vec2{X: 0, Y: 0},
			Center2: box2d.Vec2{X: 0, Y: 0},
			Radius:  0.5,
		}
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -2, Y: 0},
			Translation: box2d.Vec2{X: 4, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastCapsule(&point, &input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.375, out.Fraction, 1e-9)
		requireVec(t, box2d.Vec2{X: -1, Y: 0}, out.Normal, 1e-9)
	})
}

func TestOracleRayCastSegmentBranches(t *testing.T) {
	t.Parallel()

	segment := box2d.Segment{
		Point1: box2d.Vec2{X: 0, Y: 1},
		Point2: box2d.Vec2{X: 0, Y: -1},
	}

	t.Run("upstream one sided hit", func(t *testing.T) {
		t.Parallel()
		// Upstream RayCastShapeTest: fraction 0.5, normal (-1,0).
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -4, Y: 0},
			Translation: box2d.Vec2{X: 8, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastSegment(&segment, &input, true)
		require.True(t, out.Hit)
		require.InDelta(t, 0.5, out.Fraction, 1e-6)
		requireVec(t, box2d.Vec2{X: -1, Y: 0}, out.Normal, 1e-6)
	})

	t.Run("one sided rejects left side", func(t *testing.T) {
		t.Parallel()
		// geometry.c:718: offset = cross(origin-p1, p2-p1) < 0 is a miss.
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 4, Y: 0},
			Translation: box2d.Vec2{X: -8, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastSegment(&segment, &input, true)
		require.False(t, out.Hit)
	})

	t.Run("two sided flips the normal", func(t *testing.T) {
		t.Parallel()
		// numerator > 0 branch: hit from the right flips the right-hand
		// normal to (1,0).
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 4, Y: 0},
			Translation: box2d.Vec2{X: -8, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastSegment(&segment, &input, false)
		require.True(t, out.Hit)
		require.InDelta(t, 0.5, out.Fraction, 1e-9)
		requireVec(t, box2d.Vec2{X: 1, Y: 0}, out.Normal, 1e-9)
		requireVec(t, box2d.Vec2{X: 0, Y: 0}, out.Point, 1e-9)
	})

	t.Run("parallel ray misses", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -1, Y: 2},
			Translation: box2d.Vec2{X: 0, Y: -4},
			MaxFraction: 1,
		}
		out := box2d.RayCastSegment(&segment, &input, false)
		require.False(t, out.Hit)
	})

	t.Run("misses past the segment end", func(t *testing.T) {
		t.Parallel()
		// Crosses the infinite line at y=2, outside [p1,p2]: s out of range.
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -1, Y: 2},
			Translation: box2d.Vec2{X: 2, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastSegment(&segment, &input, false)
		require.False(t, out.Hit)
	})

	t.Run("out of fraction range misses", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -4, Y: 0},
			Translation: box2d.Vec2{X: 2, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastSegment(&segment, &input, false)
		require.False(t, out.Hit)
	})

	t.Run("zero length segment misses", func(t *testing.T) {
		t.Parallel()
		degenerate := box2d.Segment{
			Point1: box2d.Vec2{X: 0, Y: 0},
			Point2: box2d.Vec2{X: 0, Y: 0},
		}
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -4, Y: 0},
			Translation: box2d.Vec2{X: 8, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastSegment(&degenerate, &input, false)
		require.False(t, out.Hit)
	})
}

func TestOracleRayCastPolygonBranches(t *testing.T) {
	t.Parallel()

	b := box2d.MakeBox(1, 1)

	t.Run("upstream box hit", func(t *testing.T) {
		t.Parallel()
		// Upstream RayCastShapeTest: box face x=-1 reached after 3 of 8
		// units: fraction 3/8, normal (-1,0).
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -4, Y: 0},
			Translation: box2d.Vec2{X: 8, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastPolygon(&b, &input)
		require.True(t, out.Hit)
		require.InDelta(t, 3.0/8.0, out.Fraction, 1e-6)
		requireVec(t, box2d.Vec2{X: -1, Y: 0}, out.Normal, 1e-6)
	})

	t.Run("origin inside reports overlap", func(t *testing.T) {
		t.Parallel()
		// geometry.c:799 index < 0 branch: no entering plane found.
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0, Y: 0},
			Translation: box2d.Vec2{X: 4, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastPolygon(&b, &input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.0, out.Fraction, 0)
		requireVec(t, box2d.Vec2{}, out.Point, 0)
	})

	t.Run("parallel outside misses", func(t *testing.T) {
		t.Parallel()
		// denominator == 0 with numerator < 0 for the (0,1) face.
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -4, Y: 2},
			Translation: box2d.Vec2{X: 8, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastPolygon(&b, &input)
		require.False(t, out.Hit)
	})

	t.Run("short ray misses", func(t *testing.T) {
		t.Parallel()
		// lower rises above upper == maxFraction.
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -4, Y: 0},
			Translation: box2d.Vec2{X: 2, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastPolygon(&b, &input)
		require.False(t, out.Hit)
	})

	t.Run("rounded polygon defers to shape cast", func(t *testing.T) {
		t.Parallel()
		// geometry.c:799 radius > 0 path runs b2ShapeCast (distance.c:611)
		// with target = radius - linearSlop = 0.245. Conservative
		// advancement from distance 3.5 along an 8-unit translation with
		// denominator dot((8,0),(-1,0)) = -8 lands in one step:
		//   fraction = (3.5 - 0.245)/8 = 0.406875
		// with the hit point pushed to the rounded surface x = -0.75.
		rb := box2d.MakeRoundedBox(0.5, 0.5, 0.25)
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -4, Y: 0},
			Translation: box2d.Vec2{X: 8, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.RayCastPolygon(&rb, &input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.406875, out.Fraction, 1e-9)
		requireVec(t, box2d.Vec2{X: -1, Y: 0}, out.Normal, 1e-9)
		requireVec(t, box2d.Vec2{X: -0.75, Y: 0}, out.Point, 1e-9)
	})
}

// ---------------------------------------------------------------------------
// distance.go: SegmentDistance (upstream SegmentDistanceTest in
// test_distance.c plus hand-derived degenerate cases from distance.c:33)
// ---------------------------------------------------------------------------

func TestOracleSegmentDistanceTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		p1, q1, p2, q2 box2d.Vec2
		f1, f2         float64
		c1, c2         box2d.Vec2
		dsq            float64
	}{
		{
			// Upstream SegmentDistanceTest.
			name: "upstream perpendicular",
			p1:   box2d.Vec2{X: -1, Y: -1}, q1: box2d.Vec2{X: -1, Y: 1},
			p2: box2d.Vec2{X: 2, Y: 0}, q2: box2d.Vec2{X: 1, Y: 0},
			f1: 0.5, f2: 1.0,
			c1: box2d.Vec2{X: -1, Y: 0}, c2: box2d.Vec2{X: 1, Y: 0},
			dsq: 4.0,
		},
		{
			// distance.c:33 both-degenerate branch: fractions 0, points are
			// the inputs, dsq = |(3,4)|^2 = 25.
			name: "both segments degenerate",
			p1:   box2d.Vec2{}, q1: box2d.Vec2{},
			p2: box2d.Vec2{X: 3, Y: 4}, q2: box2d.Vec2{X: 3, Y: 4},
			f1: 0, f2: 0,
			c1: box2d.Vec2{}, c2: box2d.Vec2{X: 3, Y: 4},
			dsq: 25.0,
		},
		{
			// Segment 1 degenerate: f2 = clamp(rd2/dd2) = 2/4 = 0.5,
			// closest2 = (0,2), dsq = 4.
			name: "segment one degenerate",
			p1:   box2d.Vec2{}, q1: box2d.Vec2{},
			p2: box2d.Vec2{X: -1, Y: 2}, q2: box2d.Vec2{X: 1, Y: 2},
			f1: 0, f2: 0.5,
			c1: box2d.Vec2{}, c2: box2d.Vec2{X: 0, Y: 2},
			dsq: 4.0,
		},
		{
			// Segment 2 degenerate: f1 = clamp(-rd1/dd1) = 2/4 = 0.5.
			name: "segment two degenerate",
			p1:   box2d.Vec2{X: -1, Y: 2}, q1: box2d.Vec2{X: 1, Y: 2},
			p2: box2d.Vec2{}, q2: box2d.Vec2{},
			f1: 0.5, f2: 0,
			c1: box2d.Vec2{X: 0, Y: 2}, c2: box2d.Vec2{},
			dsq: 4.0,
		},
		{
			// Parallel (denominator == 0): f1 starts 0, f2 = rd2/dd2 =
			// -6/4 < 0 clamps to 0, redo f1 = -rd1/dd1 = 6/4 clamps to 1.
			name: "parallel disjoint",
			p1:   box2d.Vec2{}, q1: box2d.Vec2{X: 2, Y: 0},
			p2: box2d.Vec2{X: 3, Y: 1}, q2: box2d.Vec2{X: 5, Y: 1},
			f1: 1, f2: 0,
			c1: box2d.Vec2{X: 2, Y: 0}, c2: box2d.Vec2{X: 3, Y: 1},
			dsq: 2.0,
		},
		{
			// f2 > 1 clamp branch: f2 = 6/4 = 1.5 -> 1, then
			// f1 = (d12-rd1)/dd1 = (0+2)/4 = 0.5.
			name: "clamp f2 high",
			p1:   box2d.Vec2{}, q1: box2d.Vec2{X: 2, Y: 0},
			p2: box2d.Vec2{X: 1, Y: 3}, q2: box2d.Vec2{X: 1, Y: 1},
			f1: 0.5, f2: 1,
			c1: box2d.Vec2{X: 1, Y: 0}, c2: box2d.Vec2{X: 1, Y: 1},
			dsq: 1.0,
		},
		{
			// Interior-interior crossing: closed form f1 = 0.5, f2 = 0.75,
			// distance zero.
			name: "crossing segments",
			p1:   box2d.Vec2{X: -1, Y: 0.5}, q1: box2d.Vec2{X: 1, Y: 0.5},
			p2: box2d.Vec2{X: 0, Y: -1}, q2: box2d.Vec2{X: 0, Y: 1},
			f1: 0.5, f2: 0.75,
			c1: box2d.Vec2{X: 0, Y: 0.5}, c2: box2d.Vec2{X: 0, Y: 0.5},
			dsq: 0.0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := box2d.SegmentDistance(tc.p1, tc.q1, tc.p2, tc.q2)
			require.InDelta(t, tc.f1, res.Fraction1, 1e-9, "fraction1")
			require.InDelta(t, tc.f2, res.Fraction2, 1e-9, "fraction2")
			requireVec(t, tc.c1, res.Closest1, 1e-9)
			requireVec(t, tc.c2, res.Closest2, 1e-9)
			require.InDelta(t, tc.dsq, res.DistanceSquared, 1e-9, "distanceSquared")
		})
	}
}

// ---------------------------------------------------------------------------
// distance.go: proxies
// ---------------------------------------------------------------------------

// TestOracleMakeOffsetProxy checks b2MakeOffsetProxy (distance.c:121): each
// point is transformed by {position, rotation}. Rot 90deg maps (1,0)->(0,1)
// and (0,1)->(-1,0); adding (2,3) gives (2,4) and (1,3).
func TestOracleMakeOffsetProxy(t *testing.T) {
	t.Parallel()

	points := []box2d.Vec2{{X: 1, Y: 0}, {X: 0, Y: 1}}
	proxy := box2d.MakeOffsetProxy(points, 2, 0.3, box2d.Vec2{X: 2, Y: 3}, box2d.MakeRot(0.5*math.Pi))

	require.Equal(t, 2, proxy.Count)
	require.InDelta(t, 0.3, proxy.Radius, 0)
	requireVec(t, box2d.Vec2{X: 2, Y: 4}, proxy.Points[0], 1e-12)
	requireVec(t, box2d.Vec2{X: 1, Y: 3}, proxy.Points[1], 1e-12)

	// The plain proxy is a deep copy with no transform (distance.c:104).
	plain := box2d.MakeProxy(points, 2, 0.25)
	require.Equal(t, 2, plain.Count)
	requireVec(t, points[0], plain.Points[0], 0)
	requireVec(t, points[1], plain.Points[1], 0)
}

// ---------------------------------------------------------------------------
// distance.go: ShapeDistance (upstream ShapeDistanceTest in test_distance.c
// plus hand-derived cases against distance.c:424)
// ---------------------------------------------------------------------------

func TestOracleShapeDistance(t *testing.T) {
	t.Parallel()

	boxVerts := []box2d.Vec2{{X: -1, Y: -1}, {X: 1, Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1}}
	segVerts := []box2d.Vec2{{X: 2, Y: -1}, {X: 2, Y: 1}}

	t.Run("upstream box vs segment", func(t *testing.T) {
		t.Parallel()
		// Upstream ShapeDistanceTest: gap between the box face x=1 and the
		// segment x=2 is exactly 1.
		var input box2d.DistanceInput
		input.ProxyA = box2d.MakeProxy(boxVerts, 4, 0)
		input.ProxyB = box2d.MakeProxy(segVerts, 2, 0)
		input.TransformA = box2d.TransformIdentity
		input.TransformB = box2d.TransformIdentity
		input.UseRadii = false

		var cache box2d.SimplexCache
		out := box2d.ShapeDistance(&input, &cache, nil)
		require.InDelta(t, 1.0, out.Distance, 1e-9)
		requireVec(t, box2d.Vec2{X: 1, Y: 0}, out.Normal, 1e-9)
	})

	t.Run("radii shrink the distance", func(t *testing.T) {
		t.Parallel()
		// distance.c:424 useRadii epilogue: distance = max(0, d - rA - rB)
		// and the witness points move to the perimeters along the normal.
		// Two points 3 apart with radii 0.5 and 0.25: distance 2.25,
		// pointA = (0.5,0), pointB = (2.75,0).
		var input box2d.DistanceInput
		input.ProxyA = box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0.5)
		input.ProxyB = box2d.MakeProxy([]box2d.Vec2{{X: 3, Y: 0}}, 1, 0.25)
		input.TransformA = box2d.TransformIdentity
		input.TransformB = box2d.TransformIdentity
		input.UseRadii = true

		var cache box2d.SimplexCache
		out := box2d.ShapeDistance(&input, &cache, nil)
		require.InDelta(t, 2.25, out.Distance, 1e-9)
		requireVec(t, box2d.Vec2{X: 1, Y: 0}, out.Normal, 1e-9)
		requireVec(t, box2d.Vec2{X: 0.5, Y: 0}, out.PointA, 1e-9)
		requireVec(t, box2d.Vec2{X: 2.75, Y: 0}, out.PointB, 1e-9)
	})

	t.Run("overlap returns zero distance", func(t *testing.T) {
		t.Parallel()
		// GJK encloses the origin (3-simplex): distance 0 and identical
		// witness points (distance.c:424 overlap exit).
		var input box2d.DistanceInput
		input.ProxyA = box2d.MakeProxy(boxVerts, 4, 0)
		input.ProxyB = box2d.MakeProxy(boxVerts, 4, 0)
		input.TransformA = box2d.TransformIdentity
		input.TransformB = xf(0.5, 0.25, 0)
		input.UseRadii = false

		var cache box2d.SimplexCache
		out := box2d.ShapeDistance(&input, &cache, nil)
		require.InDelta(t, 0.0, out.Distance, 0)
		requireVec(t, out.PointA, out.PointB, 1e-12)
	})

	t.Run("point deep inside box overlaps", func(t *testing.T) {
		t.Parallel()
		var input box2d.DistanceInput
		input.ProxyA = box2d.MakeProxy(boxVerts, 4, 0)
		input.ProxyB = box2d.MakeProxy([]box2d.Vec2{{X: 0.1, Y: -0.2}}, 1, 0)
		input.TransformA = box2d.TransformIdentity
		input.TransformB = box2d.TransformIdentity
		input.UseRadii = false

		var cache box2d.SimplexCache
		out := box2d.ShapeDistance(&input, &cache, nil)
		require.InDelta(t, 0.0, out.Distance, 0)
	})

	t.Run("rotated transform", func(t *testing.T) {
		t.Parallel()
		// Box rotated 45deg has its corner at x = sqrt(2); segment at x=2:
		// distance = 2 - sqrt(2).
		var input box2d.DistanceInput
		input.ProxyA = box2d.MakeProxy(boxVerts, 4, 0)
		input.ProxyB = box2d.MakeProxy(segVerts, 2, 0)
		input.TransformA = xf(0, 0, 0.25*math.Pi)
		input.TransformB = box2d.TransformIdentity
		input.UseRadii = false

		var cache box2d.SimplexCache
		out := box2d.ShapeDistance(&input, &cache, nil)
		require.InDelta(t, 2.0-math.Sqrt2, out.Distance, 1e-9)
	})

	t.Run("debug simplex capture", func(t *testing.T) {
		t.Parallel()
		// The release build stores exactly the initial simplex
		// (distance.c:424; later stores sit under #ifndef NDEBUG and this
		// port compiles them out via debugAsserts=false).
		var input box2d.DistanceInput
		input.ProxyA = box2d.MakeProxy(boxVerts, 4, 0)
		input.ProxyB = box2d.MakeProxy(segVerts, 2, 0)
		input.TransformA = box2d.TransformIdentity
		input.TransformB = box2d.TransformIdentity
		input.UseRadii = false

		var cache box2d.SimplexCache
		simplexes := make([]box2d.Simplex, 8)
		out := box2d.ShapeDistance(&input, &cache, simplexes)
		require.Equal(t, 1, out.SimplexCount)
		require.InDelta(t, 1.0, out.Distance, 1e-9)

		// Warm restart from the produced cache converges immediately.
		out2 := box2d.ShapeDistance(&input, &cache, nil)
		require.InDelta(t, 1.0, out2.Distance, 1e-9)
	})
}

// ---------------------------------------------------------------------------
// distance.go: ShapeCast (upstream ShapeCastTest in test_distance.c plus
// hand-derived conservative-advancement cases from distance.c:611)
// ---------------------------------------------------------------------------

func TestOracleShapeCast(t *testing.T) {
	t.Parallel()

	boxVerts := []box2d.Vec2{{X: -1, Y: -1}, {X: 1, Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1}}
	segVerts := []box2d.Vec2{{X: 2, Y: -1}, {X: 2, Y: 1}}

	t.Run("upstream box vs segment", func(t *testing.T) {
		t.Parallel()
		// Upstream ShapeCastTest expects fraction 0.5 +- 0.005. The exact
		// vendored value: target = linearSlop = 0.005, gap 1, denominator
		// dot((-2,0),(1,0)) = -2, so one advancement lands at
		// fraction = (1 - 0.005)/2 = 0.4975.
		var input box2d.ShapeCastPairInput
		input.ProxyA = box2d.MakeProxy(boxVerts, 4, 0)
		input.ProxyB = box2d.MakeProxy(segVerts, 2, 0)
		input.TransformA = box2d.TransformIdentity
		input.TransformB = box2d.TransformIdentity
		input.TranslationB = box2d.Vec2{X: -2, Y: 0}
		input.MaxFraction = 1

		out := box2d.ShapeCast(&input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.5, out.Fraction, 0.005)
		require.InDelta(t, 0.4975, out.Fraction, 1e-9)
	})

	t.Run("circle regular hit", func(t *testing.T) {
		t.Parallel()
		// Point radius 0.5 vs point radius 0.4 starting 2 apart:
		// target = 0.9 - 0.005 = 0.895, one advancement gives
		// fraction = (2 - 0.895)/2 = 0.5525; the hit point sits on A's
		// perimeter at (0.5, 0).
		var input box2d.ShapeCastPairInput
		input.ProxyA = box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0.5)
		input.ProxyB = box2d.MakeProxy([]box2d.Vec2{{X: 2, Y: 0}}, 1, 0.4)
		input.TransformA = box2d.TransformIdentity
		input.TransformB = box2d.TransformIdentity
		input.TranslationB = box2d.Vec2{X: -2, Y: 0}
		input.MaxFraction = 1

		out := box2d.ShapeCast(&input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.5525, out.Fraction, 1e-9)
		requireVec(t, box2d.Vec2{X: 1, Y: 0}, out.Normal, 1e-9)
		requireVec(t, box2d.Vec2{X: 0.5, Y: 0}, out.Point, 1e-9)
	})

	t.Run("initial overlap", func(t *testing.T) {
		t.Parallel()
		// distance.c:611 iteration-0 branch without encroachment: hit with
		// zero fraction at the midpoint of the two perimeter points
		// c1 = (0.5,0), c2 = (0.2,0) -> (0.35,0).
		var input box2d.ShapeCastPairInput
		input.ProxyA = box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0.5)
		input.ProxyB = box2d.MakeProxy([]box2d.Vec2{{X: 0.6, Y: 0}}, 1, 0.4)
		input.TransformA = box2d.TransformIdentity
		input.TransformB = box2d.TransformIdentity
		input.TranslationB = box2d.Vec2{X: -2, Y: 0}
		input.MaxFraction = 1

		out := box2d.ShapeCast(&input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.0, out.Fraction, 0)
		requireVec(t, box2d.Vec2{X: 0.35, Y: 0}, out.Point, 1e-9)
	})

	t.Run("encroaching cast", func(t *testing.T) {
		t.Parallel()
		// canEncroach with initial distance 0.6 > 2*linearSlop retargets to
		// 0.6 - 0.005 = 0.595; one advancement of (0.595-0.6)/(-2) gives
		// fraction = 0.0025 (distance.c:611 canEncroach branch).
		var input box2d.ShapeCastPairInput
		input.ProxyA = box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0.5)
		input.ProxyB = box2d.MakeProxy([]box2d.Vec2{{X: 0.6, Y: 0}}, 1, 0.4)
		input.TransformA = box2d.TransformIdentity
		input.TransformB = box2d.TransformIdentity
		input.TranslationB = box2d.Vec2{X: -2, Y: 0}
		input.MaxFraction = 1
		input.CanEncroach = true

		out := box2d.ShapeCast(&input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.0025, out.Fraction, 1e-9)
	})

	t.Run("moving away misses", func(t *testing.T) {
		t.Parallel()
		// denominator = dot(translation, normal) >= 0 is a miss.
		var input box2d.ShapeCastPairInput
		input.ProxyA = box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0.5)
		input.ProxyB = box2d.MakeProxy([]box2d.Vec2{{X: 2, Y: 0}}, 1, 0.4)
		input.TransformA = box2d.TransformIdentity
		input.TransformB = box2d.TransformIdentity
		input.TranslationB = box2d.Vec2{X: 2, Y: 0}
		input.MaxFraction = 1

		out := box2d.ShapeCast(&input)
		require.False(t, out.Hit)
	})

	t.Run("max fraction short misses", func(t *testing.T) {
		t.Parallel()
		// First advancement 0.5525 already exceeds maxFraction 0.25.
		var input box2d.ShapeCastPairInput
		input.ProxyA = box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0.5)
		input.ProxyB = box2d.MakeProxy([]box2d.Vec2{{X: 2, Y: 0}}, 1, 0.4)
		input.TransformA = box2d.TransformIdentity
		input.TransformB = box2d.TransformIdentity
		input.TranslationB = box2d.Vec2{X: -2, Y: 0}
		input.MaxFraction = 0.25

		out := box2d.ShapeCast(&input)
		require.False(t, out.Hit)
	})

	t.Run("shape cast wrappers", func(t *testing.T) {
		t.Parallel()
		// geometry.c:893-952: the four wrappers forward to b2ShapeCast with
		// the shape as proxy A. A point cast at a unit circle from (3,0)
		// toward the origin travels 2 - target = 2 - 0.995 = 1.005... the
		// gap is 3-1 = 2, target = 1 - 0.005 = 0.995, so
		// fraction = (2 - 0.995)/4 with a 4-long translation: wait, the
		// advancement divides by dot(d, normal) = -4:
		//   fraction = (0.995 - 3)/(-4) with distance measured center to
		//   point: distance 3, target 0.995 -> fraction = 2.005/4 = 0.50125.
		circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 1}
		input := box2d.ShapeCastInput{
			Proxy:       box2d.MakeProxy([]box2d.Vec2{{X: 3, Y: 0}}, 1, 0),
			Translation: box2d.Vec2{X: -4, Y: 0},
			MaxFraction: 1,
		}
		out := box2d.ShapeCastCircle(&circle, &input)
		require.True(t, out.Hit)
		require.InDelta(t, 0.50125, out.Fraction, 1e-9)
		requireVec(t, box2d.Vec2{X: 1, Y: 0}, out.Normal, 1e-9)
	})
}

// ---------------------------------------------------------------------------
// distance.go: TimeOfImpact (upstream TimeOfImpactTest in test_distance.c
// plus hand-derived state cases from distance.c:1140)
// ---------------------------------------------------------------------------

func oracleStaticSweep() box2d.Sweep {
	return oracleLinearSweep(box2d.Vec2{}, box2d.Vec2{})
}

func oracleLinearSweep(c1, c2 box2d.Vec2) box2d.Sweep {
	return box2d.Sweep{
		LocalCenter: box2d.Vec2{},
		C1:          c1, C2: c2,
		Q1: box2d.RotIdentity, Q2: box2d.RotIdentity,
	}
}

func TestOracleTimeOfImpact(t *testing.T) {
	t.Parallel()

	boxVerts := []box2d.Vec2{{X: -1, Y: -1}, {X: 1, Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1}}
	segVerts := []box2d.Vec2{{X: 2, Y: -1}, {X: 2, Y: 1}}

	t.Run("upstream hit", func(t *testing.T) {
		t.Parallel()
		// Upstream TimeOfImpactTest: state hit, fraction 0.5 +- 0.005. The
		// exact separation root: gap(t) = 1 - 2t = linearSlop at
		// t = 0.4975 (distance.c:1140 with target = linearSlop).
		var input box2d.TOIInput
		input.ProxyA = box2d.MakeProxy(boxVerts, 4, 0)
		input.ProxyB = box2d.MakeProxy(segVerts, 2, 0)
		input.SweepA = oracleStaticSweep()
		input.SweepB = oracleLinearSweep(box2d.Vec2{}, box2d.Vec2{X: -2, Y: 0})
		input.MaxFraction = 1

		out := box2d.TimeOfImpact(&input)
		require.Equal(t, box2d.TOIStateHit, out.State)
		require.InDelta(t, 0.5, out.Fraction, 0.005)
		require.InDelta(t, 0.4975, out.Fraction, 1e-6)
	})

	t.Run("separated", func(t *testing.T) {
		t.Parallel()
		// B moves parallel to A staying 1 away: s2 at t=1 stays above
		// target+tolerance, so the state is separated with fraction tMax.
		var input box2d.TOIInput
		input.ProxyA = box2d.MakeProxy(boxVerts, 4, 0)
		input.ProxyB = box2d.MakeProxy(segVerts, 2, 0)
		input.SweepA = oracleStaticSweep()
		input.SweepB = oracleLinearSweep(box2d.Vec2{}, box2d.Vec2{X: 0, Y: 2})
		input.MaxFraction = 1

		out := box2d.TimeOfImpact(&input)
		require.Equal(t, box2d.TOIStateSeparated, out.State)
		require.InDelta(t, 1.0, out.Fraction, 0)
	})

	t.Run("overlapped", func(t *testing.T) {
		t.Parallel()
		// Initial overlap gives up on CCD: state overlapped, fraction 0.
		var input box2d.TOIInput
		input.ProxyA = box2d.MakeProxy(boxVerts, 4, 0)
		input.ProxyB = box2d.MakeProxy(boxVerts, 4, 0)
		input.SweepA = oracleStaticSweep()
		input.SweepB = oracleLinearSweep(box2d.Vec2{X: 0.5, Y: 0}, box2d.Vec2{X: 1, Y: 0})
		input.MaxFraction = 1

		out := box2d.TimeOfImpact(&input)
		require.Equal(t, box2d.TOIStateOverlapped, out.State)
		require.InDelta(t, 0.0, out.Fraction, 0)
	})

	t.Run("point pair sweep", func(t *testing.T) {
		t.Parallel()
		// Circle-circle: the GJK cache holds a single vertex pair, so the
		// separation function is the points type (distance.c
		// b2MakeSeparationFunction count == 1). Radii 0.25 each:
		// target = 0.5 - 0.005 = 0.495, gap(t) = 2 - 2t, root at
		// t = (2 - 0.495)/2 = 0.7525.
		var input box2d.TOIInput
		input.ProxyA = box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0.25)
		input.ProxyB = box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0.25)
		input.SweepA = oracleStaticSweep()
		input.SweepB = oracleLinearSweep(box2d.Vec2{X: 2, Y: 0}, box2d.Vec2{})
		input.MaxFraction = 1

		out := box2d.TimeOfImpact(&input)
		require.Equal(t, box2d.TOIStateHit, out.State)
		require.InDelta(t, 0.7525, out.Fraction, 0.005)
	})

	t.Run("point vs box face", func(t *testing.T) {
		t.Parallel()
		// One point on A, an edge on B: face-B separation function.
		// Gap between the point and the face x=1.5 of the box centered at
		// c(t): target = 0.25 - 0.005 = 0.245 with radius 0.25 on A:
		// gap(t) = 1.5 - 2t, root at t = (1.5-0.245)/2 = 0.6275.
		var input box2d.TOIInput
		input.ProxyA = box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0.25)
		input.ProxyB = box2d.MakeProxy([]box2d.Vec2{
			{X: -0.5, Y: -0.5}, {X: 0.5, Y: -0.5}, {X: 0.5, Y: 0.5}, {X: -0.5, Y: 0.5},
		}, 4, 0)
		input.SweepA = oracleStaticSweep()
		input.SweepB = oracleLinearSweep(box2d.Vec2{X: 2, Y: 0}, box2d.Vec2{})
		input.MaxFraction = 1

		out := box2d.TimeOfImpact(&input)
		require.Equal(t, box2d.TOIStateHit, out.State)
		require.InDelta(t, 0.6275, out.Fraction, 0.005)
	})

	t.Run("rotating sweep hits", func(t *testing.T) {
		t.Parallel()
		// A spinning 0.4x0.4 box translating from (2,0) to the origin must
		// hit the static 2x2 box (face x=1). Bounds from the C algorithm:
		// the rotating extent toward A stays within [0.2, 0.2*sqrt2], so
		// the touch time 2 - 2t = 1 + extent + linearSlop lies in
		// [(2 - 1 - 0.283 - 0.005)/2, (2 - 1 - 0.2 - 0.005)/2]
		// = [0.356, 0.3975].
		var input box2d.TOIInput
		input.ProxyA = box2d.MakeProxy(boxVerts, 4, 0)
		input.ProxyB = box2d.MakeProxy([]box2d.Vec2{
			{X: -0.2, Y: -0.2}, {X: 0.2, Y: -0.2}, {X: 0.2, Y: 0.2}, {X: -0.2, Y: 0.2},
		}, 4, 0)
		input.SweepA = oracleStaticSweep()
		input.SweepB = box2d.Sweep{
			LocalCenter: box2d.Vec2{},
			C1:          box2d.Vec2{X: 2, Y: 0}, C2: box2d.Vec2{},
			Q1: box2d.RotIdentity, Q2: box2d.MakeRot(0.5 * math.Pi),
		}
		input.MaxFraction = 1

		out := box2d.TimeOfImpact(&input)
		require.Equal(t, box2d.TOIStateHit, out.State)
		require.GreaterOrEqual(t, out.Fraction, 0.356)
		require.LessOrEqual(t, out.Fraction, 0.3975)
	})

	t.Run("touch at sweep end", func(t *testing.T) {
		t.Parallel()
		// B ends the sweep with gap exactly linearSlop: the inner loop takes
		// the s2 > target - tolerance advance branch and the outer loop
		// declares a hit at fraction 1.
		var input box2d.TOIInput
		input.ProxyA = box2d.MakeProxy(boxVerts, 4, 0)
		input.ProxyB = box2d.MakeProxy(segVerts, 2, 0)
		input.SweepA = oracleStaticSweep()
		input.SweepB = oracleLinearSweep(box2d.Vec2{}, box2d.Vec2{X: -(1.0 - box2d.LinearSlop), Y: 0})
		input.MaxFraction = 1

		out := box2d.TimeOfImpact(&input)
		require.Equal(t, box2d.TOIStateHit, out.State)
		require.InDelta(t, 1.0, out.Fraction, 1e-9)
	})
}

// ---------------------------------------------------------------------------
// hull.go (vendored hull.c:87/273)
// ---------------------------------------------------------------------------

func TestOracleHullEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("too few or too many points fail", func(t *testing.T) {
		t.Parallel()
		h := box2d.ComputeHull([]box2d.Vec2{{X: 0, Y: 0}, {X: 1, Y: 0}})
		require.Equal(t, 0, h.Count)

		var many []box2d.Vec2
		for i := range 9 {
			angle := 2.0 * math.Pi * float64(i) / 9.0
			many = append(many, box2d.Vec2{X: math.Cos(angle), Y: math.Sin(angle)})
		}
		h = box2d.ComputeHull(many)
		require.Equal(t, 0, h.Count)
	})

	t.Run("welding collapses close points", func(t *testing.T) {
		t.Parallel()
		// hull.c:87 welds points within 4*linearSlop = 0.02. Three of the
		// four points weld into one, leaving fewer than 3.
		h := box2d.ComputeHull([]box2d.Vec2{
			{X: 0, Y: 0}, {X: 0.01, Y: 0}, {X: 0, Y: 0.01}, {X: 1, Y: 1},
		})
		require.Equal(t, 0, h.Count)
	})

	t.Run("collinear points fail", func(t *testing.T) {
		t.Parallel()
		h := box2d.ComputeHull([]box2d.Vec2{
			{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0}, {X: 3, Y: 0},
		})
		require.Equal(t, 0, h.Count)
	})

	t.Run("interior points are dropped", func(t *testing.T) {
		t.Parallel()
		h := box2d.ComputeHull([]box2d.Vec2{
			{X: -1, Y: -1}, {X: 1, Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1},
			{X: 0, Y: 0}, {X: 0.5, Y: 0.25},
		})
		require.Equal(t, 4, h.Count)
		require.True(t, box2d.ValidateHull(&h))
	})

	t.Run("validate rejects bad counts", func(t *testing.T) {
		t.Parallel()
		var h box2d.Hull
		h.Count = 2
		require.False(t, box2d.ValidateHull(&h))
		h.Count = box2d.MaxPolygonVertices + 1
		require.False(t, box2d.ValidateHull(&h))
	})

	t.Run("validate rejects wrong winding", func(t *testing.T) {
		t.Parallel()
		// hull.c:273 convexity: every point must be strictly behind every
		// edge; a clockwise square fails.
		var h box2d.Hull
		h.Count = 4
		h.Points[0] = box2d.Vec2{X: -1, Y: -1}
		h.Points[1] = box2d.Vec2{X: -1, Y: 1}
		h.Points[2] = box2d.Vec2{X: 1, Y: 1}
		h.Points[3] = box2d.Vec2{X: 1, Y: -1}
		require.False(t, box2d.ValidateHull(&h))
	})

	t.Run("validate rejects near collinear points", func(t *testing.T) {
		t.Parallel()
		// Convex but the mid vertex is only 0.004 < linearSlop off the
		// (0,0)-(1,0)... off the long edge, so the collinearity check at
		// hull.c:273 rejects it.
		var h box2d.Hull
		h.Count = 3
		h.Points[0] = box2d.Vec2{X: 0, Y: 0}
		h.Points[1] = box2d.Vec2{X: 1, Y: 0}
		h.Points[2] = box2d.Vec2{X: 0.5, Y: 0.002}
		require.False(t, box2d.ValidateHull(&h))
	})
}

// TestOracleMakeOffsetRoundedPolygon checks b2MakeOffsetRoundedPolygon
// (geometry.c b2MakeOffsetRoundedPolygon): vertices and centroid map through
// {position, rotation} and the radius is stored.
func TestOracleMakeOffsetRoundedPolygon(t *testing.T) {
	t.Parallel()

	hull := box2d.ComputeHull([]box2d.Vec2{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}})
	require.Equal(t, 3, hull.Count)

	poly := box2d.MakeOffsetRoundedPolygon(&hull, box2d.Vec2{X: 2, Y: 3}, box2d.MakeRot(0.5*math.Pi), 0.25)
	require.Equal(t, 3, poly.Count)
	require.InDelta(t, 0.25, poly.Radius, 0)
	// Triangle centroid (1/3,1/3) rotated 90deg is (-1/3,1/3); plus (2,3).
	requireVec(t, box2d.Vec2{X: 2.0 - 1.0/3.0, Y: 3.0 + 1.0/3.0}, poly.Centroid, 1e-9)

	// The zero-radius wrapper (geometry.c b2MakeOffsetPolygon) produces the
	// same geometry with radius 0.
	flat := box2d.MakeOffsetPolygon(&hull, box2d.Vec2{X: 2, Y: 3}, box2d.MakeRot(0.5*math.Pi))
	require.InDelta(t, 0.0, flat.Radius, 0)
	requireVec(t, poly.Centroid, flat.Centroid, 1e-12)

	// geometry.c b2MakeOffsetRoundedPolygon "handle a bad hull when
	// assertions are disabled": an empty hull yields b2MakeSquare(0.5).
	var bad box2d.Hull
	fallback := box2d.MakeOffsetRoundedPolygon(&bad, box2d.Vec2{X: 9, Y: 9}, box2d.RotIdentity, 0.5)
	require.Equal(t, 4, fallback.Count)
	require.InDelta(t, 0.0, fallback.Radius, 0)
	requireVec(t, box2d.Vec2{X: -0.5, Y: -0.5}, fallback.Vertices[0], 0)
	requireVec(t, box2d.Vec2{X: 0.5, Y: 0.5}, fallback.Vertices[2], 0)
}

// ---------------------------------------------------------------------------
// manifold.go: circles and capsules (vendored manifold.c:40/77/141/261)
// ---------------------------------------------------------------------------

func TestOracleCollideCirclesManifold(t *testing.T) {
	t.Parallel()

	t.Run("overlapping circles", func(t *testing.T) {
		t.Parallel()
		// manifold.c:40. r=0.5 circles 0.75 apart: separation -0.25, normal
		// (1,0), contact midway between the surface points (0.5,0) and
		// (0.25,0) -> anchorA (0.375,0).
		a := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.5}
		b := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.5}
		m := box2d.CollideCircles(&a, box2d.TransformIdentity, &b, xf(0.75, 0, 0))
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: 1, Y: 0}, m.Normal, 1e-9)
		require.InDelta(t, -0.25, m.Points[0].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: 0.375, Y: 0}, m.Points[0].AnchorA, 1e-9)
		requireVec(t, box2d.Vec2{X: 0.375, Y: 0}, m.Points[0].ClipPoint, 1e-9)
		requireVec(t, box2d.Vec2{X: -0.375, Y: 0}, m.Points[0].AnchorB, 1e-9)
	})

	t.Run("beyond speculative distance misses", func(t *testing.T) {
		t.Parallel()
		// separation 0.05 > speculativeDistance 0.02.
		a := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.5}
		b := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.5}
		m := box2d.CollideCircles(&a, box2d.TransformIdentity, &b, xf(1.05, 0, 0))
		require.Equal(t, 0, m.PointCount)
	})
}

func TestOracleCollideCapsuleAndCircleManifold(t *testing.T) {
	t.Parallel()

	capsule := box2d.Capsule{
		Center1: box2d.Vec2{X: -0.5, Y: 0},
		Center2: box2d.Vec2{X: 0.5, Y: 0},
		Radius:  0.25,
	}

	t.Run("segment interior", func(t *testing.T) {
		t.Parallel()
		// manifold.c:77 interior branch: circle (0,0.4) r 0.25 above the
		// axis: distance 0.4, separation -0.1, normal (0,1), contact
		// midpoint of (0,0.25) and (0,0.15).
		circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}
		m := box2d.CollideCapsuleAndCircle(&capsule, box2d.TransformIdentity, &circle, xf(0, 0.4, 0))
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-9)
		require.InDelta(t, -0.1, m.Points[0].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: 0, Y: 0.2}, m.Points[0].AnchorA, 1e-9)
	})

	t.Run("cap region", func(t *testing.T) {
		t.Parallel()
		// s1 < 0 branch: closest feature is center1. Circle at (-0.8,0.1):
		// distance sqrt(0.09+0.01) = sqrt(0.1), separation sqrt(0.1)-0.5.
		circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}
		m := box2d.CollideCapsuleAndCircle(&capsule, box2d.TransformIdentity, &circle, xf(-0.8, 0.1, 0))
		require.Equal(t, 1, m.PointCount)
		d := math.Sqrt(0.1)
		require.InDelta(t, d-0.5, m.Points[0].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: -0.3 / d, Y: 0.1 / d}, m.Normal, 1e-9)
	})

	t.Run("far circle misses", func(t *testing.T) {
		t.Parallel()
		circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}
		m := box2d.CollideCapsuleAndCircle(&capsule, box2d.TransformIdentity, &circle, xf(0, 1, 0))
		require.Equal(t, 0, m.PointCount)
	})
}

func TestOracleCollidePolygonAndCircleManifold(t *testing.T) {
	t.Parallel()

	b := box2d.MakeSquare(0.5)

	t.Run("face region", func(t *testing.T) {
		t.Parallel()
		// manifold.c:141 default branch: circle (0.76,0) r 0.25 against
		// face x=0.5: separation 0.26 - 0.25 = 0.01 (speculative), normal
		// (1,0), cA=(0.5,0), cB=(0.51,0) -> anchor (0.505,0).
		circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}
		m := box2d.CollidePolygonAndCircle(&b, box2d.TransformIdentity, &circle, xf(0.76, 0, 0))
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: 1, Y: 0}, m.Normal, 1e-9)
		require.InDelta(t, 0.01, m.Points[0].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: 0.505, Y: 0}, m.Points[0].AnchorA, 1e-9)
	})

	t.Run("vertex region", func(t *testing.T) {
		t.Parallel()
		// u2 < 0 branch off the reference edge (0,-1): circle at
		// (0.68,-0.68) is closest to corner (0.5,-0.5): distance
		// 0.18*sqrt(2), separation 0.18*sqrt(2) - 0.25, diagonal normal.
		circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}
		m := box2d.CollidePolygonAndCircle(&b, box2d.TransformIdentity, &circle, xf(0.68, -0.68, 0))
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: invSqrt2, Y: -invSqrt2}, m.Normal, 1e-9)
		require.InDelta(t, 0.18*math.Sqrt2-0.25, m.Points[0].Separation, 1e-9)
	})

	t.Run("circle center inside polygon", func(t *testing.T) {
		t.Parallel()
		// Deep contact keeps the face branch: max separation -0.2 on the
		// (1,0) face, manifold separation -0.2 - 0.25 = -0.45.
		circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}
		m := box2d.CollidePolygonAndCircle(&b, box2d.TransformIdentity, &circle, xf(0.3, 0, 0))
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: 1, Y: 0}, m.Normal, 1e-9)
		require.InDelta(t, -0.45, m.Points[0].Separation, 1e-9)
	})

	t.Run("far circle misses", func(t *testing.T) {
		t.Parallel()
		circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}
		m := box2d.CollidePolygonAndCircle(&b, box2d.TransformIdentity, &circle, xf(2, 0, 0))
		require.Equal(t, 0, m.PointCount)
	})
}

func TestOracleCollideCapsulesManifold(t *testing.T) {
	t.Parallel()

	t.Run("parallel overlap two points", func(t *testing.T) {
		t.Parallel()
		// manifold.c:261 clip path. A (-0.5,0)-(0.5,0) r 0.25 under
		// B (-0.5,0.4)-(0.5,0.4) r 0.25: normal (0,1), both separations
		// 0.4 - 0.5 = -0.1, anchors midway at y = 0.2.
		a := box2d.Capsule{Center1: box2d.Vec2{X: -0.5, Y: 0}, Center2: box2d.Vec2{X: 0.5, Y: 0}, Radius: 0.25}
		b := box2d.Capsule{Center1: box2d.Vec2{X: -0.5, Y: 0.4}, Center2: box2d.Vec2{X: 0.5, Y: 0.4}, Radius: 0.25}
		m := box2d.CollideCapsules(&a, box2d.TransformIdentity, &b, box2d.TransformIdentity)
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-9)
		require.InDelta(t, -0.1, m.Points[0].Separation, 1e-9)
		require.InDelta(t, -0.1, m.Points[1].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: -0.5, Y: 0.2}, m.Points[0].AnchorA, 1e-9)
		requireVec(t, box2d.Vec2{X: 0.5, Y: 0.2}, m.Points[1].AnchorA, 1e-9)
		require.Equal(t, uint16(0), m.Points[0].ID)
		require.Equal(t, uint16(1), m.Points[1].ID)
	})

	t.Run("partial overlap clips to segment start", func(t *testing.T) {
		t.Parallel()
		// B shifted left so fp2 < 0 < fq2 triggers the clip-to-p1 lerp:
		// the lower contact clips to x=-1 (world), separations -0.1.
		a := box2d.Capsule{Center1: box2d.Vec2{X: -1, Y: 0}, Center2: box2d.Vec2{X: 1, Y: 0}, Radius: 0.2}
		b := box2d.Capsule{Center1: box2d.Vec2{X: -1.5, Y: 0.3}, Center2: box2d.Vec2{X: 0.5, Y: 0.3}, Radius: 0.2}
		m := box2d.CollideCapsules(&a, box2d.TransformIdentity, &b, box2d.TransformIdentity)
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-9)
		require.InDelta(t, -0.1, m.Points[0].Separation, 1e-9)
		require.InDelta(t, -0.1, m.Points[1].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: -1, Y: 0.15}, m.Points[0].AnchorA, 1e-9)
		requireVec(t, box2d.Vec2{X: 0.5, Y: 0.15}, m.Points[1].AnchorA, 1e-9)
	})

	t.Run("partial overlap clips to segment end", func(t *testing.T) {
		t.Parallel()
		// Mirror case with B's centers swapped so fq2 < 0 < fp2 clips cq
		// instead (manifold.c:261 second clip-to-p1 branch). Same geometry,
		// so the separations stay -0.1 with the clipped contact at x = -1.
		a := box2d.Capsule{Center1: box2d.Vec2{X: -1, Y: 0}, Center2: box2d.Vec2{X: 1, Y: 0}, Radius: 0.2}
		b := box2d.Capsule{Center1: box2d.Vec2{X: 0.5, Y: 0.3}, Center2: box2d.Vec2{X: -1.5, Y: 0.3}, Radius: 0.2}
		m := box2d.CollideCapsules(&a, box2d.TransformIdentity, &b, box2d.TransformIdentity)
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-9)
		require.InDelta(t, -0.1, m.Points[0].Separation, 1e-9)
		require.InDelta(t, -0.1, m.Points[1].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: 0.5, Y: 0.15}, m.Points[0].AnchorA, 1e-9)
		requireVec(t, box2d.Vec2{X: -1, Y: 0.15}, m.Points[1].AnchorA, 1e-9)
	})

	t.Run("partial overlap clips to segment far end", func(t *testing.T) {
		t.Parallel()
		// B overhangs q1 so fp2 > length1 > fq2 clips cp to x = 1
		// (manifold.c:261 clip-to-q1 branch).
		a := box2d.Capsule{Center1: box2d.Vec2{X: -1, Y: 0}, Center2: box2d.Vec2{X: 1, Y: 0}, Radius: 0.2}
		b := box2d.Capsule{Center1: box2d.Vec2{X: 1.5, Y: 0.3}, Center2: box2d.Vec2{X: -0.5, Y: 0.3}, Radius: 0.2}
		m := box2d.CollideCapsules(&a, box2d.TransformIdentity, &b, box2d.TransformIdentity)
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-9)
		require.InDelta(t, -0.1, m.Points[0].Separation, 1e-9)
		require.InDelta(t, -0.1, m.Points[1].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: 1, Y: 0.15}, m.Points[0].AnchorA, 1e-9)
		requireVec(t, box2d.Vec2{X: -0.5, Y: 0.15}, m.Points[1].AnchorA, 1e-9)
	})

	t.Run("tilted reference flips to capsule B", func(t *testing.T) {
		t.Parallel()
		// A is slightly tilted, B is flat and longer: B's SAT separation
		// (0.3) beats A's (~0.249), so B becomes the reference side
		// (manifold.c:261 separationB branch) and the normal is
		// -normalB = (0,1). Contacts are A's endpoints: separations
		// 0.4 - 0.4 = 0 and 0.3 - 0.4 = -0.1, anchors (-0.5,0.15) and
		// (0.5,0.2), ids makeID(0,0) and makeID(1,0).
		a := box2d.Capsule{Center1: box2d.Vec2{X: -0.5, Y: -0.05}, Center2: box2d.Vec2{X: 0.5, Y: 0.05}, Radius: 0.2}
		b := box2d.Capsule{Center1: box2d.Vec2{X: -1, Y: 0.35}, Center2: box2d.Vec2{X: 1, Y: 0.35}, Radius: 0.2}
		m := box2d.CollideCapsules(&a, box2d.TransformIdentity, &b, box2d.TransformIdentity)
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-9)
		require.InDelta(t, 0.0, m.Points[0].Separation, 1e-9)
		require.InDelta(t, -0.1, m.Points[1].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: -0.5, Y: 0.15}, m.Points[0].AnchorA, 1e-9)
		requireVec(t, box2d.Vec2{X: 0.5, Y: 0.2}, m.Points[1].AnchorA, 1e-9)
		require.Equal(t, uint16(0), m.Points[0].ID)
		require.Equal(t, uint16(256), m.Points[1].ID)
	})

	t.Run("cross at endpoint uses perpendicular fallback", func(t *testing.T) {
		t.Parallel()
		// B crosses A exactly at A's endpoint q1 = (1,0): both B endpoints
		// project at fp2 = fq2 = length1, so outsideA skips the clip, the
		// closest points coincide, and the degenerate normal falls back to
		// LeftPerp(u1) = (0,1) (manifold.c:261 single-point else branch).
		// Separation 0 - 0.2 = -0.2, id makeID(1,1) = 257 (f1=1, f2 interior).
		a := box2d.Capsule{Center1: box2d.Vec2{X: -1, Y: 0}, Center2: box2d.Vec2{X: 1, Y: 0}, Radius: 0.1}
		b := box2d.Capsule{Center1: box2d.Vec2{X: 1, Y: -0.5}, Center2: box2d.Vec2{X: 1, Y: 0.5}, Radius: 0.1}
		m := box2d.CollideCapsules(&a, box2d.TransformIdentity, &b, box2d.TransformIdentity)
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-9)
		require.InDelta(t, -0.2, m.Points[0].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: 1, Y: 0}, m.Points[0].AnchorA, 1e-9)
		require.Equal(t, uint16(257), m.Points[0].ID)
	})

	t.Run("perpendicular end contact single point", func(t *testing.T) {
		t.Parallel()
		// B is vertical above A's midpoint: both A endpoints project
		// outside B (outsideB), so the clip is skipped and the single-point
		// fallback runs: closest pair (0,0)-(0,0.3), separation
		// 0.3 - 0.4 = -0.1, id makeID(1,0) = 256 (f1 interior, f2 = 0).
		a := box2d.Capsule{Center1: box2d.Vec2{X: -1, Y: 0}, Center2: box2d.Vec2{X: 1, Y: 0}, Radius: 0.2}
		b := box2d.Capsule{Center1: box2d.Vec2{X: 0, Y: 0.3}, Center2: box2d.Vec2{X: 0, Y: 1.3}, Radius: 0.2}
		m := box2d.CollideCapsules(&a, box2d.TransformIdentity, &b, box2d.TransformIdentity)
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-9)
		require.InDelta(t, -0.1, m.Points[0].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: 0, Y: 0.15}, m.Points[0].AnchorA, 1e-9)
		require.Equal(t, uint16(256), m.Points[0].ID)
	})

	t.Run("far apart misses", func(t *testing.T) {
		t.Parallel()
		a := box2d.Capsule{Center1: box2d.Vec2{X: -1, Y: 0}, Center2: box2d.Vec2{X: 1, Y: 0}, Radius: 0.2}
		b := box2d.Capsule{Center1: box2d.Vec2{X: -1, Y: 1}, Center2: box2d.Vec2{X: 1, Y: 1}, Radius: 0.2}
		m := box2d.CollideCapsules(&a, box2d.TransformIdentity, &b, box2d.TransformIdentity)
		require.Equal(t, 0, m.PointCount)
	})
}

// ---------------------------------------------------------------------------
// manifold.go: polygons (vendored manifold.c:736, clip at manifold.c:545)
// ---------------------------------------------------------------------------

func TestOracleCollidePolygonsManifold(t *testing.T) {
	t.Parallel()

	t.Run("upstream overlapping boxes", func(t *testing.T) {
		t.Parallel()
		// Upstream LargeWorldManifoldTest origin block: two 1x1 boxes 0.9
		// apart overlap by 0.1. Hand trace of manifold.c:736/545: reference
		// edge A1 (normal (1,0)), incident edge B3, no clipping needed:
		// contacts at x = 0.45 (midpoint of the 0.1 overlap), separations
		// exactly -0.1, ids makeID(1,0)=256 and makeID(2,3)=515.
		boxA := box2d.MakeBox(0.5, 0.5)
		boxB := box2d.MakeBox(0.5, 0.5)
		m := box2d.CollidePolygons(&boxA, box2d.TransformIdentity, &boxB, xf(0.9, 0, 0))
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 1, Y: 0}, m.Normal, 1e-9)
		require.InDelta(t, -0.1, m.Points[0].Separation, 1e-9)
		require.InDelta(t, -0.1, m.Points[1].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: 0.45, Y: -0.5}, m.Points[0].AnchorA, 1e-9)
		requireVec(t, box2d.Vec2{X: 0.45, Y: 0.5}, m.Points[1].AnchorA, 1e-9)
		requireVec(t, box2d.Vec2{X: -0.45, Y: -0.5}, m.Points[0].AnchorB, 1e-9)
		require.Equal(t, uint16(256), m.Points[0].ID)
		require.Equal(t, uint16(515), m.Points[1].ID)
	})

	t.Run("far from origin matches origin", func(t *testing.T) {
		t.Parallel()
		// Upstream LargeWorldManifoldTest far block. NOTE upstream drift:
		// upstream main computes this through b2WorldTransform +
		// b2InvMulWorldTransforms under BOX2D_DOUBLE_PRECISION; the
		// vendored v3.2.0 API takes two b2Transforms. In exact arithmetic
		// the relative configuration is identical, so the manifold must
		// match the origin manifold; the float64 port keeps that to well
		// under the upstream 1e-4 tolerance.
		boxA := box2d.MakeBox(0.5, 0.5)
		boxB := box2d.MakeBox(0.5, 0.5)
		origin := box2d.CollidePolygons(&boxA, box2d.TransformIdentity, &boxB, xf(0.9, 0, 0))

		const d = 1.0e7
		far := box2d.CollidePolygons(&boxA, xf(d, d, 0), &boxB, xf(d+0.9, d, 0))
		require.Equal(t, origin.PointCount, far.PointCount)
		requireVec(t, origin.Normal, far.Normal, 1e-4)
		for i := range far.PointCount {
			require.InDelta(t, origin.Points[i].Separation, far.Points[i].Separation, 1e-4)
		}
	})

	t.Run("corner to corner single point", func(t *testing.T) {
		t.Parallel()
		// Vertex-vertex branch (manifold.c:736, fraction1==1 && fraction2==1
		// case): boxes offset (1.01,1.01) leave a 0.01*sqrt(2) corner gap,
		// the clip is disjoint, and the manifold is the single speculative
		// corner contact with diagonal normal and id makeID(2,0) = 512.
		boxA := box2d.MakeBox(0.5, 0.5)
		boxB := box2d.MakeBox(0.5, 0.5)
		m := box2d.CollidePolygons(&boxA, box2d.TransformIdentity, &boxB, xf(1.01, 1.01, 0))
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: invSqrt2, Y: invSqrt2}, m.Normal, 1e-6)
		require.InDelta(t, 0.01*math.Sqrt2, m.Points[0].Separation, 1e-6)
		requireVec(t, box2d.Vec2{X: 0.505, Y: 0.505}, m.Points[0].AnchorA, 1e-6)
		require.Equal(t, uint16(512), m.Points[0].ID)
	})

	t.Run("beyond speculative distance misses", func(t *testing.T) {
		t.Parallel()
		boxA := box2d.MakeBox(0.5, 0.5)
		boxB := box2d.MakeBox(0.5, 0.5)
		m := box2d.CollidePolygons(&boxA, box2d.TransformIdentity, &boxB, xf(1.05, 0, 0))
		require.Equal(t, 0, m.PointCount)
	})
}

// ---------------------------------------------------------------------------
// manifold.go: segment wrappers (manifold.c:532/538/1077/1083)
// ---------------------------------------------------------------------------

func TestOracleCollideSegmentManifolds(t *testing.T) {
	t.Parallel()

	segment := box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}}

	t.Run("segment and circle", func(t *testing.T) {
		t.Parallel()
		// manifold.c:1077 delegates to the capsule-circle path with radius
		// 0: circle (0,0.2) r 0.25: separation -0.05, normal (0,1).
		circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}
		m := box2d.CollideSegmentAndCircle(&segment, box2d.TransformIdentity, &circle, xf(0, 0.2, 0))
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-9)
		require.InDelta(t, -0.05, m.Points[0].Separation, 1e-9)
	})

	t.Run("segment and capsule", func(t *testing.T) {
		t.Parallel()
		// manifold.c:532 via b2CollideCapsules with radiusA = 0. Capsule
		// (-0.5,0.15)-(0.5,0.15) r 0.2 over the segment: two points with
		// separation 0.15 - 0.2 = -0.05 and anchors at y = -0.025 (midpoint
		// accounting for the one-sided radius).
		capsule := box2d.Capsule{
			Center1: box2d.Vec2{X: -0.5, Y: 0.15},
			Center2: box2d.Vec2{X: 0.5, Y: 0.15},
			Radius:  0.2,
		}
		m := box2d.CollideSegmentAndCapsule(&segment, box2d.TransformIdentity, &capsule, box2d.TransformIdentity)
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-9)
		require.InDelta(t, -0.05, m.Points[0].Separation, 1e-9)
		require.InDelta(t, -0.05, m.Points[1].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: -0.5, Y: -0.025}, m.Points[0].AnchorA, 1e-9)
		requireVec(t, box2d.Vec2{X: 0.5, Y: -0.025}, m.Points[1].AnchorA, 1e-9)
	})

	t.Run("segment and polygon", func(t *testing.T) {
		t.Parallel()
		// manifold.c:1083 treats the segment as a 2-gon. Box 0.8x0.8 with
		// its bottom face 0.01 above the segment: speculative face contact,
		// separations +0.01, normal (0,1), contacts at the box bottom
		// corners pushed to the midplane y = 0.005.
		b := box2d.MakeSquare(0.4)
		m := box2d.CollideSegmentAndPolygon(&segment, box2d.TransformIdentity, &b, xf(0, 0.41, 0))
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-9)
		require.InDelta(t, 0.01, m.Points[0].Separation, 1e-9)
		require.InDelta(t, 0.01, m.Points[1].Separation, 1e-9)
	})

	t.Run("polygon and capsule", func(t *testing.T) {
		t.Parallel()
		// manifold.c:538 converts the capsule to a rounded 2-gon. Box top
		// face y=0.5 under a horizontal capsule with surface y = 0.45:
		// separations 0.15 - 0.2 = -0.05, contacts midway at y = 0.475.
		b := box2d.MakeSquare(0.5)
		capsule := box2d.Capsule{
			Center1: box2d.Vec2{X: -0.3, Y: 0},
			Center2: box2d.Vec2{X: 0.3, Y: 0},
			Radius:  0.2,
		}
		m := box2d.CollidePolygonAndCapsule(&b, box2d.TransformIdentity, &capsule, xf(0, 0.65, 0))
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, m.Normal, 1e-9)
		require.InDelta(t, -0.05, m.Points[0].Separation, 1e-9)
		require.InDelta(t, -0.05, m.Points[1].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: 0.3, Y: 0.475}, m.Points[0].AnchorA, 1e-9)
		requireVec(t, box2d.Vec2{X: -0.3, Y: 0.475}, m.Points[1].AnchorA, 1e-9)
	})
}

// ---------------------------------------------------------------------------
// manifold.go: chain segments (manifold.c:1089/1177/1184/1319)
// ---------------------------------------------------------------------------

func TestOracleCollideChainSegmentAndCircleVoronoi(t *testing.T) {
	t.Parallel()

	t.Run("tail region admitted", func(t *testing.T) {
		t.Parallel()
		// manifold.c:1089 v <= 0 branch with uPrev > 0: ghost1 (-2,1) bends
		// the previous edge so the circle at (-1.05,-0.15) belongs to p1's
		// region: pA = (-1,0), distance sqrt(0.0025+0.0225) = sqrt(0.025).
		chain := box2d.ChainSegment{
			Ghost1:  box2d.Vec2{X: -2, Y: 1},
			Segment: box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}},
			Ghost2:  box2d.Vec2{X: 2, Y: 0},
		}
		circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}
		m := box2d.CollideChainSegmentAndCircle(&chain, box2d.TransformIdentity, &circle, xf(-1.05, -0.15, 0))
		require.Equal(t, 1, m.PointCount)
		d := math.Sqrt(0.025)
		require.InDelta(t, d-0.25, m.Points[0].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: -0.05 / d, Y: -0.15 / d}, m.Normal, 1e-9)
	})

	t.Run("head region admitted", func(t *testing.T) {
		t.Parallel()
		// u <= 0 with vNext <= 0 (ghost2 above): pA = p2 = (1,0), circle at
		// (1.05,-0.1): distance sqrt(0.0125).
		chain := box2d.ChainSegment{
			Ghost1:  box2d.Vec2{X: -2, Y: 0},
			Segment: box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}},
			Ghost2:  box2d.Vec2{X: 2, Y: 1},
		}
		circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}
		m := box2d.CollideChainSegmentAndCircle(&chain, box2d.TransformIdentity, &circle, xf(1.05, -0.1, 0))
		require.Equal(t, 1, m.PointCount)
		d := math.Sqrt(0.0125)
		require.InDelta(t, d-0.25, m.Points[0].Separation, 1e-9)
	})

	t.Run("interior beyond speculative distance misses", func(t *testing.T) {
		t.Parallel()
		// manifold.c:1089 interior branch speculative cull: distance 0.5,
		// separation 0.25 > 0.02.
		chain := box2d.ChainSegment{
			Ghost1:  box2d.Vec2{X: -2, Y: 0},
			Segment: box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}},
			Ghost2:  box2d.Vec2{X: 2, Y: 0},
		}
		circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}
		m := box2d.CollideChainSegmentAndCircle(&chain, box2d.TransformIdentity, &circle, xf(0, -0.5, 0))
		require.Equal(t, 0, m.PointCount)
	})

	t.Run("head region rejected by next edge", func(t *testing.T) {
		t.Parallel()
		// vNext > 0: ghost2 (2,-1) owns the region -> miss.
		chain := box2d.ChainSegment{
			Ghost1:  box2d.Vec2{X: -2, Y: 0},
			Segment: box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}},
			Ghost2:  box2d.Vec2{X: 2, Y: -1},
		}
		circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.25}
		m := box2d.CollideChainSegmentAndCircle(&chain, box2d.TransformIdentity, &circle, xf(1.05, -0.1, 0))
		require.Equal(t, 0, m.PointCount)
	})
}

func TestOracleCollideChainSegmentAndPolygonBranches(t *testing.T) {
	t.Parallel()

	collinear := box2d.ChainSegment{
		Ghost1:  box2d.Vec2{X: -2, Y: 0},
		Segment: box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}},
		Ghost2:  box2d.Vec2{X: 2, Y: 0},
	}

	t.Run("overlapping box face contact", func(t *testing.T) {
		t.Parallel()
		// manifold.c:1319 SAT path (overlap, behind1 false): box 0.8x0.8 at
		// (0,-0.3) pokes 0.1 above the segment. Segment-normal clip against
		// the box top edge: separations -0.1, normal (0,-1), anchors at
		// y = 0.05, ids makeID(0,3)=3 and makeID(1,2)=258.
		b := box2d.MakeSquare(0.4)
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndPolygon(&collinear, box2d.TransformIdentity, &b, xf(0, -0.3, 0), &cache)
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: -1}, m.Normal, 1e-9)
		require.InDelta(t, -0.1, m.Points[0].Separation, 1e-9)
		require.InDelta(t, -0.1, m.Points[1].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: -0.4, Y: 0.05}, m.Points[0].AnchorA, 1e-9)
		requireVec(t, box2d.Vec2{X: 0.4, Y: 0.05}, m.Points[1].AnchorA, 1e-9)
		require.Equal(t, uint16(3), m.Points[0].ID)
		require.Equal(t, uint16(258), m.Points[1].ID)
	})

	t.Run("wide box clips to segment ends", func(t *testing.T) {
		t.Parallel()
		// Box wider than the segment: b2ClipSegments (manifold.c:1184) hits
		// both lerp branches and clips the contact to x = -1 and x = 1.
		wide := box2d.MakeBox(2, 0.2)
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndPolygon(&collinear, box2d.TransformIdentity, &wide, xf(0, -0.1, 0), &cache)
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: -1}, m.Normal, 1e-9)
		require.InDelta(t, -0.1, m.Points[0].Separation, 1e-9)
		require.InDelta(t, -0.1, m.Points[1].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: -1, Y: 0.05}, m.Points[0].AnchorA, 1e-9)
		requireVec(t, box2d.Vec2{X: 1, Y: 0.05}, m.Points[1].AnchorA, 1e-9)
	})

	t.Run("box behind segment is culled", func(t *testing.T) {
		t.Parallel()
		b := box2d.MakeSquare(0.4)
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndPolygon(&collinear, box2d.TransformIdentity, &b, xf(0, 0.5, 0), &cache)
		require.Equal(t, 0, m.PointCount)
	})

	t.Run("box too far below misses", func(t *testing.T) {
		t.Parallel()
		b := box2d.MakeSquare(0.4)
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndPolygon(&collinear, box2d.TransformIdentity, &b, xf(0, -1, 0), &cache)
		require.Equal(t, 0, m.PointCount)
	})

	t.Run("corner past head snaps and clips away", func(t *testing.T) {
		t.Parallel()
		// Vertex-vertex normal at the collinear head is snapped
		// (b2ClassifyNormal head branch, convex2 false -> b2_normalSnap,
		// manifold.c:1319), then the segment-normal clip is disjoint
		// (box corner region starts at x=2.01 > upper1=2): no points.
		b := box2d.MakeSquare(0.1)
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndPolygon(&collinear, box2d.TransformIdentity, &b, xf(1.11, -0.11, 0), &cache)
		require.Equal(t, 0, m.PointCount)
	})

	t.Run("convex head admits corner contact", func(t *testing.T) {
		t.Parallel()
		// ghost2 (2,1) makes the head convex with normal2 = (1,-1)/sqrt2.
		// The corner-corner normal (0.01,-0.01)/|.| equals normal2, so
		// b2ClassifyNormal admits it: single point at pA = (1,0) with
		// separation 0.01*sqrt(2) and id makeID(1,3) = 259.
		chain := collinear
		chain.Ghost2 = box2d.Vec2{X: 2, Y: 1}
		b := box2d.MakeSquare(0.1)
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndPolygon(&chain, box2d.TransformIdentity, &b, xf(1.11, -0.11, 0), &cache)
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: invSqrt2, Y: -invSqrt2}, m.Normal, 1e-6)
		require.InDelta(t, 0.01*math.Sqrt2, m.Points[0].Separation, 1e-6)
		requireVec(t, box2d.Vec2{X: 1, Y: 0}, m.Points[0].AnchorA, 1e-9)
		require.Equal(t, uint16(259), m.Points[0].ID)
	})

	t.Run("convex head skips outside normal", func(t *testing.T) {
		t.Parallel()
		// Box corner nearly level with the segment: the contact normal
		// (0.016,-0.004)/|.| lies outside the [normal1, normal2] Gauss arc
		// so b2ClassifyNormal returns b2_normalSkip -> no manifold.
		chain := collinear
		chain.Ghost2 = box2d.Vec2{X: 2, Y: 1}
		b := box2d.MakeSquare(0.1)
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndPolygon(&chain, box2d.TransformIdentity, &b, xf(1.116, -0.104, 0), &cache)
		require.Equal(t, 0, m.PointCount)
	})

	t.Run("convex tail admits corner contact", func(t *testing.T) {
		t.Parallel()
		// Mirror of the head case: ghost1 (-2,1) makes the tail convex with
		// normal0 = (-1,-1)/sqrt2; the corner normal matches it exactly and
		// is admitted: id makeID(0,2) = 2 (segment point1, box vertex 2).
		chain := collinear
		chain.Ghost1 = box2d.Vec2{X: -2, Y: 1}
		b := box2d.MakeSquare(0.1)
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndPolygon(&chain, box2d.TransformIdentity, &b, xf(-1.11, -0.11, 0), &cache)
		require.Equal(t, 1, m.PointCount)
		requireVec(t, box2d.Vec2{X: -invSqrt2, Y: -invSqrt2}, m.Normal, 1e-6)
		require.InDelta(t, 0.01*math.Sqrt2, m.Points[0].Separation, 1e-6)
		requireVec(t, box2d.Vec2{X: -1, Y: 0}, m.Points[0].AnchorA, 1e-9)
		require.Equal(t, uint16(2), m.Points[0].ID)
	})

	t.Run("diamond vertex near face", func(t *testing.T) {
		t.Parallel()
		// Vertex-edge cache (2 points on A, 1 on B, ia1 != ia2): a diamond
		// whose top vertex sits 0.01 below the segment interior. The fall
		// through picks the polygon edge (v1,v2) and clips along the
		// segment normal: points at (-0.3,-0.155) and (0,-0.005) with
		// separations 0.31 and 0.01, ids makeID(0,2)=2, makeID(1,1)=257.
		var diamond box2d.Polygon
		diamond.Count = 4
		diamond.Radius = 0
		diamond.Vertices[0] = box2d.Vec2{X: 0.3, Y: -0.31}
		diamond.Vertices[1] = box2d.Vec2{X: 0, Y: -0.01}
		diamond.Vertices[2] = box2d.Vec2{X: -0.3, Y: -0.31}
		diamond.Vertices[3] = box2d.Vec2{X: 0, Y: -0.61}
		diamond.Normals[0] = box2d.Vec2{X: invSqrt2, Y: invSqrt2}
		diamond.Normals[1] = box2d.Vec2{X: -invSqrt2, Y: invSqrt2}
		diamond.Normals[2] = box2d.Vec2{X: -invSqrt2, Y: -invSqrt2}
		diamond.Normals[3] = box2d.Vec2{X: invSqrt2, Y: -invSqrt2}
		diamond.Centroid = box2d.Vec2{X: 0, Y: -0.31}

		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndPolygon(&collinear, box2d.TransformIdentity, &diamond, box2d.TransformIdentity, &cache)
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: -1}, m.Normal, 1e-9)
		require.InDelta(t, 0.31, m.Points[0].Separation, 1e-9)
		require.InDelta(t, 0.01, m.Points[1].Separation, 1e-9)
		requireVec(t, box2d.Vec2{X: -0.3, Y: -0.155}, m.Points[0].AnchorA, 1e-9)
		requireVec(t, box2d.Vec2{X: 0, Y: -0.005}, m.Points[1].AnchorA, 1e-9)
		require.Equal(t, uint16(2), m.Points[0].ID)
		require.Equal(t, uint16(257), m.Points[1].ID)
	})

	t.Run("endpoint above tilted face clips on polygon normal", func(t *testing.T) {
		t.Parallel()
		// Vertex-edge cache with one point on A (ia1 == ia2): segment
		// endpoint p2 = (1,0) hovers over the interior of a slightly tilted
		// box top face. The face slopes away from p1 so the endpoint is the
		// unique closest A feature. With a convex head (ghost2 (2,1),
		// normal2 = (1,-1)/sqrt2) the face normal is admitted by
		// b2ClassifyNormal and b2ClipSegments runs against the face
		// (manifold.c:1319 admit path):
		//   face normal n = (-sin 0.02, cos 0.02); world manifold normal is
		//   -n; contacts clip to p2 (separation dot(p2-a1, n) ~ 0.0129777)
		//   and to the face upper end (separation ~ 0.0160228); ids
		//   makeID(2,1) = 513 and makeID(3,0) = 768.
		// Tolerance note: b2MakeRot uses upstream's polynomial cosine/sine
		// approximation (math_functions.h b2ComputeCosSin), which is within
		// ~4e-4 of the exact sin/cos used in this hand derivation.
		chain := collinear
		chain.Ghost2 = box2d.Vec2{X: 2, Y: 1}
		face := box2d.MakeOffsetBox(0.3, 0.1, box2d.Vec2{X: 1.15, Y: -0.11}, box2d.MakeRot(0.02))
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndPolygon(&chain, box2d.TransformIdentity, &face, box2d.TransformIdentity, &cache)
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: math.Sin(0.02), Y: -math.Cos(0.02)}, m.Normal, 5e-4)
		require.InDelta(t, 0.0129777, m.Points[0].Separation, 5e-4)
		require.InDelta(t, 0.0160228, m.Points[1].Separation, 5e-4)
		require.Equal(t, uint16(513), m.Points[0].ID)
		require.Equal(t, uint16(768), m.Points[1].ID)
	})

	t.Run("endpoint above tilted face at tail clips on polygon normal", func(t *testing.T) {
		t.Parallel()
		// Mirror of the head case across x=0: convex tail (ghost1 (-2,1))
		// and a face tilted -0.02 rad left of p1. The incident-vertex
		// comparison takes the dot1 < dot2 branch this time and clips
		// against the same polygon edge: manifold normal
		// (-sin 0.02, -cos 0.02), separations mirrored, same ids. The same
		// b2ComputeCosSin approximation tolerance applies.
		chain := collinear
		chain.Ghost1 = box2d.Vec2{X: -2, Y: 1}
		face := box2d.MakeOffsetBox(0.3, 0.1, box2d.Vec2{X: -1.15, Y: -0.11}, box2d.MakeRot(-0.02))
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndPolygon(&chain, box2d.TransformIdentity, &face, box2d.TransformIdentity, &cache)
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: -math.Sin(0.02), Y: -math.Cos(0.02)}, m.Normal, 5e-4)
		require.InDelta(t, 0.0160232, m.Points[0].Separation, 5e-4)
		require.InDelta(t, 0.0129777, m.Points[1].Separation, 5e-4)
		require.Equal(t, uint16(513), m.Points[0].ID)
		require.Equal(t, uint16(768), m.Points[1].ID)
	})

	t.Run("endpoint above tilted face skips outside arc", func(t *testing.T) {
		t.Parallel()
		// Same configuration but the head is barely convex (ghost2
		// (2,0.012), normal2 tilt ~0.012 rad) while the face normal tilts
		// 0.03 rad: cross(normal2, normal) ~ sin(0.018) > 0.01, so
		// b2ClassifyNormal returns b2_normalSkip and the manifold is empty.
		chain := collinear
		chain.Ghost2 = box2d.Vec2{X: 2, Y: 0.012}
		face := box2d.MakeOffsetBox(0.3, 0.1, box2d.Vec2{X: 1.15, Y: -0.11}, box2d.MakeRot(0.03))
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndPolygon(&chain, box2d.TransformIdentity, &face, box2d.TransformIdentity, &cache)
		require.Equal(t, 0, m.PointCount)
	})

	t.Run("capsule against chain segment", func(t *testing.T) {
		t.Parallel()
		// manifold.c:1177 converts the capsule to a rounded 2-gon and runs
		// the polygon path: surface 0.15 below the axis at y = -0.05 gives
		// separations -0.05.
		capsule := box2d.Capsule{
			Center1: box2d.Vec2{X: -0.4, Y: -0.05},
			Center2: box2d.Vec2{X: 0.4, Y: -0.05},
			Radius:  0.15,
		}
		var cache box2d.SimplexCache
		m := box2d.CollideChainSegmentAndCapsule(&collinear, box2d.TransformIdentity, &capsule, box2d.TransformIdentity, &cache)
		require.Equal(t, 2, m.PointCount)
		requireVec(t, box2d.Vec2{X: 0, Y: -1}, m.Normal, 1e-9)
		require.InDelta(t, -0.1, m.Points[0].Separation, 1e-9)
		require.InDelta(t, -0.1, m.Points[1].Separation, 1e-9)
	})
}

// ---------------------------------------------------------------------------
// geometry.go: mover collisions (geometry.c:953-1060)
// ---------------------------------------------------------------------------

func TestOracleCollideMoverShapes(t *testing.T) {
	t.Parallel()

	mover := box2d.Capsule{
		Center1: box2d.Vec2{X: 0, Y: 0.5},
		Center2: box2d.Vec2{X: 0, Y: 1.5},
		Radius:  0.5,
	}

	t.Run("circle hit and miss", func(t *testing.T) {
		t.Parallel()
		// geometry.c:953: distance from the circle center to the mover
		// segment is 0.75; totalRadius 1.0: plane {(0,1), 0.25} with the
		// contact point at the circle center.
		circle := box2d.Circle{Center: box2d.Vec2{X: 0, Y: -0.25}, Radius: 0.5}
		res := box2d.CollideMoverAndCircle(&mover, &circle)
		require.True(t, res.Hit)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, res.Plane.Normal, 1e-9)
		require.InDelta(t, 0.25, res.Plane.Offset, 1e-9)
		requireVec(t, box2d.Vec2{X: 0, Y: -0.25}, res.Point, 1e-9)

		far := box2d.Circle{Center: box2d.Vec2{X: 0, Y: -1.2}, Radius: 0.5}
		res = box2d.CollideMoverAndCircle(&mover, &far)
		require.False(t, res.Hit)
	})

	t.Run("capsule hit and miss", func(t *testing.T) {
		t.Parallel()
		// geometry.c:980: segment-segment distance 0.5, totalRadius
		// 0.5+0.3: plane {(0,1), 0.3}, point on the shape segment (0,0).
		shape := box2d.Capsule{
			Center1: box2d.Vec2{X: -1, Y: 0},
			Center2: box2d.Vec2{X: 1, Y: 0},
			Radius:  0.3,
		}
		res := box2d.CollideMoverAndCapsule(&mover, &shape)
		require.True(t, res.Hit)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, res.Plane.Normal, 1e-9)
		require.InDelta(t, 0.3, res.Plane.Offset, 1e-9)
		requireVec(t, box2d.Vec2{X: 0, Y: 0}, res.Point, 1e-9)

		farMover := box2d.Capsule{
			Center1: box2d.Vec2{X: 0, Y: 1},
			Center2: box2d.Vec2{X: 0, Y: 2},
			Radius:  0.5,
		}
		res = box2d.CollideMoverAndCapsule(&farMover, &shape)
		require.False(t, res.Hit)
	})

	t.Run("polygon hit and miss", func(t *testing.T) {
		t.Parallel()
		// geometry.c:1007: box top y=0.5 to mover bottom center (0,0.8) is
		// 0.3; totalRadius = mover radius 0.4: plane {(0,1), 0.1}.
		shape := box2d.MakeSquare(0.5)
		closeMover := box2d.Capsule{
			Center1: box2d.Vec2{X: 0, Y: 0.8},
			Center2: box2d.Vec2{X: 0, Y: 1.8},
			Radius:  0.4,
		}
		res := box2d.CollideMoverAndPolygon(&closeMover, &shape)
		require.True(t, res.Hit)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, res.Plane.Normal, 1e-9)
		require.InDelta(t, 0.1, res.Plane.Offset, 1e-9)
		requireVec(t, box2d.Vec2{X: 0, Y: 0.5}, res.Point, 1e-9)

		res = box2d.CollideMoverAndPolygon(&mover, &box2d.Polygon{
			Vertices: shape.Vertices, Normals: shape.Normals,
			Centroid: shape.Centroid, Radius: 0, Count: 4,
		})
		// The default mover's segment endpoint (0,0.5) lies exactly on the
		// box top face, so the GJK distance is 0 and the plane offset is
		// the full totalRadius (geometry.c:1007: totalRadius - distance).
		require.True(t, res.Hit)
		require.InDelta(t, 0.5, res.Plane.Offset, 1e-9)

		farMover := box2d.Capsule{
			Center1: box2d.Vec2{X: 0, Y: 1.2},
			Center2: box2d.Vec2{X: 0, Y: 2.2},
			Radius:  0.4,
		}
		res = box2d.CollideMoverAndPolygon(&farMover, &shape)
		require.False(t, res.Hit)
	})

	t.Run("segment hit and miss", func(t *testing.T) {
		t.Parallel()
		// geometry.c:1034: totalRadius is only the mover radius. Mover
		// bottom center (0,0.3) over the segment: distance 0.3 <= 0.5:
		// plane {(0,1), 0.2}, point (0,0).
		shape := box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}}
		closeMover := box2d.Capsule{
			Center1: box2d.Vec2{X: 0, Y: 0.3},
			Center2: box2d.Vec2{X: 0, Y: 1.3},
			Radius:  0.5,
		}
		res := box2d.CollideMoverAndSegment(&closeMover, &shape)
		require.True(t, res.Hit)
		requireVec(t, box2d.Vec2{X: 0, Y: 1}, res.Plane.Normal, 1e-9)
		require.InDelta(t, 0.2, res.Plane.Offset, 1e-9)
		requireVec(t, box2d.Vec2{X: 0, Y: 0}, res.Point, 1e-9)

		farMover := box2d.Capsule{
			Center1: box2d.Vec2{X: 0, Y: 0.7},
			Center2: box2d.Vec2{X: 0, Y: 1.7},
			Radius:  0.5,
		}
		res = box2d.CollideMoverAndSegment(&farMover, &shape)
		require.False(t, res.Hit)
	})
}

// ---------------------------------------------------------------------------
// contact.go chain-segment registers via world simulation. The
// chainSegmentAndCapsuleManifold / chainSegmentAndPolygonManifold dispatch
// functions only run through b2UpdateContact-equivalent stepping, so drop
// dynamic bodies onto a chain floor. Expectations are physical invariants of
// the C solver (resting height = shape extent above the chain), not numeric
// constants read from the Go code.
// ---------------------------------------------------------------------------

func oracleChainFloorWorld(t *testing.T) *box2d.World {
	t.Helper()

	def := box2d.DefaultWorldDef()
	def.Gravity = box2d.Vec2{X: 0, Y: -10}
	w := box2d.NewWorld(&def)
	t.Cleanup(w.Destroy)

	// Points run from +x to -x so the one-sided collision normal points up
	// (chain normals point to the right of the segment direction).
	gd := box2d.DefaultBodyDef()
	ground := w.CreateBody(&gd)
	cd := box2d.DefaultChainDef()
	cd.Points = []box2d.Vec2{
		{X: 4, Y: 0}, {X: 2, Y: 0}, {X: -2, Y: 0}, {X: -4, Y: 0},
	}
	w.CreateChain(ground, &cd)
	return w
}

func TestOracleChainSegmentCapsuleWorldContact(t *testing.T) {
	t.Parallel()

	w := oracleChainFloorWorld(t)

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: 0, Y: 0.6}
	body := w.CreateBody(&bd)

	capsule := box2d.Capsule{
		Center1: box2d.Vec2{X: -0.25, Y: 0},
		Center2: box2d.Vec2{X: 0.25, Y: 0},
		Radius:  0.25,
	}
	sd := box2d.DefaultShapeDef()
	w.CreateCapsuleShape(body, &sd, &capsule)

	for range 120 {
		w.Step(1.0/60.0, 4)
	}

	// Resting height: chain surface y=0 plus the capsule radius.
	pos := w.BodyPosition(body)
	vel := w.BodyLinearVelocity(body)
	require.InDelta(t, 0.25, pos.Y, 0.015, "capsule rests on the chain")
	require.Less(t, math.Abs(vel.Y), 0.05)
}

func TestOracleChainSegmentPolygonWorldContact(t *testing.T) {
	t.Parallel()

	w := oracleChainFloorWorld(t)

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: 0.5, Y: 0.6}
	body := w.CreateBody(&bd)

	b := box2d.MakeSquare(0.25)
	sd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(body, &sd, &b)

	for range 120 {
		w.Step(1.0/60.0, 4)
	}

	pos := w.BodyPosition(body)
	vel := w.BodyLinearVelocity(body)
	require.InDelta(t, 0.25, pos.Y, 0.015, "box rests on the chain")
	require.Less(t, math.Abs(vel.Y), 0.05)
}
