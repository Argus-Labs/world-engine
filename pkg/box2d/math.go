// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file include/box2d/math_functions.h.
// This port uses float64 where upstream uses float; all multiply-accumulate
// expressions are explicitly rounded (see math_fma.go).

package box2d

import "math"

// Pi matches the upstream B2_PI literal exactly (not math.Pi) so ported
// expressions stay diffable against the C source.
const Pi = 3.14159265359

// Vec2 is a 2D vector. It can represent a point or a free vector.
type Vec2 struct {
	X, Y float64
}

// CosSin is a cosine and sine pair produced by ComputeCosSin.
type CosSin struct {
	Cosine, Sine float64
}

// Rot is a 2D rotation, similar to using a complex number for rotation.
type Rot struct {
	C, S float64
}

// Transform is a 2D rigid transform.
type Transform struct {
	P Vec2
	Q Rot
}

// Mat22 is a 2-by-2 matrix stored as columns.
type Mat22 struct {
	CX, CY Vec2
}

// AABB is an axis-aligned bounding box.
type AABB struct {
	LowerBound, UpperBound Vec2
}

// Plane satisfies: separation = dot(normal, point) - offset.
type Plane struct {
	Normal Vec2
	Offset float64
}

// Identity and zero values (upstream b2Vec2_zero, b2Rot_identity, ...).
// Treat as read-only.
var (
	Vec2Zero          = Vec2{}
	RotIdentity       = Rot{C: 1.0}
	TransformIdentity = Transform{Q: Rot{C: 1.0}}
	Mat22Zero         = Mat22{}
)

// minInt returns the minimum of two integers (upstream b2MinInt).
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// maxInt returns the maximum of two integers (upstream b2MaxInt).
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// absInt returns the absolute value of an integer (upstream b2AbsInt).
//
//nolint:unused // upstream math_functions.h parity
func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// clampInt clamps an integer between a lower and upper bound (upstream b2ClampInt).
//
//nolint:unused // upstream math_functions.h parity
func clampInt(a, lower, upper int) int {
	if a < lower {
		return lower
	}
	if a > upper {
		return upper
	}
	return a
}

// ceilingInt returns ceil(numerator/denominator) for non-negative numerator
// and positive denominator (upstream b2CeilingInt).
//
//nolint:unused // upstream math_functions.h parity
func ceilingInt(numerator, denominator int) int {
	return (numerator + denominator - 1) / denominator
}

// minFloat returns the minimum of two floats (upstream b2MinFloat).
// NOTE: deliberately NOT the min builtin — C's ternary returns b when the
// comparison involves NaN, while Go's builtin propagates NaN.
func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// maxFloat returns the maximum of two floats (upstream b2MaxFloat).
func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// absFloat returns the absolute value of a float (upstream b2AbsFloat).
func absFloat(a float64) float64 {
	if a < 0 {
		return -a
	}
	return a
}

// clampFloat clamps a float between a lower and upper bound (upstream b2ClampFloat).
func clampFloat(a, lower, upper float64) float64 {
	if a < lower {
		return lower
	}
	if a > upper {
		return upper
	}
	return a
}

// Dot returns the vector dot product.
func Dot(a, b Vec2) float64 {
	return dot2(a.X, b.X, a.Y, b.Y)
}

// Cross returns the 2D vector cross product (a scalar).
func Cross(a, b Vec2) float64 {
	return cross2(a.X, b.Y, a.Y, b.X)
}

// CrossVS returns the cross product of a vector and a scalar (a vector).
func CrossVS(v Vec2, s float64) Vec2 {
	return Vec2{s * v.Y, -s * v.X}
}

// CrossSV returns the cross product of a scalar and a vector (a vector).
func CrossSV(s float64, v Vec2) Vec2 {
	return Vec2{-s * v.Y, s * v.X}
}

// LeftPerp returns a left-pointing perpendicular vector, equivalent to CrossSV(1, v).
func LeftPerp(v Vec2) Vec2 {
	return Vec2{-v.Y, v.X}
}

