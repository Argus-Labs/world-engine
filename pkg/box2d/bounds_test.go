// Tests for the always-on public API preconditions that keep a caller-supplied
// count from indexing past a fixed-size array deep inside the collision, mass
// or solver code. Polygon and SimplexCache are exported structs with exported
// fields, so a caller can hand-build one with a count its arrays cannot hold
// instead of going through MakePolygon/MakeBox or zero initializing the cache.
// These checks are independent of the debugAsserts build flag, so they must
// hold in a normal (release) test build.
//
// The helpers requirePanicMessage, preconditionWorld and preconditionBody live
// in precondition_test.go.

package box2d_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

const (
	// wantPolygonCountPanic is the panic text for the collision and mass entry
	// points, which also accept the degenerate 1- and 2-vertex polygons the
	// package builds internally for circles and capsules.
	wantPolygonCountPanic = "box2d: Polygon.Count is invalid: must be between 1 and MaxPolygonVertices; " +
		"build polygons with MakePolygon, MakeBox or a related constructor"

	// wantPolygonShapeCountPanic is the panic text for the shape creation entry
	// points, where only a real convex polygon is meaningful.
	wantPolygonShapeCountPanic = "box2d: Polygon.Count is invalid: must be between 3 and MaxPolygonVertices; " +
		"build polygons with MakePolygon, MakeBox or a related constructor"

	// wantSimplexCacheCountPanic is the panic text for a warm-start cache that
	// claims more vertices than a GJK simplex can hold.
	wantSimplexCacheCountPanic = "box2d: SimplexCache.Count is invalid: must be between 0 and 3; " +
		"zero initialize SimplexCache before the first ShapeDistance call"
)

// badPolygon returns a Polygon literal with the given Count and vertices that
// are otherwise fine. This is exactly the mistake the preconditions catch: the
// struct is exported, so nothing stops a caller from filling it in by hand.
func badPolygon(count int) box2d.Polygon {
	polygon := box2d.MakeBox(0.5, 0.5)
	polygon.Count = count
	return polygon
}

// testChainSegment returns a chain segment along the x axis with both ghost
// vertices in place, suitable for CollideChainSegmentAndPolygon.
func testChainSegment() box2d.ChainSegment {
	return box2d.ChainSegment{
		Ghost1:  box2d.Vec2{X: -2.0, Y: 0.0},
		Segment: box2d.Segment{Point1: box2d.Vec2{X: -1.0, Y: 0.0}, Point2: box2d.Vec2{X: 1.0, Y: 0.0}},
		Ghost2:  box2d.Vec2{X: 2.0, Y: 0.0},
		ChainID: -1,
	}
}

func TestComputePolygonMassRejectsInvalidCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		count int
	}{
		{name: "count above MaxPolygonVertices", count: box2d.MaxPolygonVertices + 1},
		{name: "count far above MaxPolygonVertices", count: 20},
		{name: "zero count", count: 0},
		{name: "negative count", count: -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			polygon := badPolygon(tc.count)
			requirePanicMessage(t, wantPolygonCountPanic, "Polygon.Count", func() {
				box2d.ComputePolygonMass(&polygon, 1.0)
			})
		})
	}
}

func TestComputePolygonMassAcceptsValidPolygons(t *testing.T) {
	t.Parallel()

	// A 1x1 box of density 1 has mass 1 and its centroid at the box center.
	box := box2d.MakeBox(0.5, 0.5)
	mass := box2d.ComputePolygonMass(&box, 1.0)
	require.InDelta(t, 1.0, mass.Mass, 1e-12)
	require.InDelta(t, 0.0, mass.Center.X, 1e-12)
	require.InDelta(t, 0.0, mass.Center.Y, 1e-12)

	// The 1- and 2-vertex polygons the package uses for circles and capsules
	// stay legal at this entry point.
	for _, count := range []int{1, 2, box2d.MaxPolygonVertices} {
		polygon := badPolygon(count)
		require.NotPanics(t, func() {
			box2d.ComputePolygonMass(&polygon, 1.0)
		}, "count %d must remain valid", count)
	}
}

func TestCollideChainSegmentAndPolygonRejectsInvalidCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		count int
	}{
		{name: "count above MaxPolygonVertices", count: box2d.MaxPolygonVertices + 1},
		{name: "count far above MaxPolygonVertices", count: 20},
		{name: "zero count", count: 0},
		{name: "negative count", count: -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			segment := testChainSegment()
			polygon := badPolygon(tc.count)
			var cache box2d.SimplexCache
			requirePanicMessage(t, wantPolygonCountPanic, "Polygon.Count", func() {
				box2d.CollideChainSegmentAndPolygon(&segment, box2d.TransformIdentity,
					&polygon, box2d.TransformIdentity, &cache)
			})
		})
	}
}

