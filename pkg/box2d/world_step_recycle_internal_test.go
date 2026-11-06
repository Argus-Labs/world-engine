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

// recycleTestWorld builds a world with a static ground (top surface at y=0,
// centered at y=-0.5) and a single dynamic box of the given half-extents placed
// at pos. Gravity is disabled and sleep is off, so the body only moves where
// the test drives it. Contact events are enabled on both shapes when
// enableEvents is true. The recycle distance is left at the default
// (ContactRecycleDistance) so callers can override it for an isolated cosine
// gate (SetContactRecycleDistance) or keep the default to exercise the
// distance/sine gate. Density is left at the ShapeDef default (1.0).
func recycleTestWorld(t *testing.T, halfExtent Vec2, pos Vec2, enableEvents bool) (*World, BodyID, ShapeID, ShapeID) {
	t.Helper()

	def := DefaultWorldDef()
	def.Gravity = Vec2Zero
	def.EnableSleep = false
	w := NewWorld(&def)
	t.Cleanup(w.Destroy)

	// Static ground: top surface at y = 0, center at y = -0.5.
	gbd := DefaultBodyDef()
	gbd.Position = Vec2{X: 0.0, Y: -0.5}
	ground := w.CreateBody(&gbd)
	groundBox := MakeBox(50.0, 0.5)
	gsd := DefaultShapeDef()
	gsd.EnableContactEvents = enableEvents
	groundShape := w.CreatePolygonShape(ground, &gsd, &groundBox)

	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = pos
	body := w.CreateBody(&bd)
	box := MakeBox(halfExtent.X, halfExtent.Y)
	sd := DefaultShapeDef()
	sd.EnableContactEvents = enableEvents
	bodyShape := w.CreatePolygonShape(body, &sd, &box)

	return w, body, groundShape, bodyShape
}

// recycleGuardTestWorld builds a small dynamic box (half-extents 0.05, i.e.
// 0.1 x 0.1, maxExtent ≈ 0.07) resting on the ground at y = 0.04, configured so
// the contact-recycle sin metric passes even for large rotations but the
// per-body cos guard is the deciding factor. The recycle distance is set very
// large (200× the default) so distance + maxExtent*|sin θ| never blocks
// recycling on its own.
func recycleGuardTestWorld(t *testing.T) (*World, BodyID) {
	t.Helper()
	w, body, _, _ := recycleTestWorld(t, Vec2{X: 0.05, Y: 0.05}, Vec2{X: 0.0, Y: 0.04}, false)
	w.SetContactRecycleDistance(10.0)
	return w, body
}

// recycleEventTestWorld builds a tall thin dynamic plank (half-extents 0.05 x
// 0.5, i.e. 0.1 x 1.0) standing upright, centered at y = 0.49 so its base
// overlaps the ground by 0.01 — the contact starts touching. Contact events are
// enabled on both shapes. The recycle distance is set very large (200× the
// default) so the sin metric alone never blocks recycling — the cos guard is
// the only gate that can force a full update. Gravity is disabled and the test
// resets the plank transform before each collision step, so the body only
// moves where the test drives it via SetBodyTransform. Rotating the plank about
// its center swings its base corner up off the ground: at 30° the lowest corner
// rises to about 0.032, beyond SpeculativeDistance (0.02), so the manifold loses
// its contact points and the contact transitions touching -> non-touching,
// reporting an end event.
func recycleEventTestWorld(t *testing.T) (*World, BodyID, ShapeID, ShapeID) {
	t.Helper()
	w, body, groundShape, bodyShape := recycleTestWorld(t, Vec2{X: 0.05, Y: 0.5}, Vec2{X: 0.0, Y: 0.49}, true)
	w.SetContactRecycleDistance(10.0)
	return w, body, groundShape, bodyShape
}

// stepAndRotate teleports the body to the given angle, resetting its position
// to (0, 0.04) — it does not preserve the body's post-solve position — and
// steps the world once. It returns the contact sim re-fetched after the step.
func stepAndRotate(t *testing.T, w *World, body BodyID, angle float64) *contactSim {
	t.Helper()
	w.SetBodyTransform(body, Vec2{X: 0.0, Y: 0.04}, MakeRot(angle))
	stepWorld(w, 1)
	sim := findContactSimForBody(w, body)
	require.NotNil(t, sim, "contact must still exist after rotation to %.1f deg", angle*180.0/math.Pi)
	return sim
}

