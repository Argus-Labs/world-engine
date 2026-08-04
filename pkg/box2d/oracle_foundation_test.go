// Oracle-based tests for the pkg/box2d foundations (math, ids, core, constants).
//
// Every expected value in this file comes from one of two oracles, never from
// running this Go port:
//
//   - The C source this package was ported from, Box2D v3.2.0
//     (include/box2d/math_functions.h, src/math_functions.c, include/box2d/id.h,
//     include/box2d/constants.h, src/core.c, src/timer.c). Numeric references
//     were produced by compiling and running that C, so they are float32-exact
//     values of the C algorithm.
//   - The upstream unit tests (test/test_math.c, test/test_id.c), whose cases
//     and tolerances are ported literally.
//
// This port computes in float64 where the C computes in float32, so a C
// reference value is only reproducible to within the C's own rounding. Each
// assertion carries a tolerance justified in a comment; upstream does the same
// thing with its ENSURE_SMALL macro (test/test_macros.h:55).

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// oracleFltEpsilon is FLT_EPSILON, the unit upstream test/test_math.c builds
// all of its transform tolerances from.
const oracleFltEpsilon = 1.19209290e-7

// oracleAtanTol is ATAN_TOL from upstream test/test_math.c:12 ("0.0023 degrees").
const oracleAtanTol = 0.00004

// oracleC32Tol is the default budget for comparing this float64 port against a
// value that the C oracle produced in float32. A float32 result carries a
// rounding error of about 2^-24 (6e-8) per operation, and the C references used
// here are the product of a handful of operations on operands of order 1..10,
// so 1e-6 is a comfortable but still meaningful bound. Rows that need more (a
// larger result magnitude, or angle unwinding, which amplifies the B2_PI
// representation difference documented in
// TestOracleUnwindAngle_B2PiConstantDivergence) carry their own tolerance.
const oracleC32Tol = 1e-6

// oracleFloatCase is one C-oracle reference value and the port's value for it.
type oracleFloatCase struct {
	name string
	want float64
	got  float64
	tol  float64
}

// checkOracleFloats asserts every case against its C reference value.
func checkOracleFloats(t *testing.T, cases []oracleFloatCase) {
	t.Helper()

	for _, c := range cases {
		assert.InDelta(t, c.want, c.got, c.tol, "%s", c.name)
	}
}

// ---------------------------------------------------------------------------
// Ports of upstream test/test_math.c (MathTest).
//
// Not portable from that file: the b2Pos / b2WorldTransform "large world"
// helpers exercised at test_math.c:177-226 (b2OffsetPos, b2SubPos, b2ToVec2,
// b2IsValidPosition, b2IsValidWorldTransform, b2TransformWorldPoint,
// b2InvTransformWorldPoint, b2InvMulWorldTransforms). Those were added upstream
// after v3.2.0 and have no counterpart in this port, so there is nothing to
// assert about them here.
// ---------------------------------------------------------------------------

// TestOracleMathTest_RotSweep ports test_math.c:16-44. It sweeps the angle
// B2_PI*t for t in [-10, 10) and checks the cosine/sine approximation, the
// unwind range, and the round trip through Atan2.
func TestOracleMathTest_RotSweep(t *testing.T) {
	t.Parallel()

	for tt := -10.0; tt < 10.0; tt += 0.01 {
		angle := box2d.Pi * tt
		r := box2d.MakeRot(angle)
		c := math.Cos(angle)
		s := math.Sin(angle)

		// test_math.c:23-26: "The cosine and sine approximations are accurate
		// to about 0.1 degrees (0.002 radians)". Running the same sweep through
		// the vendored C gives a worst case of 1.61e-3, so 2e-3 is the oracle's
		// own bound and is not slack invented here.
		assert.InDelta(t, c, r.C, 0.002, "MakeRot(%v).C", angle)
		assert.InDelta(t, s, r.S, 0.002, "MakeRot(%v).S", angle)

		// test_math.c:28-29.
		xn := box2d.UnwindAngle(angle)
		assert.GreaterOrEqual(t, xn, -box2d.Pi, "UnwindAngle(%v) below -pi", angle)
		assert.LessOrEqual(t, xn, box2d.Pi, "UnwindAngle(%v) above pi", angle)

		// test_math.c:31-43.
		a := box2d.Atan2(s, c)
		require.True(t, box2d.IsValidFloat(a), "Atan2(%v, %v) not finite", s, c)

		diff := math.Abs(a - xn)
		if diff > box2d.Pi {
			diff -= 2.0 * box2d.Pi
		}

		assert.InDelta(t, 0.0, diff, oracleAtanTol, "Atan2 round trip at angle %v", angle)
	}
}

// TestOracleMathTest_Atan2Grid ports test_math.c:46-56. The vendored C reaches a
// worst-case error of 2.77e-5 over this grid, inside the upstream ATAN_TOL.
func TestOracleMathTest_Atan2Grid(t *testing.T) {
	t.Parallel()

	for y := -1.0; y <= 1.0; y += 0.01 {
		for x := -1.0; x <= 1.0; x += 0.01 {
			a1 := box2d.Atan2(y, x)
			a2 := math.Atan2(y, x)
			require.True(t, box2d.IsValidFloat(a1), "Atan2(%v, %v) not finite", y, x)
			assert.InDelta(t, a2, a1, oracleAtanTol, "Atan2(%v, %v)", y, x)
		}
	}
}

// TestOracleMathTest_Atan2Axes ports the five axis cases at test_math.c:58-96.
func TestOracleMathTest_Atan2Axes(t *testing.T) {
	t.Parallel()

	pairs := [][2]float64{
		{1.0, 0.0},  // test_math.c:59
		{-1.0, 0.0}, // test_math.c:67
		{0.0, 1.0},  // test_math.c:75
		{0.0, -1.0}, // test_math.c:83
		{0.0, 0.0},  // test_math.c:91
	}

	for _, p := range pairs {
		a1 := box2d.Atan2(p[0], p[1])
		a2 := math.Atan2(p[0], p[1])
		require.True(t, box2d.IsValidFloat(a1), "Atan2(%v, %v) not finite", p[0], p[1])
		assert.InDelta(t, a2, a1, oracleAtanTol, "Atan2(%v, %v)", p[0], p[1])
	}
}

// TestOracleMathTest_VectorAlgebra ports test_math.c:98-109.
func TestOracleMathTest_VectorAlgebra(t *testing.T) {
	t.Parallel()

	zero := box2d.Vec2Zero
	one := box2d.Vec2{X: 1.0, Y: 1.0}
	two := box2d.Vec2{X: 2.0, Y: 2.0}

	v := box2d.Add(one, two)
	assert.InDelta(t, 3.0, v.X, 0.0, "Add(one, two).X")
	assert.InDelta(t, 3.0, v.Y, 0.0, "Add(one, two).Y")

	v = box2d.Sub(zero, two)
	assert.InDelta(t, -2.0, v.X, 0.0, "Sub(zero, two).X")
	assert.InDelta(t, -2.0, v.Y, 0.0, "Sub(zero, two).Y")

	// test_math.c:108-109 only asserts the components are not 5; asserting the
	// exact sum of 4 is the stronger statement and implies it.
	v = box2d.Add(two, two)
	assert.InDelta(t, 4.0, v.X, 0.0, "Add(two, two).X")
	assert.InDelta(t, 4.0, v.Y, 0.0, "Add(two, two).Y")
}

// TestOracleMathTest_TransformComposition ports test_math.c:111-127: composing
// two transforms must agree with applying them in sequence, and a transform
// followed by its inverse must be the identity.
func TestOracleMathTest_TransformComposition(t *testing.T) {
	t.Parallel()

	transform1 := box2d.Transform{P: box2d.Vec2{X: -2.0, Y: 3.0}, Q: box2d.MakeRot(1.0)}
	transform2 := box2d.Transform{P: box2d.Vec2{X: 1.0, Y: 0.0}, Q: box2d.MakeRot(-2.0)}
	two := box2d.Vec2{X: 2.0, Y: 2.0}

	transform := box2d.MulTransforms(transform2, transform1)
	v := box2d.TransformPoint(transform2, box2d.TransformPoint(transform1, two))
	u := box2d.TransformPoint(transform, two)

	// test_math.c:120-121.
	assert.InDelta(t, v.X, u.X, 10.0*oracleFltEpsilon, "MulTransforms associativity x")
	assert.InDelta(t, v.Y, u.Y, 10.0*oracleFltEpsilon, "MulTransforms associativity y")

	// test_math.c:123-127.
	v = box2d.TransformPoint(transform1, two)
	v = box2d.InvTransformPoint(transform1, v)
	assert.InDelta(t, two.X, v.X, 8.0*oracleFltEpsilon, "InvTransformPoint round trip x")
	assert.InDelta(t, two.Y, v.Y, 8.0*oracleFltEpsilon, "InvTransformPoint round trip y")
}