// RightPerp returns a right-pointing perpendicular vector, equivalent to CrossVS(v, 1).
func RightPerp(v Vec2) Vec2 {
	return Vec2{v.Y, -v.X}
}

// Add returns a + b.
func Add(a, b Vec2) Vec2 {
	return Vec2{a.X + b.X, a.Y + b.Y}
}

// Sub returns a - b.
func Sub(a, b Vec2) Vec2 {
	return Vec2{a.X - b.X, a.Y - b.Y}
}

// Neg returns -a.
func Neg(a Vec2) Vec2 {
	return Vec2{-a.X, -a.Y}
}

// Lerp returns the linear interpolation (1-t)*a + t*b.
func Lerp(a, b Vec2, t float64) Vec2 {
	omt := 1.0 - t
	return Vec2{dot2(omt, a.X, t, b.X), dot2(omt, a.Y, t, b.Y)}
}

// Mul returns the component-wise product of two vectors.
func Mul(a, b Vec2) Vec2 {
	return Vec2{a.X * b.X, a.Y * b.Y}
}

// MulSV returns the product of a scalar and a vector.
func MulSV(s float64, v Vec2) Vec2 {
	return Vec2{s * v.X, s * v.Y}
}

// MulAdd returns a + s*b.
func MulAdd(a Vec2, s float64, b Vec2) Vec2 {
	return Vec2{mulAdd(s, b.X, a.X), mulAdd(s, b.Y, a.Y)}
}

// MulSub returns a - s*b.
func MulSub(a Vec2, s float64, b Vec2) Vec2 {
	return Vec2{a.X - float64(s*b.X), a.Y - float64(s*b.Y)}
}

// Abs returns the component-wise absolute vector.
func Abs(a Vec2) Vec2 {
	return Vec2{absFloat(a.X), absFloat(a.Y)}
}

// Min returns the component-wise minimum vector.
func Min(a, b Vec2) Vec2 {
	return Vec2{minFloat(a.X, b.X), minFloat(a.Y, b.Y)}
}

// Max returns the component-wise maximum vector.
func Max(a, b Vec2) Vec2 {
	return Vec2{maxFloat(a.X, b.X), maxFloat(a.Y, b.Y)}
}

// Clamp clamps vector v component-wise into the range [a, b].
func Clamp(v, a, b Vec2) Vec2 {
	return Vec2{clampFloat(v.X, a.X, b.X), clampFloat(v.Y, a.Y, b.Y)}
}

// Length returns the length (norm) of the vector.
func Length(v Vec2) float64 {
	return math.Sqrt(dot2(v.X, v.X, v.Y, v.Y))
}

// Distance returns the distance between two points.
func Distance(a, b Vec2) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	return math.Sqrt(dot2(dx, dx, dy, dy))
}

// Normalize converts a vector into a unit vector if possible, otherwise
// returns the zero vector.
func Normalize(v Vec2) Vec2 {
	length := math.Sqrt(dot2(v.X, v.X, v.Y, v.Y))
	if length < epsilon {
		return Vec2{}
	}

	invLength := 1.0 / length
	return Vec2{invLength * v.X, invLength * v.Y}
}

// IsNormalized reports whether norm(a) == 1 within tolerance.
func IsNormalized(a Vec2) bool {
	aa := Dot(a, a)
	return absFloat(1.0-aa) < 100.0*epsilon
}

// GetLengthAndNormalize returns the unit vector of v (or the zero vector) and
// the length of v (upstream b2GetLengthAndNormalize with an out parameter).
func GetLengthAndNormalize(v Vec2) (Vec2, float64) {
	length := math.Sqrt(dot2(v.X, v.X, v.Y, v.Y))
	if length < epsilon {
		return Vec2{}, length
	}

	invLength := 1.0 / length
	return Vec2{invLength * v.X, invLength * v.Y}, length
}

