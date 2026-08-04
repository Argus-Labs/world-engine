// Build-tag mirror of the internal debugAsserts constant for external tests.

//go:build box2d_asserts

package box2d_test

// buildWithAsserts mirrors debugAsserts (core_asserts_on.go); see
// asserts_tag_off_test.go for why external tests need the mirror.
const buildWithAsserts = true
