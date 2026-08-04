// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/core.h (B2_ASSERT wiring).

//go:build !box2d_asserts

package box2d

// debugAsserts gates the port's *internal* invariant checks only.
//
// This package splits upstream's single B2_ASSERT into two tiers:
//
//   - Internal invariants (the hundreds of assert calls across the solver,
//     collision and broad-phase code) state facts the port must uphold on its
//     own. They are compiled out in the default build so release builds pay
//     nothing for them, and compiled in under the box2d_asserts build tag
//     (core_asserts_on.go). CI runs the full suite with the tag so the
//     invariant net is exercised on every change; run it locally with
//     `go test -tags box2d_asserts ./pkg/box2d/`.
//
//   - Public API preconditions (a caller passing a definition struct that was
//     never built by its Default* constructor, or a field value that would
//     silently corrupt the simulation rather than fail loudly) are *always*
//     checked, regardless of this flag, via requireInitialized and
//     requireValidDefField in core.go. Those are programmer errors in calling
//     code, they are cheap to detect at creation time, and leaving them
//     unchecked turns a one-line mistake into garbage simulation or a
//     confusing panic far from the cause.
//
// When adding a check, ask whose bug it catches: the port's (assert) or the
// caller's (require*).
const debugAsserts = false
