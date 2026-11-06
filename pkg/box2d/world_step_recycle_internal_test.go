// Internal tests for the contact-recycling rotation guard in collideTask
// (pkg/box2d/world_step.go). Internal package tests because they inspect
// contactSim.cachedTransformA/B directly to distinguish the recycle fast path
// (cache not refreshed) from the full-update path (cache refreshed).

package box2d

import (
	"fmt"
	"math"
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findContactSimForBody returns the contact sim for the first contact on the
// given body, or nil if the body has no contacts.
func findContactSimForBody(w *World, bodyID BodyID) *contactSim {
	b := w.getBodyFullID(bodyID)
	key := b.headContactKey
	if key == NullIndex {
		return nil
	}
	contactID := key >> 1
	c := &w.contacts[contactID]
	return w.getContactSim(c)
}

// dynamicCachedRotation returns the cached rotation of the dynamic body in a
// contact sim. The contact stores cached transforms for bodyA and bodyB; this
// returns the one belonging to the given dynamicBodyID.
func dynamicCachedRotation(sim *contactSim, w *World, dynamicBodyID BodyID) Rot {
	shapeA := &w.shapes[sim.shapeIDA]
	if shapeA.bodyID == w.getBodyFullID(dynamicBodyID).id {
		return sim.cachedTransformA.Q
	}
	return sim.cachedTransformB.Q
}

// recycleWorld creates a world with gravity disabled and sleep off, the
// shared initialization for the recycling fixtures. The world is registered
// for test cleanup.
func recycleWorld(t *testing.T) *World {
	t.Helper()

	def := DefaultWorldDef()
	def.Gravity = Vec2Zero
	def.EnableSleep = false
	w := NewWorld(&def)
	t.Cleanup(w.Destroy)
	return w
}

// createBoxBody creates a dynamic body with a box shape of the given
// half-extents at pos, with contact events enabled per enableEvents. It
// returns the body and shape.
func createBoxBody(t *testing.T, w *World, halfExtent Vec2, pos Vec2, enableEvents bool) (BodyID, ShapeID) {
	t.Helper()

	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = pos
	body := w.CreateBody(&bd)
	box := MakeBox(halfExtent.X, halfExtent.Y)
	sd := DefaultShapeDef()
	sd.EnableContactEvents = enableEvents
	shape := w.CreatePolygonShape(body, &sd, &box)
	return body, shape
}

// recycleTestWorld builds a world with a static ground (top surface at y=0,
// centered at y=-0.5) and a single dynamic box of the given half-extents placed
// at pos. Gravity is disabled and sleep is off; callers that rotate the body
// reset its transform before each collision step, so any solver movement
// during a step is discarded before the next collision. Contact events are
// enabled on both
// shapes when enableEvents is true. The recycle distance is left at the default
// (ContactRecycleDistance) so callers can override it for an isolated cosine
// gate (SetContactRecycleDistance) or keep the default to exercise the
// distance/sine gate. Density is left at the ShapeDef default (1.0).
func recycleTestWorld(t *testing.T, halfExtent Vec2, pos Vec2, enableEvents bool) (*World, BodyID, ShapeID, ShapeID) {
	t.Helper()

	w := recycleWorld(t)

	gbd := DefaultBodyDef()
	gbd.Position = Vec2{X: 0.0, Y: -0.5}
	ground := w.CreateBody(&gbd)
	groundBox := MakeBox(50.0, 0.5)
	gsd := DefaultShapeDef()
	gsd.EnableContactEvents = enableEvents
	groundShape := w.CreatePolygonShape(ground, &gsd, &groundBox)

	body, bodyShape := createBoxBody(t, w, halfExtent, pos, enableEvents)

	return w, body, groundShape, bodyShape
}

// recycleGuardTestWorld builds a small dynamic box (half-extents 0.05, i.e.
// 0.1 x 0.1, maxExtent ≈ 0.07) resting on the ground at y = 0.04, configured so
// the contact-recycle sin metric passes even for large rotations but the
// per-body cos guard is the deciding factor. The recycle distance is set to
// 200× the default so distance + maxExtent*|sin θ| never blocks recycling on
// its own.
func recycleGuardTestWorld(t *testing.T) (*World, BodyID) {
	t.Helper()
	w, body, _, _ := recycleTestWorld(t, Vec2{X: 0.05, Y: 0.05}, Vec2{X: 0.0, Y: 0.04}, false)
	w.SetContactRecycleDistance(200 * ContactRecycleDistance)
	return w, body
}

// rotateAndStep teleports the body to its rest position (0, 0.04) and the
// given angle, then steps the world once. This is the rest position of the
// recycleGuardTestWorld box, so the helper shares the transform-and-step
// operation. It returns the contact sim re-fetched after the step.
func rotateAndStep(t *testing.T, w *World, body BodyID, angle float64) *contactSim {
	t.Helper()
	w.SetBodyTransform(body, Vec2{X: 0.0, Y: 0.04}, MakeRot(angle))
	stepWorld(w, 1)
	sim := findContactSimForBody(w, body)
	require.NotNil(t, sim, "contact must still exist after rotation to %.1f deg", angle*180.0/math.Pi)
	return sim
}

// requireCachedRot asserts the cached rotation of the dynamic body equals the
// expected Rot. The cached transform is copied verbatim from the body's
// pre-solve transform during collideTask (and SetBodyTransform writes that
// transform directly), so the cached rotation is bit-identical to the rotation
// set by the test.
func requireCachedRot(t *testing.T, sim *contactSim, w *World, body BodyID, expected Rot, msg string) {
	t.Helper()
	cached := dynamicCachedRotation(sim, w, body)
	require.Equal(t, expected, cached, "%s: cached rotation mismatch", msg)
}

// requireShapeIDPair asserts the two event shape IDs are exactly ground and
// body in either order.
func requireShapeIDPair(t *testing.T, idA, idB, ground, body ShapeID, msg string) {
	t.Helper()
	require.True(t, (idA == ground && idB == body) || (idA == body && idB == ground),
		"%s: event shape IDs {%v, %v} must be {%v, %v}", msg, idA, idB, ground, body)
}

// stepAndFindContact performs the shared initial-step setup: step the world
// once, then find and validate the contact sim for the given body. It returns
// the contact sim with its cached relative transform confirmed valid.
func stepAndFindContact(t *testing.T, w *World, body BodyID) *contactSim {
	t.Helper()
	stepWorld(w, 1)
	sim := findContactSimForBody(w, body)
	require.NotNil(t, sim, "contact must exist after first step")
	require.NotEqual(t, uint32(0), sim.simFlags&simRelativeTransformValid,
		"cached relative transform must be valid after first step")
	return sim
}

// requireOldRecyclePredicatePasses asserts the legacy distance/sine recycle
// predicate (without the per-body cos guard) would recycle this contact, so a
// cache refresh can only be attributed to the new guard. It recomputes the
// predicate from the contact's current body transforms and cached relative
// transform, mirroring collideTask. Call this after setting the rotated
// transform but before stepping, while the cache still holds the pre-rotation
// value. This guards against a ground/body sort that makes the old distance
// check reject recycling on its own, which would let the regression pass
// without the guard.
func requireOldRecyclePredicatePasses(t *testing.T, w *World, sim *contactSim, msg string) {
	t.Helper()
	shapeA := &w.shapes[sim.shapeIDA]
	shapeB := &w.shapes[sim.shapeIDB]
	bodyA := &w.bodies[shapeA.bodyID]
	bodyB := &w.bodies[shapeB.bodyID]
	transformA := w.getBodySim(bodyA).transform
	transformB := w.getBodySim(bodyB).transform
	xf := InvMulTransforms(transformA, transformB)
	xfc := InvMulTransforms(sim.cachedTransformA, sim.cachedTransformB)
	maxExtentA := 0.0
	if bodyA.bodyType != StaticBody {
		maxExtentA = w.getBodySim(bodyA).maxExtent
	}
	maxExtentB := 0.0
	if bodyB.bodyType != StaticBody {
		maxExtentB = w.getBodySim(bodyB).maxExtent
	}
	maxExtent := maxFloat(maxExtentA, maxExtentB)
	distance := Distance(xf.P, xfc.P)
	qr := InvMulRot(xf.Q, xfc.Q)
	recycleDistance := w.contactRecycleDistance
	tolerance := minFloat(recycleDistance, SpeculativeDistance)
	if sim.simFlags&simTouchingFlag != 0 {
		tolerance = recycleDistance
	}
	wouldRecycle := distance+float64(maxExtent*absFloat(qr.S)) < tolerance
	require.True(t, wouldRecycle, "%s: old distance/sine predicate must pass so recycling would happen without the guard", msg)
}

// TestContactRecycleCosAngleConstant verifies the port mirrors upstream's
// B2_CONTACT_RECYCLE_COS_ANGLE (0.98f ~= 11.5 degrees).
func TestContactRecycleCosAngleConstant(t *testing.T) {
	t.Parallel()
	tassert.Equal(t, 0.98, ContactRecycleCosAngle) //nolint:testifylint // exact constant equality is intended, not a tolerance comparison
}

// TestContactRecycleRotationGuardBlocksLargeRotation verifies that the
// per-body rotation guard (minCos > ContactRecycleCosAngle) prevents the
// contact-recycling fast path when a body has rotated more than ~11.5 degrees
// since its cached transform.
func TestContactRecycleRotationGuardBlocksLargeRotation(t *testing.T) {
	t.Parallel()
	testContactRecycleRotationGuard(t, 30.0*math.Pi/180.0, true)
}

// TestContactRecycleRotationGuardAllowsSmallRotation verifies that the guard
// still permits recycling when the accumulated rotation is small (< ~11.5
// degrees), so the fast path is taken and the cached transform is NOT
// refreshed. The fixture's static ground contributes cos = 1.0, so only the
// dynamic body's rotation gates recycling; this also covers the static-body
// case.
func TestContactRecycleRotationGuardAllowsSmallRotation(t *testing.T) {
	t.Parallel()
	testContactRecycleRotationGuard(t, 5.0*math.Pi/180.0, false)
}

// TestContactRecycleRotationGuardAllowsNearCutoff verifies the guard boundary
// just inside the threshold: 11° has cos = 0.9816 > 0.98, so recycling is
// allowed and the cache is unchanged. The enlarged recycle distance isolates
// the cosine gate.
func TestContactRecycleRotationGuardAllowsNearCutoff(t *testing.T) {
	t.Parallel()
	testContactRecycleRotationGuard(t, 11.0*math.Pi/180.0, false)
}

// TestContactRecycleRotationGuardBlocksNearCutoff verifies the guard boundary
// just outside the threshold: 12° has cos = 0.9781 < 0.98, so recycling is
// blocked and the cache is refreshed. The enlarged recycle distance isolates
// the cosine gate.
func TestContactRecycleRotationGuardBlocksNearCutoff(t *testing.T) {
	t.Parallel()
	testContactRecycleRotationGuard(t, 12.0*math.Pi/180.0, true)
}

// testContactRecycleRotationGuard steps the recycle-guard test world once to
// establish the contact and cache, then rotates the body by angle and steps
// again. If expectRefresh is true, the guard must block recycling and refresh
// the cached rotation to the pre-solve rotation (full update path). If false,
// the guard must allow recycling and the cached rotation must stay at the
// pre-rotation value (recycle path).
func testContactRecycleRotationGuard(t *testing.T, angle float64, expectRefresh bool) {
	t.Helper()

	w, body := recycleGuardTestWorld(t)

	sim := stepAndFindContact(t, w, body)
	requireCachedRot(t, sim, w, body, RotIdentity, "initial cache must be 0 degrees")

	sim = rotateAndStep(t, w, body, angle)

	if expectRefresh {
		requireCachedRot(t, sim, w, body, MakeRot(angle), "cache must be refreshed after full update")
	} else {
		requireCachedRot(t, sim, w, body, RotIdentity, "cache must be unchanged after recycle")
	}
}

// TestContactRecycleAccumulatedRotation verifies the guard tracks accumulated
// rotation since the cache, not just single-step rotation. A body rotated 5°
// per step recycles for the first two steps (accumulated 5° and 10°, cos > 0.98)
// but gets a full update on the third step when the accumulated rotation
// crosses 11.5 degrees (cos(15°) = 0.966 < 0.98), refreshing the cache.
func TestContactRecycleAccumulatedRotation(t *testing.T) {
	t.Parallel()

	w, body := recycleGuardTestWorld(t)

	stepAndFindContact(t, w, body)

	// 5° then 10° accumulated from the 0° cache: cos > 0.98, so recycling
	// keeps the cache at 0°.
	for step := 1; step <= 2; step++ {
		angle := float64(step) * 5.0 * math.Pi / 180.0
		sim := rotateAndStep(t, w, body, angle)
		requireCachedRot(t, sim, w, body, RotIdentity,
			fmt.Sprintf("step %d: cache must stay at 0 degrees (recycled, not refreshed)", step+1))
	}

	// 15° accumulated from the 0° cache: cos(15°) < 0.98, so the guard blocks
	// recycling and the full update refreshes the cache to 15°.
	angle15 := 15.0 * math.Pi / 180.0
	rot15 := MakeRot(angle15)
	sim := rotateAndStep(t, w, body, angle15)
	requireCachedRot(t, sim, w, body, rot15,
		"step 4: cache must be refreshed to 15 degrees after full update")

	// The cache is now at 15°, so 20° is only 5° further: cos > 0.98 again,
	// recycling keeps the cache at 15°.
	sim = rotateAndStep(t, w, body, 20.0*math.Pi/180.0)
	requireCachedRot(t, sim, w, body, rot15,
		"step 5: cache must stay at 15 degrees (recycled, not refreshed)")
}

// twoOverlappingBoxes builds a world with two overlapping 0.1×0.1 dynamic
// boxes (half-extents 0.05) centered at x = ∓0.03, the configuration the
// co-rotation and asymmetric-rotation regressions share. Gravity and sleep are
// off. The asymmetric-rotation test sets an enlarged recycle distance; the
// co-rotation test keeps the default.
func twoOverlappingBoxes(t *testing.T) (*World, BodyID, BodyID) {
	t.Helper()

	w := recycleWorld(t)
	boxHalf := Vec2{X: 0.05, Y: 0.05}
	bodyA, _ := createBoxBody(t, w, boxHalf, Vec2{X: -0.03, Y: 0.0}, false)
	bodyB, _ := createBoxBody(t, w, boxHalf, Vec2{X: 0.03, Y: 0.0}, false)

	return w, bodyA, bodyB
}

// TestContactRecycleCoRotatingDynamicBodies covers the case the old relative
// metric misses but the per-body guard catches: two dynamic bodies rotated
// together. Only their relative orientation is unchanged (qr = identity);
// rotating both in place shifts the relative translation by about 0.031,
// still below the default 0.05 recycle distance, so the old recycle metric
// (distance + maxExtent*|sin(qr)|) is satisfied. But each body has rotated
// away from its own cached transform, so minCos drops below the guard and the
// full update refreshes both caches.
func TestContactRecycleCoRotatingDynamicBodies(t *testing.T) {
	t.Parallel()

	w, bodyA, bodyB := twoOverlappingBoxes(t)

	sim := stepAndFindContact(t, w, bodyA)

	// Rotate both bodies by 30°: relative orientation is unchanged but each
	// body rotated 30° from its cache, so the guard forces a full update.
	rot := MakeRot(30.0 * math.Pi / 180.0)
	w.SetBodyTransform(bodyA, Vec2{X: -0.03, Y: 0.0}, rot)
	w.SetBodyTransform(bodyB, Vec2{X: 0.03, Y: 0.0}, rot)
	requireOldRecyclePredicatePasses(t, w, sim, "co-rotation default distance")
	stepWorld(w, 1)

	sim = findContactSimForBody(w, bodyA)
	require.NotNil(t, sim, "contact must still exist after co-rotation")
	requireCachedRot(t, sim, w, bodyA, rot, "body A cache must be refreshed to 30 degrees after full update")
	requireCachedRot(t, sim, w, bodyB, rot, "body B cache must be refreshed to 30 degrees after full update")
}

// TestContactRecycleAsymmetricRotation covers a guard that only checks cosB:
// only body A rotates past the cutoff while B stays fixed, so a cosB-only
// guard (cosB = 1.0 > 0.98) would still recycle. The enlarged recycle distance
// makes the old distance/sine predicate pass, so only the per-body cos gate
// (cosA = cos(30°) = 0.866 < 0.98) blocks recycling and refreshes body A's
// cache. This isolates the cosA check.
func TestContactRecycleAsymmetricRotation(t *testing.T) {
	t.Parallel()

	w, bodyA, _ := twoOverlappingBoxes(t)
	w.SetContactRecycleDistance(200 * ContactRecycleDistance)

	sim := stepAndFindContact(t, w, bodyA)
	require.Equal(t, w.getBodyFullID(bodyA).id, w.shapes[sim.shapeIDA].bodyID,
		"body A must be shapeA so cosA gates this regression")

	// Rotate only body A by 30°; body B stays fixed (cosB = 1.0). The old
	// predicate passes at the enlarged distance; only cosA trips the guard.
	rot := MakeRot(30.0 * math.Pi / 180.0)
	w.SetBodyTransform(bodyA, Vec2{X: -0.03, Y: 0.0}, rot)
	requireOldRecyclePredicatePasses(t, w, sim, "asymmetric enlarged distance")
	stepWorld(w, 1)

	sim = findContactSimForBody(w, bodyA)
	require.NotNil(t, sim, "contact must still exist after asymmetric rotation")
	requireCachedRot(t, sim, w, bodyA, rot, "body A cache must be refreshed to 30 degrees after full update")
}

// TestContactRecycleNear180Rotation covers a near-180° flip at the default
// recycle distance. At 175° the sine term is small again (sin(175°) =
// sin(5°) ≈ 0.087, maxExtent*|sin| ≈ 0.006 < 0.05), so the old distance/sine
// gate would recycle the stale manifold at the default setting. The per-body
// guard sees cos(175°) ≈ -0.996 < 0.98 and forces a full update that refreshes
// the cache. The default recycle distance (0.05) is used; the flipped box still
// overlaps ground so the contact survives the rotation.
func TestContactRecycleNear180Rotation(t *testing.T) {
	t.Parallel()

	w, body, _, _ := recycleTestWorld(t, Vec2{X: 0.05, Y: 0.05}, Vec2{X: 0.0, Y: 0.04}, false)

	sim := stepAndFindContact(t, w, body)

	angle175 := 175.0 * math.Pi / 180.0
	rot := MakeRot(angle175)
	w.SetBodyTransform(body, Vec2{X: 0.0, Y: 0.04}, rot)
	requireOldRecyclePredicatePasses(t, w, sim, "near-180 default distance")
	stepWorld(w, 1)

	sim = findContactSimForBody(w, body)
	require.NotNil(t, sim, "contact must still exist after 175 degree rotation")
	requireCachedRot(t, sim, w, body, rot, "cache must be refreshed to 175 degrees after full update")
}

// runRecycleEndEvent is the shared behavioral regression for the reported
// contact-event symptom. The contact starts touching; the plank is then
// rotated so its base corner lifts off the ground and the shapes stop
// touching. Without the per-body guard, collideTask would recycle the stale
// touching manifold and keep simTouchingFlag set, so the end-touch event
// would be missed. The guard forces a full update that detects the
// transition and reports the event. recycleDistance is set explicitly: pass
// ContactRecycleDistance for the default distance/sine gate, or a larger
// value to isolate the cosine gate. The old distance/sine predicate is
// asserted to pass first, so the cache refresh is attributable to the new
// guard regardless of ground/body sorting.
func runRecycleEndEvent(t *testing.T, halfExtent Vec2, pos Vec2, angle float64, recycleDistance float64) {
	t.Helper()

	w, body, groundShape, bodyShape := recycleTestWorld(t, halfExtent, pos, true)
	w.SetContactRecycleDistance(recycleDistance)

	// Step 1: establish the touching contact and cache the transform at 0°.
	sim := stepAndFindContact(t, w, body)
	beginEvents := w.ContactEvents().BeginEvents
	require.Len(t, beginEvents, 1, "begin event must fire when the plank first touches")
	requireShapeIDPair(t, beginEvents[0].ShapeIDA, beginEvents[0].ShapeIDB, groundShape, bodyShape, "begin event")
	require.NotEqual(t, uint32(0), sim.simFlags&simTouchingFlag, "contact must start touching")

	// Rotate the plank so its base corner lifts off the ground. The old
	// distance/sine predicate passes; the guard blocks recycling and the
	// full update detects the end-touching transition.
	rot := MakeRot(angle)
	w.SetBodyTransform(body, pos, rot)
	requireOldRecyclePredicatePasses(t, w, sim, "end-event recycle gate")
	stepWorld(w, 1)

	endEvents := w.ContactEvents().EndEvents
	require.Len(t, endEvents, 1, "end event must fire once the rotated plank separates")
	requireShapeIDPair(t, endEvents[0].ShapeIDA, endEvents[0].ShapeIDB, groundShape, bodyShape, "end event")
}

// TestContactRecycleRotationGuardEndEvent is the behavioral regression at the
// enlarged recycle distance that isolates the cosine gate. Rotating the plank
// past the guard threshold (30°, cos = 0.866 < 0.98) swings its base corner up
// off the ground so the shapes stop touching.
func TestContactRecycleRotationGuardEndEvent(t *testing.T) {
	t.Parallel()
	runRecycleEndEvent(t, Vec2{X: 0.05, Y: 0.5}, Vec2{X: 0.0, Y: 0.49}, 30.0*math.Pi/180.0, 200*ContactRecycleDistance)
}

// TestContactRecycleRotationGuardEndEventDefaultDistance is the behavioral
// regression at the default recycle distance (0.05), matching how the bug
// surfaces in a default-configured world. The plank is thin (half-extents
// 0.01 x 0.5) so maxExtent times |sin(175°)| stays under the default 0.05 and
// the old gate would recycle. A near-180° flip is used because separating a
// centered box by rotation alone needs a large |sin θ|, which the small
// default distance would not let through; near 180° the sine term is small
// again while cos(175°) ≈ -0.996 still trips the per-body guard. The plank
// height is derived from its half-height, SpeculativeDistance, and an initial
// margin so it starts touching and the 175° flip lifts its lowest corner just
// beyond SpeculativeDistance.
func TestContactRecycleRotationGuardEndEventDefaultDistance(t *testing.T) {
	t.Parallel()

	const (
		plankHalfHeight = 0.5
		initialMargin   = 0.001
	)
	plankY := plankHalfHeight + SpeculativeDistance - initialMargin
	runRecycleEndEvent(t, Vec2{X: 0.01, Y: plankHalfHeight}, Vec2{X: 0.0, Y: plankY}, 175.0*math.Pi/180.0, ContactRecycleDistance)
}
