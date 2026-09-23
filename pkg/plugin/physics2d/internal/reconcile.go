package internal

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
)

// ReconcileFromECS incrementally syncs the Box2D world from authoritative ECS entries
// using shadow-copy diffing. It is the hot-path counterpart to FullRebuildFromECS.
//
// Structural vs mutable changes:
//
//   - Structural: anything that changes fixture identity -- shape count/order, a shape's local
//     offset/rotation, kind, geometry or sensor flag. Handled by destroying all shapes on the
//     body and re-attaching.
//
//   - Mutable: body transform, linear/angular velocity, body type/damping/gravity scale,
//     and per-shape friction, restitution, density, and filter category/mask/group.
//     Applied in place without recreating shapes.
//
// Requires a live world on this runtime (for example after an initial
// FullRebuildFromECS). Entries are sorted by EntityID; duplicate IDs are an error. Entities
// absent from entries are removed from the runtime (body destroyed, shadow dropped).
//
// ReconcileFromECS does not touch SuppressContactsStep or Emitter; it does not step the world.
func (rt *Runtime) ReconcileFromECS(entries []PhysicsRebuildEntry) error {
	if rt.World == nil {
		return errors.New("physics2d: reconcile requires a live world (run FullRebuildFromECS first)")
	}

	sorted, err := rt.cloneSortAndCheckDuplicateReconcileEntries(entries)
	if err != nil {
		return err
	}
	rt.destroyOrphanBodies(sorted)
	// One failing entity must not skip the rest; each entity's failure path leaves that
	// entity in a clean state.
	var errs []error
	for _, e := range sorted {
		if err := rt.reconcileOneEntry(e); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// cloneSortAndCheckDuplicateReconcileEntries returns entries sorted by EntityID or an error if
// any ID repeats. The returned slice is backed by rt.reconcileSortScratch (reused across ticks
// to avoid re-cloning every reconcile); it is only valid until the next call. The scratch tail
// past the new length is cleared per the Runtime scratch RULE: PhysicsRebuildEntry holds the
// slot slice, so a bare [:0] would pin component memory for destroyed entities after the
// entity count shrinks.
func (rt *Runtime) cloneSortAndCheckDuplicateReconcileEntries(
	entries []PhysicsRebuildEntry,
) ([]PhysicsRebuildEntry, error) {
	rt.reconcileSortScratch = clearScratchTail(append(rt.reconcileSortScratch[:0], entries...))
	sorted := rt.reconcileSortScratch
	slices.SortFunc(sorted, func(a, b PhysicsRebuildEntry) int {
		return cmp.Compare(a.EntityID, b.EntityID)
	})
	for i := 1; i < len(sorted); i++ {
		if sorted[i].EntityID == sorted[i-1].EntityID {
			return nil, fmt.Errorf("physics2d: duplicate entity_id %d in reconcile entries", sorted[i].EntityID)
		}
	}
	return sorted, nil
}

// destroyOrphanBodies removes bodies (and shadow/active-contact rows) for entities not present
// in sorted. Membership uses binary search on the EntityID-sorted entries, avoiding a per-tick
// set allocation.
func (rt *Runtime) destroyOrphanBodies(sorted []PhysicsRebuildEntry) {
	var orphans []cardinal.EntityID
	for id := range rt.KnownEntities {
		if !sortedEntriesContainID(sorted, id) {
			orphans = append(orphans, id)
		}
	}
	slices.SortFunc(orphans, cmp.Compare)
	for _, id := range orphans {
		rt.DestroyEntityBody(id)
		delete(rt.KnownEntities, id)
		delete(rt.Shadow, id)
		rt.PruneActiveContactsInvolvingEntity(id)
	}
}

// sortedEntriesContainID reports whether an EntityID-sorted entries slice contains id.
// Index-based binary search: comparisons touch only the EntityID field instead of copying
// whole PhysicsRebuildEntry values (transform, velocity and the body with its slot slice)
// on every step the way slices.BinarySearchFunc's by-value comparator would.
//
// The midpoint is lo+(hi-lo)/2 rather than (lo+hi)/2: same overflow safety, but no unsigned
// round trip, so no integer-conversion lint suppression is needed either.
func sortedEntriesContainID(sorted []PhysicsRebuildEntry, id cardinal.EntityID) bool {
	lo, hi := 0, len(sorted)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if sorted[mid].EntityID < id {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo < len(sorted) && sorted[lo].EntityID == id
}

// reconcileOneEntry creates a body if missing, no-ops if shadow matches live ECS, else patches the existing body.
func (rt *Runtime) reconcileOneEntry(e PhysicsRebuildEntry) error {
	prev, hadPrev := rt.Shadow[e.EntityID]
	_, hadBody := rt.KnownEntities[e.EntityID]
	if !hadBody {
		return rt.createBodyForEntry(e)
	}
	if hadPrev && !prev.PhysicsDiffers(e.Transform, e.Velocity, e.PhysicsBody) {
		return nil
	}
	if err := rt.reconcileExistingBody(hadPrev, prev, e); err != nil {
		return fmt.Errorf("physics2d: entity %d: %w", e.EntityID, err)
	}
	rt.Shadow[e.EntityID] = NewShadowState(e.Transform, e.Velocity, e.PhysicsBody)
	return nil
}

// createBodyForEntry builds a new body with shapes and records KnownEntities and Shadow.
func (rt *Runtime) createBodyForEntry(e PhysicsRebuildEntry) error {
	if err := rt.CreateBodyWithCollider(
		e.EntityID,
		e.Transform,
		e.Velocity,
		e.PhysicsBody,
	); err != nil {
		return err
	}
	rt.KnownEntities[e.EntityID] = struct{}{}
	rt.Shadow[e.EntityID] = NewShadowState(e.Transform, e.Velocity, e.PhysicsBody)
	return nil
}

// reconcileExistingBody applies component diffs to the body; rebuilds if shadow was missing or inconsistent.
func (rt *Runtime) reconcileExistingBody(
	hadPrev bool,
	prev ShadowState,
	e PhysicsRebuildEntry,
) error {
	if !hadPrev {
		// No shadow: treat as inconsistent; rebuild this body from scratch.
		rt.DestroyEntityBody(e.EntityID)
		delete(rt.KnownEntities, e.EntityID)
		delete(rt.Shadow, e.EntityID)
		rt.PruneActiveContactsInvolvingEntity(e.EntityID)
		return rt.createBodyForEntry(e)
	}

	if err := validatePhysicsRebuildEntry(e); err != nil {
		return err
	}

	bodyID := rt.Bodies[e.EntityID]

	if prev.BodyParamsDiffer(e.PhysicsBody) {
		rt.applyBodyParamsInPlace(e.EntityID, e.PhysicsBody, prev.PhysicsBody.Awake)
	}
	if prev.TransformDiffers(e.Transform) {
		rt.World.SetBodyTransform(bodyID,
			box2d.Vec2{X: e.Transform.Position.X, Y: e.Transform.Position.Y},
			box2d.MakeRot(e.Transform.Rotation))
		// b2Body_SetTransform deliberately does not wake, so a teleport onto a sleeping
		// island would raise no contact events. Wake the moved body — unless this same tick
		// explicitly wrote Awake=false, which is intent and wins.
		explicitSleep := prev.PhysicsBody.Awake && !e.PhysicsBody.Awake
		if !explicitSleep {
			rt.World.SetBodyAwake(bodyID, true)
		}
	}
	if prev.ShapesDiffer(e.PhysicsBody) {
		if err := rt.reconcileShapesChange(e.EntityID, prev.PhysicsBody.Shapes, e.PhysicsBody.Shapes); err != nil {
			return err
		}
	}
	// Manual bodies always have zero velocity in Box2D (ECS owns position, not velocity).
	// FixedRotation bodies always have zero angular velocity in Box2D (see CreateBody comment).
	// For all other body types, push ECS velocity into Box2D when it changes.
	//
	// A body whose velocity Box2D does not track keeps the gameplay Velocity2D in its
	// shadow, so on the tick it starts being tracked VelocityDiffers reports "no change"
	// and the push would be skipped, leaving Box2D at zero where FullRebuildFromECS would
	// have created the body born-moving. Force the push so both paths agree.
	leftUntrackedVelocity := untrackedVelocity(prev.PhysicsBody.BodyType) &&
		!untrackedVelocity(e.PhysicsBody.BodyType)
	switch {
	case e.PhysicsBody.BodyType == component.BodyTypeManual:
		rt.World.SetBodyLinearVelocity(bodyID, box2d.Vec2{})
		rt.World.SetBodyAngularVelocity(bodyID, 0)
	case e.PhysicsBody.FixedRotation:
		rt.World.SetBodyAngularVelocity(bodyID, 0)
		if prev.VelocityDiffers(e.Velocity) || leftUntrackedVelocity {
			rt.World.SetBodyLinearVelocity(bodyID, box2d.Vec2{X: e.Velocity.Linear.X, Y: e.Velocity.Linear.Y})
		}
	case prev.VelocityDiffers(e.Velocity) || leftUntrackedVelocity:
		rt.World.SetBodyLinearVelocity(bodyID, box2d.Vec2{X: e.Velocity.Linear.X, Y: e.Velocity.Linear.Y})
		rt.World.SetBodyAngularVelocity(bodyID, e.Velocity.Angular)
	}
	return nil
}

// untrackedVelocity reports body types whose Box2D velocity is not driven by ECS: Manual
// is forced to zero, Static ignores it. Writeback skips both (see WritebackFromStepResults),
// so their shadow velocity is gameplay bookkeeping, not a mirror of Box2D.
func untrackedVelocity(t component.BodyType) bool {
	return t == component.BodyTypeManual || t == component.BodyTypeStatic
}

// reconcileShapesChange applies a structural fixture rebuild or in-place mutable updates when
// the shadow's shapes differ from ECS.
func (rt *Runtime) reconcileShapesChange(
	entityID cardinal.EntityID,
	prev, live immutable.Slice[component.Shape],
) error {
	if shapesStructuralEqual(prev, live) {
		return rt.applyMutableShapeFixtures(entityID, prev, live)
	}
	rt.destroyAllShapesForEntity(entityID)
	if err := rt.AttachColliderFixtures(entityID, live); err != nil {
		// Half a body is worse than none: drop it entirely so the next tick treats the entity
		// as new, retries the attach, and logs the same failure until the game fixes it.
		rt.DestroyEntityBody(entityID)
		delete(rt.KnownEntities, entityID)
		delete(rt.Shadow, entityID)
		rt.PruneActiveContactsInvolvingEntity(entityID)
		return err
	}
	rt.PruneActiveContactsInvolvingEntity(entityID)
	return nil
}

// validatePhysicsRebuildEntry runs component Validate on each field for an existing-body update path.
func validatePhysicsRebuildEntry(e PhysicsRebuildEntry) error {
	if err := e.Transform.Validate(); err != nil {
		return fmt.Errorf("physics2d: entity %d transform: %w", e.EntityID, err)
	}
	if err := e.Velocity.Validate(); err != nil {
		return fmt.Errorf("physics2d: entity %d velocity: %w", e.EntityID, err)
	}
	if err := e.PhysicsBody.Validate(); err != nil {
		return fmt.Errorf("physics2d: entity %d physics_body: %w", e.EntityID, err)
	}
	return nil
}

// applyBodyParamsInPlace sets body type, damping, gravity scale, and body flags in place.
// prevAwake is the shadow's Awake. A param change that left Awake untouched counts as a
// disturbance and wakes the body, so the new params act instead of waiting for an impact.
func (rt *Runtime) applyBodyParamsInPlace(entityID cardinal.EntityID, pb component.PhysicsBody2D, prevAwake bool) {
	bodyID, ok := rt.Bodies[entityID]
	if !ok {
		return
	}
	rt.World.SetBodyType(bodyID, mapBodyType(pb.BodyType))
	rt.World.SetBodyLinearDamping(bodyID, pb.LinearDamping)
	rt.World.SetBodyAngularDamping(bodyID, pb.AngularDamping)
	rt.World.SetBodyGravityScale(bodyID, pb.GravityScale)
	rt.setBodyEnabled(bodyID, pb.Active)
	rt.World.SetBodyBullet(bodyID, pb.Bullet)
	rt.setFixedRotation(bodyID, pb.FixedRotation)
	rt.World.EnableBodySleep(bodyID, pb.SleepingAllowed)
	awake := pb.Awake
	if pb.Awake == prevAwake {
		awake = true
	}
	rt.World.SetBodyAwake(bodyID, awake)
}

// setBodyEnabled enables or disables the body only when the state actually changes,
// matching the CGO bridge's bridge_set_body_enabled.
func (rt *Runtime) setBodyEnabled(bodyID box2d.BodyID, enabled bool) {
	switch {
	case enabled && !rt.World.IsBodyEnabled(bodyID):
		rt.World.EnableBody(bodyID)
	case !enabled && rt.World.IsBodyEnabled(bodyID):
		rt.World.DisableBody(bodyID)
	}
}

// setFixedRotation toggles the angular-Z motion lock, preserving the linear locks.
func (rt *Runtime) setFixedRotation(bodyID box2d.BodyID, flag bool) {
	locks := rt.World.BodyMotionLocks(bodyID)
	locks.AngularZ = flag
	rt.World.SetBodyMotionLocks(bodyID, locks)
}

// shapesStructuralEqual reports whether the live shapes can be applied to the fixtures built
// from prev without recreating them: same count, and each pair builds the same fixture.
func shapesStructuralEqual(prev, live immutable.Slice[component.Shape]) bool {
	return prev.EqualFunc(live, component.Shape.StructuralEqual)
}

// applyMutableShapeFixtures pushes friction, restitution, density and filter into the fixture
// of every shape whose material or filter changed. Requires shapesStructuralEqual(prev, live).
func (rt *Runtime) applyMutableShapeFixtures(
	entityID cardinal.EntityID,
	prev, live immutable.Slice[component.Shape],
) error {
	slots := rt.Shapes[entityID]
	for i, l := range live.All() {
		p := prev.At(i)
		sameMaterial := p.Friction == l.Friction && p.Restitution == l.Restitution && p.Density == l.Density
		sameFilter := p.CategoryBits == l.CategoryBits && p.MaskBits == l.MaskBits && p.GroupIndex == l.GroupIndex
		if sameMaterial && sameFilter {
			continue
		}
		// Chain slots hold a null ShapeID; the chain is updated through its own id.
		if i >= len(slots) || slots[i].IsNull() {
			rt.applyMutableChain(entityID, i, !sameMaterial, sameFilter, l)
			continue
		}
		sid := slots[i]
		rt.World.SetShapeFriction(sid, l.Friction)
		rt.World.SetShapeRestitution(sid, l.Restitution)
		// The trailing true is Box2D's updateBodyMass: a density change re-derives the body's
		// mass here, so nothing further up needs to track whether density moved.
		rt.World.SetShapeDensity(sid, l.Density, true)
		rt.World.SetShapeFilter(sid, shapeFilter(l))
	}
	return nil
}

// applyMutableChain sets a chain slot's material (one for every segment, as it was built) and
// the filter on each segment shape. Chains have no mass, so density does not apply.
//
// Each half is skipped when its own input did not move, which matters here and not on the
// single-shape path above: Box2D's SetShapeFilter and SetShapeDensity return early on an
// unchanged value, but SetChainSurfaceMaterial writes to every segment unconditionally, and
// reaching the segments to set filters costs a query and a slice.
func (rt *Runtime) applyMutableChain(
	entityID cardinal.EntityID, shapeIndex int, newMaterial, sameFilter bool, s component.Shape,
) {
	for _, ch := range rt.Chains[entityID] {
		if ch.Index != shapeIndex {
			continue
		}
		if newMaterial {
			material := box2d.DefaultSurfaceMaterial()
			material.Friction = s.Friction
			material.Restitution = s.Restitution
			rt.World.SetChainSurfaceMaterial(ch.ID, material, 0)
		}
		if !sameFilter {
			segments := make([]box2d.ShapeID, rt.World.ChainSegmentCount(ch.ID))
			n := rt.World.ChainSegments(ch.ID, segments)
			for _, sid := range segments[:n] {
				rt.World.SetShapeFilter(sid, shapeFilter(s))
			}
		}
		return
	}
}
