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

// recycleGuardTestWorld builds a world with a static ground (top at y=0) and a
// small dynamic box resting on it, configured so the contact-recycle sin metric
// passes even for large rotations but the per-body cos guard is the deciding
// factor. The ground is kept close to the dynamic body (lever arm ~0.54) and
// the recycle distance is set very large so that distance + maxExtent*|sin θ|
// never blocks recycling on its own.
func recycleGuardTestWorld(t *testing.T) (*World, BodyID) {
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
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	// Small dynamic box (0.1 x 0.1, maxExtent ≈ 0.07), slightly overlapping
	// the ground so the contact is touching.
	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = Vec2{X: 0.0, Y: 0.04}
	body := w.CreateBody(&bd)
	box := MakeBox(0.05, 0.05)
	sd := DefaultShapeDef()
	sd.Density = 1.0
	w.CreatePolygonShape(body, &sd, &box)

	// Very large recycle distance so the sin metric alone never blocks
	// recycling — the cos guard is the only gate that can force a full
	// update.
	w.SetContactRecycleDistance(10.0)

	return w, body
}

// stepAndRotate teleports the body to the given angle (keeping position
// fixed) and steps the world once. It returns the contact sim re-fetched
// after the step.
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
// refreshed.
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

	cachedRotBefore := dynamicCachedRotation(sim, w, body)
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
		requireCachedRot(t, sim, w, body, cachedRotBefore, "cache must be unchanged after recycle")
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
	rot15 := MakeRot(15.0 * math.Pi / 180.0)
	sim := stepAndRotate(t, w, body, 15.0*math.Pi/180.0)
	requireCachedRot(t, sim, w, body, rot15,
		"step 4: cache must be refreshed to 15 degrees after full update")

	// Step 5: rotate to 20°. The cache was refreshed to 15° on step 4, so
	// the accumulated rotation is now 5°, cos(5°) = 0.996 > 0.98, and the
	// guard allows recycling again — the cache stays at 15°.
	sim = stepAndRotate(t, w, body, 20.0*math.Pi/180.0)
	requireCachedRot(t, sim, w, body, rot15,
		"step 5: cache must stay at 15 (recycled, not refreshed)")
}

// TestContactRecycleGuardStaticBodyDoesNotBlock verifies that a static body
// (which never rotates, cos = 1.0) does not trigger the guard on its own —
// recycling is controlled by the dynamic body's rotation alone. The dynamic
// body is rotated by 5° (under the 11.5° guard) before the second step; a full
// update would refresh the cache to 5°, so asserting the cache stays at 0°
// proves recycling was allowed despite the static body.
func TestContactRecycleGuardStaticBodyDoesNotBlock(t *testing.T) {
	t.Parallel()

	w, body := recycleGuardTestWorld(t)

	// Step once to establish contact + cache.
	stepAndFindContact(t, w, body)

	// Rotate the dynamic body by 5° and step. The static ground contributes
	// cos = 1.0, so only the dynamic body's 5° rotation gates recycling.
	// cos(5°) = 0.996 > 0.98, so the guard allows recycling and the cache is
	// NOT refreshed — it stays at 0°.
	sim := stepAndRotate(t, w, body, 5.0*math.Pi/180.0)
	requireCachedRot(t, sim, w, body, RotIdentity, "no-rotation-block: cache must stay at 0 (recycled)")
}

// recycleEventTestWorld builds a world with a static ground (top at y=0) and a
// tall thin dynamic plank standing upright, its base slightly overlapping the
// ground so the contact starts touching. Contact events are enabled on both
// shapes. The recycle distance is set very large so the sin metric alone never
// blocks recycling — the cos guard is the only gate that can force a full
// update. The plank is tall (0.05 x 0.5) so rotating it about its center
// swings its base corner up off the ground: at 30° the lowest point rises
// above the ground (separation > linearSlop) while the fat AABBs still overlap,
// so the contact transitions touching -> non-touching and reports an end event.
func recycleEventTestWorld(t *testing.T) (*World, BodyID, ShapeID, ShapeID) {
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
	gsd.EnableContactEvents = true
	groundShape := w.CreatePolygonShape(ground, &gsd, &groundBox)

	// Tall thin dynamic plank (0.05 x 0.5) standing upright, centered at
	// y = 0.49 so its base overlaps the ground by 0.01 (> linearSlop) — the
	// contact is solidly touching. Gravity is zero so the plank only moves
	// where the test drives it via SetBodyTransform.
	bd := DefaultBodyDef()
	bd.Type = DynamicBody
	bd.Position = Vec2{X: 0.0, Y: 0.49}
	body := w.CreateBody(&bd)
	box := MakeBox(0.05, 0.5)
	sd := DefaultShapeDef()
	sd.Density = 1.0
	sd.EnableContactEvents = true
	bodyShape := w.CreatePolygonShape(body, &sd, &box)

	w.SetContactRecycleDistance(10.0)

	return w, body, groundShape, bodyShape
}

// TestContactRecycleRotationGuardEndEvent is the behavioral regression for the
// reported contact-event symptom. The contact starts touching. The plank is
// then rotated past the guard threshold (30°, cos = 0.866 < 0.98), which
// swings its base corner up off the ground so the shapes stop touching.
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
	// 0°. The first contact reports a begin event.
	stepWorld(w, 1)
	beginEvents := w.ContactEvents().BeginEvents
	require.Len(t, beginEvents, 1, "begin event must fire when the plank first touches")
	beginIDs := []ShapeID{beginEvents[0].ShapeIDA, beginEvents[0].ShapeIDB}
	require.Contains(t, beginIDs, groundShape)
	require.Contains(t, beginIDs, bodyShape)

	// Confirm the contact is touching so the regression is meaningful: a
	// transition exists for the next step to report.
	sim := findContactSimForBody(w, body)
	require.NotNil(t, sim, "contact must exist after first step")
	require.NotEqual(t, uint32(0), sim.simFlags&simTouchingFlag,
		"contact must start touching")
	require.NotEqual(t, uint32(0), sim.simFlags&simRelativeTransformValid,
		"cached relative transform must be valid after first step")

	// Rotate the plank by 30° — past the 11.5° guard — swinging the base
	// corner up off the ground so the shapes stop touching. Keep the
	// position fixed so only rotation drives the transition.
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