// NormalizeRot normalizes a rotation.
func NormalizeRot(q Rot) Rot {
	mag := math.Sqrt(dot2(q.S, q.S, q.C, q.C))
	invMag := 0.0
	if mag > 0.0 {
		invMag = 1.0 / mag
	}
	return Rot{q.C * invMag, q.S * invMag}
}

// IntegrateRotation integrates a rotation from an angular displacement in
// radians and renormalizes.
func IntegrateRotation(q1 Rot, deltaAngle float64) Rot {
	// dc/dt = -omega * sin(t)
	// ds/dt = omega * cos(t)
	// c2 = c1 - omega * h * s1
	// s2 = s1 + omega * h * c1
	q2 := Rot{q1.C - float64(deltaAngle*q1.S), mulAdd(deltaAngle, q1.C, q1.S)}
	mag := math.Sqrt(dot2(q2.S, q2.S, q2.C, q2.C))
	invMag := 0.0
	if mag > 0.0 {
		invMag = 1.0 / mag
	}
	return Rot{q2.C * invMag, q2.S * invMag}
}

// LengthSquared returns the squared length of the vector.
func LengthSquared(v Vec2) float64 {
	return dot2(v.X, v.X, v.Y, v.Y)
}

// DistanceSquared returns the squared distance between two points.
func DistanceSquared(a, b Vec2) float64 {
	cx := b.X - a.X
	cy := b.Y - a.Y
	return dot2(cx, cx, cy, cy)
}

// MakeRot makes a rotation from an angle in radians.
func MakeRot(radians float64) Rot {
	cs := ComputeCosSin(radians)
	return Rot{cs.Cosine, cs.Sine}
}

// MakeRotFromUnitVector makes a rotation from a unit vector.
func MakeRotFromUnitVector(unitVector Vec2) Rot {
	assert(IsNormalized(unitVector))
	return Rot{unitVector.X, unitVector.Y}
}

// IsNormalizedRot reports whether the rotation is normalized.
func IsNormalizedRot(q Rot) bool {
	// Larger tolerance due to failure on mingw 32-bit (kept from upstream).
	qq := dot2(q.S, q.S, q.C, q.C)
	return 1.0-0.0006 < qq && qq < 1.0+0.0006
}

// InvertRot returns the inverse of a rotation.
func InvertRot(a Rot) Rot {
	return Rot{a.C, -a.S}
}

// NLerp returns the normalized linear interpolation of two rotations.
func NLerp(q1, q2 Rot, t float64) Rot {
	omt := 1.0 - t
	q := Rot{
		dot2(omt, q1.C, t, q2.C),
		dot2(omt, q1.S, t, q2.S),
	}

	mag := math.Sqrt(dot2(q.S, q.S, q.C, q.C))
	invMag := 0.0
	if mag > 0.0 {
		invMag = 1.0 / mag
	}
	return Rot{q.C * invMag, q.S * invMag}
}

// ComputeAngularVelocity computes the angular velocity necessary to rotate
// between two rotations over a given time (invH is the inverse time step).
func ComputeAngularVelocity(q1, q2 Rot, invH float64) float64 {
	// omega * h = s2 * c1 - c2 * s1 = sin(a2 - a1) ~= a2 - a1 for small delta
	return invH * cross2(q2.S, q1.C, q2.C, q1.S)
}

// RotGetAngle returns the angle in radians in the range [-pi, pi].
func RotGetAngle(q Rot) float64 {
	return Atan2(q.S, q.C)
}

// RotGetXAxis returns the x-axis of the rotation.
func RotGetXAxis(q Rot) Vec2 {
	return Vec2{q.C, q.S}
}

// RotGetYAxis returns the y-axis of the rotation.
func RotGetYAxis(q Rot) Vec2 {
	return Vec2{-q.S, q.C}
}

// MulRot multiplies two rotations: q * r.
func MulRot(q, r Rot) Rot {
	// s(q + r) = qs * rc + qc * rs
	// c(q + r) = qc * rc - qs * rs
	return Rot{
		S: dot2(q.S, r.C, q.C, r.S),
		C: cross2(q.C, r.C, q.S, r.S),
	}
}