// TestOracleMathTest_RotationBetweenUnitVectors ports test_math.c:129-147.
func TestOracleMathTest_RotationBetweenUnitVectors(t *testing.T) {
	t.Parallel()

	v := box2d.Normalize(box2d.Vec2{X: 0.2, Y: -0.5})

	for y := -1.0; y <= 1.0; y += 0.01 {
		for x := -1.0; x <= 1.0; x += 0.01 {
			if x == 0.0 && y == 0.0 {
				continue
			}

			u := box2d.Normalize(box2d.Vec2{X: x, Y: y})
			if u == (box2d.Vec2{}) {
				// The float64 loop accumulator crosses zero at ~7.5e-16 rather
				// than exactly 0.0, so the x==0&&y==0 guard above (upstream's,
				// written for a float32 accumulator with a larger residual)
				// misses the singular point and Normalize returns the zero
				// vector — not a unit vector, so the comparison below would be
				// zero-against-zero and ComputeRotationBetweenUnitVectors'
				// unit-length precondition (asserted under box2d_asserts)
				// would be violated. Skip it like upstream intends.
				continue
			}
			r := box2d.ComputeRotationBetweenUnitVectors(v, u)
			w := box2d.RotateVector(r, v)

			// test_math.c:144-145.
			assert.InDelta(t, u.X, w.X, 4.0*oracleFltEpsilon, "rotation between (%v, %v) x", x, y)
			assert.InDelta(t, u.Y, w.Y, 4.0*oracleFltEpsilon, "rotation between (%v, %v) y", x, y)
		}
	}
}

// TestOracleMathTest_NLerp ports test_math.c:149-161. Upstream notes the NLerp
// of a b2Rot has an error of over 4 degrees, so it allows 5 degrees.
func TestOracleMathTest_NLerp(t *testing.T) {
	t.Parallel()

	q1 := box2d.RotIdentity
	q2 := box2d.MakeRot(0.5 * box2d.Pi)

	const n = 100

	for i := range n + 1 {
		alpha := float64(i) / float64(n)
		q := box2d.NLerp(q1, q2, alpha)
		angle := box2d.RotGetAngle(q)

		// test_math.c:159.
		assert.InDelta(t, alpha*0.5*box2d.Pi, angle, 5.0*box2d.Pi/180.0, "NLerp at alpha %v", alpha)
	}
}

// TestOracleMathTest_RelativeAngle ports test_math.c:163-175.
//
// The upstream tolerance of 0.1 degrees is tight: the vendored C reaches
// 1.66e-3 against a 1.75e-3 bound, so almost all of the headroom is consumed by
// the Bhaskara cosine/sine approximation rather than by float precision.
func TestOracleMathTest_RelativeAngle(t *testing.T) {
	t.Parallel()

	baseAngle := 0.75 * box2d.Pi
	q1 := box2d.MakeRot(baseAngle)
	tolerance := 0.1 * box2d.Pi / 180.0

	for tt := -10.0; tt < 10.0; tt += 0.01 {
		// The explicit conversion rounds the product before it reaches the
		// subtraction below, which both matches the C (where "float angle =
		// B2_PI * t" rounds to float32) and keeps TestNoFusedMultiplyAdd happy.
		angle := float64(box2d.Pi * tt)
		q2 := box2d.MakeRot(angle)

		relativeAngle := box2d.RelativeAngle(q1, q2)
		unwoundAngle := box2d.UnwindAngle(angle - baseAngle)

		assert.InDelta(t, unwoundAngle, relativeAngle, tolerance, "RelativeAngle at angle %v", angle)
	}
}

// ---------------------------------------------------------------------------
// C reference values (compiled and run from the vendored Box2D v3.2.0 sources).
// ---------------------------------------------------------------------------

// TestOracleComputeCosSin_CReference checks ComputeCosSin against the vendored
// src/math_functions.c:110-142 for a table of binary-exact angles.
func TestOracleComputeCosSin_CReference(t *testing.T) {
	t.Parallel()

	type row struct {
		radians float64
		cosine  float64
		sine    float64
		tol     float64
	}

	// The angles are all exactly representable in both float32 and float64, so
	// the only difference between the two runs is arithmetic rounding plus, for
	// |radians| > pi, the B2_PI representation difference that UnwindAngle
	// multiplies by the wrap count (see
	// TestOracleUnwindAngle_B2PiConstantDivergence).
	rows := []row{
		{-10.0, -0.838847935, 0.544365823, oracleC32Tol},
		{-3.5, -0.936012268, 0.351967514, oracleC32Tol},
		{-1.0, 0.540659428, -0.841241539, oracleC32Tol},
		{-0.5, 0.877261877, -0.480012119, oracleC32Tol},
		{0.0, 1.0, 0.0, oracleC32Tol},
		{0.5, 0.877261877, 0.480012119, oracleC32Tol},
		{1.0, 0.540659428, 0.841241539, oracleC32Tol},
		{2.5, -0.801000178, 0.598664224, oracleC32Tol},
		{3.5, -0.936012268, -0.351967514, oracleC32Tol},
		{7.0, 0.753837109, 0.657061338, oracleC32Tol},
		{10.0, -0.838847935, -0.544365823, oracleC32Tol},
		// 100 rad unwinds through 16 full turns, so the B2_PI difference is
		// amplified 16x (see the divergence test); 5e-6 covers it.
		{100.0, 0.862036467, -0.506846309, 5e-6},
	}

	for _, r := range rows {
		cs := box2d.ComputeCosSin(r.radians)
		assert.InDelta(t, r.cosine, cs.Cosine, r.tol, "ComputeCosSin(%v).Cosine", r.radians)
		assert.InDelta(t, r.sine, cs.Sine, r.tol, "ComputeCosSin(%v).Sine", r.radians)

		// b2MakeRot is defined as a thin wrapper over b2ComputeCosSin
		// (math_functions.h:380-384), so the same references apply.
		q := box2d.MakeRot(r.radians)
		assert.InDelta(t, r.cosine, q.C, r.tol, "MakeRot(%v).C", r.radians)
		assert.InDelta(t, r.sine, q.S, r.tol, "MakeRot(%v).S", r.radians)

		// Every rotation the approximation produces is renormalized
		// (math_functions.c:136-141), so it must satisfy b2IsNormalizedRot.
		assert.True(t, box2d.IsNormalizedRot(q), "MakeRot(%v) not normalized", r.radians)
	}
}

// TestOracleAtan2_CReference checks Atan2 against the vendored
// src/math_functions.c:68-105. The polynomial and the 1.57079637 / 3.14159274
// literals are carried over verbatim by this port, so the only difference is
// float32 versus float64 rounding.
func TestOracleAtan2_CReference(t *testing.T) {
	t.Parallel()

	checkOracleFloats(t, []oracleFloatCase{
		{"Atan2(1, 2)", 0.463634968, box2d.Atan2(1.0, 2.0), oracleC32Tol},
		{"Atan2(-1, 2)", -0.463634968, box2d.Atan2(-1.0, 2.0), oracleC32Tol},
		{"Atan2(1, -2)", 2.67795777, box2d.Atan2(1.0, -2.0), oracleC32Tol},
		{"Atan2(-1, -2)", -2.67795777, box2d.Atan2(-1.0, -2.0), oracleC32Tol},
		{"Atan2(0.5, 0.5)", 0.785425782, box2d.Atan2(0.5, 0.5), oracleC32Tol},
		{"Atan2(3, 0)", 1.57079637, box2d.Atan2(3.0, 0.0), oracleC32Tol},
		{"Atan2(-3, 0)", -1.57079637, box2d.Atan2(-3.0, 0.0), oracleC32Tol},
		{"Atan2(0, 4)", 0.0, box2d.Atan2(0.0, 4.0), oracleC32Tol},
		{"Atan2(0, -4)", 3.14159274, box2d.Atan2(0.0, -4.0), oracleC32Tol},
		{"Atan2(2, -3)", 2.5535953, box2d.Atan2(2.0, -3.0), oracleC32Tol},
		{"Atan2(-7.5, 0.25)", -1.53747535, box2d.Atan2(-7.5, 0.25), oracleC32Tol},
		// math_functions.c:71-74 short circuits (0, 0) to 0 to match atan2f.
		{"Atan2(0, 0)", 0.0, box2d.Atan2(0.0, 0.0), 0.0},
	})
}

