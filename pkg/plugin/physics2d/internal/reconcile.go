package internal

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// ReconcileFromECS incrementally syncs the Box2D world from authoritative ECS entries
// using shadow-copy diffing. It is the hot-path counterpart to FullRebuildFromECS.
//
// Structural vs mutable changes:
//
//   - Structural: anything that changes shape identity -- shape count/order, per-shape type,
//     local offset/rotation, or geometry (radius, half-extents, vertices, chain points).
//     Handled by destroying all shapes on the body and re-attaching from ECS.
//
//   - Mutable: body transform, linear/angular velocity, body type/damping/gravity scale,
//     and per-shape sensor, friction, restitution, density, and filter category/mask/group.
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
	for _, e := range sorted {
		if err := rt.reconcileOneEntry(e); err != nil {
			return err
		}
	}
	return nil
}

// cloneSortAndCheckDuplicateReconcileEntries returns entries sorted by EntityID or an error if
// any ID repeats. The returned slice is backed by rt.reconcileSortScratch (reused across ticks
// to avoid re-cloning every reconcile); it is only valid until the next call.
func (rt *Runtime) cloneSortAndCheckDuplicateReconcileEntries(
	entries []PhysicsRebuildEntry,
) ([]PhysicsRebuildEntry, error) {
	rt.reconcileSortScratch = append(rt.reconcileSortScratch[:0], entries...)
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
// whole PhysicsRebuildEntry values the way slices.BinarySearchFunc would.
func sortedEntriesContainID(sorted []PhysicsRebuildEntry, id cardinal.EntityID) bool {
	lo, hi := 0, len(sorted)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1) //nolint:gosec // lo,hi are non-negative slice indices
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
	if len(e.PhysicsBody.Shapes) == 0 {
		return fmt.Errorf("physics2d: entity %d: collider has no shapes", e.EntityID)
	}
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
		rt.applyBodyParamsInPlace(e.EntityID, e.PhysicsBody)
	}
	if prev.TransformDiffers(e.Transform) {
		rt.World.SetBodyTransform(bodyID,
			box2d.Vec2{X: e.Transform.Position.X, Y: e.Transform.Position.Y},
			box2d.MakeRot(e.Transform.Rotation))
	}
	if prev.ShapesDiffer(e.PhysicsBody) {
		if err := rt.reconcileShapesChange(e.EntityID, prev.PhysicsBody.Shapes, e.PhysicsBody.Shapes); err != nil {
			return err
		}
	}
	// Manual bodies always have zero velocity in Box2D (ECS owns position, not velocity).
	// FixedRotation bodies always have zero angular velocity in Box2D (see CreateBody comment).
	// For all other body types, push ECS velocity into Box2D when it changes.
	switch {
	case e.PhysicsBody.BodyType == component.BodyTypeManual:
		rt.World.SetBodyLinearVelocity(bodyID, box2d.Vec2{})
		rt.World.SetBodyAngularVelocity(bodyID, 0)
	case e.PhysicsBody.FixedRotation:
		rt.World.SetBodyAngularVelocity(bodyID, 0)
		if prev.VelocityDiffers(e.Velocity) {
			rt.World.SetBodyLinearVelocity(bodyID, box2d.Vec2{X: e.Velocity.Linear.X, Y: e.Velocity.Linear.Y})
		}
	case prev.VelocityDiffers(e.Velocity):
		rt.World.SetBodyLinearVelocity(bodyID, box2d.Vec2{X: e.Velocity.Linear.X, Y: e.Velocity.Linear.Y})
		rt.World.SetBodyAngularVelocity(bodyID, e.Velocity.Angular)
	}
	return nil
}

// reconcileShapesChange applies structural shape rebuild or in-place mutable updates when
// shadow shapes differ from ECS.
func (rt *Runtime) reconcileShapesChange(
	entityID cardinal.EntityID,
	prev, live []component.ColliderShape,
) error {
	if ShapesStructuralEqual(prev, live) {
		return rt.applyMutableShapeFixtures(entityID, prev, live)
	}
	rt.destroyAllShapesForEntity(entityID)
	if err := rt.AttachColliderFixtures(entityID, live); err != nil {
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
func (rt *Runtime) applyBodyParamsInPlace(entityID cardinal.EntityID, pb component.PhysicsBody2D) {
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
	rt.World.SetBodyAwake(bodyID, pb.Awake)
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

// applyMutableShapeFixtures updates sensor, friction, restitution, density, and filter per shape index in place.
func (rt *Runtime) applyMutableShapeFixtures(
	entityID cardinal.EntityID,
	prev []component.ColliderShape,
	live []component.ColliderShape,
) error {
	for i := range live {
		if err := live[i].Validate(); err != nil {
			return fmt.Errorf("physics2d: shapes[%d]: %w", i, err)
		}
	}
	slots := rt.Shapes[entityID]
	var densityTouched bool
	for i := range live {
		if ColliderShapeMutableFieldsEqual(prev[i], live[i]) {
			continue
		}
		if prev[i].Density != live[i].Density {
			densityTouched = true
		}
		// Chain slots hold a null ShapeID and are skipped, matching the CGO bridge
		// (its per-shape setters could not resolve chain shape indices either).
		if i >= len(slots) || slots[i].IsNull() {
			continue
		}
		sid := slots[i]
		sh := live[i]
		rt.World.SetShapeFriction(sid, sh.Friction)
		rt.World.SetShapeRestitution(sid, sh.Restitution)
		rt.World.SetShapeDensity(sid, sh.Density, true)
		rt.World.SetShapeFilter(sid, box2d.Filter{
			CategoryBits: sh.CategoryBits,
			MaskBits:     sh.MaskBits,
			GroupIndex:   int(sh.GroupIndex),
		})
	}
	if densityTouched {
		if bodyID, ok := rt.Bodies[entityID]; ok {
			rt.World.ApplyBodyMassFromShapes(bodyID)
		}
	}
	return nil
}
