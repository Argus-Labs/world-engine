// Tests for the float64 port of Box2D v3.2.0 src/geometry.c and src/aabb.c.

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// rot90 is an exact quarter turn. It avoids MakeRot, whose ComputeCosSin is an
// approximation, so expected values stay exact.
var rot90 = box2d.Rot{C: 0, S: 1}

func assertVec2(t *testing.T, want, got box2d.Vec2, delta float64) {
	t.Helper()
	assert.InDelta(t, want.X, got.X, delta)
	assert.InDelta(t, want.Y, got.Y, delta)
}

// ---------------------------------------------------------------------------
// Shape construction
// ---------------------------------------------------------------------------

func TestMakeBoxAndSquare(t *testing.T) {
	t.Parallel()

	box := box2d.MakeBox(2, 3)

	require.Equal(t, 4, box.Count)
	assert.Equal(t, box2d.Vec2{X: -2, Y: -3}, box.Vertices[0])
	assert.Equal(t, box2d.Vec2{X: 2, Y: -3}, box.Vertices[1])
	assert.Equal(t, box2d.Vec2{X: 2, Y: 3}, box.Vertices[2])
	assert.Equal(t, box2d.Vec2{X: -2, Y: 3}, box.Vertices[3])
	assert.Equal(t, box2d.Vec2{X: 0, Y: -1}, box.Normals[0])
	assert.Equal(t, box2d.Vec2{X: 1, Y: 0}, box.Normals[1])
	assert.Equal(t, box2d.Vec2{X: 0, Y: 1}, box.Normals[2])
	assert.Equal(t, box2d.Vec2{X: -1, Y: 0}, box.Normals[3])
	assert.Equal(t, box2d.Vec2{}, box.Centroid)
	assert.Zero(t, box.Radius)

	square := box2d.MakeSquare(1.5)
	assert.Equal(t, box2d.MakeBox(1.5, 1.5), square)

	rounded := box2d.MakeRoundedBox(2, 3, 0.25)
	assert.InDelta(t, 0.25, rounded.Radius, 0)
	assert.Equal(t, box.Vertices, rounded.Vertices)
	assert.Equal(t, box.Normals, rounded.Normals)
}

func TestMakeBoxOffset(t *testing.T) {
	t.Parallel()

	// Pure translation.
	box := box2d.MakeOffsetBox(1, 1, box2d.Vec2{X: 5, Y: -2}, box2d.RotIdentity)
	assert.Equal(t, box2d.Vec2{X: 4, Y: -3}, box.Vertices[0])
	assert.Equal(t, box2d.Vec2{X: 6, Y: -3}, box.Vertices[1])
	assert.Equal(t, box2d.Vec2{X: 6, Y: -1}, box.Vertices[2])
	assert.Equal(t, box2d.Vec2{X: 4, Y: -1}, box.Vertices[3])
	assert.Equal(t, box2d.Vec2{X: 5, Y: -2}, box.Centroid)
	assert.Equal(t, box2d.Vec2{X: 0, Y: -1}, box.Normals[0])

	// Quarter turn about the origin: (x,y) -> (-y,x).
	turned := box2d.MakeOffsetBox(2, 1, box2d.Vec2{}, rot90)
	assert.Equal(t, box2d.Vec2{X: 1, Y: -2}, turned.Vertices[0])
	assert.Equal(t, box2d.Vec2{X: 1, Y: 2}, turned.Vertices[1])
	assert.Equal(t, box2d.Vec2{X: -1, Y: 2}, turned.Vertices[2])
	assert.Equal(t, box2d.Vec2{X: -1, Y: -2}, turned.Vertices[3])
	assert.Equal(t, box2d.Vec2{X: 1, Y: 0}, turned.Normals[0])

	roundedOffset := box2d.MakeOffsetRoundedBox(2, 1, box2d.Vec2{}, rot90, 0.5)
	assert.InDelta(t, 0.5, roundedOffset.Radius, 0)
	assert.Equal(t, turned.Vertices, roundedOffset.Vertices)
}

