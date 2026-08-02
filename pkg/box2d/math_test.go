// Tests for the float64 port of Box2D v3.2.0 math_functions.

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// Upstream documents Atan2 accuracy as ~0.0023 degrees.
const atan2Tol = 0.0023 * math.Pi / 180.0 * 1.05 // small slack over the documented bound

func TestAtan2(t *testing.T) {
	t.Parallel()

	assert.Zero(t, box2d.Atan2(0, 0), "upstream defines Atan2(0,0) = 0")

	for y := -10.0; y <= 10.0; y += 0.31 {
		for x := -10.0; x <= 10.0; x += 0.29 {
			if x == 0 && y == 0 {
				continue
			}
			got := box2d.Atan2(y, x)
			want := math.Atan2(y, x)
			assert.InDelta(t, want, got, atan2Tol, "Atan2(%v, %v)", y, x)
		}
	}

	// Axes.
	assert.InDelta(t, math.Pi/2, box2d.Atan2(1, 0), atan2Tol)
	assert.InDelta(t, -math.Pi/2, box2d.Atan2(-1, 0), atan2Tol)
	assert.InDelta(t, 0.0, box2d.Atan2(0, 1), atan2Tol)
	assert.InDelta(t, math.Pi, box2d.Atan2(0, -1), atan2Tol)
}

func TestComputeCosSin(t *testing.T) {
	t.Parallel()

	// Bhāskara approximation: absolute error is below ~2e-3 after
	// normalization.
	const tol = 2.5e-3
	for r := -30.0; r <= 30.0; r += 0.0137 {
		cs := box2d.ComputeCosSin(r)
		assert.InDelta(t, math.Cos(r), cs.Cosine, tol, "cos(%v)", r)
		assert.InDelta(t, math.Sin(r), cs.Sine, tol, "sin(%v)", r)

		// Result is normalized to the unit circle.
		mag := cs.Cosine*cs.Cosine + cs.Sine*cs.Sine
		assert.InDelta(t, 1.0, mag, 1e-12, "magnitude at %v", r)
	}
}

func TestUnwindAngle(t *testing.T) {
	t.Parallel()

	for r := -100.0; r <= 100.0; r += 0.517 {
		u := box2d.UnwindAngle(r)
		assert.LessOrEqual(t, u, box2d.Pi+1e-9)
		assert.GreaterOrEqual(t, u, -box2d.Pi-1e-9)
		// Same angle modulo 2*pi (distance to the nearest multiple).
		assert.InDelta(t, 0.0, math.Abs(math.Remainder(r-u, 2.0*box2d.Pi)), 1e-9)
	}
}

func TestMakeRotRoundTrip(t *testing.T) {
	t.Parallel()

	for a := -3.1; a <= 3.1; a += 0.0731 {
		q := box2d.MakeRot(a)
		require.True(t, box2d.IsNormalizedRot(q), "MakeRot(%v) not normalized", a)
		assert.InDelta(t, a, box2d.RotGetAngle(q), 5e-3, "angle round trip at %v", a)
	}
}

func TestVectorOps(t *testing.T) {
	t.Parallel()

	a := box2d.Vec2{X: 3, Y: -4}
	b := box2d.Vec2{X: -2, Y: 5}

	assert.Equal(t, box2d.Vec2{X: 1, Y: 1}, box2d.Add(a, b))
	assert.Equal(t, box2d.Vec2{X: 5, Y: -9}, box2d.Sub(a, b))
	assert.Equal(t, box2d.Vec2{X: -3, Y: 4}, box2d.Neg(a))
	assert.InDelta(t, -26.0, box2d.Dot(a, b), 0)
	assert.InDelta(t, 7.0, box2d.Cross(a, b), 0)
	assert.InDelta(t, 5.0, box2d.Length(a), 0)
	assert.InDelta(t, 25.0, box2d.LengthSquared(a), 0)

	// Lerp endpoints are exact.
	assert.Equal(t, a, box2d.Lerp(a, b, 0.0))
	assert.Equal(t, b, box2d.Lerp(a, b, 1.0))

	// Perp identities.
	assert.Equal(t, box2d.CrossSV(1.0, a), box2d.LeftPerp(a))
	assert.Equal(t, box2d.CrossVS(a, 1.0), box2d.RightPerp(a))

	// MulAdd / MulSub.
	assert.Equal(t, box2d.Vec2{X: 3 + 2*-2, Y: -4 + 2*5}, box2d.MulAdd(a, 2.0, b))
	assert.Equal(t, box2d.Vec2{X: 3 - 2*-2, Y: -4 - 2*5}, box2d.MulSub(a, 2.0, b))
}

