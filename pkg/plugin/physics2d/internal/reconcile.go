package internal

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/ByteArena/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/query"
)

// ReconcileFromECS incrementally syncs Box2D from authoritative ECS entries using shadow-copy
// diffing. It is the hot-path counterpart to FullRebuildFromECS.
//
// Structural vs mutable changes:
//
//   - Structural: anything that changes fixture shape identity — shape count/order (v1 fixture
//     slots), per-shape type, local offset/rotation, or geometry (radius, half-extents, vertices,
//     chain points). Handled by destroying all fixtures on the body and re-attaching from ECS.
//
//   - Mutable: body transform, linear/angular velocity, rigidbody type/damping/gravity scale
//     (SetType and damping setters), and per-fixture sensor, friction, restitution, density, and
//     filter category/mask/group (fixture setters). Applied in place without recreating shapes.
//
// Requires non-nil Runtime() with a non-nil World (for example after an initial
// FullRebuildFromECS). Entries are sorted by EntityID; duplicate IDs are an error. Entities
// absent from entries are removed from the runtime (body destroyed, shadow dropped).
//
// ReconcileFromECS does not touch SuppressContactsStep or Emitter; it does not step the world.
func ReconcileFromECS(entries []PhysicsRebuildEntry) error {
	rt := Runtime()
	if rt.World == nil {
		return errors.New("physics2d: reconcile requires a non-nil World (run FullRebuildFromECS first)")
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

// destroyOrphanBodies removes Box2D bodies (and shadow/active-contact rows) for entities not present in sorted.
func destroyOrphanBodies(rt *PhysicsRuntime, sorted []PhysicsRebuildEntry) {
	wanted := make(map[cardinal.EntityID]struct{}, len(sorted))
	for _, e := range sorted {
		wanted[e.EntityID] = struct{}{}
	}
	var orphans []cardinal.EntityID
	for id := range rt.Bodies {
		if _, ok := wanted[id]; !ok {
			orphans = append(orphans, id)
		}
	}
	slices.SortFunc(orphans, cmp.Compare)
	for _, id := range orphans {
		if b := rt.Bodies[id]; b != nil {
			rt.World.DestroyBody(b)
		}
		delete(rt.Bodies, id)
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
	body, hadBody := rt.Bodies[e.EntityID]
	if !hadBody || body == nil {
		return createBodyForEntry(rt, e)
	}
	if hadPrev && !prev.PhysicsDiffers(e.Transform, e.Velocity, e.PhysicsBody) {
		return nil
	}
	if err := reconcileExistingBody(rt, body, hadPrev, prev, e); err != nil {
		return fmt.Errorf("physics2d: entity %d: %w", e.EntityID, err)
	}
	rt.Shadow[e.EntityID] = NewShadowState(e.Transform, e.Velocity, e.PhysicsBody)
	return nil
}

// createBodyForEntry builds a new Box2D body with fixtures and records Bodies and Shadow.
func createBodyForEntry(rt *PhysicsRuntime, e PhysicsRebuildEntry) error {
	body, err := CreateBodyWithCollider(
		rt.World,
		e.EntityID,
		e.Transform,
		e.Velocity,
		e.PhysicsBody,
	)
	if err != nil {
		return err
	}
	rt.Bodies[e.EntityID] = body
	rt.Shadow[e.EntityID] = NewShadowState(e.Transform, e.Velocity, e.PhysicsBody)
	return nil
}

// reconcileExistingBody applies component diffs to body; recreates the body if shadow was missing or inconsistent.
func reconcileExistingBody(
	rt *PhysicsRuntime,
	body *box2d.B2Body,
	hadPrev bool,
	prev ShadowState,
	e PhysicsRebuildEntry,
) error {
	if !hadPrev {
		// No shadow: treat as inconsistent; rebuild this body from scratch.
		rt.World.DestroyBody(body)
		delete(rt.Bodies, e.EntityID)
		delete(rt.Shadow, e.EntityID)
		rt.PruneActiveContactsInvolvingEntity(e.EntityID)
		return createBodyForEntry(rt, e)
	}

	if err := validatePhysicsRebuildEntry(e); err != nil {
		return err
	}

	if prev.BodyParamsDiffer(e.PhysicsBody) {
		applyBodyParamsInPlace(body, e.PhysicsBody)
	}
	if prev.TransformDiffers(e.Transform) {
		body.SetTransform(
			box2d.MakeB2Vec2(e.Transform.Position.X, e.Transform.Position.Y),
			e.Transform.Rotation,
		)
	}
	if prev.ShapesDiffer(e.PhysicsBody) {
		if err := reconcileShapesChange(rt, body, e.EntityID, prev.PhysicsBody.Shapes, e.PhysicsBody.Shapes); err != nil {
			return err
		}
		// A fixture may have toggled IsSensor without body params changing; re-enforce the
		// sensor-sleep policy so newly-sensored kinematic/manual bodies stay awake.
		enforceSensorAwakePolicy(body, e.PhysicsBody)
	}
	// Manual bodies always have zero velocity in Box2D (ECS owns position, not velocity).
	// For all other body types, push ECS velocity into Box2D when it changes.
	if e.PhysicsBody.BodyType == component.BodyTypeManual {
		body.SetLinearVelocity(box2d.MakeB2Vec2(0, 0))
		body.SetAngularVelocity(0)
	} else if prev.VelocityDiffers(e.Velocity) {
		body.SetLinearVelocity(box2d.MakeB2Vec2(e.Velocity.Linear.X, e.Velocity.Linear.Y))
		body.SetAngularVelocity(e.Velocity.Angular)
	}
	return nil
}

// reconcileShapesChange applies structural fixture rebuild or in-place mutable updates when
// shadow shapes differ from ECS.
func reconcileShapesChange(
	rt *PhysicsRuntime,
	body *box2d.B2Body,
	entityID cardinal.EntityID,
	prev, live []component.ColliderShape,
) error {
	if ShapesStructuralEqual(prev, live) {
		return applyMutableShapeFixtures(body, prev, live)
	}
	destroyAllFixtures(body)
	if err := AttachColliderFixtures(body, entityID, live); err != nil {
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

// applyBodyParamsInPlace sets body type, damping, gravity scale, and body flags from PhysicsBody2D.
func applyBodyParamsInPlace(body *box2d.B2Body, pb component.PhysicsBody2D) {
	body.SetType(mapBodyType(pb.BodyType))
	body.SetLinearDamping(pb.LinearDamping)
	body.SetAngularDamping(pb.AngularDamping)
	body.SetGravityScale(pb.GravityScale)
	body.SetActive(pb.Active)
	body.SetBullet(pb.Bullet)
	body.SetFixedRotation(pb.FixedRotation)
	enforceSensorAwakePolicy(body, pb)
}

// destroyAllFixtures removes every fixture from body (used before re-attaching a structurally changed collider).
func destroyAllFixtures(body *box2d.B2Body) {
	var fixtures []*box2d.B2Fixture
	for f := body.GetFixtureList(); f != nil; f = f.GetNext() {
		fixtures = append(fixtures, f)
	}
	for _, f := range fixtures {
		body.DestroyFixture(f)
	}
}

// fixturesByShapeIndex maps shape index from fixture user data to fixture pointers; errors on duplicate indices.
func fixturesByShapeIndex(body *box2d.B2Body) (map[int]*box2d.B2Fixture, error) {
	m := make(map[int]*box2d.B2Fixture)
	for f := body.GetFixtureList(); f != nil; f = f.GetNext() {
		_, shapeIndex, ok := query.FixtureUserDataFrom(f.GetUserData())
		if !ok {
			continue
		}
		if _, dup := m[shapeIndex]; dup {
			return nil, fmt.Errorf("duplicate fixture userData shapeIndex %d", shapeIndex)
		}
		m[shapeIndex] = f
	}
	return m, nil
}

// applyMutableShapeFixtures updates sensor, friction, restitution, density, and filter per shape index in place.
func applyMutableShapeFixtures(
	body *box2d.B2Body,
	prev []component.ColliderShape,
	live []component.ColliderShape,
) error {
	for i := range live {
		if err := live[i].Validate(); err != nil {
			return fmt.Errorf("physics2d: shapes[%d]: %w", i, err)
		}
	}
	byIdx, err := fixturesByShapeIndex(body)
	if err != nil {
		return err
	}
	var densityTouched bool
	for i := range live {
		if ColliderShapeMutableFieldsEqual(prev[i], live[i]) {
			continue
		}
		if prev[i].Density != live[i].Density {
			densityTouched = true
		}
		fix := byIdx[i]
		if fix == nil {
			return fmt.Errorf("missing fixture for shape index %d", i)
		}
		sh := live[i]
		fix.SetSensor(sh.IsSensor)
		fix.SetFriction(sh.Friction)
		fix.SetRestitution(sh.Restitution)
		fix.SetDensity(sh.Density)
		fltr := box2d.MakeB2Filter()
		fltr.CategoryBits = sh.CategoryBits
		fltr.MaskBits = sh.MaskBits
		fltr.GroupIndex = sh.GroupIndex
		fix.SetFilterData(fltr)
	}
	if densityTouched && body.GetType() != box2d.B2BodyType.B2_staticBody {
		body.ResetMassData()
	}
	return nil
}