// TestOracleUnwindAngle_CReference checks UnwindAngle (math_functions.h:508,
// remainderf against 2*B2_PI) for arguments that wrap at most once.
func TestOracleUnwindAngle_CReference(t *testing.T) {
	t.Parallel()

	checkOracleFloats(t, []oracleFloatCase{
		{"UnwindAngle(0)", 0.0, box2d.UnwindAngle(0.0), 0.0},
		{"UnwindAngle(1)", 1.0, box2d.UnwindAngle(1.0), 0.0},
		{"UnwindAngle(4)", -2.28318548, box2d.UnwindAngle(4.0), oracleC32Tol},
		{"UnwindAngle(-4)", 2.28318548, box2d.UnwindAngle(-4.0), oracleC32Tol},
		{"UnwindAngle(7)", 0.716814518, box2d.UnwindAngle(7.0), oracleC32Tol},
		{"UnwindAngle(-7)", -0.716814518, box2d.UnwindAngle(-7.0), oracleC32Tol},
	})
}

// b2PiRepresentationDelta is the amount by which the C's B2_PI exceeds this
// port's Pi. B2_PI is written 3.14159265359f in include/box2d/math_functions.h:79
// and, being a float, evaluates to 3.14159274101257324. This port's Pi
// (math.go:11) is the same decimal literal held as a float64, so it keeps its
// written value of 3.14159265359 instead.
const b2PiRepresentationDelta = 3.14159274101257324 - box2d.Pi

// TestOracleUnwindAngle_B2PiConstantDivergence documents a genuine divergence
// between this port and the C oracle.
//
// KNOWN DIVERGENCE. UnwindAngle is remainder(radians, 2*Pi). Because the C's
// B2_PI is 8.742257e-08 larger than this port's Pi, the modulus differs by
// 2*8.742257e-08 = 1.748451e-07, and the difference in the result grows with
// the number of full turns unwound. For radians = 100 that is 16 turns, so the
// results differ by 2.797e-06: the C returns -0.530967712 where this port
// returns approximately -0.530964915.
//
// The strict assertion below is written the way the C oracle demands and is
// skipped, exactly so the divergence is recorded rather than papered over.
// TestOracleUnwindAngle_B2PiDivergenceBound then asserts the analytic bound the
// port does satisfy, so the behaviour is still covered.
func TestOracleUnwindAngle_B2PiConstantDivergence(t *testing.T) {
	t.Parallel()

	t.Skipf("KNOWN DIVERGENCE: C B2_PI evaluates to 3.14159274101257324 (float) but this "+
		"port's Pi is the float64 literal 3.14159265359, a difference of %g; UnwindAngle "+
		"therefore differs from the C by wrapCount*%g, e.g. 2.797e-06 at radians=100",
		b2PiRepresentationDelta, 2.0*b2PiRepresentationDelta)

	// C reference: b2UnwindAngle(100.0f) = -0.530967712,
	// b2UnwindAngle(-100.0f) = 0.530967712 (src/math_functions.c via
	// math_functions.h:508).
	assert.InDelta(t, -0.530967712, box2d.UnwindAngle(100.0), oracleC32Tol, "UnwindAngle(100)")
	assert.InDelta(t, 0.530967712, box2d.UnwindAngle(-100.0), oracleC32Tol, "UnwindAngle(-100)")
}

// TestOracleUnwindAngle_B2PiDivergenceBound asserts that the divergence above is
// exactly the predicted constant-representation effect and nothing more. The
// bound is derived analytically from the two Pi values, not measured from this
// port.
func TestOracleUnwindAngle_B2PiDivergenceBound(t *testing.T) {
	t.Parallel()

	// 100 / (2*pi) = 15.9155..., so remainder unwinds 16 full turns.
	const wrapCount = 16.0

	// Predicted difference plus one float64-scale allowance for the arithmetic.
	tol := wrapCount*2.0*b2PiRepresentationDelta + 1e-12

	assert.InDelta(t, -0.530967712, box2d.UnwindAngle(100.0), tol, "UnwindAngle(100)")
	assert.InDelta(t, 0.530967712, box2d.UnwindAngle(-100.0), tol, "UnwindAngle(-100)")
}

// TestOracleVec2Ops_CReference checks the vector helpers of
// include/box2d/math_functions.h against the C.
func TestOracleVec2Ops_CReference(t *testing.T) {
	t.Parallel()

	lerp := box2d.Lerp(box2d.Vec2{X: -1.0, Y: 2.0}, box2d.Vec2{X: 3.0, Y: -4.0}, 0.25)
	mulAdd := box2d.MulAdd(box2d.Vec2{X: 1.0, Y: 2.0}, 3.0, box2d.Vec2{X: -0.5, Y: 0.25})
	mulSub := box2d.MulSub(box2d.Vec2{X: 1.0, Y: 2.0}, 3.0, box2d.Vec2{X: -0.5, Y: 0.25})
	mul := box2d.Mul(box2d.Vec2{X: 1.5, Y: -2.0}, box2d.Vec2{X: -4.0, Y: 0.25})
	clamp := box2d.Clamp(
		box2d.Vec2{X: -3.0, Y: 7.0},
		box2d.Vec2{X: -1.0, Y: -1.0},
		box2d.Vec2{X: 1.0, Y: 1.0},
	)

	checkOracleFloats(t, []oracleFloatCase{
		// math_functions.h:225-228.
		{"Lerp.x", 0.0, lerp.X, 0.0},
		{"Lerp.y", 0.5, lerp.Y, 0.0},
		// math_functions.h:243-247.
		{"MulAdd.x", -0.5, mulAdd.X, 0.0},
		{"MulAdd.y", 2.75, mulAdd.Y, 0.0},
		// math_functions.h:249-253.
		{"MulSub.x", 2.5, mulSub.X, 0.0},
		{"MulSub.y", 1.25, mulSub.Y, 0.0},
		// math_functions.h:231-234, component-wise product: (1.5*-4, -2*0.25).
		{"Mul.x", -6.0, mul.X, 0.0},
		{"Mul.y", -0.5, mul.Y, 0.0},
		// math_functions.h:282-288, component-wise clamp into [-1, 1].
		{"Clamp.x", -1.0, clamp.X, 0.0},
		{"Clamp.y", 1.0, clamp.Y, 0.0},
		// Lengths and products.
		{"Length(3,4)", 5.0, box2d.Length(box2d.Vec2{X: 3.0, Y: 4.0}), 0.0},
		{"LengthSquared(3,4)", 25.0, box2d.LengthSquared(box2d.Vec2{X: 3.0, Y: 4.0}), 0.0},
		{
			"Distance((1,2),(4,6))",
			5.0,
			box2d.Distance(box2d.Vec2{X: 1.0, Y: 2.0}, box2d.Vec2{X: 4.0, Y: 6.0}),
			0.0,
		},
		{
			"DistanceSquared((1,2),(4,6))",
			25.0,
			box2d.DistanceSquared(box2d.Vec2{X: 1.0, Y: 2.0}, box2d.Vec2{X: 4.0, Y: 6.0}),
			0.0,
		},
		{"Dot((1,2),(3,4))", 11.0, box2d.Dot(box2d.Vec2{X: 1.0, Y: 2.0}, box2d.Vec2{X: 3.0, Y: 4.0}), 0.0},
		{"Cross((1,2),(3,4))", -2.0, box2d.Cross(box2d.Vec2{X: 1.0, Y: 2.0}, box2d.Vec2{X: 3.0, Y: 4.0}), 0.0},
		// math_functions.h b2PlaneSeparation: dot((0,1),(5,3.5)) - 2.
		{
			"PlaneSeparation",
			1.5,
			box2d.PlaneSeparation(
				box2d.Plane{Normal: box2d.Vec2{X: 0.0, Y: 1.0}, Offset: 2.0},
				box2d.Vec2{X: 5.0, Y: 3.5},
			),
			0.0,
		},
	})
}