func TestGeometryMakePolygonFromHull(t *testing.T) {
	t.Parallel()

	hull := box2d.ComputeHull([]box2d.Vec2{
		{X: -1, Y: -1}, {X: 1, Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1},
	})
	require.Equal(t, 4, hull.Count)

	poly := box2d.MakePolygon(&hull, 0.125)
	require.Equal(t, 4, poly.Count)
	assert.InDelta(t, 0.125, poly.Radius, 0)
	assert.Equal(t, box2d.MakeBox(1, 1).Vertices, poly.Vertices)
	assert.Equal(t, box2d.MakeBox(1, 1).Normals, poly.Normals)
	assertVec2(t, box2d.Vec2{}, poly.Centroid, 1e-15)

	offset := box2d.MakeOffsetPolygon(&hull, box2d.Vec2{X: 3, Y: 4}, box2d.RotIdentity)
	assert.InDelta(t, 0.0, offset.Radius, 0)
	assert.Equal(t, box2d.Vec2{X: 2, Y: 3}, offset.Vertices[0])
	assertVec2(t, box2d.Vec2{X: 3, Y: 4}, offset.Centroid, 1e-15)
}

func TestGeometryTransformPolygon(t *testing.T) {
	t.Parallel()

	box := box2d.MakeBox(2, 1)
	xf := box2d.Transform{P: box2d.Vec2{X: 1, Y: 1}, Q: rot90}
	got := box2d.TransformPolygon(xf, &box)

	require.Equal(t, 4, got.Count)
	assert.Equal(t, box2d.Vec2{X: 2, Y: -1}, got.Vertices[0])
	assert.Equal(t, box2d.Vec2{X: 2, Y: 3}, got.Vertices[1])
	assert.Equal(t, box2d.Vec2{X: 0, Y: 3}, got.Vertices[2])
	assert.Equal(t, box2d.Vec2{X: 0, Y: -1}, got.Vertices[3])
	assert.Equal(t, box2d.Vec2{X: 1, Y: 0}, got.Normals[0])
	assert.Equal(t, box2d.Vec2{X: 1, Y: 1}, got.Centroid)
}

func TestGeometryIsValidRay(t *testing.T) {
	t.Parallel()

	good := box2d.RayCastInput{
		Origin:      box2d.Vec2{X: 0, Y: 0},
		Translation: box2d.Vec2{X: 1, Y: 0},
		MaxFraction: 1,
	}
	assert.True(t, box2d.IsValidRay(&good))

	negative := good
	negative.MaxFraction = -0.1
	assert.False(t, box2d.IsValidRay(&negative))

	tooLarge := good
	tooLarge.MaxFraction = box2d.Huge
	assert.False(t, box2d.IsValidRay(&tooLarge))

	nan := good
	nan.Origin.X = math.NaN()
	assert.False(t, box2d.IsValidRay(&nan))

	inf := good
	inf.Translation.Y = math.Inf(1)
	assert.False(t, box2d.IsValidRay(&inf))
}

// ---------------------------------------------------------------------------
// Mass properties
// ---------------------------------------------------------------------------

func TestMassCircle(t *testing.T) {
	t.Parallel()

	const density = 1.5
	circle := box2d.Circle{Center: box2d.Vec2{X: 3, Y: -1}, Radius: 2}
	md := box2d.ComputeCircleMass(&circle, density)

	wantMass := density * box2d.Pi * 4.0
	assert.InDelta(t, wantMass, md.Mass, 1e-12)
	assert.Equal(t, circle.Center, md.Center)
	// I = m * r^2 / 2
	assert.InDelta(t, wantMass*0.5*4.0, md.RotationalInertia, 1e-12)
}

func TestMassBox(t *testing.T) {
	t.Parallel()

	const (
		density    = 3.0
		halfWidth  = 2.0
		halfHeight = 1.0
	)
	w, h := 2*halfWidth, 2*halfHeight

	box := box2d.MakeBox(halfWidth, halfHeight)
	md := box2d.ComputePolygonMass(&box, density)

	assert.InDelta(t, density*w*h, md.Mass, 1e-9)
	assertVec2(t, box2d.Vec2{}, md.Center, 1e-9)

	// Analytic rectangle inertia about its center: m * (w^2 + h^2) / 12.
	wantI := md.Mass * (w*w + h*h) / 12.0
	assert.InDelta(t, wantI, md.RotationalInertia, 1e-9)
}

