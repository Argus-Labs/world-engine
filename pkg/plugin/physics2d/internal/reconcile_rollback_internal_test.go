package internal

import (
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
)

// Regression coverage for the structural-change branch of reconcileShapesChange.
// reconcileShapesChange destroys the existing fixtures before re-attaching the new
// shape set; if AttachColliderFixtures fails partway, the body used to be left alive
// with zero/partial fixtures, a mass recomputed against that partial set, still
// registered in Bodies/KnownEntities/Shadow, and its ActiveContacts entries were not
// pruned (PruneActiveContactsInvolvingEntity was gated on the success path). The
// pipeline's "log and continue" then stepped, flushed, and wrote back that
// half-reconciled body. The fix tears the body down to "as if never created" on the
// error path, mirroring CreateBodyWithCollider's attach-failure rollback and
// destroyOrphanBodies' full teardown.

const (
	rollbackFloorID   cardinal.EntityID = 1
	rollbackBallID    cardinal.EntityID = 2
	rollbackControlID cardinal.EntityID = 3
)

func rollbackBoxShape(hw, hh float64) component.ColliderShape {
	return component.ColliderShape{
		ShapeType: component.ShapeTypeBox, Density: 1, Friction: 0.6,
		HalfExtents:  component.Vec2{X: hw, Y: hh},
		CategoryBits: 1, MaskBits: ^uint64(0),
	}
}

func rollbackCircleShape(r float64) component.ColliderShape {
	return component.ColliderShape{
		ShapeType: component.ShapeTypeCircle, Radius: r, Density: 1, Friction: 0.6,
		CategoryBits: 1, MaskBits: ^uint64(0),
	}
}

// rollbackDegeneratePolygon passes ColliderShape.Validate (VertexCount is within
// [0, MaxPolygonVertices] and all vertices are finite) but is rejected by attachShape
// because len(src) < 3 — the exact gap between Validate and attachShape that makes the
// structural-change failure mode reachable.
func rollbackDegeneratePolygon() component.ColliderShape {
	return component.ColliderShape{
		ShapeType:   component.ShapeTypeConvexPolygon,
		VertexCount: 2,
		Vertices:    [component.MaxPolygonVertices]component.Vec2{{X: -1, Y: 0}, {X: 1, Y: 0}},
		Density:     1, Friction: 0.6,
		CategoryBits: 1, MaskBits: ^uint64(0),
	}
}

// rollbackOffendingShapes is [circle, degenerate-2-vertex-polygon]: the circle attaches
// at slot 0, the polygon is rejected at attachShape, so AttachColliderFixtures returns
// "physics2d: shapes[1]: AddPolygonShape failed" after a partial attach.
func rollbackOffendingShapes() ShapeSlice {
	return immutable.SliceOf(rollbackCircleShape(0.5), rollbackDegeneratePolygon())
}

type rollbackEmitter struct {
	begins, ends int
	beginKeys    []ContactPairKey
	endKeys      []ContactPairKey
}

func (c *rollbackEmitter) EmitContactBegin(e event.ContactBeginEvent) {
	c.begins++
	c.beginKeys = append(c.beginKeys, normalizeContactPairKey(e.EntityA, e.ShapeIndexA, e.EntityB, e.ShapeIndexB))
}

func (c *rollbackEmitter) EmitContactEnd(e event.ContactEndEvent) {
	c.ends++
	c.endKeys = append(c.endKeys, normalizeContactPairKey(e.EntityA, e.ShapeIndexA, e.EntityB, e.ShapeIndexB))
}

func (c *rollbackEmitter) EmitTriggerBegin(event.TriggerBeginEvent) {}
func (c *rollbackEmitter) EmitTriggerEnd(event.TriggerEndEvent)     {}

func (c *rollbackEmitter) reset() { c.begins, c.ends = 0, 0; c.beginKeys = nil; c.endKeys = nil }