func TestNormalize(t *testing.T) {
	t.Parallel()

	n := box2d.Normalize(box2d.Vec2{X: 3, Y: 4})
	assert.InDelta(t, 0.6, n.X, 1e-15)
	assert.InDelta(t, 0.8, n.Y, 1e-15)
	assert.True(t, box2d.IsNormalized(n))

	assert.Equal(t, box2d.Vec2{}, box2d.Normalize(box2d.Vec2{}), "zero vector normalizes to zero")

	u, length := box2d.GetLengthAndNormalize(box2d.Vec2{X: 3, Y: 4})
	assert.InDelta(t, 5.0, length, 1e-15)
	assert.Equal(t, n, u)
}

func TestRotOps(t *testing.T) {
	t.Parallel()

	q := box2d.MakeRot(0.7)
	r := box2d.MakeRot(-1.3)

	// q * inv(q) == identity.
	id := box2d.MulRot(q, box2d.InvertRot(q))
	assert.InDelta(t, 1.0, id.C, 1e-12)
	assert.InDelta(t, 0.0, id.S, 1e-12)

	// InvMulRot(a, b) == inv(a) * b.
	m1 := box2d.InvMulRot(q, r)
	m2 := box2d.MulRot(box2d.InvertRot(q), r)
	assert.InDelta(t, m2.C, m1.C, 1e-12)
	assert.InDelta(t, m2.S, m1.S, 1e-12)

	// Rotate / inverse-rotate round trip.
	v := box2d.Vec2{X: 2.5, Y: -1.5}
	w := box2d.InvRotateVector(q, box2d.RotateVector(q, v))
	assert.InDelta(t, v.X, w.X, 1e-12)
	assert.InDelta(t, v.Y, w.Y, 1e-12)

	// Axes.
	assert.Equal(t, box2d.Vec2{X: q.C, Y: q.S}, box2d.RotGetXAxis(q))
	assert.Equal(t, box2d.Vec2{X: -q.S, Y: q.C}, box2d.RotGetYAxis(q))

	// RelativeAngle recovers the angle difference.
	assert.InDelta(t, -2.0, box2d.RelativeAngle(q, r), 1e-2)

	// NLerp endpoints.
	n0 := box2d.NLerp(q, r, 0.0)
	assert.InDelta(t, q.C, n0.C, 1e-12)
	assert.InDelta(t, q.S, n0.S, 1e-12)

	// IntegrateRotation stays normalized.
	acc := box2d.RotIdentity
	for range 1000 {
		acc = box2d.IntegrateRotation(acc, 0.01)
	}
	assert.True(t, box2d.IsNormalizedRot(acc))

	// ComputeRotationBetweenUnitVectors.
	v1 := box2d.Vec2{X: 1, Y: 0}
	v2 := box2d.Normalize(box2d.Vec2{X: 1, Y: 1})
	rot := box2d.ComputeRotationBetweenUnitVectors(v1, v2)
	assert.InDelta(t, math.Pi/4, box2d.RotGetAngle(rot), 1e-3)
}

func TestTransformOps(t *testing.T) {
	t.Parallel()

	xf := box2d.Transform{P: box2d.Vec2{X: 1, Y: 2}, Q: box2d.MakeRot(0.5)}
	p := box2d.Vec2{X: -3, Y: 4}

	// Transform / inverse round trip.
	q := box2d.InvTransformPoint(xf, box2d.TransformPoint(xf, p))
	assert.InDelta(t, p.X, q.X, 1e-12)
	assert.InDelta(t, p.Y, q.Y, 1e-12)

	// MulTransforms is composition.
	xf2 := box2d.Transform{P: box2d.Vec2{X: -0.5, Y: 3}, Q: box2d.MakeRot(-1.1)}
	composed := box2d.MulTransforms(xf, xf2)
	want := box2d.TransformPoint(xf, box2d.TransformPoint(xf2, p))
	got := box2d.TransformPoint(composed, p)
	assert.InDelta(t, want.X, got.X, 1e-12)
	assert.InDelta(t, want.Y, got.Y, 1e-12)

	// InvMulTransforms undoes composition: InvMul(A, A*B) == B.
	rel := box2d.InvMulTransforms(xf, composed)
	assert.InDelta(t, xf2.P.X, rel.P.X, 1e-12)
	assert.InDelta(t, xf2.P.Y, rel.P.Y, 1e-12)
	assert.InDelta(t, xf2.Q.C, rel.Q.C, 1e-12)
	assert.InDelta(t, xf2.Q.S, rel.Q.S, 1e-12)
}

func TestMat22(t *testing.T) {
	t.Parallel()

	m := box2d.Mat22{CX: box2d.Vec2{X: 2, Y: 1}, CY: box2d.Vec2{X: -1, Y: 3}}
	b := box2d.Vec2{X: 5, Y: -2}

	x := box2d.Solve22(m, b)
	back := box2d.MulMV(m, x)
	assert.InDelta(t, b.X, back.X, 1e-12)
	assert.InDelta(t, b.Y, back.Y, 1e-12)

	inv := box2d.GetInverse22(m)
	viaInv := box2d.MulMV(inv, b)
	assert.InDelta(t, x.X, viaInv.X, 1e-12)
	assert.InDelta(t, x.Y, viaInv.Y, 1e-12)

	// Singular matrix yields zeros, not NaN (upstream behavior).
	sing := box2d.Mat22{CX: box2d.Vec2{X: 1, Y: 2}, CY: box2d.Vec2{X: 2, Y: 4}}
	s := box2d.Solve22(sing, b)
	assert.Equal(t, box2d.Vec2{}, s)
}