func TestCollideChainSegmentAndPolygonAcceptsValidPolygon(t *testing.T) {
	t.Parallel()

	segment := testChainSegment()

	// A box resting on the colliding side of the chain segment still produces
	// the two-point manifold it produced before the precondition was added.
	box := box2d.MakeOffsetBox(0.5, 0.5, box2d.Vec2{X: 0.0, Y: -0.4}, box2d.MakeRot(0.0))
	var cache box2d.SimplexCache
	manifold := box2d.CollideChainSegmentAndPolygon(&segment, box2d.TransformIdentity,
		&box, box2d.TransformIdentity, &cache)
	require.Equal(t, 2, manifold.PointCount)

	// CollideChainSegmentAndCapsule funnels a 2-vertex polygon through the same
	// entry point, so that path must not trip the precondition either.
	capsule := box2d.Capsule{
		Center1: box2d.Vec2{X: -0.25, Y: -0.4},
		Center2: box2d.Vec2{X: 0.25, Y: -0.4},
		Radius:  0.25,
	}
	var capsuleCache box2d.SimplexCache
	require.NotPanics(t, func() {
		box2d.CollideChainSegmentAndCapsule(&segment, box2d.TransformIdentity,
			&capsule, box2d.TransformIdentity, &capsuleCache)
	})
}

func TestShapeDistanceRejectsInvalidCacheCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		count uint16
	}{
		{name: "one past the simplex size", count: 4},
		{name: "far past the simplex size", count: 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			input := shapeDistanceInput()
			cache := box2d.SimplexCache{Count: tc.count}
			requirePanicMessage(t, wantSimplexCacheCountPanic, "SimplexCache.Count", func() {
				box2d.ShapeDistance(&input, &cache, nil)
			})
		})
	}
}

func TestShapeDistanceAcceptsValidCacheCount(t *testing.T) {
	t.Parallel()

	// A zero cache is the documented first-call value and must keep working.
	input := shapeDistanceInput()
	var cache box2d.SimplexCache
	output := box2d.ShapeDistance(&input, &cache, nil)
	require.InDelta(t, 1.0, output.Distance, 1e-9)

	// So must a cache warm started by the previous call, and every count a GJK
	// simplex can legally report.
	require.LessOrEqual(t, cache.Count, uint16(3))
	warmInput := shapeDistanceInput()
	warmOutput := box2d.ShapeDistance(&warmInput, &cache, nil)
	require.InDelta(t, output.Distance, warmOutput.Distance, 0.0)

	for count := uint16(0); count <= 3; count++ {
		boundaryInput := shapeDistanceInput()
		boundaryCache := box2d.SimplexCache{Count: count}
		require.NotPanics(t, func() {
			box2d.ShapeDistance(&boundaryInput, &boundaryCache, nil)
		}, "cache count %d must remain valid", count)
	}
}

// shapeDistanceInput returns two unit boxes separated by a gap of 1 along x.
func shapeDistanceInput() box2d.DistanceInput {
	boxA := box2d.MakeBox(0.5, 0.5)
	boxB := box2d.MakeBox(0.5, 0.5)

	var input box2d.DistanceInput
	input.ProxyA = box2d.MakeProxy(boxA.Vertices[:], boxA.Count, 0.0)
	input.ProxyB = box2d.MakeProxy(boxB.Vertices[:], boxB.Count, 0.0)
	input.TransformA = box2d.TransformIdentity
	input.TransformB = box2d.Transform{P: box2d.Vec2{X: 2.0, Y: 0.0}, Q: box2d.MakeRot(0.0)}
	input.UseRadii = false
	return input
}

func TestCreatePolygonShapeRejectsInvalidCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		count int
	}{
		{name: "count above MaxPolygonVertices", count: box2d.MaxPolygonVertices + 1},
		{name: "count far above MaxPolygonVertices", count: 20},
		{name: "degenerate two vertex polygon", count: 2},
		{name: "zero count", count: 0},
		{name: "negative count", count: -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := preconditionWorld(t)
			bd := box2d.DefaultBodyDef()
			bd.Type = box2d.DynamicBody
			bodyID := w.CreateBody(&bd)

			sd := box2d.DefaultShapeDef()
			polygon := badPolygon(tc.count)
			requirePanicMessage(t, wantPolygonShapeCountPanic, "Polygon.Count", func() {
				w.CreatePolygonShape(bodyID, &sd, &polygon)
			})
		})
	}
}