func TestMassBoxOffset(t *testing.T) {
	t.Parallel()

	const density = 2.0
	center := box2d.Vec2{X: -4, Y: 7}
	box := box2d.MakeOffsetBox(1, 3, center, box2d.RotIdentity)
	md := box2d.ComputePolygonMass(&box, density)

	assert.InDelta(t, density*2.0*6.0, md.Mass, 1e-9)
	assertVec2(t, center, md.Center, 1e-9)

	// Inertia is about the center of mass, so the offset must not change it.
	wantI := md.Mass * (2.0*2.0 + 6.0*6.0) / 12.0
	assert.InDelta(t, wantI, md.RotationalInertia, 1e-9)
}

func TestMassCapsule(t *testing.T) {
	t.Parallel()

	const (
		density = 2.0
		radius  = 0.5
	)
	capsule := box2d.Capsule{
		Center1: box2d.Vec2{X: -1, Y: 0},
		Center2: box2d.Vec2{X: 1, Y: 0},
		Radius:  radius,
	}
	md := box2d.ComputeCapsuleMass(&capsule, density)

	length := 2.0
	wantMass := density*(box2d.Pi*radius*radius) + density*(2.0*radius*length)
	assert.InDelta(t, wantMass, md.Mass, 1e-12)
	assert.Positive(t, md.Mass)

	// Symmetric capsule: the centroid is the segment midpoint.
	assertVec2(t, box2d.Vec2{X: 0, Y: 0}, md.Center, 1e-15)
	assert.Positive(t, md.RotationalInertia)

	// Offset capsule keeps the midpoint property.
	offset := box2d.Capsule{
		Center1: box2d.Vec2{X: 2, Y: 3},
		Center2: box2d.Vec2{X: 6, Y: 3},
		Radius:  radius,
	}
	mdOffset := box2d.ComputeCapsuleMass(&offset, density)
	assertVec2(t, box2d.Vec2{X: 4, Y: 3}, mdOffset.Center, 1e-15)
}

func TestMassPolygonDegenerateCounts(t *testing.T) {
	t.Parallel()

	// count == 1 delegates to the circle formula.
	var one box2d.Polygon
	one.Count = 1
	one.Radius = 2
	one.Vertices[0] = box2d.Vec2{X: 1, Y: 1}
	circle := box2d.Circle{Center: one.Vertices[0], Radius: 2}
	assert.Equal(t, box2d.ComputeCircleMass(&circle, 1.0), box2d.ComputePolygonMass(&one, 1.0))

	// count == 2 delegates to the capsule formula.
	var two box2d.Polygon
	two.Count = 2
	two.Radius = 0.5
	two.Vertices[0] = box2d.Vec2{X: -1, Y: 0}
	two.Vertices[1] = box2d.Vec2{X: 1, Y: 0}
	capsule := box2d.Capsule{Center1: two.Vertices[0], Center2: two.Vertices[1], Radius: 0.5}
	assert.Equal(t, box2d.ComputeCapsuleMass(&capsule, 1.0), box2d.ComputePolygonMass(&two, 1.0))
}

func TestMassRoundedPolygonIsHeavier(t *testing.T) {
	t.Parallel()

	sharp := box2d.MakeBox(1, 1)
	rounded := box2d.MakeRoundedBox(1, 1, 0.25)

	mdSharp := box2d.ComputePolygonMass(&sharp, 1.0)
	mdRounded := box2d.ComputePolygonMass(&rounded, 1.0)

	// Upstream approximates rounded mass by pushing the vertices outwards.
	assert.Greater(t, mdRounded.Mass, mdSharp.Mass)
	assertVec2(t, box2d.Vec2{}, mdRounded.Center, 1e-9)
}

// ---------------------------------------------------------------------------
// AABB computation
// ---------------------------------------------------------------------------