// InvMulRot transpose-multiplies two rotations: inv(a) * b. This rotates a
// vector local in frame b into a vector local in frame a.
func InvMulRot(a, b Rot) Rot {
	// s(a - b) = ac * bs - as * bc
	// c(a - b) = ac * bc + as * bs
	return Rot{
		S: cross2(a.C, b.S, a.S, b.C),
		C: dot2(a.C, b.C, a.S, b.S),
	}
}

// RelativeAngle returns the relative angle between a and b.
func RelativeAngle(a, b Rot) float64 {
	// sin(b - a) = bs * ac - bc * as
	// cos(b - a) = bc * ac + bs * as
	s := cross2(a.C, b.S, a.S, b.C)
	c := dot2(a.C, b.C, a.S, b.S)
	return Atan2(s, c)
}

// UnwindAngle converts any angle into the range [-pi, pi].
func UnwindAngle(radians float64) float64 {
	// IEEE remainder is exact, hence deterministic.
	return math.Remainder(radians, 2.0*Pi)
}

// RotateVector rotates a vector.
func RotateVector(q Rot, v Vec2) Vec2 {
	return Vec2{cross2(q.C, v.X, q.S, v.Y), dot2(q.S, v.X, q.C, v.Y)}
}

// InvRotateVector inverse-rotates a vector.
func InvRotateVector(q Rot, v Vec2) Vec2 {
	return Vec2{dot2(q.C, v.X, q.S, v.Y), cross2(q.C, v.Y, q.S, v.X)}
}

// TransformPoint transforms a point (e.g. local space to world space).
func TransformPoint(t Transform, p Vec2) Vec2 {
	x := cross2(t.Q.C, p.X, t.Q.S, p.Y) + t.P.X
	y := dot2(t.Q.S, p.X, t.Q.C, p.Y) + t.P.Y
	return Vec2{x, y}
}

// InvTransformPoint inverse-transforms a point (e.g. world space to local space).
func InvTransformPoint(t Transform, p Vec2) Vec2 {
	vx := p.X - t.P.X
	vy := p.Y - t.P.Y
	return Vec2{dot2(t.Q.C, vx, t.Q.S, vy), cross2(t.Q.C, vy, t.Q.S, vx)}
}

// MulTransforms multiplies two transforms. If the result is applied to a
// point p local to frame B, the transform first converts p to a point local
// to frame A, then into a point in the world frame.
//
//	v2 = A.q.Rot(B.q.Rot(v1) + B.p) + A.p
//	   = (A.q * B.q).Rot(v1) + A.q.Rot(B.p) + A.p
func MulTransforms(a, b Transform) Transform {
	return Transform{
		Q: MulRot(a.Q, b.Q),
		P: Add(RotateVector(a.Q, b.P), a.P),
	}
}

// InvMulTransforms creates a transform that converts a local point in frame B
// to a local point in frame A.
//
//	v2 = A.q' * (B.q * v1 + B.p - A.p)
//	   = A.q' * B.q * v1 + A.q' * (B.p - A.p)
func InvMulTransforms(a, b Transform) Transform {
	return Transform{
		Q: InvMulRot(a.Q, b.Q),
		P: InvRotateVector(a.Q, Sub(b.P, a.P)),
	}
}

// MulMV multiplies a 2-by-2 matrix by a 2D vector.
func MulMV(a Mat22, v Vec2) Vec2 {
	return Vec2{
		dot2(a.CX.X, v.X, a.CY.X, v.Y),
		dot2(a.CX.Y, v.X, a.CY.Y, v.Y),
	}
}