// TestOracleNormalize_CReference checks b2Normalize and
// b2GetLengthAndNormalize (math_functions.h) against the C, including the
// below-FLT_EPSILON short circuit that returns the zero vector.
func TestOracleNormalize_CReference(t *testing.T) {
	t.Parallel()

	type row struct {
		name    string
		in      box2d.Vec2
		wantX   float64
		wantY   float64
		wantLen float64
		tol     float64
	}

	rows := []row{
		{"(3,4)", box2d.Vec2{X: 3.0, Y: 4.0}, 0.600000024, 0.800000012, 5.0, oracleC32Tol},
		{"(0.2,-0.5)", box2d.Vec2{X: 0.2, Y: -0.5}, 0.3713907, -0.928476751, 0.538516462, oracleC32Tol},
		{"(-1,0)", box2d.Vec2{X: -1.0, Y: 0.0}, -1.0, 0.0, 1.0, 0.0},
		// Below FLT_EPSILON, so math_functions.h returns the zero vector but
		// still reports the true length.
		{"(1e-9,0)", box2d.Vec2{X: 1e-9, Y: 0.0}, 0.0, 0.0, 1e-9, 1e-15},
		{"(0,0)", box2d.Vec2Zero, 0.0, 0.0, 0.0, 0.0},
		{"(-7.5,2.25)", box2d.Vec2{X: -7.5, Y: 2.25}, -0.957826316, 0.287347913, 7.83022976, oracleC32Tol},
	}

	for _, r := range rows {
		n := box2d.Normalize(r.in)
		assert.InDelta(t, r.wantX, n.X, r.tol, "Normalize%s.X", r.name)
		assert.InDelta(t, r.wantY, n.Y, r.tol, "Normalize%s.Y", r.name)

		u, length := box2d.GetLengthAndNormalize(r.in)
		assert.InDelta(t, r.wantX, u.X, r.tol, "GetLengthAndNormalize%s unit X", r.name)
		assert.InDelta(t, r.wantY, u.Y, r.tol, "GetLengthAndNormalize%s unit Y", r.name)
		assert.InDelta(t, r.wantLen, length, r.tol, "GetLengthAndNormalize%s length", r.name)
	}
}

// TestOracleRotOps_CReference checks the rotation helpers against the C using
// r1 = b2MakeRot(1) and r2 = b2MakeRot(-2), the same rotations upstream builds
// its transforms from at test_math.c:111-112.
func TestOracleRotOps_CReference(t *testing.T) {
	t.Parallel()

	r1 := box2d.MakeRot(1.0)
	r2 := box2d.MakeRot(-2.0)

	mulRot := box2d.MulRot(r1, r2)
	invMulRot := box2d.InvMulRot(r1, r2)
	integrated := box2d.IntegrateRotation(r1, 0.25)
	nlerp := box2d.NLerp(box2d.RotIdentity, box2d.MakeRot(0.5*box2d.Pi), 0.25)
	normalized := box2d.NormalizeRot(box2d.Rot{C: 3.0, S: 4.0})
	between := box2d.ComputeRotationBetweenUnitVectors(
		box2d.Normalize(box2d.Vec2{X: 0.2, Y: -0.5}),
		box2d.Normalize(box2d.Vec2{X: -0.7, Y: 0.3}),
	)
	inverted := box2d.InvertRot(r1)
	xAxis := box2d.RotGetXAxis(r1)
	yAxis := box2d.RotGetYAxis(r1)

	checkOracleFloats(t, []oracleFloatCase{
		{"MakeRot(1).C", 0.540659428, r1.C, oracleC32Tol},
		{"MakeRot(1).S", 0.841241539, r1.S, oracleC32Tol},
		{"MakeRot(-2).C", -0.417019784, r2.C, oracleC32Tol},
		{"MakeRot(-2).S", -0.9088974, r2.S, oracleC32Tol},
		{"MulRot.C", 0.539136589, mulRot.C, oracleC32Tol},
		{"MulRot.S", -0.84221828, mulRot.S, oracleC32Tol},
		{"InvMulRot.C", -0.990067899, invMulRot.C, oracleC32Tol},
		{"InvMulRot.S", -0.14058958, invMulRot.S, oracleC32Tol},
		{"RelativeAngle(r1, r2)", -3.0005331, box2d.RelativeAngle(r1, r2), oracleC32Tol},
		{"RotGetAngle(r1)", 0.999586284, box2d.RotGetAngle(r1), oracleC32Tol},
		// Magnitude 8.4, so the float32 reference carries ~1e-6 of absolute error.
		{"ComputeAngularVelocity(r1, r2, 60)", -8.43537521, box2d.ComputeAngularVelocity(r1, r2, 60.0), 1e-5},
		{"IntegrateRotation(r1, 0.25).C", 0.320485651, integrated.C, oracleC32Tol},
		{"IntegrateRotation(r1, 0.25).S", 0.947253406, integrated.S, oracleC32Tol},
		{"NLerp(identity, pi/2, 0.25).C", 0.948683262, nlerp.C, oracleC32Tol},
		{"NLerp(identity, pi/2, 0.25).S", 0.316227764, nlerp.S, oracleC32Tol},
		{"NormalizeRot(3, 4).C", 0.600000024, normalized.C, oracleC32Tol},
		{"NormalizeRot(3, 4).S", 0.800000012, normalized.S, oracleC32Tol},
		{"ComputeRotationBetweenUnitVectors.C", -0.707106829, between.C, oracleC32Tol},
		{"ComputeRotationBetweenUnitVectors.S", -0.707106829, between.S, oracleC32Tol},
		// math_functions.h b2InvertRot / b2Rot_GetXAxis / b2Rot_GetYAxis are
		// pure field shuffles, so they must be exact.
		{"InvertRot(r1).C", r1.C, inverted.C, 0.0},
		{"InvertRot(r1).S", -r1.S, inverted.S, 0.0},
		{"RotGetXAxis(r1).X", r1.C, xAxis.X, 0.0},
		{"RotGetXAxis(r1).Y", r1.S, xAxis.Y, 0.0},
		{"RotGetYAxis(r1).X", -r1.S, yAxis.X, 0.0},
		{"RotGetYAxis(r1).Y", r1.C, yAxis.Y, 0.0},
	})

	// math_functions.h:387-391: b2MakeRotFromUnitVector copies the vector into
	// the rotation without change.
	unit := box2d.Normalize(box2d.Vec2{X: 0.2, Y: -0.5})
	fromUnit := box2d.MakeRotFromUnitVector(unit)
	assert.InDelta(t, unit.X, fromUnit.C, 0.0, "MakeRotFromUnitVector.C")
	assert.InDelta(t, unit.Y, fromUnit.S, 0.0, "MakeRotFromUnitVector.S")
	assert.True(t, box2d.IsNormalizedRot(fromUnit), "MakeRotFromUnitVector not normalized")
}