func rollbackStepAndFlush(rt *Runtime, em event.ContactEventEmitter) {
	rt.SetStepEmitter(em)
	rt.Step()
	rt.FlushBufferedContacts()
}

// rollbackBuildAndSettle creates a static floor and a dynamic box dropped onto it,
// rebuilds the world, and steps until the floor-ball contact has begun and is in
// rt.ActiveContacts. Returns the runtime, the rebuild entries, the floor-ball pair
// key, and the post-settle begin count.
func rollbackBuildAndSettle(t *testing.T) (*Runtime, []PhysicsRebuildEntry, ContactPairKey, int) {
	t.Helper()
	gravity := component.Vec2{Y: -10}
	rt := NewRuntime(gravity, 1.0/60.0, 4, 0)
	entries := []PhysicsRebuildEntry{
		{
			EntityID:    rollbackFloorID,
			Transform:   component.Transform2D{Position: component.Vec2{Y: -1}},
			PhysicsBody: component.NewPhysicsBody2D(component.BodyTypeStatic, rollbackBoxShape(5, 0.5)),
		},
		{
			EntityID:    rollbackBallID,
			Transform:   component.Transform2D{Position: component.Vec2{Y: 2}},
			PhysicsBody: component.NewPhysicsBody2D(component.BodyTypeDynamic, rollbackBoxShape(0.5, 0.5)),
		},
	}
	if err := rt.FullRebuildFromECS(gravity, entries); err != nil {
		t.Fatalf("FullRebuildFromECS: %v", err)
	}
	em := &rollbackEmitter{}
	for range 300 {
		rollbackStepAndFlush(rt, em)
	}
	pair := normalizeContactPairKey(rollbackFloorID, 0, rollbackBallID, 0)
	if em.begins < 1 {
		t.Fatalf("ball never contacted floor after settle: begins=%d", em.begins)
	}
	if _, ok := rt.ActiveContacts[pair]; !ok {
		t.Fatalf("floor-ball pair %v not in ActiveContacts after settle", pair)
	}
	return rt, entries, pair, em.begins
}

// mutateBallShapes returns the settle entries with the ball's Shapes replaced by shapes,
// keeping Transform/Velocity/BodyParams identical so the reconciler takes the structural
// shape-change branch.
func mutateBallShapes(entries []PhysicsRebuildEntry, shapes ShapeSlice) []PhysicsRebuildEntry {
	out := make([]PhysicsRebuildEntry, len(entries))
	copy(out, entries)
	for i := range out {
		if out[i].EntityID == rollbackBallID {
			pb := out[i].PhysicsBody
			pb.Shapes = shapes
			out[i].PhysicsBody = pb
		}
	}
	return out
}

