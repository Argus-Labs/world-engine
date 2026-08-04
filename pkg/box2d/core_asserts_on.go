// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/core.h (B2_ASSERT wiring).

//go:build box2d_asserts

package box2d

// debugAsserts is on in this build: the box2d_asserts tag compiles the port's
// internal invariant checks in, so any assert(...) violation panics instead of
// being compiled out. CI runs the suite this way; release builds never do.
// See core_asserts_off.go for the full story of the two assertion tiers.
const debugAsserts = true