// TestOracleTransforms_CReference checks the transform helpers against the C
// using upstream's transform1 and transform2 from test_math.c:111-112.
func TestOracleTransforms_CReference(t *testing.T) {
	t.Parallel()

	t1 := box2d.Transform{P: box2d.Vec2{X: -2.0, Y: 3.0}, Q: box2d.MakeRot(1.0)}
	t2 := box2d.Transform{P: box2d.Vec2{X: 1.0, Y: 0.0}, Q: box2d.MakeRot(-2.0)}
	two := box2d.Vec2{X: 2.0, Y: 2.0}
	v := box2d.Vec2{X: 2.0, Y: -3.0}

	mul := box2d.MulTransforms(t2, t1)
	invMul := box2d.InvMulTransforms(t1, t2)
	point := box2d.TransformPoint(t1, two)
	invPoint := box2d.InvTransformPoint(t1, two)
	rotated := box2d.RotateVector(t1.Q, v)
	invRotated := box2d.InvRotateVector(t1.Q, v)

	checkOracleFloats(t, []oracleFloatCase{
		{"MulTransforms(t2, t1).P.X", 4.56073189, mul.P.X, oracleC32Tol},
		{"MulTransforms(t2, t1).P.Y", 0.566735506, mul.P.Y, oracleC32Tol},
		{"MulTransforms(t2, t1).Q.C", 0.539136589, mul.Q.C, oracleC32Tol},
		{"MulTransforms(t2, t1).Q.S", -0.842218339, mul.Q.S, oracleC32Tol},
		{"InvMulTransforms(t1, t2).P.X", -0.901746273, invMul.P.X, oracleC32Tol},
		{"InvMulTransforms(t1, t2).P.Y", -4.14570284, invMul.P.Y, oracleC32Tol},
		{"InvMulTransforms(t1, t2).Q.C", -0.990067899, invMul.Q.C, oracleC32Tol},
		{"InvMulTransforms(t1, t2).Q.S", -0.14058958, invMul.Q.S, oracleC32Tol},
		{"TransformPoint(t1, (2,2)).X", -2.60116434, point.X, oracleC32Tol},
		{"TransformPoint(t1, (2,2)).Y", 5.76380205, point.Y, oracleC32Tol},
		{"InvTransformPoint(t1, (2,2)).X", 1.32139611, invPoint.X, oracleC32Tol},
		{"InvTransformPoint(t1, (2,2)).Y", -3.90562558, invPoint.Y, oracleC32Tol},
		{"RotateVector(r1, (2,-3)).X", 3.60504341, rotated.X, oracleC32Tol},
		{"RotateVector(r1, (2,-3)).Y", 0.0605047941, rotated.Y, oracleC32Tol},
		{"InvRotateVector(r1, (2,-3)).X", -1.4424057, invRotated.X, oracleC32Tol},
		{"InvRotateVector(r1, (2,-3)).Y", -3.30446148, invRotated.Y, oracleC32Tol},
	})
}

// TestOracleMat22_CReference checks the 2x2 matrix helpers, including the
// det == 0 short circuit that both b2GetInverse22 and b2Solve22 take.
func TestOracleMat22_CReference(t *testing.T) {
	t.Parallel()

	m := box2d.Mat22{CX: box2d.Vec2{X: 4.0, Y: 3.0}, CY: box2d.Vec2{X: 6.0, Y: 3.0}}
	inv := box2d.GetInverse22(m)
	sol := box2d.Solve22(m, box2d.Vec2{X: 2.0, Y: 1.0})
	mv := box2d.MulMV(m, box2d.Vec2{X: 2.0, Y: 1.0})

	// det = 4*3 - 6*3 = -6, so every entry is a short exact quotient except
	// 2/-6, which the C rounds to -0.666666687 in float32.
	singular := box2d.Mat22{CX: box2d.Vec2{X: 1.0, Y: 2.0}, CY: box2d.Vec2{X: 2.0, Y: 4.0}}
	singularInv := box2d.GetInverse22(singular)
	singularSol := box2d.Solve22(singular, box2d.Vec2{X: 1.0, Y: 1.0})

	checkOracleFloats(t, []oracleFloatCase{
		{"GetInverse22.CX.X", -0.5, inv.CX.X, oracleC32Tol},
		{"GetInverse22.CX.Y", 0.5, inv.CX.Y, oracleC32Tol},
		{"GetInverse22.CY.X", 1.0, inv.CY.X, oracleC32Tol},
		{"GetInverse22.CY.Y", -0.666666687, inv.CY.Y, oracleC32Tol},
		{"Solve22.X", 0.0, sol.X, oracleC32Tol},
		{"Solve22.Y", 0.333333343, sol.Y, oracleC32Tol},
		{"MulMV.X", 14.0, mv.X, 0.0},
		{"MulMV.Y", 9.0, mv.Y, 0.0},
		// Singular matrix: det stays 0, so every product below is zero.
		{"GetInverse22(singular).CX.X", 0.0, singularInv.CX.X, 0.0},
		{"GetInverse22(singular).CX.Y", 0.0, singularInv.CX.Y, 0.0},
		{"GetInverse22(singular).CY.X", 0.0, singularInv.CY.X, 0.0},
		{"GetInverse22(singular).CY.Y", 0.0, singularInv.CY.Y, 0.0},
		{"Solve22(singular).X", 0.0, singularSol.X, 0.0},
		{"Solve22(singular).Y", 0.0, singularSol.Y, 0.0},
	})
}

// TestOracleSpringDamper_CReference checks b2SpringDamper
// (math_functions.h:677-682) against the C. The inputs are chosen to be exactly
// representable in float32 and float64 so the only differences are the B2_PI
// representation and float32 rounding.
func TestOracleSpringDamper_CReference(t *testing.T) {
	t.Parallel()

	checkOracleFloats(t, []oracleFloatCase{
		{
			"SpringDamper(2, 1, 0.5, 0, 0.015625)",
			-0.861972868,
			box2d.SpringDamper(2.0, 1.0, 0.5, 0.0, 0.015625),
			oracleC32Tol,
		},
		{
			// Magnitude 21, so the float32 reference carries ~2e-6 of error.
			"SpringDamper(8, 0.5, -1.25, 2, 0.015625)",
			21.3749847,
			box2d.SpringDamper(8.0, 0.5, -1.25, 2.0, 0.015625),
			1e-5,
		},
		{
			// hertz == 0 makes omega zero, so the velocity passes through.
			"SpringDamper(0, 0, 5, 3, 0.015625)",
			3.0,
			box2d.SpringDamper(0.0, 0.0, 5.0, 3.0, 0.015625),
			0.0,
		},
		{
			"SpringDamper(1, 0, 1, 0, 0.5)",
			-1.8160007,
			box2d.SpringDamper(1.0, 0.0, 1.0, 0.0, 0.5),
			oracleC32Tol,
		},
	})
}

// TestOracleAABBOps_CReference checks the AABB helpers of math.go against the
// b2AABB inline functions in include/box2d/math_functions.h.
func TestOracleAABBOps_CReference(t *testing.T) {
	t.Parallel()

	a := box2d.AABB{
		LowerBound: box2d.Vec2{X: -2.0, Y: -3.0},
		UpperBound: box2d.Vec2{X: 4.0, Y: 5.0},
	}
	inner := box2d.AABB{
		LowerBound: box2d.Vec2{X: -1.0, Y: -1.0},
		UpperBound: box2d.Vec2{X: 1.0, Y: 1.0},
	}
	disjoint := box2d.AABB{
		LowerBound: box2d.Vec2{X: 10.0, Y: 10.0},
		UpperBound: box2d.Vec2{X: 12.0, Y: 12.0},
	}

	assert.True(t, box2d.AABBContains(a, inner), "AABBContains(a, inner)")
	assert.False(t, box2d.AABBContains(inner, a), "AABBContains(inner, a)")
	assert.True(t, box2d.AABBOverlaps(a, inner), "AABBOverlaps(a, inner)")
	assert.False(t, box2d.AABBOverlaps(a, disjoint), "AABBOverlaps(a, disjoint)")

	center := box2d.AABBCenter(a)
	extents := box2d.AABBExtents(a)
	union := box2d.AABBUnion(a, disjoint)

	checkOracleFloats(t, []oracleFloatCase{
		// b2AABB_Center: 0.5*(lower + upper).
		{"AABBCenter.X", 1.0, center.X, 0.0},
		{"AABBCenter.Y", 1.0, center.Y, 0.0},
		// b2AABB_Extents: 0.5*(upper - lower).
		{"AABBExtents.X", 3.0, extents.X, 0.0},
		{"AABBExtents.Y", 4.0, extents.Y, 0.0},
		{"AABBUnion.LowerBound.X", -2.0, union.LowerBound.X, 0.0},
		{"AABBUnion.LowerBound.Y", -3.0, union.LowerBound.Y, 0.0},
		{"AABBUnion.UpperBound.X", 12.0, union.UpperBound.X, 0.0},
		{"AABBUnion.UpperBound.Y", 12.0, union.UpperBound.Y, 0.0},
	})

	// b2MakeAABB: bounding box of the points, inflated by radius.
	made := box2d.MakeAABB([]box2d.Vec2{
		{X: 1.0, Y: 2.0},
		{X: -3.0, Y: 0.5},
		{X: 0.0, Y: -4.0},
	}, 0.25)

	checkOracleFloats(t, []oracleFloatCase{
		{"MakeAABB.LowerBound.X", -3.25, made.LowerBound.X, 0.0},
		{"MakeAABB.LowerBound.Y", -4.25, made.LowerBound.Y, 0.0},
		{"MakeAABB.UpperBound.X", 1.25, made.UpperBound.X, 0.0},
		{"MakeAABB.UpperBound.Y", 2.25, made.UpperBound.Y, 0.0},
	})
}