// TestReconcileShapesChange_RollbackOnPartialAttachFailure exercises the full failure
// mode end-to-end: a settled dynamic body has its Shapes structurally mutated to a set
// whose tail shape passes Validate but is rejected by attachShape. The reconciler must
// roll the body back to "as if never created" so the pipeline's subsequent Step/Flush/
// Writeback run on a clean world.
func TestReconcileShapesChange_RollbackOnPartialAttachFailure(t *testing.T) {
	t.Parallel()
	rt, entries, pair, _ := rollbackBuildAndSettle(t)

	mutated := mutateBallShapes(entries, rollbackOffendingShapes())
	err := rt.ReconcileFromECS(mutated)
	if err == nil {
		t.Fatalf("expected structural reconcile to fail on degenerate tail shape")
	}
	if !strings.Contains(err.Error(), "AddPolygonShape failed") || !strings.Contains(err.Error(), "entity 2") {
		t.Fatalf("error should name entity and attachShape failure, got: %v", err)
	}

	// Rollback: body and all derived tracking for the ball must be gone.
	if _, ok := rt.Bodies[rollbackBallID]; ok {
		t.Errorf("rt.Bodies[ball] still present after failed reconcile (no DestroyEntityBody rollback)")
	}
	if _, ok := rt.KnownEntities[rollbackBallID]; ok {
		t.Errorf("rt.KnownEntities[ball] still present after failed reconcile")
	}
	if _, ok := rt.Shadow[rollbackBallID]; ok {
		t.Errorf("rt.Shadow[ball] still present after failed reconcile")
	}
	if _, ok := rt.Shapes[rollbackBallID]; ok {
		t.Errorf("rt.Shapes[ball] still present after failed reconcile")
	}

	// ActiveContacts must be pruned on the error path too; otherwise the stale pair
	// would survive into the persisted singleton and break next tick's dedupe baseline.
	if _, ok := rt.ActiveContacts[pair]; ok {
		t.Errorf("stale ActiveContacts pair %v survived the failed reconcile (Prune skipped on error)", pair)
	}
	if len(rt.pendingEndEvents) != 1 {
		t.Errorf("expected exactly 1 synthesized End pending after prune, got %d", len(rt.pendingEndEvents))
	}

	// Begin/End contract: the next step+flush must close the lifecycle with a single End
	// (from the prune) and emit no Begin (the rolled-back body is gone, so the overlap
	// cannot re-form). Without the fix, the half-reconciled body re-creates the overlap
	// every tick, accumulating Begins with no matching End.
	em := &rollbackEmitter{}
	rollbackStepAndFlush(rt, em)
	if em.begins != 0 {
		t.Errorf("tick after failure: expected 0 Begins (body rolled back), got %d", em.begins)
	}
	if em.ends != 1 {
		t.Errorf("tick after failure: expected exactly 1 End (prune-synthesized), got %d", em.ends)
	}
	if len(em.endKeys) == 1 && em.endKeys[0] != pair {
		t.Errorf("tick after failure: End key %v does not match settled pair %v", em.endKeys[0], pair)
	}
	if _, ok := rt.ActiveContacts[pair]; ok {
		t.Errorf("ActiveContacts pair reappeared after the post-failure flush")
	}

	// Re-reconciling the still-mutated ECS each tick (as the pipeline does) must keep
	// the body rolled back: the create-path re-fails and re-rolls-back, the world keeps
	// stepping cleanly, and no spurious Begin/End emerges for the absent body.
	for tick := range 4 {
		em.reset()
		if rerr := rt.ReconcileFromECS(mutated); rerr == nil {
			t.Fatalf("tick %d: expected re-reconcile of offending shapes to keep failing", tick)
		}
		rollbackStepAndFlush(rt, em)
		if _, ok := rt.Bodies[rollbackBallID]; ok {
			t.Errorf("tick %d: rt.Bodies[ball] reappeared after re-reconcile", tick)
		}
		if em.begins != 0 || em.ends != 0 {
			t.Errorf("tick %d: expected 0 Begin/0 End for absent body, got begins=%d ends=%d", tick, em.begins, em.ends)
		}
		if _, ok := rt.ActiveContacts[pair]; ok {
			t.Errorf("tick %d: stale ActiveContacts pair reappeared", tick)
		}
	}

	// Fresh-create control: the symmetric sibling (CreateBodyWithCollider) must also
	// roll back the same offending shape set — the existing-body path now mirrors it.
	offending := []component.ColliderShape{rollbackCircleShape(0.5), rollbackDegeneratePolygon()}
	ctrl := PhysicsRebuildEntry{
		EntityID:    rollbackControlID,
		Transform:   component.Transform2D{Position: component.Vec2{Y: 5}},
		PhysicsBody: component.NewPhysicsBody2D(component.BodyTypeDynamic, offending...),
	}
	if cerr := rt.CreateBodyWithCollider(ctrl.EntityID, ctrl.Transform, ctrl.Velocity, ctrl.PhysicsBody); cerr == nil {
		t.Fatalf("expected fresh-create of offending shape set to fail")
	}
	if _, ok := rt.Bodies[rollbackControlID]; ok {
		t.Errorf("rt.Bodies[control] present after CreateBodyWithCollider rollback")
	}
	if _, ok := rt.KnownEntities[rollbackControlID]; ok {
		t.Errorf("rt.KnownEntities[control] present after CreateBodyWithCollider rollback")
	}
}

