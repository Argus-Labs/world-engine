// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d).

package box2d

// This file defines the only approved patterns for float multiply-accumulate
// arithmetic in this package.
//
// The Go compiler may fuse x*y + z into a single fused-multiply-add (FMA)
// instruction on some architectures (e.g. arm64). FMA skips the intermediate
// rounding of x*y and therefore yields different bits than architectures that
// round the product first (e.g. amd64 without FMA codegen). An explicit
// float64 conversion of a product forces the intermediate rounding and forbids
// fusion (Go spec, "Floating point operators"). pkg/box2d requires
// bit-identical results across architectures, so no `*` expression may appear
// as a direct operand of `+` or `-` anywhere in this package; use these
// helpers or an explicit float64(...) conversion at the call site.

// mulAdd returns a*b + c with the product explicitly rounded (no FMA).
func mulAdd(a, b, c float64) float64 { return float64(a*b) + c }

// mulSub returns a*b - c with the product explicitly rounded (no FMA).
//
//nolint:unused // part of the documented FMA-safe helper set (see file header)
func mulSub(a, b, c float64) float64 { return float64(a*b) - c }

// dot2 returns a*b + c*d with both products explicitly rounded (no FMA).
func dot2(a, b, c, d float64) float64 { return float64(a*b) + float64(c*d) }

// cross2 returns a*b - c*d with both products explicitly rounded (no FMA).
func cross2(a, b, c, d float64) float64 { return float64(a*b) - float64(c*d) }