func TestAABBOps(t *testing.T) {
	t.Parallel()

	a := box2d.AABB{LowerBound: box2d.Vec2{X: -1, Y: -1}, UpperBound: box2d.Vec2{X: 2, Y: 3}}
	b := box2d.AABB{LowerBound: box2d.Vec2{X: 0, Y: 0}, UpperBound: box2d.Vec2{X: 1, Y: 1}}
	c := box2d.AABB{LowerBound: box2d.Vec2{X: 5, Y: 5}, UpperBound: box2d.Vec2{X: 6, Y: 6}}

	assert.True(t, box2d.AABBContains(a, b))
	assert.False(t, box2d.AABBContains(b, a))
	assert.True(t, box2d.AABBOverlaps(a, b))
	assert.False(t, box2d.AABBOverlaps(a, c))

	u := box2d.AABBUnion(a, c)
	assert.Equal(t, box2d.Vec2{X: -1, Y: -1}, u.LowerBound)
	assert.Equal(t, box2d.Vec2{X: 6, Y: 6}, u.UpperBound)

	assert.Equal(t, box2d.Vec2{X: 0.5, Y: 1}, box2d.AABBCenter(a))
	assert.Equal(t, box2d.Vec2{X: 1.5, Y: 2}, box2d.AABBExtents(a))

	pts := []box2d.Vec2{{X: 0, Y: 0}, {X: 2, Y: -1}, {X: 1, Y: 4}}
	m := box2d.MakeAABB(pts, 0.5)
	assert.Equal(t, box2d.Vec2{X: -0.5, Y: -1.5}, m.LowerBound)
	assert.Equal(t, box2d.Vec2{X: 2.5, Y: 4.5}, m.UpperBound)
}

func TestValidity(t *testing.T) {
	t.Parallel()

	nan := math.NaN()
	inf := math.Inf(1)

	assert.True(t, box2d.IsValidFloat(1.5))
	assert.False(t, box2d.IsValidFloat(nan))
	assert.False(t, box2d.IsValidFloat(inf))

	assert.True(t, box2d.IsValidVec2(box2d.Vec2{X: 1, Y: 2}))
	assert.False(t, box2d.IsValidVec2(box2d.Vec2{X: nan, Y: 2}))
	assert.False(t, box2d.IsValidVec2(box2d.Vec2{X: 1, Y: inf}))

	assert.True(t, box2d.IsValidRotation(box2d.MakeRot(1.0)))
	assert.False(t, box2d.IsValidRotation(box2d.Rot{C: 5, S: 5}), "unnormalized rotation is invalid")

	assert.True(t, box2d.IsValidTransform(box2d.TransformIdentity))
	assert.False(t, box2d.IsValidTransform(box2d.Transform{P: box2d.Vec2{X: nan}, Q: box2d.RotIdentity}))

	assert.True(t, box2d.IsValidPlane(box2d.Plane{Normal: box2d.Vec2{X: 0, Y: 1}, Offset: 2}))
	assert.False(t, box2d.IsValidPlane(box2d.Plane{Normal: box2d.Vec2{X: 0, Y: 2}, Offset: 2}))
}

func TestPlaneSeparation(t *testing.T) {
	t.Parallel()

	p := box2d.Plane{Normal: box2d.Vec2{X: 0, Y: 1}, Offset: 1}
	assert.InDelta(t, 1.0, box2d.PlaneSeparation(p, box2d.Vec2{X: 7, Y: 2}), 0)
	assert.InDelta(t, -1.0, box2d.PlaneSeparation(p, box2d.Vec2{X: -3, Y: 0}), 0)
}

func TestSpringDamper(t *testing.T) {
	t.Parallel()

	// Critically damped spring should converge toward zero position.
	position := 1.0
	velocity := 0.0
	const h = 1.0 / 60.0
	for range 600 {
		velocity = box2d.SpringDamper(5.0, 1.0, position, velocity, h)
		position += h * velocity
	}
	assert.InDelta(t, 0.0, position, 1e-3)
	assert.InDelta(t, 0.0, velocity, 1e-2)
}

func TestComputeAngularVelocity(t *testing.T) {
	t.Parallel()

	q1 := box2d.MakeRot(0.5)
	q2 := box2d.MakeRot(0.52)
	const h = 1.0 / 60.0
	omega := box2d.ComputeAngularVelocity(q1, q2, 1.0/h)
	assert.InDelta(t, 0.02/h, omega, 0.05)
}