// requireCachedRot asserts the cached rotation of the dynamic body matches the
// expected Rot. The cached transform is captured during collideTask (pre-solve),
// so it matches the rotation set via SetBodyTransform, not the post-solve body
// rotation.
func requireCachedRot(t *testing.T, sim *contactSim, w *World, body BodyID, expected Rot, msg string) {
	t.Helper()
	cached := dynamicCachedRotation(sim, w, body)
	require.InDelta(t, expected.C, cached.C, 1e-15, "%s: C mismatch", msg)
	require.InDelta(t, expected.S, cached.S, 1e-15, "%s: S mismatch", msg)
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

// TestContactRecycleCosAngleConstant verifies the port mirrors upstream's
// B2_CONTACT_RECYCLE_COS_ANGLE (0.98f ~= 11.5 degrees).
func TestContactRecycleCosAngleConstant(t *testing.T) {
	t.Parallel()
	tassert.InDelta(t, 0.98, ContactRecycleCosAngle, 0.0)
}

// TestContactRecycleRotationGuardBlocksLargeRotation verifies that the
// per-body rotation guard (minCos > ContactRecycleCosAngle) prevents the
// contact-recycling fast path when a body has rotated more than ~11.5 degrees
// since its cached transform. Without the guard the port would recycle the
// stale manifold across hundreds of degrees of accumulated rotation.
func TestContactRecycleRotationGuardBlocksLargeRotation(t *testing.T) {
	t.Parallel()
	testContactRecycleRotationGuard(t, 30.0*math.Pi/180.0, true)
}

// TestContactRecycleRotationGuardAllowsSmallRotation verifies that the guard
// still permits recycling when the accumulated rotation is small (< ~11.5
// degrees), so the fast path is taken and the cached transform is NOT
// refreshed. The fixture's static ground contributes cos = 1.0, so only the
// dynamic body's rotation gates recycling.
func TestContactRecycleRotationGuardAllowsSmallRotation(t *testing.T) {
	t.Parallel()
	testContactRecycleRotationGuard(t, 5.0*math.Pi/180.0, false)
}

// testContactRecycleRotationGuard steps the recycle-guard test world once to
// establish the contact and cache, then rotates the body by angle and steps
// again. If expectRefresh is true, the guard must block recycling and the
// cached rotation must be refreshed to the pre-solve rotation (full update
// path). If false, the guard must allow recycling and the cached rotation
// must stay at the pre-rotation value (recycle path).
func testContactRecycleRotationGuard(t *testing.T, angle float64, expectRefresh bool) {
	t.Helper()

	w, body := recycleGuardTestWorld(t)

	// Step once to establish the contact and cache the transform at 0°.
	sim := stepAndFindContact(t, w, body)
	requireCachedRot(t, sim, w, body, RotIdentity, "initial cache must be 0 degrees")

	// Rotate the body by angle and step.
	sim = stepAndRotate(t, w, body, angle)

	if expectRefresh {
		// The guard blocked recycling: a full update ran and refreshed the
		// cached transform to the pre-solve rotation (the rotation set via
		// SetBodyTransform, which collideTask reads before the solver runs).
		requireCachedRot(t, sim, w, body, MakeRot(angle), "cache must be refreshed after full update")
	} else {
		// The guard allowed recycling: the cached transform was NOT refreshed,
		// so it still matches the pre-rotation value (0°).
		requireCachedRot(t, sim, w, body, RotIdentity, "cache must be unchanged after recycle")
	}
}

// TestContactRecycleAccumulatedRotation verifies that the per-body rotation
// guard tracks accumulated rotation since the cache, not just single-step
// rotation. A body rotated 5 degrees per step recycles for the first two
// steps (accumulated 5° and 10°, cos > 0.98) but gets a full update on the
// third step when the accumulated rotation crosses 11.5 degrees (cos(15°) =
// 0.966 < 0.98), refreshing the cache.
func TestContactRecycleAccumulatedRotation(t *testing.T) {
	t.Parallel()

	w, body := recycleGuardTestWorld(t)

	// Step 1: establish contact + cache at rotation 0.
	stepAndFindContact(t, w, body)

	// Steps 2-3: rotate 5° per step. Accumulated rotation from the cache
	// (still at 0°) is 5° then 10°, both with cos > 0.98, so the guard
	// allows recycling and the cache is NOT refreshed.
	for step := 1; step <= 2; step++ {
		angle := float64(step) * 5.0 * math.Pi / 180.0
		sim := stepAndRotate(t, w, body, angle)
		requireCachedRot(t, sim, w, body, RotIdentity,
			fmt.Sprintf("step %d: cache must stay at 0 (recycled, not refreshed)", step+1))
	}

	// Step 4: rotate to 15°. Accumulated rotation from the cache (still at
	// 0°) is 15°, cos(15°) = 0.966 < 0.98, so the guard blocks recycling and
	// the full update refreshes the cache to 15°.
	angle15 := 15.0 * math.Pi / 180.0
	rot15 := MakeRot(angle15)
	sim := stepAndRotate(t, w, body, angle15)
	requireCachedRot(t, sim, w, body, rot15,
		"step 4: cache must be refreshed to 15 degrees after full update")

	// Step 5: rotate to 20°. The cache was refreshed to 15° on step 4, so
	// the accumulated rotation is now 5°, cos(5°) = 0.996 > 0.98, and the
	// guard allows recycling again — the cache stays at 15°.
	sim = stepAndRotate(t, w, body, 20.0*math.Pi/180.0)
	requireCachedRot(t, sim, w, body, rot15,
		"step 5: cache must stay at 15 (recycled, not refreshed)")
}

// TestContactRecycleCoRotatingDynamicBodies covers the case the old relative
// metric misses but the per-body guard catches: two dynamic bodies rotated
// together. Their relative transform is unchanged, so the old recycle metric
// (distance + maxExtent*|sin(qr)|) is trivially satisfied at the default
// recycle distance (distance = 0, qr = identity). But each body has rotated
// away from its own cached transform, so minCos drops below the guard and the
// full update refreshes both caches. All existing rotation fixtures use one
// dynamic body against static ground; this exercises two dynamic bodies.
func TestContactRecycleCoRotatingDynamicBodies(t *testing.T) {
	t.Parallel()

	def := DefaultWorldDef()
	def.Gravity = Vec2Zero
	def.EnableSleep = false
	w := NewWorld(&def)
	t.Cleanup(w.Destroy)

	// Two overlapping dynamic boxes at the default recycle distance.
	box := MakeBox(0.05, 0.05)
	bdA := DefaultBodyDef()
	bdA.Type = DynamicBody
	bdA.Position = Vec2{X: -0.03, Y: 0.0}
	bodyA := w.CreateBody(&bdA)
	sdA := DefaultShapeDef()
	w.CreatePolygonShape(bodyA, &sdA, &box)

	bdB := DefaultBodyDef()
	bdB.Type = DynamicBody
	bdB.Position = Vec2{X: 0.03, Y: 0.0}
	bodyB := w.CreateBody(&bdB)
	sdB := DefaultShapeDef()
	w.CreatePolygonShape(bodyB, &sdB, &box)

	// Step once to establish the contact and cache both transforms at 0°.
	stepAndFindContact(t, w, bodyA)

	// Rotate both bodies by 30° together and step. The relative transform is
	// unchanged (qr = identity, distance = 0), so the old recycle metric would
	// recycle the stale manifold. The per-body guard sees cos(30°) = 0.866 <
	// 0.98 for each body and forces a full update that refreshes both caches.
	rot := MakeRot(30.0 * math.Pi / 180.0)
	w.SetBodyTransform(bodyA, Vec2{X: -0.03, Y: 0.0}, rot)
	w.SetBodyTransform(bodyB, Vec2{X: 0.03, Y: 0.0}, rot)
	stepWorld(w, 1)

	sim := findContactSimForBody(w, bodyA)
	require.NotNil(t, sim, "contact must still exist after co-rotation")
	requireCachedRot(t, sim, w, bodyA, rot, "body A cache must be refreshed to 30 degrees after full update")
}

// TestContactRecycleNear180Rotation covers a near-180° flip at the default
// recycle distance. At 175° the sine term is small again (sin(175°) =
// sin(5°) ≈ 0.087, maxExtent*|sin| ≈ 0.006 < 0.05), so the old distance/sine
// gate would recycle the stale manifold at the default setting. The per-body
// guard sees cos(175°) ≈ -0.996 < 0.98 and forces a full update that refreshes
// the cache. All other large-rotation fixtures use the enlarged recycle
// distance; this one keeps the default to exercise the default-setting
// failure mode.
func TestContactRecycleNear180Rotation(t *testing.T) {
	t.Parallel()

	// Default recycle distance (0.05); small box stays within its circumcircle
	// so the contact survives the rotation.
	w, body, _, _ := recycleTestWorld(t, Vec2{X: 0.05, Y: 0.05}, Vec2{X: 0.0, Y: 0.04}, false)

	stepAndFindContact(t, w, body)

	// Rotate to 175° and step. The old gate passes (small sine term at default
	// distance); the guard blocks recycling and refreshes the cache to 175°.
	rot := MakeRot(175.0 * math.Pi / 180.0)
	sim := stepAndRotate(t, w, body, 175.0*math.Pi/180.0)
	requireCachedRot(t, sim, w, body, rot, "cache must be refreshed to 175 degrees after full update")
}

// TestContactRecycleRotationGuardEndEvent is the behavioral regression for the
// reported contact-event symptom, at the enlarged recycle distance that
// isolates the cosine gate. The contact starts touching. The plank is then
// rotated past the guard threshold (30°, cos = 0.866 < 0.98), which swings its
// base corner up off the ground so the shapes stop touching.
//
// Without the per-body rotation guard, collideTask would recycle the stale
// touching manifold (the sin metric passes with the large recycle distance),
// skip updateContact, and keep simTouchingFlag set — so the end-touch event
// the application subscribed to would be missed. The guard forces a full
// update that detects the transition and reports the event.
func TestContactRecycleRotationGuardEndEvent(t *testing.T) {
	t.Parallel()

	w, body, groundShape, bodyShape := recycleEventTestWorld(t)

	// Step 1: establish the contact (touching) and cache the transform at
	// 0°. stepAndFindContact confirms the cached relative transform is valid.
	sim := stepAndFindContact(t, w, body)
	beginEvents := w.ContactEvents().BeginEvents
	require.Len(t, beginEvents, 1, "begin event must fire when the plank first touches")
	beginIDs := []ShapeID{beginEvents[0].ShapeIDA, beginEvents[0].ShapeIDB}
	require.Contains(t, beginIDs, groundShape)
	require.Contains(t, beginIDs, bodyShape)
	// Confirm the contact is touching so the regression is meaningful: a
	// transition exists for the next step to report.
	require.NotEqual(t, uint32(0), sim.simFlags&simTouchingFlag,
		"contact must start touching")

	// Rotate the plank by 30° — past the 11.5° guard — swinging the base
	// corner up off the ground so the shapes stop touching. Keep the position
	// fixed so only rotation drives the transition.
	w.SetBodyTransform(body, Vec2{X: 0.0, Y: 0.49}, MakeRot(30.0*math.Pi/180.0))
	stepWorld(w, 1)

	// The guard must have blocked recycling and run a full update that
	// detected the end-touching transition and reported the event.
	endEvents := w.ContactEvents().EndEvents
	require.Len(t, endEvents, 1, "end event must fire once the rotated plank separates")
	endIDs := []ShapeID{endEvents[0].ShapeIDA, endEvents[0].ShapeIDB}
	require.Contains(t, endIDs, groundShape)
	require.Contains(t, endIDs, bodyShape)
}

// TestContactRecycleRotationGuardEndEventDefaultDistance is the behavioral
// regression at the default recycle distance (0.05). The enlarged-distance
// end-event test isolates the cosine gate; this one keeps the default so the
// distance/sine gate would pass on its own, matching how the bug surfaces in a
// default-configured world.
//
// The plank is thin (half-extents 0.01 x 0.5, i.e. 0.02 x 1.0) so maxExtent
// (≈0.500) times |sin(175°)| (≈0.087) stays under the default 0.05 and the old
// gate would recycle. A near-180° flip is used because separating a centered
// box by rotation alone needs a large |sin θ|, which the small default
// distance would not let through; near 180° the sine term is small again while
// cos(175°) ≈ -0.996 still trips the per-body guard. The plank is positioned
// (center y = 0.519) so it starts touching within SpeculativeDistance (0.02)
// and the 175° flip lifts its lowest corner to about 0.021, just beyond
// SpeculativeDistance, so the manifold loses its contact points and the
// contact transitions touching -> non-touching, reporting an end event.
func TestContactRecycleRotationGuardEndEventDefaultDistance(t *testing.T) {
	t.Parallel()

	// Default recycle distance (0.05). Thin tall plank, events enabled.
	w, body, groundShape, bodyShape := recycleTestWorld(t, Vec2{X: 0.01, Y: 0.5}, Vec2{X: 0.0, Y: 0.519}, true)

	// Step 1: establish the touching contact and cache. stepAndFindContact
	// confirms the cached relative transform is valid.
	sim := stepAndFindContact(t, w, body)
	beginEvents := w.ContactEvents().BeginEvents
	require.Len(t, beginEvents, 1, "begin event must fire when the plank first touches")
	beginIDs := []ShapeID{beginEvents[0].ShapeIDA, beginEvents[0].ShapeIDB}
	require.Contains(t, beginIDs, groundShape)
	require.Contains(t, beginIDs, bodyShape)
	require.NotEqual(t, uint32(0), sim.simFlags&simTouchingFlag,
		"contact must start touching")

	// Flip the plank to 175°. The old gate passes at the default distance
	// (small sine term); the guard blocks recycling and the full update
	// detects the end-touching transition.
	w.SetBodyTransform(body, Vec2{X: 0.0, Y: 0.519}, MakeRot(175.0*math.Pi/180.0))
	stepWorld(w, 1)

	endEvents := w.ContactEvents().EndEvents
	require.Len(t, endEvents, 1, "end event must fire once the flipped plank separates")
	endIDs := []ShapeID{endEvents[0].ShapeIDA, endEvents[0].ShapeIDB}
	require.Contains(t, endIDs, groundShape)
	require.Contains(t, endIDs, bodyShape)
}