func TestAABBComputeCircle(t *testing.T) {
	t.Parallel()

	circle := box2d.Circle{Center: box2d.Vec2{X: 1, Y: 2}, Radius: 0.5}

	got := box2d.ComputeCircleAABB(&circle, box2d.TransformIdentity)
	assert.Equal(t, box2d.AABB{
		LowerBound: box2d.Vec2{X: 0.5, Y: 1.5},
		UpperBound: box2d.Vec2{X: 1.5, Y: 2.5},
	}, got)

	// Quarter turn then translation: (1,2) -> (-2,1) -> (8,1).
	xf := box2d.Transform{P: box2d.Vec2{X: 10, Y: 0}, Q: rot90}
	got = box2d.ComputeCircleAABB(&circle, xf)
	assert.Equal(t, box2d.AABB{
		LowerBound: box2d.Vec2{X: 7.5, Y: 0.5},
		UpperBound: box2d.Vec2{X: 8.5, Y: 1.5},
	}, got)
}

func TestAABBComputeCapsule(t *testing.T) {
	t.Parallel()

	capsule := box2d.Capsule{
		Center1: box2d.Vec2{X: -1, Y: 0},
		Center2: box2d.Vec2{X: 1, Y: 0},
		Radius:  0.5,
	}

	got := box2d.ComputeCapsuleAABB(&capsule, box2d.TransformIdentity)
	assert.Equal(t, box2d.AABB{
		LowerBound: box2d.Vec2{X: -1.5, Y: -0.5},
		UpperBound: box2d.Vec2{X: 1.5, Y: 0.5},
	}, got)

	// Quarter turn: the capsule becomes vertical.
	xf := box2d.Transform{P: box2d.Vec2{X: 0, Y: 0}, Q: rot90}
	got = box2d.ComputeCapsuleAABB(&capsule, xf)
	assert.Equal(t, box2d.AABB{
		LowerBound: box2d.Vec2{X: -0.5, Y: -1.5},
		UpperBound: box2d.Vec2{X: 0.5, Y: 1.5},
	}, got)
}

func TestAABBComputePolygon(t *testing.T) {
	t.Parallel()

	box := box2d.MakeBox(1, 2)

	got := box2d.ComputePolygonAABB(&box, box2d.TransformIdentity)
	assert.Equal(t, box2d.AABB{
		LowerBound: box2d.Vec2{X: -1, Y: -2},
		UpperBound: box2d.Vec2{X: 1, Y: 2},
	}, got)

	// The polygon radius inflates the box.
	rounded := box2d.MakeRoundedBox(1, 2, 0.5)
	got = box2d.ComputePolygonAABB(&rounded, box2d.TransformIdentity)
	assert.Equal(t, box2d.AABB{
		LowerBound: box2d.Vec2{X: -1.5, Y: -2.5},
		UpperBound: box2d.Vec2{X: 1.5, Y: 2.5},
	}, got)

	// Quarter turn swaps the extents.
	xf := box2d.Transform{P: box2d.Vec2{X: 1, Y: 1}, Q: rot90}
	got = box2d.ComputePolygonAABB(&box, xf)
	assert.Equal(t, box2d.AABB{
		LowerBound: box2d.Vec2{X: -1, Y: 0},
		UpperBound: box2d.Vec2{X: 3, Y: 2},
	}, got)
}

func TestAABBComputeSegment(t *testing.T) {
	t.Parallel()

	segment := box2d.Segment{Point1: box2d.Vec2{X: 1, Y: 5}, Point2: box2d.Vec2{X: -2, Y: 3}}

	got := box2d.ComputeSegmentAABB(&segment, box2d.TransformIdentity)
	assert.Equal(t, box2d.AABB{
		LowerBound: box2d.Vec2{X: -2, Y: 3},
		UpperBound: box2d.Vec2{X: 1, Y: 5},
	}, got)

	xf := box2d.Transform{P: box2d.Vec2{X: 0, Y: -1}, Q: box2d.RotIdentity}
	got = box2d.ComputeSegmentAABB(&segment, xf)
	assert.Equal(t, box2d.AABB{
		LowerBound: box2d.Vec2{X: -2, Y: 2},
		UpperBound: box2d.Vec2{X: 1, Y: 4},
	}, got)
}

