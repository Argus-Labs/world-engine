// Build-tag mirror of the internal debugAsserts constant for external tests.

//go:build !box2d_asserts

package box2d_test

// buildWithAsserts mirrors debugAsserts (core_asserts_off.go), which package
// box2d_test cannot see. Tests that pin release-build guard behavior (silently
// ignore bad input) skip when it is true, because the box2d_asserts build
// panics in those guards instead, like an upstream debug build.
const buildWithAsserts = false