// TestReconcileShapesChange_SuccessPathStructuralRebuildUnchanged guards that the fix
// did not alter the success branch: a structural shape change with valid geometry must
// still rebuild the fixtures in place, keep the body alive, and prune the entity's
// existing ActiveContacts (the documented behavior of the trailing Prune call).
func TestReconcileShapesChange_SuccessPathStructuralRebuildUnchanged(t *testing.T) {
	t.Parallel()
	rt, entries, pair, _ := rollbackBuildAndSettle(t)

	valid := immutable.SliceOf(rollbackCircleShape(0.5), rollbackBoxShape(0.3, 0.3))
	mutated := mutateBallShapes(entries, valid)
	if err := rt.ReconcileFromECS(mutated); err != nil {
		t.Fatalf("valid structural reconcile should succeed, got: %v", err)
	}

	bodyID, hasBody := rt.Bodies[rollbackBallID]
	if !hasBody {
		t.Fatalf("rt.Bodies[ball] must remain after successful structural rebuild")
	}
	if got := rt.World.BodyShapeCount(bodyID); got != 2 {
		t.Errorf("expected 2 fixtures after structural rebuild, got %d", got)
	}
	if _, hasKnown := rt.KnownEntities[rollbackBallID]; !hasKnown {
		t.Errorf("rt.KnownEntities[ball] must remain after successful rebuild")
	}
	if _, hasShadow := rt.Shadow[rollbackBallID]; !hasShadow {
		t.Errorf("rt.Shadow[ball] must be refreshed after successful rebuild")
	}

	// Success path still prunes the entity's ActiveContacts and synthesizes the End.
	if _, hasPair := rt.ActiveContacts[pair]; hasPair {
		t.Errorf("ActiveContacts pair %v should have been pruned on successful structural rebuild", pair)
	}
	if len(rt.pendingEndEvents) != 1 {
		t.Errorf("expected 1 synthesized End after success-path prune, got %d", len(rt.pendingEndEvents))
	}
}

// TestReconcileShapesChange_MutableBranchUnchanged guards the non-structural (in-place
// mutable) early-return branch: a geometry-identical shape set with only mutable-field
// changes must not destroy/recreate fixtures, must not prune ActiveContacts, and must
// keep the body's fixture count unchanged.
func TestReconcileShapesChange_MutableBranchUnchanged(t *testing.T) {
	t.Parallel()
	rt, entries, pair, beginCount := rollbackBuildAndSettle(t)

	original := entries[1].PhysicsBody.Shapes
	mutated := make([]component.ColliderShape, 0, original.Len())
	for _, sh := range original.All() {
		sh.Friction = 0.25
		sh.Restitution = 0.5
		mutated = append(mutated, sh)
	}
	mEntries := mutateBallShapes(entries, immutable.SliceOf(mutated...))
	if err := rt.ReconcileFromECS(mEntries); err != nil {
		t.Fatalf("mutable reconcile should succeed, got: %v", err)
	}

	bodyID, hasBody := rt.Bodies[rollbackBallID]
	if !hasBody {
		t.Fatalf("rt.Bodies[ball] must remain after mutable update")
	}
	if got := rt.World.BodyShapeCount(bodyID); got != 1 {
		t.Errorf("mutable update must not change fixture count, got %d", got)
	}
	// Mutable path does not prune ActiveContacts; the settled pair stays.
	if _, hasPair := rt.ActiveContacts[pair]; !hasPair {
		t.Errorf("ActiveContacts pair %v must survive a mutable update; begins so far=%d", pair, beginCount)
	}
	if len(rt.pendingEndEvents) != 0 {
		t.Errorf("mutable path must not synthesize Ends, got %d", len(rt.pendingEndEvents))
	}
}