func TestAABBIsValid(t *testing.T) {
	t.Parallel()

	assert.True(t, box2d.IsValidAABB(box2d.AABB{
		LowerBound: box2d.Vec2{X: -1, Y: -1},
		UpperBound: box2d.Vec2{X: 1, Y: 1},
	}))

	// Degenerate but well ordered is still valid.
	assert.True(t, box2d.IsValidAABB(box2d.AABB{
		LowerBound: box2d.Vec2{X: 1, Y: 1},
		UpperBound: box2d.Vec2{X: 1, Y: 1},
	}))

	assert.False(t, box2d.IsValidAABB(box2d.AABB{
		LowerBound: box2d.Vec2{X: 1, Y: -1},
		UpperBound: box2d.Vec2{X: -1, Y: 1},
	}), "inverted x range")

	assert.False(t, box2d.IsValidAABB(box2d.AABB{
		LowerBound: box2d.Vec2{X: -1, Y: 1},
		UpperBound: box2d.Vec2{X: 1, Y: -1},
	}), "inverted y range")

	assert.False(t, box2d.IsValidAABB(box2d.AABB{
		LowerBound: box2d.Vec2{X: math.NaN(), Y: -1},
		UpperBound: box2d.Vec2{X: 1, Y: 1},
	}), "NaN bound")

	assert.False(t, box2d.IsValidAABB(box2d.AABB{
		LowerBound: box2d.Vec2{X: -1, Y: -1},
		UpperBound: box2d.Vec2{X: math.Inf(1), Y: 1},
	}), "infinite bound")
}

// ---------------------------------------------------------------------------
// Point containment
// ---------------------------------------------------------------------------

func TestPointInCircleShape(t *testing.T) {
	t.Parallel()

	circle := box2d.Circle{Center: box2d.Vec2{X: 1, Y: 1}, Radius: 2}

	assert.True(t, box2d.PointInCircle(&circle, box2d.Vec2{X: 1, Y: 1}), "center")
	assert.True(t, box2d.PointInCircle(&circle, box2d.Vec2{X: 2, Y: 1}), "inside")
	assert.True(t, box2d.PointInCircle(&circle, box2d.Vec2{X: 3, Y: 1}), "on the boundary is inside")
	assert.False(t, box2d.PointInCircle(&circle, box2d.Vec2{X: 3.001, Y: 1}), "just outside")
	assert.False(t, box2d.PointInCircle(&circle, box2d.Vec2{X: 10, Y: 10}), "far outside")
}

func TestPointInCapsuleShape(t *testing.T) {
	t.Parallel()

	capsule := box2d.Capsule{
		Center1: box2d.Vec2{X: -1, Y: 0},
		Center2: box2d.Vec2{X: 1, Y: 0},
		Radius:  1,
	}

	assert.True(t, box2d.PointInCapsule(&capsule, box2d.Vec2{X: 0, Y: 0}), "center")
	assert.True(t, box2d.PointInCapsule(&capsule, box2d.Vec2{X: 0, Y: 1}), "on the flat boundary")
	assert.False(t, box2d.PointInCapsule(&capsule, box2d.Vec2{X: 0, Y: 1.5}), "above the flat side")
	assert.True(t, box2d.PointInCapsule(&capsule, box2d.Vec2{X: 1.5, Y: 0}), "inside the end cap")
	assert.True(t, box2d.PointInCapsule(&capsule, box2d.Vec2{X: 2, Y: 0}), "on the end cap boundary")
	assert.False(t, box2d.PointInCapsule(&capsule, box2d.Vec2{X: 2.5, Y: 0}), "past the end cap")

	// A zero length capsule behaves as a circle.
	degenerate := box2d.Capsule{
		Center1: box2d.Vec2{X: 4, Y: 4},
		Center2: box2d.Vec2{X: 4, Y: 4},
		Radius:  1,
	}
	assert.True(t, box2d.PointInCapsule(&degenerate, box2d.Vec2{X: 4.5, Y: 4}))
	assert.False(t, box2d.PointInCapsule(&degenerate, box2d.Vec2{X: 6, Y: 4}))
}

// ---------------------------------------------------------------------------
// Ray casts
// ---------------------------------------------------------------------------