// TestOracleValidity_CReference checks the b2IsValid* family of
// src/math_functions.c:10-66, including the NaN and infinity branches.
func TestOracleValidity_CReference(t *testing.T) {
	t.Parallel()

	nan := math.NaN()
	inf := math.Inf(1)
	negInf := math.Inf(-1)

	// math_functions.c:10-22.
	assert.True(t, box2d.IsValidFloat(0.0), "IsValidFloat(0)")
	assert.True(t, box2d.IsValidFloat(-1e30), "IsValidFloat(-1e30)")
	assert.False(t, box2d.IsValidFloat(nan), "IsValidFloat(NaN)")
	assert.False(t, box2d.IsValidFloat(inf), "IsValidFloat(+Inf)")
	assert.False(t, box2d.IsValidFloat(negInf), "IsValidFloat(-Inf)")

	// math_functions.c:24-37.
	assert.True(t, box2d.IsValidVec2(box2d.Vec2{X: 1.0, Y: -2.0}), "IsValidVec2 finite")
	assert.False(t, box2d.IsValidVec2(box2d.Vec2{X: nan, Y: 0.0}), "IsValidVec2 NaN x")
	assert.False(t, box2d.IsValidVec2(box2d.Vec2{X: 0.0, Y: nan}), "IsValidVec2 NaN y")
	assert.False(t, box2d.IsValidVec2(box2d.Vec2{X: inf, Y: 0.0}), "IsValidVec2 Inf x")
	assert.False(t, box2d.IsValidVec2(box2d.Vec2{X: 0.0, Y: negInf}), "IsValidVec2 Inf y")

	// math_functions.c:39-52. The NaN and infinity branches come before the
	// normalization check, so they must reject even a rotation that would
	// otherwise look normalized.
	assert.True(t, box2d.IsValidRotation(box2d.RotIdentity), "IsValidRotation identity")
	assert.False(t, box2d.IsValidRotation(box2d.Rot{C: nan, S: 0.0}), "IsValidRotation NaN c")
	assert.False(t, box2d.IsValidRotation(box2d.Rot{C: 1.0, S: nan}), "IsValidRotation NaN s")
	assert.False(t, box2d.IsValidRotation(box2d.Rot{C: inf, S: 0.0}), "IsValidRotation Inf c")
	assert.False(t, box2d.IsValidRotation(box2d.Rot{C: 0.0, S: negInf}), "IsValidRotation Inf s")
	// Not normalized: qq = 4, outside 1 +/- 0.0006.
	assert.False(t, box2d.IsValidRotation(box2d.Rot{C: 2.0, S: 0.0}), "IsValidRotation unnormalized")

	// math_functions.h:396-402: the tolerance band is exactly 1 +/- 0.0006.
	assert.True(t, box2d.IsNormalizedRot(box2d.Rot{C: 1.0, S: 0.0}), "IsNormalizedRot identity")
	assert.False(t, box2d.IsNormalizedRot(box2d.Rot{C: 1.001, S: 0.0}), "IsNormalizedRot above band")
	assert.False(t, box2d.IsNormalizedRot(box2d.Rot{C: 0.999, S: 0.0}), "IsNormalizedRot below band")

	// math_functions.h:319-324: b2IsNormalized uses 100*FLT_EPSILON on the
	// squared length.
	assert.True(t, box2d.IsNormalized(box2d.Vec2{X: 1.0, Y: 0.0}), "IsNormalized unit x")
	assert.True(t, box2d.IsNormalized(box2d.Normalize(box2d.Vec2{X: 3.0, Y: 4.0})), "IsNormalized (3,4)")
	assert.False(t, box2d.IsNormalized(box2d.Vec2{X: 1.0, Y: 1.0}), "IsNormalized (1,1)")

	// math_functions.c:54-62.
	assert.True(t, box2d.IsValidTransform(box2d.TransformIdentity), "IsValidTransform identity")
	assert.False(
		t,
		box2d.IsValidTransform(box2d.Transform{P: box2d.Vec2{X: nan, Y: 0.0}, Q: box2d.RotIdentity}),
		"IsValidTransform NaN position",
	)
	assert.False(
		t,
		box2d.IsValidTransform(box2d.Transform{P: box2d.Vec2Zero, Q: box2d.Rot{C: 2.0, S: 0.0}}),
		"IsValidTransform unnormalized rotation",
	)

	// math_functions.c:64-66.
	assert.True(
		t,
		box2d.IsValidPlane(box2d.Plane{Normal: box2d.Vec2{X: 0.0, Y: 1.0}, Offset: 3.0}),
		"IsValidPlane unit normal",
	)
	assert.False(
		t,
		box2d.IsValidPlane(box2d.Plane{Normal: box2d.Vec2{X: 2.0, Y: 0.0}, Offset: 3.0}),
		"IsValidPlane non-unit normal",
	)
	assert.False(
		t,
		box2d.IsValidPlane(box2d.Plane{Normal: box2d.Vec2{X: 0.0, Y: 1.0}, Offset: nan}),
		"IsValidPlane NaN offset",
	)
}

// TestOraclePerpAndCross_CReference checks the perpendicular and scalar-cross
// helpers of include/box2d/math_functions.h. Upstream documents b2LeftPerp as
// b2CrossSV(1, v) and b2RightPerp as b2CrossVS(v, 1), so those identities are
// the oracle.
func TestOraclePerpAndCross_CReference(t *testing.T) {
	t.Parallel()

	v := box2d.Vec2{X: 3.0, Y: -7.0}

	left := box2d.LeftPerp(v)
	right := box2d.RightPerp(v)
	crossSV := box2d.CrossSV(1.0, v)
	crossVS := box2d.CrossVS(v, 1.0)
	neg := box2d.Neg(v)
	abs := box2d.Abs(v)
	minV := box2d.Min(box2d.Vec2{X: 1.0, Y: 5.0}, box2d.Vec2{X: 4.0, Y: 2.0})
	maxV := box2d.Max(box2d.Vec2{X: 1.0, Y: 5.0}, box2d.Vec2{X: 4.0, Y: 2.0})
	mulSV := box2d.MulSV(-2.0, v)

	checkOracleFloats(t, []oracleFloatCase{
		{"LeftPerp.X", 7.0, left.X, 0.0},
		{"LeftPerp.Y", 3.0, left.Y, 0.0},
		{"RightPerp.X", -7.0, right.X, 0.0},
		{"RightPerp.Y", -3.0, right.Y, 0.0},
		{"CrossSV(1, v).X", left.X, crossSV.X, 0.0},
		{"CrossSV(1, v).Y", left.Y, crossSV.Y, 0.0},
		{"CrossVS(v, 1).X", right.X, crossVS.X, 0.0},
		{"CrossVS(v, 1).Y", right.Y, crossVS.Y, 0.0},
		{"Neg.X", -3.0, neg.X, 0.0},
		{"Neg.Y", 7.0, neg.Y, 0.0},
		{"Abs.X", 3.0, abs.X, 0.0},
		{"Abs.Y", 7.0, abs.Y, 0.0},
		{"Min.X", 1.0, minV.X, 0.0},
		{"Min.Y", 2.0, minV.Y, 0.0},
		{"Max.X", 4.0, maxV.X, 0.0},
		{"Max.Y", 5.0, maxV.Y, 0.0},
		{"MulSV.X", -6.0, mulSV.X, 0.0},
		{"MulSV.Y", 14.0, mulSV.Y, 0.0},
	})
}

// ---------------------------------------------------------------------------
// Ports of upstream test/test_id.c (IdTest) plus the contact id helpers.
// ---------------------------------------------------------------------------