// GetInverse22 returns the inverse of a 2-by-2 matrix.
func GetInverse22(m Mat22) Mat22 {
	a, b, c, d := m.CX.X, m.CY.X, m.CX.Y, m.CY.Y
	det := cross2(a, d, b, c)
	if det != 0.0 {
		det = 1.0 / det
	}

	return Mat22{
		CX: Vec2{det * d, -det * c},
		CY: Vec2{-det * b, det * a},
	}
}

// Solve22 solves A * x = b, where b is a column vector. This is more
// efficient than computing the inverse in one-shot cases.
func Solve22(m Mat22, b Vec2) Vec2 {
	a11, a12, a21, a22 := m.CX.X, m.CY.X, m.CX.Y, m.CY.Y
	det := cross2(a11, a22, a12, a21)
	if det != 0.0 {
		det = 1.0 / det
	}
	return Vec2{
		det * cross2(a22, b.X, a12, b.Y),
		det * cross2(a11, b.Y, a21, b.X),
	}
}

// AABBContains reports whether a fully contains b.
func AABBContains(a, b AABB) bool {
	s := true
	s = s && a.LowerBound.X <= b.LowerBound.X
	s = s && a.LowerBound.Y <= b.LowerBound.Y
	s = s && b.UpperBound.X <= a.UpperBound.X
	s = s && b.UpperBound.Y <= a.UpperBound.Y
	return s
}

// AABBCenter returns the center of the AABB.
func AABBCenter(a AABB) Vec2 {
	return Vec2{0.5 * (a.LowerBound.X + a.UpperBound.X), 0.5 * (a.LowerBound.Y + a.UpperBound.Y)}
}

// AABBExtents returns the extents (half-widths) of the AABB.
func AABBExtents(a AABB) Vec2 {
	return Vec2{0.5 * (a.UpperBound.X - a.LowerBound.X), 0.5 * (a.UpperBound.Y - a.LowerBound.Y)}
}

// AABBUnion returns the union of two AABBs.
func AABBUnion(a, b AABB) AABB {
	var c AABB
	c.LowerBound.X = minFloat(a.LowerBound.X, b.LowerBound.X)
	c.LowerBound.Y = minFloat(a.LowerBound.Y, b.LowerBound.Y)
	c.UpperBound.X = maxFloat(a.UpperBound.X, b.UpperBound.X)
	c.UpperBound.Y = maxFloat(a.UpperBound.Y, b.UpperBound.Y)
	return c
}

// AABBOverlaps reports whether a and b overlap.
func AABBOverlaps(a, b AABB) bool {
	return !(b.LowerBound.X > a.UpperBound.X || b.LowerBound.Y > a.UpperBound.Y ||
		a.LowerBound.X > b.UpperBound.X || a.LowerBound.Y > b.UpperBound.Y)
}

// MakeAABB computes the bounding box of an array of points inflated by radius.
func MakeAABB(points []Vec2, radius float64) AABB {
	assert(len(points) > 0)
	a := AABB{points[0], points[0]}
	for i := 1; i < len(points); i++ {
		a.LowerBound = Min(a.LowerBound, points[i])
		a.UpperBound = Max(a.UpperBound, points[i])
	}

	r := Vec2{radius, radius}
	a.LowerBound = Sub(a.LowerBound, r)
	a.UpperBound = Add(a.UpperBound, r)
	return a
}

// PlaneSeparation returns the signed separation of a point from a plane.
func PlaneSeparation(plane Plane, point Vec2) float64 {
	return Dot(plane.Normal, point) - plane.Offset
}

// SpringDamper is a one-dimensional mass-spring-damper simulation. It returns
// the new velocity given the position and time step. The new position is then
// position += timeStep * newVelocity. This drives towards a zero position
// with a stable implicit solution that needs no transcendental functions.
func SpringDamper(hertz, dampingRatio, position, velocity, timeStep float64) float64 {
	omega := 2.0 * Pi * hertz
	omegaH := omega * timeStep
	num := velocity - float64(float64(omega*omegaH)*position)
	den := 1.0 + float64(float64(2.0*dampingRatio)*omegaH) + float64(omegaH*omegaH)
	return num / den
}