func TestRayCastCircleShape(t *testing.T) {
	t.Parallel()

	circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 1}

	t.Run("hit", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -3, Y: 0},
			Translation: box2d.Vec2{X: 6, Y: 0},
			MaxFraction: 1,
		}
		got := box2d.RayCastCircle(&circle, &input)
		require.True(t, got.Hit)
		assert.InDelta(t, 1.0/3.0, got.Fraction, 1e-14)
		assertVec2(t, box2d.Vec2{X: -1, Y: 0}, got.Normal, 1e-14)
		assertVec2(t, box2d.Vec2{X: -1, Y: 0}, got.Point, 1e-14)
	})

	t.Run("miss", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -3, Y: 2},
			Translation: box2d.Vec2{X: 6, Y: 0},
			MaxFraction: 1,
		}
		got := box2d.RayCastCircle(&circle, &input)
		assert.False(t, got.Hit)
		assert.Zero(t, got.Fraction)
	})

	t.Run("too short", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -3, Y: 0},
			Translation: box2d.Vec2{X: 1, Y: 0},
			MaxFraction: 1,
		}
		got := box2d.RayCastCircle(&circle, &input)
		assert.False(t, got.Hit, "the ray stops before reaching the circle")
	})

	t.Run("starts inside", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0, Y: 0},
			Translation: box2d.Vec2{X: 6, Y: 0},
			MaxFraction: 1,
		}
		got := box2d.RayCastCircle(&circle, &input)
		require.True(t, got.Hit, "initial overlap reports a hit")
		assert.Zero(t, got.Fraction)
		assert.Equal(t, input.Origin, got.Point)
	})

	t.Run("zero length ray inside", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0.5, Y: 0},
			Translation: box2d.Vec2{},
			MaxFraction: 1,
		}
		got := box2d.RayCastCircle(&circle, &input)
		require.True(t, got.Hit)
		assert.Equal(t, input.Origin, got.Point)
	})
}

func TestRayCastCapsuleShape(t *testing.T) {
	t.Parallel()

	capsule := box2d.Capsule{
		Center1: box2d.Vec2{X: -1, Y: 0},
		Center2: box2d.Vec2{X: 1, Y: 0},
		Radius:  1,
	}

	t.Run("hits the flat side", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0, Y: -3},
			Translation: box2d.Vec2{X: 0, Y: 6},
			MaxFraction: 1,
		}
		got := box2d.RayCastCapsule(&capsule, &input)
		require.True(t, got.Hit)
		assert.InDelta(t, 1.0/3.0, got.Fraction, 1e-14)
		assertVec2(t, box2d.Vec2{X: 0, Y: -1}, got.Normal, 1e-14)
		assertVec2(t, box2d.Vec2{X: 0, Y: -1}, got.Point, 1e-14)
	})

	t.Run("hits an end cap", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -5, Y: 0},
			Translation: box2d.Vec2{X: 10, Y: 0},
			MaxFraction: 1,
		}
		got := box2d.RayCastCapsule(&capsule, &input)
		require.True(t, got.Hit)
		// The cap is centered at (-1,0) with radius 1, so the surface is at x = -2.
		assertVec2(t, box2d.Vec2{X: -2, Y: 0}, got.Point, 1e-14)
		assertVec2(t, box2d.Vec2{X: -1, Y: 0}, got.Normal, 1e-14)
		assert.InDelta(t, 0.3, got.Fraction, 1e-14)
	})

	t.Run("miss", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0, Y: -3},
			Translation: box2d.Vec2{X: 0, Y: 1},
			MaxFraction: 1,
		}
		got := box2d.RayCastCapsule(&capsule, &input)
		assert.False(t, got.Hit)
	})

	t.Run("starts inside", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0, Y: 0},
			Translation: box2d.Vec2{X: 0, Y: 6},
			MaxFraction: 1,
		}
		got := box2d.RayCastCapsule(&capsule, &input)
		require.True(t, got.Hit)
		assert.Zero(t, got.Fraction)
		assert.Equal(t, input.Origin, got.Point)
	})

	t.Run("degenerate capsule is a circle", func(t *testing.T) {
		t.Parallel()
		degenerate := box2d.Capsule{
			Center1: box2d.Vec2{},
			Center2: box2d.Vec2{},
			Radius:  1,
		}
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -3, Y: 0},
			Translation: box2d.Vec2{X: 6, Y: 0},
			MaxFraction: 1,
		}
		circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 1}
		assert.Equal(t, box2d.RayCastCircle(&circle, &input), box2d.RayCastCapsule(&degenerate, &input))
	})
}

