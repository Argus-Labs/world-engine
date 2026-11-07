// Internal tests for the contact-recycling rotation guard in collideTask
// (pkg/box2d/world_step.go). Internal package tests because they inspect
// contactSim.cachedTransformA/B directly to distinguish the recycle fast path
// (cache not refreshed) from the full-update path (cache refreshed).

package box2d

import (
	"math"
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// enlargedRecycleDistance is 200x the default, large enough that distance +
// maxExtent*|sin θ| never blocks recycling on its own, isolating the cosine
// gate.
var enlargedRecycleDistance = 200 * ContactRecycleDistance

// guardRestPos is the rest position shared by the recycle-guard fixtures and
// rotateAndStep.
var guardRestPos = Vec2{X: 0.0, Y: 0.04}

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
// centered at y=-0.5) and a single dynamic box of the given half-extents at
// pos. Gravity and sleep are off; contact events are enabled on both shapes
// when enableEvents is true. The recycle distance is left at the default
// (ContactRecycleDistance) so callers can override it for an isolated cosine
// gate (SetContactRecycleDistance) or keep the default to exercise the
// distance/sine gate. Transform-setting tests reset the pose before each
// step; the spinning test preserves integrated motion.
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
// 0.1 x 0.1, maxExtent ≈ 0.07) overlapping the ground by 0.01 (center y = 0.04,
// half-height 0.05), configured so
// the contact-recycle sin metric passes even for large rotations but the
// per-body cos guard is the deciding factor. The recycle distance is set to
// 200× the default so distance + maxExtent*|sin θ| never blocks recycling on
// its own.
func recycleGuardTestWorld(t *testing.T) (*World, BodyID) {
	t.Helper()
	w, body, _, _ := recycleTestWorld(t, Vec2{X: 0.05, Y: 0.05}, guardRestPos, false)
	w.SetContactRecycleDistance(enlargedRecycleDistance)
	return w, body
}

// rotateAndStep teleports the body to its rest position (guardRestPos) and the
// given angle, then steps the world once and returns the contact sim and the
// rotation it applied, so callers can reuse the rotation instead of recomputing
// MakeRot for the expected cache value.
func rotateAndStep(t *testing.T, w *World, body BodyID, angle float64) (*contactSim, Rot) {
	t.Helper()
	rot := MakeRot(angle)
	w.SetBodyTransform(body, guardRestPos, rot)
	sim := stepFindNotNil(t, w, body, "contact must still exist after rotation to %.1f deg", angle*180.0/Pi)
	return sim, rot
}

// requireCachedRot asserts the cached rotation of the dynamic body equals the
// expected Rot. The cached transform is copied verbatim from the body's
// pre-solve transform during collideTask (and SetBodyTransform writes that
// transform directly), so the cached rotation is bit-identical to the rotation
// set by the test.
func requireCachedRot(t *testing.T, sim *contactSim, w *World, body BodyID, expected Rot, msgAndArgs ...any) {
	t.Helper()
	cached := dynamicCachedRotation(sim, w, body)
	require.Equal(t, expected, cached, msgAndArgs...)
}

func requireShapeIDPair(t *testing.T, idA, idB, ground, body ShapeID, msg string) {
	t.Helper()
	if (idA == ground && idB == body) || (idA == body && idB == ground) {
		return
	}
	require.Failf(t, msg, "expected shape-id pair {ground=%v, body=%v}, got {a=%v, b=%v}", ground, body, idA, idB)
}

func stepFindNotNil(t *testing.T, w *World, body BodyID, msgAndArgs ...any) *contactSim {
	t.Helper()
	stepWorld(w, 1)
	sim := findContactSimForBody(w, body)
	require.NotNil(t, sim, msgAndArgs...)
	return sim
}

// stepAndFindContact steps once and returns the contact sim for body with its
// cached relative transform established for later guard assertions.
func stepAndFindContact(t *testing.T, w *World, body BodyID) *contactSim {
	t.Helper()
	sim := stepFindNotNil(t, w, body, "contact must exist after first step")
	require.NotEqual(t, uint32(0), sim.simFlags&simRelativeTransformValid,
		"cached relative transform must be valid after first step")
	return sim
}

// requireDistanceMetricPasses asserts the distance/sine recycle predicate
// (the part of the live recycle check that does not include the per-body cos
// guard) would recycle this contact, so a cache refresh can only be attributed
// to the new guard. It recomputes the predicate from the contact's current body
// transforms and cached relative transform, mirroring collideTask. Call this
// after setting the rotated transform but before stepping, while the cache
// still holds the pre-rotation value. This guards against a ground/body sort
// that makes the distance check reject recycling on its own, which would let
// the regression pass without the guard.
func requireDistanceMetricPasses(t *testing.T, w *World, sim *contactSim, msg string) {
	t.Helper()
	shapeA := &w.shapes[sim.shapeIDA]
	shapeB := &w.shapes[sim.shapeIDB]
	bodyA := &w.bodies[shapeA.bodyID]
	bodyB := &w.bodies[shapeB.bodyID]
	bodySimA := w.getBodySim(bodyA)
	bodySimB := w.getBodySim(bodyB)
	transformA := bodySimA.transform
	transformB := bodySimB.transform
	xf := InvMulTransforms(transformA, transformB)
	xfc := InvMulTransforms(sim.cachedTransformA, sim.cachedTransformB)
	maxExtentA := 0.0
	if bodyA.bodyType != StaticBody {
		maxExtentA = bodySimA.maxExtent
	}
	maxExtentB := 0.0
	if bodyB.bodyType != StaticBody {
		maxExtentB = bodySimB.maxExtent
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
	require.True(t, wouldRecycle, "%s: distance/sine predicate must pass so recycling would happen without the guard", msg)
}

// TestContactRecycleCosAngleConstant pins the local ContactRecycleCosAngle
// constant to 0.98 (~11.5 degrees). It is a local value pin, not an
// independent upstream-parity check.
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
	testContactRecycleRotationGuard(t, 30.0*Pi/180.0, true)
}

// TestContactRecycleRotationGuardAllowsSmallRotation verifies that the guard
// still permits recycling when the accumulated rotation is small (< ~11.5
// degrees), so the fast path is taken and the cached transform is NOT
// refreshed. The fixture's static ground contributes cos = 1.0, so only the
// dynamic body's rotation gates recycling; this also covers the static-body
// case.
func TestContactRecycleRotationGuardAllowsSmallRotation(t *testing.T) {
	t.Parallel()
	testContactRecycleRotationGuard(t, 5.0*Pi/180.0, false)
}

// TestContactRecycleRotationGuardAllowsNearCutoff verifies the guard boundary
// just inside the threshold: 11° has cos = 0.9816 > 0.98, so recycling is
// allowed and the cache is unchanged. The enlarged recycle distance isolates
// the cosine gate.
func TestContactRecycleRotationGuardAllowsNearCutoff(t *testing.T) {
	t.Parallel()
	testContactRecycleRotationGuard(t, 11.0*Pi/180.0, false)
}

// TestContactRecycleRotationGuardBlocksNearCutoff verifies the guard boundary
// just outside the threshold: 12° has cos = 0.9781 < 0.98, so recycling is
// blocked and the cache is refreshed. The enlarged recycle distance isolates
// the cosine gate.
func TestContactRecycleRotationGuardBlocksNearCutoff(t *testing.T) {
	t.Parallel()
	testContactRecycleRotationGuard(t, 12.0*Pi/180.0, true)
}

// TestContactRecycleRotationGuardExactCutoff verifies the guard boundary at
// minCos == ContactRecycleCosAngle. The guard uses a strict comparison
// (minCos > ContactRecycleCosAngle), so equality must block recycling and
// refresh the cache. The rotation's C is set directly to ContactRecycleCosAngle
// (without trigonometric rounding) so minCos hits the threshold exactly. The
// enlarged recycle distance isolates the cosine gate.
func TestContactRecycleRotationGuardExactCutoff(t *testing.T) {
	t.Parallel()

	w, body := recycleGuardTestWorld(t)

	sim := stepAndFindContact(t, w, body)
	requireCachedRot(t, sim, w, body, RotIdentity, "initial cache must be 0 degrees")

	// The ground is static, so its rotation is identity and its cosine
	// against the cache is 1.0; the rotating body's cosine against the
	// identity cache is its own C. Setting the body's C to exactly the
	// threshold gives min(cosBody, cosGround) == min(ContactRecycleCosAngle,
	// 1.0) == ContactRecycleCosAngle regardless of which shape is A or B.
	exactRot := Rot{C: ContactRecycleCosAngle, S: math.Sqrt(1.0 - ContactRecycleCosAngle*ContactRecycleCosAngle)}
	w.SetBodyTransform(body, guardRestPos, exactRot)
	sim = stepFindNotNil(t, w, body, "contact must still exist after exact-cutoff rotation")
	requireCachedRot(t, sim, w, body, exactRot, "cache must be refreshed at the exact cutoff (strict > comparison)")
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

	sim, rot := rotateAndStep(t, w, body, angle)

	if expectRefresh {
		requireCachedRot(t, sim, w, body, rot, "cache must be refreshed after full update")
	} else {
		requireCachedRot(t, sim, w, body, RotIdentity, "cache must be unchanged after recycle")
	}
}

// TestContactRecycleAccumulatedRotation verifies the guard tracks accumulated
// rotation since the cache, not just single-step rotation. A body rotated 5°
// per step recycles for the first two steps (accumulated 5° and 10°, cos > 0.98)
// but gets a full update on the third rotation step (step 4, counting the
// initial setup step) when the accumulated rotation crosses 11.5 degrees
// (cos(15°) = 0.966 < 0.98), refreshing the cache.
func TestContactRecycleAccumulatedRotation(t *testing.T) {
	t.Parallel()

	w, body := recycleGuardTestWorld(t)

	stepAndFindContact(t, w, body)

	// 5° then 10° accumulated from the 0° cache: cos > 0.98, so recycling
	// keeps the cache at 0°.
	for step := 1; step <= 2; step++ {
		angle := float64(step) * 5.0 * Pi / 180.0
		sim, _ := rotateAndStep(t, w, body, angle)
		requireCachedRot(t, sim, w, body, RotIdentity,
			"step %d: cache must stay at 0 degrees (recycled, not refreshed)", step+1)
	}

	// 15° accumulated from the 0° cache: cos(15°) < 0.98, so the guard blocks
	// recycling and the full update refreshes the cache to 15°.
	sim, rot15 := rotateAndStep(t, w, body, 15.0*Pi/180.0)
	requireCachedRot(t, sim, w, body, rot15,
		"step 4: cache must be refreshed to 15 degrees after full update")

	// The cache is now at 15°, so 20° is only 5° further: cos > 0.98 again,
	// recycling keeps the cache at 15°.
	sim, _ = rotateAndStep(t, w, body, 20.0*Pi/180.0)
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

// TestContactRecycleCoRotatingDynamicBodies covers the case the relative
// metric misses but the per-body guard catches: two dynamic bodies rotated
// together. Only their relative orientation is unchanged (qr = identity);
// rotating both in place shifts the relative translation by about 0.031,
// still below the default 0.05 recycle distance, so the recycle distance/sine
// metric (distance + maxExtent*|sin(qr)|) is satisfied. But each body has
// rotated away from its own cached transform, so minCos drops below the guard
// and the full update refreshes both caches.
func TestContactRecycleCoRotatingDynamicBodies(t *testing.T) {
	t.Parallel()

	w, bodyA, bodyB := twoOverlappingBoxes(t)

	sim := stepAndFindContact(t, w, bodyA)

	// Rotate both bodies by 30°: relative orientation is unchanged but each
	// body rotated 30° from its cache, so the guard forces a full update.
	rot := MakeRot(30.0 * Pi / 180.0)
	w.SetBodyTransform(bodyA, Vec2{X: -0.03, Y: 0.0}, rot)
	w.SetBodyTransform(bodyB, Vec2{X: 0.03, Y: 0.0}, rot)
	requireDistanceMetricPasses(t, w, sim, "co-rotation default distance")
	sim = stepFindNotNil(t, w, bodyA, "contact must still exist after co-rotation")
	requireCachedRot(t, sim, w, bodyA, rot, "body A cache must be refreshed to 30 degrees after full update")
	requireCachedRot(t, sim, w, bodyB, rot, "body B cache must be refreshed to 30 degrees after full update")
}

// TestContactRecycleAsymmetricRotation covers a guard that only checks cosB:
// only body A rotates past the cutoff while B stays fixed, so a cosB-only
// guard (cosB = 1.0 > 0.98) would still recycle. The enlarged recycle distance
// makes the distance/sine predicate pass, so only the per-body cos gate
// (cosA = cos(30°) = 0.866 < 0.98) blocks recycling and refreshes body A's
// cache. This isolates the cosA check.
func TestContactRecycleAsymmetricRotation(t *testing.T) {
	t.Parallel()

	w, bodyA, _ := twoOverlappingBoxes(t)
	w.SetContactRecycleDistance(enlargedRecycleDistance)

	sim := stepAndFindContact(t, w, bodyA)
	require.Equal(t, w.getBodyFullID(bodyA).id, w.shapes[sim.shapeIDA].bodyID,
		"body A must be shapeA so cosA gates this regression")

	// Rotate only body A by 30°; body B stays fixed (cosB = 1.0). The
	// predicate passes at the enlarged distance; only cosA trips the guard.
	rot := MakeRot(30.0 * Pi / 180.0)
	w.SetBodyTransform(bodyA, Vec2{X: -0.03, Y: 0.0}, rot)
	requireDistanceMetricPasses(t, w, sim, "asymmetric enlarged distance")
	sim = stepFindNotNil(t, w, bodyA, "contact must still exist after asymmetric rotation")
	requireCachedRot(t, sim, w, bodyA, rot, "body A cache must be refreshed to 30 degrees after full update")
}

// TestContactRecycleNear180Rotation covers a near-180° flip at the default
// recycle distance. At 175° the sine term is small again (sin(175°) =
// sin(5°) ≈ 0.087, maxExtent*|sin| ≈ 0.006 < 0.05), so the distance/sine
// gate would recycle the stale manifold at the default setting. The per-body
// guard sees cos(175°) ≈ -0.996 < 0.98 and forces a full update that refreshes
// the cache. The default recycle distance (0.05) is used; the flipped box still
// overlaps ground so the contact survives the rotation.
func TestContactRecycleNear180Rotation(t *testing.T) {
	t.Parallel()

	w, body, _, _ := recycleTestWorld(t, Vec2{X: 0.05, Y: 0.05}, guardRestPos, false)

	sim := stepAndFindContact(t, w, body)

	angle175 := 175.0 * Pi / 180.0
	rot := MakeRot(angle175)
	w.SetBodyTransform(body, guardRestPos, rot)
	requireDistanceMetricPasses(t, w, sim, "near-180 default distance")
	sim = stepFindNotNil(t, w, body, "contact must still exist after 175 degree rotation")
	requireCachedRot(t, sim, w, body, rot, "cache must be refreshed to 175 degrees after full update")
}

// setupRecycleEventWorld builds the event-enabled recycle test world, sets the
// recycle distance, and steps once to establish the contact and cache the
// transform at 0°. The shared setup for the end- and begin-event regressions;
// callers assert their distinct initial state and event expectations.
func setupRecycleEventWorld(t *testing.T, halfExtent Vec2, pos Vec2, recycleDistance float64) (*World, BodyID, ShapeID, ShapeID, *contactSim) {
	t.Helper()

	w, body, groundShape, bodyShape := recycleTestWorld(t, halfExtent, pos, true)
	w.SetContactRecycleDistance(recycleDistance)

	sim := stepAndFindContact(t, w, body)
	return w, body, groundShape, bodyShape, sim
}

// runRecycleEndEvent is the shared behavioral regression for the reported
// contact-event symptom. The contact starts touching; the plank is then
// rotated so its base corner lifts off the ground and the shapes stop
// touching. Without the per-body guard, collideTask would recycle the stale
// touching manifold and keep simTouchingFlag set, so the end-touch event
// would be missed. The guard forces a full update that detects the
// transition and reports the event. recycleDistance is set explicitly: pass
// ContactRecycleDistance for the default distance/sine gate, or a larger
// value to isolate the cosine gate. The distance/sine predicate is
// asserted to pass first, so the cache refresh is attributable to the new
// guard regardless of ground/body sorting.
func runRecycleEndEvent(t *testing.T, halfExtent Vec2, pos Vec2, angle float64, recycleDistance float64) {
	t.Helper()

	w, body, groundShape, bodyShape, sim := setupRecycleEventWorld(t, halfExtent, pos, recycleDistance)
	beginEvents := w.ContactEvents().BeginEvents
	require.Len(t, beginEvents, 1, "begin event must fire when the plank first touches")
	requireShapeIDPair(t, beginEvents[0].ShapeIDA, beginEvents[0].ShapeIDB, groundShape, bodyShape, "begin event")
	require.NotEqual(t, uint32(0), sim.simFlags&simTouchingFlag, "contact must start touching")

	// Rotate the plank so its base corner lifts off the ground. The
	// distance/sine predicate passes; the guard blocks recycling and the
	// full update detects the end-touching transition.
	rot := MakeRot(angle)
	w.SetBodyTransform(body, pos, rot)
	requireDistanceMetricPasses(t, w, sim, "end-event recycle gate")
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
	runRecycleEndEvent(t, Vec2{X: 0.05, Y: 0.5}, Vec2{X: 0.0, Y: 0.49}, 30.0*Pi/180.0, enlargedRecycleDistance)
}

// TestContactRecycleRotationGuardEndEventDefaultDistance is the behavioral
// regression at the default recycle distance (0.05), matching how the bug
// surfaces in a default-configured world. The plank is thin (half-extents
// 0.01 x 0.5) so maxExtent times |sin(175°)| stays under the default 0.05 and
// the distance/sine gate would recycle. A near-180° flip is used because separating a
// centered box by rotation alone needs a large |sin θ|, which the small
// default distance would not let through; near 180° the sine term is small
// again while cos(175°) ≈ -0.996 still trips the per-body guard. The plank's
// center y is derived from its half-height, SpeculativeDistance, and an initial
// margin so it starts touching and the 175° flip lifts its lowest corner just
// beyond SpeculativeDistance.
func TestContactRecycleRotationGuardEndEventDefaultDistance(t *testing.T) {
	t.Parallel()

	const (
		plankHalfHeight = 0.5
		initialMargin   = 0.001
	)
	plankY := plankHalfHeight + SpeculativeDistance - initialMargin
	runRecycleEndEvent(t, Vec2{X: 0.01, Y: plankHalfHeight}, Vec2{X: 0.0, Y: plankY}, 175.0*Pi/180.0, ContactRecycleDistance)
}

// runRecycleBeginEvent is the inverse behavioral regression: the contact
// starts non-touching (wasTouching = false) and a per-body rotation brings a
// corner into contact, so a begin-touch event must fire. The end-event
// regressions above all start touching and exercise separation, so they do
// not cover the newly-touching path a non-touching contact follows. For a
// non-touching contact the recycle tolerance is the tighter
// min(recycleDistance, SpeculativeDistance), so the distance/sine gate alone
// rejects recycling once the rotation moves the corner past SpeculativeDistance
// and the full update reports the begin event; this test locks that behavior
// in so a later change that recycles non-touching contacts cannot silently
// drop the begin event.
func runRecycleBeginEvent(t *testing.T, halfExtent Vec2, pos Vec2, angle float64, recycleDistance float64) {
	t.Helper()

	w, body, groundShape, bodyShape, sim := setupRecycleEventWorld(t, halfExtent, pos, recycleDistance)
	require.Empty(t, w.ContactEvents().BeginEvents, "no begin event before the body rotates into contact")
	require.Equal(t, uint32(0), sim.simFlags&simTouchingFlag, "contact must start non-touching")

	// Rotate so a corner comes into contact. The recycle fast path is
	// rejected (the corner moves past SpeculativeDistance), so the full
	// update runs and detects the begin-touching transition.
	rot := MakeRot(angle)
	w.SetBodyTransform(body, pos, rot)
	stepWorld(w, 1)

	beginEvents := w.ContactEvents().BeginEvents
	require.Len(t, beginEvents, 1, "begin event must fire once the rotated body touches")
	requireShapeIDPair(t, beginEvents[0].ShapeIDA, beginEvents[0].ShapeIDB, groundShape, bodyShape, "begin event")
}

// TestContactRecycleBeginEvent is the non-touching → rotate → begin-event
// regression at the default recycle distance. A wide, flat box
// (half-extents 0.3 x 0.1) rests above the ground (center y = 0.13, gap 0.03)
// so the contact is created non-touching (gap > SpeculativeDistance) but the
// fat AABBs overlap. Rotating 30° drops a bottom corner into a clear
// overlap, and the begin-touch event must fire.
func TestContactRecycleBeginEvent(t *testing.T) {
	t.Parallel()
	runRecycleBeginEvent(t, Vec2{X: 0.3, Y: 0.1}, Vec2{X: 0.0, Y: 0.13}, 30.0*Pi/180.0, ContactRecycleDistance)
}

// TestContactRecycleSpinningBody exercises the guard through normal angular
// integration at the default recycle distance. Unlike the transform-setting
// fixtures above, the body spins via SetBodyAngularVelocity and the world
// integrates its rotation each step. The cache stays at 0° while the integrated
// rotation remains under the ~11.5° guard threshold, then refreshes once the
// accumulated rotation crosses it. The distance/sine gate is asserted to still
// pass at that first refresh, so the refresh is attributable to the rotation
// guard rather than the distance gate (which a longer loop could block
// recycling on its own).
func TestContactRecycleSpinningBody(t *testing.T) {
	t.Parallel()

	w, body, _, _ := recycleTestWorld(t, Vec2{X: 0.05, Y: 0.05}, guardRestPos, false)

	sim := stepAndFindContact(t, w, body)
	requireCachedRot(t, sim, w, body, RotIdentity, "initial cache must be 0 degrees")

	// Spin through normal integration; the contact survives the small overlap.
	// spinAngularVelocity rotates the body ~9.55°/step at 1/60 s per step
	// (10 rad/s * 1/60 s). The guard threshold is
	// acos(ContactRecycleCosAngle) ≈ 11.48°, so the cache refreshes by step 3;
	// spinSteps is sized with a generous margin over that.
	const (
		spinAngularVelocity = 10.0
		spinSteps           = 16
	)
	w.SetBodyAngularVelocity(body, spinAngularVelocity)

	// cosBody for this fixture is the dynamic body's cosine against its cached
	// rotation (the static ground contributes 1.0); the guard trips when that
	// cosine drops to ContactRecycleCosAngle.
	prevCosBody := 1.0
	for step := 1; step <= spinSteps; step++ {
		// Before stepping, the cache still holds the pre-rotation value;
		// the distance/sine gate must pass so a refresh can only come from
		// the rotation guard, not the distance gate.
		requireDistanceMetricPasses(t, w, sim, "spinning body distance gate before refresh")

		// Pre-step cosine of the dynamic body vs its cached rotation, the
		// same value the guard compares against ContactRecycleCosAngle.
		bodySim := w.getBodySim(w.getBodyFullID(body))
		cachedRot := dynamicCachedRotation(sim, w, body)
		cosBody := dot2(bodySim.transform.Q.C, cachedRot.C, bodySim.transform.Q.S, cachedRot.S)

		sim = stepFindNotNil(t, w, body, "contact must survive the spin")
		if cached := dynamicCachedRotation(sim, w, body); cached != cachedRot {
			// The guard tripped at this step: the pre-step cosine reached
			// the threshold and the previous step's cosine was still above
			// it, so this is the first threshold crossing — a refresh the
			// distance gate (asserted passing above) could not have caused.
			require.LessOrEqual(t, cosBody, ContactRecycleCosAngle,
				"pre-step cosine must reach the threshold at the refresh step (step %d)", step)
			require.Greater(t, prevCosBody, ContactRecycleCosAngle,
				"previous step must still be above the threshold (first crossing, step %d)", step)
			return
		}
		prevCosBody = cosBody
	}
	t.Fatal("cache was never refreshed away from 0 degrees within the spin step budget")
}