func TestSetShapePolygonRejectsInvalidCount(t *testing.T) {
	t.Parallel()

	w := preconditionWorld(t)
	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bodyID := w.CreateBody(&bd)

	sd := box2d.DefaultShapeDef()
	box := box2d.MakeBox(0.5, 0.5)
	shapeID := w.CreatePolygonShape(bodyID, &sd, &box)

	polygon := badPolygon(box2d.MaxPolygonVertices + 1)
	requirePanicMessage(t, wantPolygonShapeCountPanic, "Polygon.Count", func() {
		w.SetShapePolygon(shapeID, &polygon)
	})
}

func TestPolygonShapeCreationAcceptsValidPolygons(t *testing.T) {
	t.Parallel()

	w := preconditionWorld(t)
	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bodyID := w.CreateBody(&bd)

	sd := box2d.DefaultShapeDef()
	box := box2d.MakeBox(0.5, 0.5)
	shapeID := w.CreatePolygonShape(bodyID, &sd, &box)
	require.True(t, shapeID.IsNonNull())

	// A hull built the supported way (ComputeHull plus MakePolygon) is accepted
	// at both counts a caller is likely to hit: the triangle lower bound and the
	// MaxPolygonVertices upper bound.
	triangleHull := box2d.ComputeHull([]box2d.Vec2{
		{X: -0.5, Y: -0.5}, {X: 0.5, Y: -0.5}, {X: 0.0, Y: 0.5},
	})
	triangle := box2d.MakePolygon(&triangleHull, 0.0)
	require.Equal(t, 3, triangle.Count)
	require.NotPanics(t, func() {
		w.SetShapePolygon(shapeID, &triangle)
	})

	octagonPoints := make([]box2d.Vec2, 0, box2d.MaxPolygonVertices)
	for i := range box2d.MaxPolygonVertices {
		angle := 2.0 * box2d.Pi * float64(i) / float64(box2d.MaxPolygonVertices)
		rot := box2d.MakeRot(angle)
		octagonPoints = append(octagonPoints, box2d.Vec2{X: rot.C, Y: rot.S})
	}
	octagonHull := box2d.ComputeHull(octagonPoints)
	octagon := box2d.MakePolygon(&octagonHull, 0.0)
	require.Equal(t, box2d.MaxPolygonVertices, octagon.Count)
	require.NotPanics(t, func() {
		w.SetShapePolygon(shapeID, &octagon)
	})

	// The world still steps with a valid polygon shape attached.
	require.NotPanics(t, func() {
		w.Step(1.0/60.0, 4)
	})
}

// ---------------------------------------------------------------------------
// ShapeProxy.Count / SimplexCache indices
// ---------------------------------------------------------------------------

// validProxy builds a proxy the supported way, for the happy-path assertions.
func validProxy(offset float64) box2d.ShapeProxy {
	return box2d.MakeProxy([]box2d.Vec2{
		{X: offset, Y: 0}, {X: offset + 1, Y: 0}, {X: offset + 1, Y: 1}, {X: offset, Y: 1},
	}, 4, 0)
}

