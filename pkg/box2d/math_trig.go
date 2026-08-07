// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/math_functions.c.
// This port uses float64 where upstream uses float; all multiply-accumulate
// expressions are explicitly rounded (see math_fma.go).

package box2d

import "math"

// epsilon mirrors upstream's use of FLT_EPSILON as a tolerance value. It is a
// tuning constant, not a precision artifact, so the float32 machine epsilon is
// kept even though this port computes in float64.
const epsilon = 1.19209290e-7 // FLT_EPSILON

// IsValidFloat reports whether a is a valid number (not NaN or infinity).
func IsValidFloat(a float64) bool {
	if math.IsNaN(a) {
		return false
	}
	if math.IsInf(a, 0) {
		return false
	}
	return true
}

// IsValidVec2 reports whether v is a valid vector (not NaN or infinity).
func IsValidVec2(v Vec2) bool {
	if math.IsNaN(v.X) || math.IsNaN(v.Y) {
		return false
	}
	if math.IsInf(v.X, 0) || math.IsInf(v.Y, 0) {
		return false
	}
	return true
}

// IsValidRotation reports whether q is a valid, normalized rotation.
func IsValidRotation(q Rot) bool {
	if math.IsNaN(q.S) || math.IsNaN(q.C) {
		return false
	}
	if math.IsInf(q.S, 0) || math.IsInf(q.C, 0) {
		return false
	}
	return IsNormalizedRot(q)
}

// IsValidTransform reports whether t is a valid transform with a normalized
// rotation.
func IsValidTransform(t Transform) bool {
	if !IsValidVec2(t.P) {
		return false
	}
	return IsValidRotation(t.Q)
}

// IsValidPlane reports whether the plane has a valid unit normal and offset.
func IsValidPlane(a Plane) bool {
	return IsValidVec2(a.Normal) && IsNormalized(a.Normal) && IsValidFloat(a.Offset)
}

// Atan2 computes an approximate arctangent in the range [-pi, pi]. This is
// hand coded for cross-platform determinism (the standard library atan2 is
// not guaranteed bit-identical across platforms). Accurate to around 0.0023
// degrees.
//
// The polynomial constants and the pi/2, pi literals are kept exactly as
// upstream wrote them (float32 roundings) for behavioral parity.
// https://stackoverflow.com/questions/46210708
func Atan2(y, x float64) float64 {
	// Added check for (0,0) to match atan2f and avoid NaN (upstream comment).
	if x == 0.0 && y == 0.0 {
		return 0.0
	}

	ax := absFloat(x)
	ay := absFloat(y)
	mx := maxFloat(ay, ax)
	mn := minFloat(ay, ax)
	a := mn / mx

	// Minimax polynomial approximation to atan(a) on [0,1]
	s := a * a
	c := s * a
	q := s * s
	r := mulAdd(0.024840285, q, 0.18681418)
	t := -float64(0.094097948*q) - 0.33213072
	r = mulAdd(r, s, t)
	r = mulAdd(r, c, a)

	// Map to full circle
	if ay > ax {
		r = 1.57079637 - r
	}
	if x < 0 {
		r = 3.14159274 - r
	}
	if y < 0 {
		r = -r
	}

	return r
}

// ComputeCosSin computes the cosine and sine of an angle in radians using
// Bhāskara I's approximation, implemented for cross-platform determinism.
// https://en.wikipedia.org/wiki/Bh%C4%81skara_I%27s_sine_approximation_formula
func ComputeCosSin(radians float64) CosSin {
	x := UnwindAngle(radians)
	pi2 := Pi * Pi

	// cosine needs angle in [-pi/2, pi/2]
	var c float64
	switch {
	case x < -0.5*Pi:
		y := x + Pi
		y2 := float64(y * y)
		c = -(pi2 - float64(4.0*y2)) / (pi2 + y2)
	case x > 0.5*Pi:
		y := x - Pi
		y2 := float64(y * y)
		c = -(pi2 - float64(4.0*y2)) / (pi2 + y2)
	default:
		y2 := float64(x * x)
		c = (pi2 - float64(4.0*y2)) / (pi2 + y2)
	}

	// sine needs angle in [0, pi]
	var s float64
	if x < 0.0 {
		y := x + Pi
		s = -float64(16.0*y) * (Pi - y) / (float64(5.0*pi2) - float64(float64(4.0*y)*(Pi-y)))
	} else {
		s = float64(16.0*x) * (Pi - x) / (float64(5.0*pi2) - float64(float64(4.0*x)*(Pi-x)))
	}

	mag := math.Sqrt(dot2(s, s, c, c))
	invMag := 0.0
	if mag > 0.0 {
		invMag = 1.0 / mag
	}
	return CosSin{Cosine: c * invMag, Sine: s * invMag}
}

// ComputeRotationBetweenUnitVectors computes the rotation between two unit
// vectors.
func ComputeRotationBetweenUnitVectors(v1, v2 Vec2) Rot {
	assert(absFloat(1.0-Length(v1)) < 100.0*epsilon)
	assert(absFloat(1.0-Length(v2)) < 100.0*epsilon)

	rot := Rot{
		C: Dot(v1, v2),
		S: Cross(v1, v2),
	}
	return NormalizeRot(rot)
}
