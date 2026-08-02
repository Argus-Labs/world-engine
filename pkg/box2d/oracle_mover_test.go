// Oracle tests for mover.go (upstream src/mover.c + docs/character.md).
// Expectations are hand-derived from the C algorithm, never from running the
// Go port. Citations reference the vendored mover.c.

package box2d_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// rigid returns an effectively-rigid push limit (upstream uses FLT_MAX).
const rigidLimit = 1e30

// TestOracleSolvePlanesNoPlanes: with no planes the loop body never runs,
// totalPush is 0 < tolerance on the first iteration, so the C returns the
// target delta with iterationCount 1 (the for counts the completed pass;
// mover.c:17-49 breaks after iteration 0 completes, then returns iteration,
// which the break leaves at 0... the break happens before the increment, so
// iterationCount is 0). Verify the exact C control flow: `break` exits with
// `iteration` still 0.
func TestOracleSolvePlanesNoPlanes(t *testing.T) {
	t.Parallel()
	res := box2d.SolvePlanes(box2d.Vec2{X: 3, Y: -2}, nil)
	assert.Equal(t, box2d.Vec2{X: 3, Y: -2}, res.Translation, "no planes leave the delta untouched")
	assert.Equal(t, 0, res.IterationCount, "mover.c:44-47: break fires in the first pass before iteration increments")
}

// TestOracleSolvePlanesSingleWall: one rigid plane with normal +X at offset 0
// and a target delta pushing into it. Per mover.c:26-38, the plane pushes the
// delta out so the final separation is -LinearSlop (separation+slop driven to
// zero): dot(n, delta) - offset = -LinearSlop. With n=(1,0), offset=0 and
// target x=-1, the solved translation.x must be -LinearSlop within the
// solver's convergence tolerance (LinearSlop).
func TestOracleSolvePlanesSingleWall(t *testing.T) {
	t.Parallel()
	planes := []box2d.CollisionPlane{{
		Plane:        box2d.Plane{Normal: box2d.Vec2{X: 1, Y: 0}, Offset: 0},
		PushLimit:    rigidLimit,
		ClipVelocity: true,
	}}
	res := box2d.SolvePlanes(box2d.Vec2{X: -1, Y: 0.5}, planes)

	// Push is along +X only, so Y passes through untouched.
	assert.InDelta(t, 0.5, res.Translation.Y, 0)
	// Final separation ~ -LinearSlop; tolerance is the solver's own
	// convergence bound (mover.c:15).
	assert.InDelta(t, -box2d.LinearSlop, res.Translation.X, 2*box2d.LinearSlop)
	// The plane accumulated the push it applied: from -1 toward -slop,
	// so push ~ 1 - slop.
	assert.InDelta(t, 1.0-box2d.LinearSlop, planes[0].Push, 2*box2d.LinearSlop)
}

// TestOracleSolvePlanesPushLimit: a soft plane cannot push more than its
// PushLimit (mover.c:35-37 clamps the accumulated push). With limit 0.25
// against a 1.0 penetration, the translation only recovers 0.25.
func TestOracleSolvePlanesPushLimit(t *testing.T) {
	t.Parallel()
	planes := []box2d.CollisionPlane{{
		Plane:     box2d.Plane{Normal: box2d.Vec2{X: 1, Y: 0}, Offset: 0},
		PushLimit: 0.25,
	}}
	res := box2d.SolvePlanes(box2d.Vec2{X: -1, Y: 0}, planes)
	assert.InDelta(t, 0.25, planes[0].Push, 0, "push clamps exactly at the limit")
	assert.InDelta(t, -0.75, res.Translation.X, 1e-12, "delta recovers exactly the clamped push")
	// Iteration 0 applies the clamped 0.25 push (totalPush 0.25, continue);
	// iteration 1 computes a positive raw push but the accumulated clamp at
	// the limit makes the incremental push exactly 0, so totalPush < tolerance
	// and the loop breaks with iteration == 1 (mover.c:33-47).
	assert.Equal(t, 1, res.IterationCount)
}

// TestOracleSolvePlanesCorner: two rigid perpendicular walls (+X and +Y).
// docs/character.md: the solver finds a position satisfying all planes, so
// both final separations sit at ~-LinearSlop.
func TestOracleSolvePlanesCorner(t *testing.T) {
	t.Parallel()
	planes := []box2d.CollisionPlane{
		{Plane: box2d.Plane{Normal: box2d.Vec2{X: 1, Y: 0}, Offset: 0}, PushLimit: rigidLimit},
		{Plane: box2d.Plane{Normal: box2d.Vec2{X: 0, Y: 1}, Offset: 0}, PushLimit: rigidLimit},
	}
	res := box2d.SolvePlanes(box2d.Vec2{X: -0.5, Y: -0.5}, planes)
	assert.InDelta(t, -box2d.LinearSlop, res.Translation.X, 2*box2d.LinearSlop)
	assert.InDelta(t, -box2d.LinearSlop, res.Translation.Y, 2*box2d.LinearSlop)
}