func TestRayCastSegmentShape(t *testing.T) {
	t.Parallel()

	segment := box2d.Segment{Point1: box2d.Vec2{X: -1, Y: 0}, Point2: box2d.Vec2{X: 1, Y: 0}}

	t.Run("hit from below", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0, Y: -2},
			Translation: box2d.Vec2{X: 0, Y: 4},
			MaxFraction: 1,
		}
		got := box2d.RayCastSegment(&segment, &input, false)
		require.True(t, got.Hit)
		assert.InDelta(t, 0.5, got.Fraction, 1e-15)
		assertVec2(t, box2d.Vec2{X: 0, Y: 0}, got.Point, 1e-15)
		assertVec2(t, box2d.Vec2{X: 0, Y: -1}, got.Normal, 1e-15)
	})

	t.Run("hit from above two-sided", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0, Y: 2},
			Translation: box2d.Vec2{X: 0, Y: -4},
			MaxFraction: 1,
		}
		got := box2d.RayCastSegment(&segment, &input, false)
		require.True(t, got.Hit)
		assert.InDelta(t, 0.5, got.Fraction, 1e-15)
		assertVec2(t, box2d.Vec2{X: 0, Y: 1}, got.Normal, 1e-15)
	})

	t.Run("one-sided skips the left side", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0, Y: 2},
			Translation: box2d.Vec2{X: 0, Y: -4},
			MaxFraction: 1,
		}
		assert.False(t, box2d.RayCastSegment(&segment, &input, true).Hit)

		fromRight := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0, Y: -2},
			Translation: box2d.Vec2{X: 0, Y: 4},
			MaxFraction: 1,
		}
		assert.True(t, box2d.RayCastSegment(&segment, &fromRight, true).Hit)
	})

	t.Run("misses past the end", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 5, Y: -2},
			Translation: box2d.Vec2{X: 0, Y: 4},
			MaxFraction: 1,
		}
		assert.False(t, box2d.RayCastSegment(&segment, &input, false).Hit)
	})

	t.Run("parallel misses", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -5, Y: 0},
			Translation: box2d.Vec2{X: 10, Y: 0},
			MaxFraction: 1,
		}
		assert.False(t, box2d.RayCastSegment(&segment, &input, false).Hit)
	})

	t.Run("zero length segment misses", func(t *testing.T) {
		t.Parallel()
		degenerate := box2d.Segment{Point1: box2d.Vec2{}, Point2: box2d.Vec2{}}
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0, Y: -2},
			Translation: box2d.Vec2{X: 0, Y: 4},
			MaxFraction: 1,
		}
		assert.False(t, box2d.RayCastSegment(&degenerate, &input, false).Hit)
	})
}

