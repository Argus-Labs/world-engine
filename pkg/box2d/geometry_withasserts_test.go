// Tests for the float64 port of Box2D v3.2.0 src/geometry.c — box2d_asserts half.

//go:build box2d_asserts

package box2d_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// TestGeometryMakePolygonFromDegenerateHullPanicsWithAsserts is the
// box2d_asserts twin of TestGeometryMakePolygonFromDegenerateHull: the same
// degenerate hull that falls back to a 0.5 half-width square in the release
// build panics in assert(ValidateHull(hull)) here, like an upstream debug
// build (geometry.c:58 and :98).
func TestGeometryMakePolygonFromDegenerateHullPanicsWithAsserts(t *testing.T) {
	t.Parallel()

	bad := box2d.Hull{Count: 2}
	assert.Panics(t, func() { box2d.MakePolygon(&bad, 0) })
	assert.Panics(t, func() { box2d.MakeOffsetRoundedPolygon(&bad, box2d.Vec2{}, box2d.RotIdentity, 0.5) })
}