// TestOracleIdTest_RoundTrip ports test_id.c:8-45.
func TestOracleIdTest_RoundTrip(t *testing.T) {
	t.Parallel()

	// test_id.c:10.
	const a = uint32(0x01234567)

	worldID := box2d.UnpackWorldID(a)
	assert.Equal(t, a, box2d.PackWorldID(worldID), "world id round trip")

	// test_id.c:18.
	const x = uint64(0x0123456789ABCDEF)

	assert.Equal(t, x, box2d.PackBodyID(box2d.UnpackBodyID(x)), "body id round trip")
	assert.Equal(t, x, box2d.PackShapeID(box2d.UnpackShapeID(x)), "shape id round trip")
	assert.Equal(t, x, box2d.PackChainID(box2d.UnpackChainID(x)), "chain id round trip")
	assert.Equal(t, x, box2d.PackJointID(box2d.UnpackJointID(x)), "joint id round trip")
}

// TestOracleIdTest_NegativeIndexRoundTrip extends test_id.c with a value whose
// index1 field is negative. In the C the store shifts a sign-extended int32_t
// left by 32 (include/box2d/id.h:124), so the sign extension is shifted out and
// the round trip still holds; running the C confirms
// b2StoreBodyId(b2LoadBodyId(0xFFFFFFFF89ABCDEF)) == 0xFFFFFFFF89ABCDEF with
// index1 == -1.
func TestOracleIdTest_NegativeIndexRoundTrip(t *testing.T) {
	t.Parallel()

	const x = uint64(0xFFFFFFFF89ABCDEF)

	assert.Equal(t, x, box2d.PackBodyID(box2d.UnpackBodyID(x)), "body id round trip")
	assert.Equal(t, x, box2d.PackShapeID(box2d.UnpackShapeID(x)), "shape id round trip")
	assert.Equal(t, x, box2d.PackChainID(box2d.UnpackChainID(x)), "chain id round trip")
	assert.Equal(t, x, box2d.PackJointID(box2d.UnpackJointID(x)), "joint id round trip")

	// index1 is non-zero, so B2_IS_NON_NULL holds (include/box2d/id.h:102-105).
	assert.True(t, box2d.UnpackBodyID(x).IsNonNull(), "body id non-null")
	assert.False(t, box2d.UnpackBodyID(x).IsNull(), "body id null")
}

// TestOracleIds_NullMacros checks B2_IS_NULL and B2_IS_NON_NULL
// (include/box2d/id.h:102-105) for every id type. The macros test index1
// against zero, so an id whose only non-zero fields are world0 and generation
// is still null, and the zero value is null for all of them (id.h:92-99).
func TestOracleIds_NullMacros(t *testing.T) {
	t.Parallel()

	// index1 == 0, world0 == 0xABCD, generation == 0xEF01: null despite the
	// other fields being set.
	const indexZero = uint64(0x00000000ABCDEF01)

	// index1 == 1: non-null.
	const indexOne = uint64(0x00000001ABCDEF01)

	assert.True(t, box2d.UnpackWorldID(0x0000BEEF).IsNull(), "world id with index1 == 0 is null")
	assert.False(t, box2d.UnpackWorldID(0x0000BEEF).IsNonNull(), "world id with index1 == 0")
	assert.True(t, box2d.UnpackWorldID(0x0001BEEF).IsNonNull(), "world id with index1 == 1")
	assert.False(t, box2d.UnpackWorldID(0x0001BEEF).IsNull(), "world id with index1 == 1")

	assert.True(t, box2d.UnpackBodyID(indexZero).IsNull(), "body id null")
	assert.False(t, box2d.UnpackBodyID(indexZero).IsNonNull(), "body id null")
	assert.True(t, box2d.UnpackBodyID(indexOne).IsNonNull(), "body id non-null")

	assert.True(t, box2d.UnpackShapeID(indexZero).IsNull(), "shape id null")
	assert.False(t, box2d.UnpackShapeID(indexZero).IsNonNull(), "shape id null")
	assert.True(t, box2d.UnpackShapeID(indexOne).IsNonNull(), "shape id non-null")
	assert.False(t, box2d.UnpackShapeID(indexOne).IsNull(), "shape id non-null")

	assert.True(t, box2d.UnpackChainID(indexZero).IsNull(), "chain id null")
	assert.False(t, box2d.UnpackChainID(indexZero).IsNonNull(), "chain id null")
	assert.True(t, box2d.UnpackChainID(indexOne).IsNonNull(), "chain id non-null")
	assert.False(t, box2d.UnpackChainID(indexOne).IsNull(), "chain id non-null")

	assert.True(t, box2d.UnpackJointID(indexZero).IsNull(), "joint id null")
	assert.False(t, box2d.UnpackJointID(indexZero).IsNonNull(), "joint id null")
	assert.True(t, box2d.UnpackJointID(indexOne).IsNonNull(), "joint id non-null")
	assert.False(t, box2d.UnpackJointID(indexOne).IsNull(), "joint id non-null")

	// Zero initialization yields a null id for every type (id.h:92-99).
	var (
		worldID box2d.WorldID
		bodyID  box2d.BodyID
		shapeID box2d.ShapeID
		chainID box2d.ChainID
		jointID box2d.JointID
	)

	assert.True(t, worldID.IsNull(), "zero-value world id")
	assert.True(t, bodyID.IsNull(), "zero-value body id")
	assert.True(t, shapeID.IsNull(), "zero-value shape id")
	assert.True(t, chainID.IsNull(), "zero-value chain id")
	assert.True(t, jointID.IsNull(), "zero-value joint id")
	assert.False(t, worldID.IsNonNull(), "zero-value world id")
	assert.False(t, bodyID.IsNonNull(), "zero-value body id")
	assert.False(t, shapeID.IsNonNull(), "zero-value shape id")
	assert.False(t, chainID.IsNonNull(), "zero-value chain id")
	assert.False(t, jointID.IsNonNull(), "zero-value joint id")
}

// TestOracleContactID_CReference checks b2StoreContactId / b2LoadContactId
// (include/box2d/id.h:176-192) and the B2_IS_NULL / B2_IS_NON_NULL macros
// (id.h:102-105) against the C.
func TestOracleContactID_CReference(t *testing.T) {
	t.Parallel()

	// Running the C on values = {0x89ABCDEF, 0x0000BEEF, 0xDEADBEEF} yields
	// index1 = -1985229329, world0 = 48879, padding = 0,
	// generation = 3735928559, and storing it back reproduces the same three
	// words.
	values := [3]uint32{0x89ABCDEF, 0x0000BEEF, 0xDEADBEEF}
	id := box2d.UnpackContactID(values)

	assert.Equal(t, values, box2d.PackContactID(id), "contact id round trip")
	assert.True(t, id.IsNonNull(), "contact id non-null")
	assert.False(t, id.IsNull(), "contact id null")

	// A zero-valued id is null (id.h:88-99).
	zero := box2d.UnpackContactID([3]uint32{0, 0, 0})
	assert.True(t, zero.IsNull(), "zero contact id null")
	assert.False(t, zero.IsNonNull(), "zero contact id non-null")
	assert.Equal(t, [3]uint32{0, 0, 0}, box2d.PackContactID(zero), "zero contact id round trip")

	// world0 is a uint16_t, so b2LoadContactId truncates values[1]. The C
	// reports world0 == 9029 (0x2345) for values[1] == 0x00012345, and storing
	// the id back therefore yields the truncated word.
	truncating := box2d.UnpackContactID([3]uint32{7, 0x00012345, 9})
	assert.Equal(t, [3]uint32{7, 0x2345, 9}, box2d.PackContactID(truncating), "world0 truncation")

	// Slot 0 must feed index1 SPECIFICALLY, not merely survive a round trip:
	// index1 and generation are both lossless 32-bit fields, so every check
	// above would also pass if both Pack and Unpack swapped slots 0 and 2 in
	// concert. IsNull is defined as index1 == 0, which makes it the one
	// external observer that can tell the slots apart. PackContactID's doc
	// promises slot 0 as a compact table key (physics2d's contact gather
	// indexes its seen-stamp table by it), so pin the order here.
	assert.True(t, box2d.UnpackContactID([3]uint32{7, 5, 0}).IsNonNull(),
		"a nonzero slot 0 alone makes the id non-null: slot 0 is index1")
	assert.True(t, box2d.UnpackContactID([3]uint32{0, 5, 7}).IsNull(),
		"a nonzero generation alone leaves the id null: slot 2 is not index1")

	// The zero value of the Go struct is also null, matching the C's
	// documented "you may also use zero initialization to get null".
	var zeroValue box2d.ContactID

	assert.True(t, zeroValue.IsNull(), "zero-value contact id null")
	assert.False(t, zeroValue.IsNonNull(), "zero-value contact id non-null")
}