func TestRayCastPolygonShape(t *testing.T) {
	t.Parallel()

	box := box2d.MakeBox(1, 1)

	t.Run("hit", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -3, Y: 0},
			Translation: box2d.Vec2{X: 6, Y: 0},
			MaxFraction: 1,
		}
		got := box2d.RayCastPolygon(&box, &input)
		require.True(t, got.Hit)
		assert.InDelta(t, 1.0/3.0, got.Fraction, 1e-15)
		assert.Equal(t, box2d.Vec2{X: -1, Y: 0}, got.Normal)
		assertVec2(t, box2d.Vec2{X: -1, Y: 0}, got.Point, 1e-15)
	})

	t.Run("hit from below", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0, Y: -5},
			Translation: box2d.Vec2{X: 0, Y: 10},
			MaxFraction: 1,
		}
		got := box2d.RayCastPolygon(&box, &input)
		require.True(t, got.Hit)
		assert.InDelta(t, 0.4, got.Fraction, 1e-15)
		assert.Equal(t, box2d.Vec2{X: 0, Y: -1}, got.Normal)
		assertVec2(t, box2d.Vec2{X: 0, Y: -1}, got.Point, 1e-15)
	})

	t.Run("miss", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -3, Y: 3},
			Translation: box2d.Vec2{X: 6, Y: 0},
			MaxFraction: 1,
		}
		assert.False(t, box2d.RayCastPolygon(&box, &input).Hit)
	})

	t.Run("too short", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: -3, Y: 0},
			Translation: box2d.Vec2{X: 1, Y: 0},
			MaxFraction: 1,
		}
		assert.False(t, box2d.RayCastPolygon(&box, &input).Hit)
	})

	t.Run("starts inside", func(t *testing.T) {
		t.Parallel()
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0, Y: 0},
			Translation: box2d.Vec2{X: 6, Y: 0},
			MaxFraction: 1,
		}
		got := box2d.RayCastPolygon(&box, &input)
		require.True(t, got.Hit, "initial overlap reports a hit")
		assert.Zero(t, got.Fraction)
		assert.Equal(t, input.Origin, got.Point)
	})

	t.Run("offset polygon", func(t *testing.T) {
		t.Parallel()
		offset := box2d.MakeOffsetBox(1, 1, box2d.Vec2{X: 10, Y: 0}, box2d.RotIdentity)
		input := box2d.RayCastInput{
			Origin:      box2d.Vec2{X: 0, Y: 0},
			Translation: box2d.Vec2{X: 20, Y: 0},
			MaxFraction: 1,
		}
		got := box2d.RayCastPolygon(&offset, &input)
		require.True(t, got.Hit)
		assert.InDelta(t, 9.0/20.0, got.Fraction, 1e-15)
		assertVec2(t, box2d.Vec2{X: 9, Y: 0}, got.Point, 1e-14)
		assertVec2(t, box2d.Vec2{X: -1, Y: 0}, got.Normal, 1e-15)
	})
}

func TestRayCastAABB(t *testing.T) {
	t.Parallel()

	aabb := box2d.AABB{
		LowerBound: box2d.Vec2{X: -1, Y: -1},
		UpperBound: box2d.Vec2{X: 1, Y: 1},
	}

	t.Run("hit along x", func(t *testing.T) {
		t.Parallel()
		got := box2d.AABBRayCast(aabb, box2d.Vec2{X: -3, Y: 0}, box2d.Vec2{X: 3, Y: 0})
		require.True(t, got.Hit)
		assert.InDelta(t, 1.0/3.0, got.Fraction, 1e-15)
		assert.Equal(t, box2d.Vec2{X: -1, Y: 0}, got.Normal)
		assertVec2(t, box2d.Vec2{X: -1, Y: 0}, got.Point, 1e-15)
	})

	t.Run("hit along y", func(t *testing.T) {
		t.Parallel()
		// The ray spans 10 units and enters the box after 4 of them.
		got := box2d.AABBRayCast(aabb, box2d.Vec2{X: 0, Y: 5}, box2d.Vec2{X: 0, Y: -5})
		require.True(t, got.Hit)
		assert.InDelta(t, 0.4, got.Fraction, 1e-15)
		assert.Equal(t, box2d.Vec2{X: 0, Y: 1}, got.Normal)
		assertVec2(t, box2d.Vec2{X: 0, Y: 1}, got.Point, 1e-15)
	})

	t.Run("parallel miss", func(t *testing.T) {
		t.Parallel()
		got := box2d.AABBRayCast(aabb, box2d.Vec2{X: -3, Y: 2}, box2d.Vec2{X: 3, Y: 2})
		assert.False(t, got.Hit)
	})

	t.Run("diagonal miss", func(t *testing.T) {
		t.Parallel()
		got := box2d.AABBRayCast(aabb, box2d.Vec2{X: -3, Y: -3}, box2d.Vec2{X: -3, Y: 3})
		assert.False(t, got.Hit)
	})

	t.Run("starts inside is a miss", func(t *testing.T) {
		t.Parallel()
		got := box2d.AABBRayCast(aabb, box2d.Vec2{X: 0, Y: 0}, box2d.Vec2{X: 3, Y: 0})
		assert.False(t, got.Hit, "upstream rejects rays that start inside")
	})

	t.Run("stops short", func(t *testing.T) {
		t.Parallel()
		got := box2d.AABBRayCast(aabb, box2d.Vec2{X: -3, Y: 0}, box2d.Vec2{X: -2, Y: 0})
		assert.False(t, got.Hit)
	})
}
