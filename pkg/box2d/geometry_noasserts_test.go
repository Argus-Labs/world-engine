// Tests for the float64 port of Box2D v3.2.0 src/geometry.c — asserts-off fallback paths.

//go:build !box2d_asserts

package box2d_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// TestGeometryMakePolygonFromDegenerateHull pins the fallback MakePolygon takes
// when handed a hull that never passed ValidateHull: a 0.5 half-width square,
// matching upstream's "handle a bad hull when assertions are disabled" branch.
// Under the box2d_asserts build tag the same call panics in
// assert(ValidateHull(hull)) — the intended behavior there — so this test only
// builds without the tag.
func TestGeometryMakePolygonFromDegenerateHull(t *testing.T) {
	t.Parallel()

	bad := box2d.Hull{Count: 2}
	assert.Equal(t, box2d.MakeSquare(0.5), box2d.MakePolygon(&bad, 0))
}