// TestProxyCountIsValidated covers the point clouds a caller hands to the
// distance routines. ShapeProxy has exported fields, so Count can name more
// points than the fixed-size array holds; each of these entry points then
// copies or subscripts that array.
func TestProxyCountIsValidated(t *testing.T) {
	t.Parallel()

	const wantA = "box2d: DistanceInput.ProxyA.Count is invalid: " +
		"must be between 1 and MaxPolygonVertices; build proxies with MakeProxy"

	t.Run("ShapeDistance oversized", func(t *testing.T) {
		t.Parallel()
		in := box2d.DistanceInput{
			ProxyA:     validProxy(0),
			ProxyB:     validProxy(5),
			TransformA: box2d.TransformIdentity,
			TransformB: box2d.TransformIdentity,
			UseRadii:   false,
		}
		in.ProxyA.Count = box2d.MaxPolygonVertices + 1
		var cache box2d.SimplexCache
		requirePanicMessage(t, wantA, "MakeProxy", func() {
			box2d.ShapeDistance(&in, &cache, nil)
		})
	})

	t.Run("ShapeDistance zero", func(t *testing.T) {
		t.Parallel()
		in := box2d.DistanceInput{
			ProxyA:     validProxy(0),
			ProxyB:     validProxy(5),
			TransformA: box2d.TransformIdentity,
			TransformB: box2d.TransformIdentity,
		}
		in.ProxyA.Count = 0
		var cache box2d.SimplexCache
		requirePanicMessage(t, wantA, "MakeProxy", func() {
			box2d.ShapeDistance(&in, &cache, nil)
		})
	})

	t.Run("ShapeCast oversized", func(t *testing.T) {
		t.Parallel()
		in := box2d.ShapeCastPairInput{
			ProxyA:       validProxy(0),
			ProxyB:       validProxy(5),
			TransformA:   box2d.TransformIdentity,
			TransformB:   box2d.TransformIdentity,
			TranslationB: box2d.Vec2{X: -10, Y: 0},
			MaxFraction:  1,
		}
		in.ProxyB.Count = 99
		requirePanicMessage(t,
			"box2d: ShapeCastPairInput.ProxyB.Count is invalid: "+
				"must be between 1 and MaxPolygonVertices; build proxies with MakeProxy",
			"MakeProxy", func() { box2d.ShapeCast(&in) })
	})

	t.Run("TimeOfImpact oversized", func(t *testing.T) {
		t.Parallel()
		in := box2d.TOIInput{
			ProxyA:      validProxy(0),
			ProxyB:      validProxy(5),
			SweepA:      box2d.Sweep{Q1: box2d.RotIdentity, Q2: box2d.RotIdentity},
			SweepB:      box2d.Sweep{Q1: box2d.RotIdentity, Q2: box2d.RotIdentity},
			MaxFraction: 1,
		}
		in.ProxyA.Count = box2d.MaxPolygonVertices + 3
		requirePanicMessage(t,
			"box2d: TOIInput.ProxyA.Count is invalid: "+
				"must be between 1 and MaxPolygonVertices; build proxies with MakeProxy",
			"MakeProxy", func() { box2d.TimeOfImpact(&in) })
	})
}

// TestSimplexCacheIndicesAreValidated covers a warm-start cache that names
// points the proxies do not have -- a stale cache reused against smaller
// shapes, or one built by hand. The indices are used to subscript the proxy
// point clouds directly.
func TestSimplexCacheIndicesAreValidated(t *testing.T) {
	t.Parallel()

	newInput := func() box2d.DistanceInput {
		return box2d.DistanceInput{
			ProxyA:     box2d.MakeProxy([]box2d.Vec2{{X: 0, Y: 0}}, 1, 0),
			ProxyB:     box2d.MakeProxy([]box2d.Vec2{{X: 4, Y: 0}}, 1, 0),
			TransformA: box2d.TransformIdentity,
			TransformB: box2d.TransformIdentity,
		}
	}

	t.Run("IndexA past ProxyA", func(t *testing.T) {
		t.Parallel()
		in := newInput()
		cache := box2d.SimplexCache{Count: 1, IndexA: [3]uint8{7}, IndexB: [3]uint8{0}}
		requirePanicMessage(t,
			"box2d: SimplexCache.IndexA is invalid: must index a point of ProxyA; "+
				"pass the cache from the previous call on the same shapes",
			"ProxyA", func() { box2d.ShapeDistance(&in, &cache, nil) })
	})

	t.Run("IndexB past ProxyB", func(t *testing.T) {
		t.Parallel()
		in := newInput()
		cache := box2d.SimplexCache{Count: 1, IndexA: [3]uint8{0}, IndexB: [3]uint8{5}}
		requirePanicMessage(t,
			"box2d: SimplexCache.IndexB is invalid: must index a point of ProxyB; "+
				"pass the cache from the previous call on the same shapes",
			"ProxyB", func() { box2d.ShapeDistance(&in, &cache, nil) })
	})

	t.Run("warm start round trip still works", func(t *testing.T) {
		t.Parallel()
		in := newInput()
		var cache box2d.SimplexCache
		first := box2d.ShapeDistance(&in, &cache, nil)
		require.InDelta(t, 4.0, first.Distance, 1e-12)

		// Feeding the returned cache back in is the supported warm-start path
		// and must not trip the new validation.
		second := box2d.ShapeDistance(&in, &cache, nil)
		require.InDelta(t, first.Distance, second.Distance, 0)
	})
}