// ---------------------------------------------------------------------------
// src/core.c, src/timer.c and include/box2d/constants.h.
// ---------------------------------------------------------------------------

// TestOracleGetVersion_CReference checks GetVersion against src/core.c:100-107.
func TestOracleGetVersion_CReference(t *testing.T) {
	t.Parallel()

	assert.Equal(t, box2d.Version{Major: 3, Minor: 2, Revision: 0}, box2d.GetVersion(), "GetVersion")
}

// TestOracleHash_CReference checks the djb2 hash of src/timer.c:297-306 with
// B2_HASH_INIT from include/box2d/base.h:144. The values below were produced by
// running that C function.
func TestOracleHash_CReference(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 5381, box2d.HashInit, "B2_HASH_INIT")

	type row struct {
		name string
		data []byte
		want uint32
	}

	rows := []row{
		{`hash("")`, []byte{}, 5381},
		{`hash("a")`, []byte("a"), 177670},
		{`hash("box2d")`, []byte("box2d"), 254493924},
		{`hash("hello world")`, []byte("hello world"), 894552257},
		{"hash(00 FF 80 7F)", []byte{0x00, 0xFF, 0x80, 0x7F}, 2086755651},
	}

	for _, r := range rows {
		assert.Equal(t, r.want, box2d.Hash(box2d.HashInit, r.data), "%s", r.name)
	}

	// Chaining: hashing the binary block on top of "box2d" must equal hashing
	// the concatenation, and the C reports 2337236130 for it.
	chained := box2d.Hash(box2d.Hash(box2d.HashInit, []byte("box2d")), []byte{0x00, 0xFF, 0x80, 0x7F})
	assert.Equal(t, uint32(2337236130), chained, "chained hash")

	// A nil slice must behave like an empty one: the loop body never runs.
	assert.Equal(t, uint32(5381), box2d.Hash(box2d.HashInit, nil), "hash(nil)")
}

// TestOracleConstants_CReference checks the constants of
// include/box2d/constants.h and src/core.h at the default length unit of 1.
//
// The C evaluates these as floats, so the tolerances below are the exact
// float32 representation error of each literal; running the C prints
// B2_LINEAR_SLOP as 0.004999999888241291 and B2_CONTACT_RECYCLE_DISTANCE as
// 0.049999997019767761.
func TestOracleConstants_CReference(t *testing.T) {
	t.Parallel()

	// Exact integer constants.
	assert.Equal(t, -1, box2d.NullIndex, "B2_NULL_INDEX (core.h:10)")
	assert.Equal(t, 1152023, box2d.SecretCookie, "B2_SECRET_COOKIE (core.h:112)")
	assert.Equal(t, 32, box2d.Alignment, "B2_ALIGNMENT (core.c:121)")
	assert.Equal(t, 64, box2d.MaxWorkers, "B2_MAX_WORKERS (constants.h:13)")
	assert.Equal(t, 24, box2d.GraphColorCount, "B2_GRAPH_COLOR_COUNT (constants.h:18)")
	assert.Equal(t, 128, box2d.MaxWorlds, "B2_MAX_WORLDS (constants.h:28)")

	checkOracleFloats(t, []oracleFloatCase{
		// Exactly representable in float32, so the C prints them unchanged.
		{"B2_AABB_MARGIN_FRACTION", 0.125, box2d.AABBMarginFraction, 0.0},
		{"B2_TIME_TO_SLEEP", 0.5, box2d.TimeToSleep, 0.0},
		{"B2_HUGE", 100000.0, box2d.Huge, 0.0},
		// Decimal literals the C rounds to float32.
		{"B2_LINEAR_SLOP", 0.004999999888241291, box2d.LinearSlop, 1e-9},
		{"B2_SPECULATIVE_DISTANCE", 0.019999999552965164, box2d.SpeculativeDistance, 1e-9},
		{"B2_CONTACT_RECYCLE_DISTANCE", 0.049999997019767761, box2d.ContactRecycleDistance, 1e-8},
		{"B2_MAX_AABB_MARGIN", 0.05000000074505806, box2d.MaxAABBMargin, 1e-9},
		// B2_MAX_ROTATION is 0.25f*B2_PI, so it inherits the B2_PI
		// representation difference documented in
		// TestOracleUnwindAngle_B2PiConstantDivergence: the C evaluates it to
		// 0.78539818525314331, this port to 0.25*3.14159265359.
		{"B2_MAX_ROTATION", 0.78539818525314331, box2d.MaxRotation, 0.25*b2PiRepresentationDelta + 1e-12},
	})
}

// TestOracleLengthUnits_CReference checks b2SetLengthUnitsPerMeter /
// b2GetLengthUnitsPerMeter (src/core.c:38-47) together with the length-scaled
// macros of include/box2d/constants.h, which re-read the length unit at every
// use.
//
// The length unit is process-global, so this test must not run in parallel and
// restores the default before returning.
func TestOracleLengthUnits_CReference(t *testing.T) {
	original := box2d.GetLengthUnitsPerMeter()
	t.Cleanup(func() {
		box2d.SetLengthUnitsPerMeter(original)
	})

	require.InDelta(t, 1.0, original, 0.0, "default length unit is 1 (core.c:36)")

	// A power of two so the scaling is exact in binary floating point.
	const units = 32.0

	box2d.SetLengthUnitsPerMeter(units)

	assert.InDelta(t, units, box2d.GetLengthUnitsPerMeter(), 0.0, "GetLengthUnitsPerMeter")

	checkOracleFloats(t, []oracleFloatCase{
		// constants.h:24 - 0.005f * b2GetLengthUnitsPerMeter().
		{"B2_LINEAR_SLOP", 0.005 * units, box2d.LinearSlop, 1e-9},
		// constants.h:10 - 100000.0f * b2GetLengthUnitsPerMeter().
		{"B2_HUGE", 100000.0 * units, box2d.Huge, 0.0},
		// constants.h:39 - 4.0f * B2_LINEAR_SLOP.
		{"B2_SPECULATIVE_DISTANCE", 4.0 * 0.005 * units, box2d.SpeculativeDistance, 1e-9},
		// constants.h:42 - 10.0f * B2_LINEAR_SLOP.
		{"B2_CONTACT_RECYCLE_DISTANCE", 10.0 * 0.005 * units, box2d.ContactRecycleDistance, 1e-9},
		// constants.h:48 - 0.05f * b2GetLengthUnitsPerMeter().
		{"B2_MAX_AABB_MARGIN", 0.05 * units, box2d.MaxAABBMargin, 1e-9},
	})

	// constants.h:34 and :51-54 are dimensionless, so they must not move.
	checkOracleFloats(t, []oracleFloatCase{
		{"B2_MAX_ROTATION", 0.25 * box2d.Pi, box2d.MaxRotation, 0.0},
		{"B2_AABB_MARGIN_FRACTION", 0.125, box2d.AABBMarginFraction, 0.0},
		{"B2_TIME_TO_SLEEP", 0.5, box2d.TimeToSleep, 0.0},
	})

	// Restoring the default unit must restore the exact default values.
	box2d.SetLengthUnitsPerMeter(1.0)

	checkOracleFloats(t, []oracleFloatCase{
		{"B2_LINEAR_SLOP restored", 0.005, box2d.LinearSlop, 0.0},
		{"B2_HUGE restored", 100000.0, box2d.Huge, 0.0},
		{"B2_SPECULATIVE_DISTANCE restored", 0.02, box2d.SpeculativeDistance, 0.0},
		{"B2_CONTACT_RECYCLE_DISTANCE restored", 0.05, box2d.ContactRecycleDistance, 0.0},
		{"B2_MAX_AABB_MARGIN restored", 0.05, box2d.MaxAABBMargin, 0.0},
	})
}