// TestOracleSolvePlanesResetsPush: mover.c:9-12 zeroes every plane's push on
// entry, so reusing a planes slice does not accumulate across calls.
func TestOracleSolvePlanesResetsPush(t *testing.T) {
	t.Parallel()
	planes := []box2d.CollisionPlane{{
		Plane:     box2d.Plane{Normal: box2d.Vec2{X: 1, Y: 0}, Offset: 0},
		PushLimit: rigidLimit,
		Push:      123.0, // stale value from a previous call
	}}
	res := box2d.SolvePlanes(box2d.Vec2{X: 1, Y: 0}, planes)
	// Target is already outside the plane (separation 1+slop > 0), so the
	// clamp at zero keeps push at 0 and the delta passes through.
	require.InDelta(t, 0.0, planes[0].Push, 0)
	assert.InDelta(t, 1.0, res.Translation.X, 0)
}

// TestOracleClipVector: mover.c:57-71. Only planes with nonzero push AND
// ClipVelocity clip; clipping removes the into-plane component
// (v -= min(0, dot(v,n)) * n) and never adds an outward one.
func TestOracleClipVector(t *testing.T) {
	t.Parallel()

	wall := box2d.Plane{Normal: box2d.Vec2{X: 1, Y: 0}, Offset: 0}

	t.Run("clips into-plane component", func(t *testing.T) {
		t.Parallel()
		planes := []box2d.CollisionPlane{{Plane: wall, Push: 0.1, ClipVelocity: true}}
		v := box2d.ClipVector(box2d.Vec2{X: -3, Y: 2}, planes)
		assert.InDelta(t, 0.0, v.X, 0, "into-plane velocity removed exactly")
		assert.InDelta(t, 2.0, v.Y, 0, "tangential velocity preserved")
	})

	t.Run("outward velocity untouched", func(t *testing.T) {
		t.Parallel()
		planes := []box2d.CollisionPlane{{Plane: wall, Push: 0.1, ClipVelocity: true}}
		v := box2d.ClipVector(box2d.Vec2{X: 3, Y: 2}, planes)
		assert.Equal(t, box2d.Vec2{X: 3, Y: 2}, v, "dot > 0 means min(0, dot) = 0: no change")
	})

	t.Run("zero push skipped", func(t *testing.T) {
		t.Parallel()
		planes := []box2d.CollisionPlane{{Plane: wall, Push: 0, ClipVelocity: true}}
		v := box2d.ClipVector(box2d.Vec2{X: -3, Y: 2}, planes)
		assert.Equal(t, box2d.Vec2{X: -3, Y: 2}, v, "mover.c:63: push == 0 skips the plane")
	})

	t.Run("soft planes skipped", func(t *testing.T) {
		t.Parallel()
		planes := []box2d.CollisionPlane{{Plane: wall, Push: 0.1, ClipVelocity: false}}
		v := box2d.ClipVector(box2d.Vec2{X: -3, Y: 2}, planes)
		assert.Equal(t, box2d.Vec2{X: -3, Y: 2}, v, "mover.c:63: clipVelocity == false skips the plane")
	})
}

// TestOracleMoverPipeline is the docs/character.md loop: CollideMover gathers
// planes, SolvePlanes resolves the position, ClipVector adjusts velocity. A
// capsule mover overlapping a static wall must be pushed out along the wall
// normal and lose its into-wall velocity.
func TestOracleMoverPipeline(t *testing.T) {
	t.Parallel()

	def := box2d.DefaultWorldDef()
	w := box2d.NewWorld(&def)
	defer w.Destroy()

	// Static wall: box occupying x in [1, 2].
	bd := box2d.DefaultBodyDef()
	bd.Position = box2d.Vec2{X: 1.5, Y: 0}
	wallBody := w.CreateBody(&bd)
	sd := box2d.DefaultShapeDef()
	wallBox := box2d.MakeBox(0.5, 4)
	w.CreatePolygonShape(wallBody, &sd, &wallBox)

	// Capsule mover overlapping the wall's left face.
	mover := box2d.Capsule{
		Center1: box2d.Vec2{X: 0.9, Y: -0.25},
		Center2: box2d.Vec2{X: 0.9, Y: 0.25},
		Radius:  0.3,
	}

	var planes []box2d.CollisionPlane
	w.CollideMover(&mover, box2d.DefaultQueryFilter(), func(shapeID box2d.ShapeID, pr *box2d.PlaneResult, _ any) bool {
		planes = append(planes, box2d.CollisionPlane{
			Plane:        pr.Plane,
			PushLimit:    rigidLimit,
			ClipVelocity: true,
		})
		return true
	}, nil)
	require.NotEmpty(t, planes, "an overlapping mover must produce at least one collision plane")

	// The wall's left face pushes along -X.
	res := box2d.SolvePlanes(box2d.Vec2{}, planes)
	assert.Negative(t, res.Translation.X, "mover is pushed out of the wall, along -X")

	v := box2d.ClipVector(box2d.Vec2{X: 5, Y: 1}, planes)
	assert.LessOrEqual(t, v.X, 1e-9, "into-wall velocity clipped")
	assert.InDelta(t, 1.0, v.Y, 1e-9, "tangential velocity preserved")
}
