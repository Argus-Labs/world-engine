package internal

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/cbridge"
)

// ReconcileFromECS incrementally syncs the C-side Box2D world from authoritative ECS entries
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
// Requires non-nil Runtime() with a live C-side world (for example after an initial
// FullRebuildFromECS). Entries are sorted by EntityID; duplicate IDs are an error. Entities
// absent from entries are removed from the runtime (body destroyed, shadow dropped).
//
// ReconcileFromECS does not touch SuppressContactsStep or Emitter; it does not step the world.
func ReconcileFromECS(entries []PhysicsRebuildEntry) error {
	rt := Runtime()
	if !cbridge.WorldExists() {
		return errors.New("physics2d: reconcile requires a live world (run FullRebuildFromECS first)")
	}

	sorted, err := cloneSortAndCheckDuplicateReconcileEntries(entries)
	if err != nil {
		return err
	}
	destroyOrphanBodies(rt, sorted)
	for _, e := range sorted {
		if err := reconcileOneEntry(rt, e); err != nil {
			return err
		}
	}
	return nil
}

// cloneSortAndCheckDuplicateReconcileEntries returns entries sorted by EntityID or an error if any ID repeats.
func cloneSortAndCheckDuplicateReconcileEntries(entries []PhysicsRebuildEntry) ([]PhysicsRebuildEntry, error) {
	sorted := slices.Clone(entries)
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

// destroyOrphanBodies removes C-side bodies (and shadow/active-contact rows) for entities not present in sorted.
func destroyOrphanBodies(rt *PhysicsRuntime, sorted []PhysicsRebuildEntry) {
	wanted := make(map[cardinal.EntityID]struct{}, len(sorted))
	for _, e := range sorted {
		wanted[e.EntityID] = struct{}{}
	}
	var orphans []cardinal.EntityID
	for id := range rt.KnownEntities {
		if _, ok := wanted[id]; !ok {
			orphans = append(orphans, id)
		}
	}
	slices.SortFunc(orphans, cmp.Compare)
	for _, id := range orphans {
		cbridge.DestroyBody(uint32(id))
		delete(rt.KnownEntities, id)
		delete(rt.Shadow, id)
		rt.PruneActiveContactsInvolvingEntity(id)
	}
}

// reconcileOneEntry creates a body if missing, no-ops if shadow matches live ECS, else patches the existing body.
func reconcileOneEntry(rt *PhysicsRuntime, e PhysicsRebuildEntry) error {
	if len(e.PhysicsBody.Shapes) == 0 {
		return fmt.Errorf("physics2d: entity %d: collider has no shapes", e.EntityID)
	}
	prev, hadPrev := rt.Shadow[e.EntityID]
	_, hadBody := rt.KnownEntities[e.EntityID]
	if !hadBody {
		return createBodyForEntry(rt, e)
	}
	if hadPrev && !prev.PhysicsDiffers(e.Transform, e.Velocity, e.PhysicsBody) {
		return nil
	}
	if err := reconcileExistingBody(rt, hadPrev, prev, e); err != nil {
		return fmt.Errorf("physics2d: entity %d: %w", e.EntityID, err)
	}
	rt.Shadow[e.EntityID] = NewShadowState(e.Transform, e.Velocity, e.PhysicsBody)
	return nil
}

// createBodyForEntry builds a new C-side body with shapes and records KnownEntities and Shadow.
func createBodyForEntry(rt *PhysicsRuntime, e PhysicsRebuildEntry) error {
	if err := CreateBodyWithCollider(
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

// reconcileExistingBody applies component diffs to the C-side body; rebuilds if shadow was missing or inconsistent.
func reconcileExistingBody(
	rt *PhysicsRuntime,
	hadPrev bool,
	prev ShadowState,
	e PhysicsRebuildEntry,
) error {
	if !hadPrev {
		// No shadow: treat as inconsistent; rebuild this body from scratch.
		cbridge.DestroyBody(uint32(e.EntityID))
		delete(rt.KnownEntities, e.EntityID)
		delete(rt.Shadow, e.EntityID)
		rt.PruneActiveContactsInvolvingEntity(e.EntityID)
		return createBodyForEntry(rt, e)
	}

	if err := validatePhysicsRebuildEntry(e); err != nil {
		return err
	}

	eid := uint32(e.EntityID)

	if prev.BodyParamsDiffer(e.PhysicsBody) {
		applyBodyParamsInPlace(e.EntityID, e.PhysicsBody)
	}
	if prev.TransformDiffers(e.Transform) {
		cbridge.SetTransform(eid, e.Transform.Position.X, e.Transform.Position.Y, e.Transform.Rotation)
	}
	if prev.ShapesDiffer(e.PhysicsBody) {
		if err := reconcileShapesChange(rt, e.EntityID, prev.PhysicsBody.Shapes, e.PhysicsBody.Shapes); err != nil {
			return err
		}
	}
	// Manual bodies always have zero velocity in Box2D (ECS owns position, not velocity).
	// FixedRotation bodies always have zero angular velocity in Box2D (see CreateBody comment).
	// For all other body types, push ECS velocity into Box2D when it changes.
	switch {
	case e.PhysicsBody.BodyType == component.BodyTypeManual:
		cbridge.SetLinearVelocity(eid, 0, 0)
		cbridge.SetAngularVelocity(eid, 0)
	case e.PhysicsBody.FixedRotation:
		cbridge.SetAngularVelocity(eid, 0)
		if prev.VelocityDiffers(e.Velocity) {
			cbridge.SetLinearVelocity(eid, e.Velocity.Linear.X, e.Velocity.Linear.Y)
		}
	case prev.VelocityDiffers(e.Velocity):
		cbridge.SetLinearVelocity(eid, e.Velocity.Linear.X, e.Velocity.Linear.Y)
		cbridge.SetAngularVelocity(eid, e.Velocity.Angular)
	}
	return nil
}

// reconcileShapesChange applies structural shape rebuild or in-place mutable updates when
// shadow shapes differ from ECS.
func reconcileShapesChange(
	rt *PhysicsRuntime,
	entityID cardinal.EntityID,
	prev, live []component.ColliderShape,
) error {
	if ShapesStructuralEqual(prev, live) {
		return applyMutableShapeFixtures(entityID, prev, live)
	}
	cbridge.DestroyAllShapes(uint32(entityID))
	if err := AttachColliderFixtures(entityID, live); err != nil {
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

// applyBodyParamsInPlace sets body type, damping, gravity scale, and body flags via cbridge.
func applyBodyParamsInPlace(entityID cardinal.EntityID, pb component.PhysicsBody2D) {
	eid := uint32(entityID)
	cbridge.SetBodyType(eid, mapBodyType(pb.BodyType))
	cbridge.SetLinearDamping(eid, pb.LinearDamping)
	cbridge.SetAngularDamping(eid, pb.AngularDamping)
	cbridge.SetGravityScale(eid, pb.GravityScale)
	cbridge.SetBodyEnabled(eid, pb.Active)
	cbridge.SetBullet(eid, pb.Bullet)
	cbridge.SetFixedRotation(eid, pb.FixedRotation)
	cbridge.SetSleepEnabled(eid, pb.SleepingAllowed)
	cbridge.SetAwake(eid, pb.Awake)
}

// applyMutableShapeFixtures updates sensor, friction, restitution, density, and filter per shape index in place.
func applyMutableShapeFixtures(
	entityID cardinal.EntityID,
	prev []component.ColliderShape,
	live []component.ColliderShape,
) error {
	for i := range live {
		if err := live[i].Validate(); err != nil {
			return fmt.Errorf("physics2d: shapes[%d]: %w", i, err)
		}
	}
	eid := uint32(entityID)
	var densityTouched bool
	for i := range live {
		if ColliderShapeMutableFieldsEqual(prev[i], live[i]) {
			continue
		}
		if prev[i].Density != live[i].Density {
			densityTouched = true
		}
		sh := live[i]
		cbridge.SetShapeFriction(eid, i, sh.Friction)
		cbridge.SetShapeRestitution(eid, i, sh.Restitution)
		cbridge.SetShapeDensity(eid, i, sh.Density)
		cbridge.SetShapeFilter(eid, i, sh.CategoryBits, sh.MaskBits, sh.GroupIndex)
	}
	if densityTouched {
		// Only non-static bodies need mass recalculation after density changes.
		// The cbridge handles the static check internally.
		cbridge.ResetMassData(eid)
	}
	return nil
}
