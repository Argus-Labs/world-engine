// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d).

package box2d

// This file defines the approved primitives for float multiply-accumulate
// arithmetic in this package.
//
// The Go compiler may fuse a multiply and an add into a single FMA
// instruction on targets that have one (arm64 always; amd64 at GOAMD64=v3).
// FMA skips the intermediate rounding of the product, so a fused expression
// yields different bits than the same expression evaluated on a target
// without FMA. pkg/box2d promises bit-identical results across
// architectures, so no fusion may occur anywhere in the package.
//
// Only an explicit float64(...) conversion forces the intermediate rounding
// that forbids fusion (Go spec, "Floating point operators"). The subtle part
// is which value needs the conversion: fusion is legal whenever an
// *unrounded* product reaches a + or -, and the product does not have to be
// written inline. All of these fuse:
//
//	t := x * y; r = t + z          // product via a local
//	r = mulAdd(a, b, x*y)          // product via a parameter, after inlining
//	r = Add(MulSV(s, v), w)        // product via a returned struct field
//
// So these helpers round *both* the products they form and the operands they
// receive: rounding a parameter means the caller's product is rounded too
// once the call is inlined, which is usually the smallest way to kill a
// fusion site. Callers that do arithmetic outside these helpers must round
// their own products with float64(...).
//
// The rule is enforced mechanically, not by review: TestNoFusedMultiplyAdd
// compiles the package for FMA-capable targets and fails if the assembler
// output contains any FMA instruction.

// mulAdd returns a*b + c, rounding the product and the addend (no FMA).
func mulAdd(a, b, c float64) float64 { return float64(a*b) + float64(c) }

// mulSub returns a*b - c, rounding the product and the subtrahend (no FMA).
//
//nolint:unused // part of the documented FMA-safe helper set (see file header)
func mulSub(a, b, c float64) float64 { return float64(a*b) - float64(c) }

// dot2 returns a*b + c*d with both products rounded (no FMA).
func dot2(a, b, c, d float64) float64 { return float64(a*b) + float64(c*d) }

// cross2 returns a*b - c*d with both products rounded (no FMA).
func cross2(a, b, c, d float64) float64 { return float64(a*b) - float64(c*d) }

// addF returns a + b with both operands rounded, for use where an operand may
// be an unrounded product formed by the caller (no FMA).
func addF(a, b float64) float64 { return float64(a) + float64(b) }

// subF returns a - b with both operands rounded, for use where an operand may
// be an unrounded product formed by the caller (no FMA).
func subF(a, b float64) float64 { return float64(a) - float64(b) }
