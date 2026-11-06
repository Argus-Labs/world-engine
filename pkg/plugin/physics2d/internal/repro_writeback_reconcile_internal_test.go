package internal

import (
	"math"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// TestReconcileFirstErrorLetsWritebackErasePendingEdit is the regression test for the bug where
// ReconcileFromECS aborted on the first per-entity error, leaving higher-ID entities unreconciled
// while the pipeline still stepped and wrote back — erasing their pending ECS edits and syncing
// the shadow onto the stomped value, making the loss permanent.
//
// With the fix, ReconcileFromECS continues past per-entity errors so every entity gets its
// reconcile attempt each tick, and failed entities are recorded in reconcileFailed so
// WritebackFromStepResults can skip them.
//
// Scenario: entity A (lower EntityID) has its Shapes emptied (reconcileOneEntry rejects this
// outright with "collider has no shapes"); entity B (higher EntityID) is teleported to
// {100,100}. With the bug, B was never reconciled and the teleport was lost. With the fix, B is
// reconciled and the teleport sticks.
func TestReconcileFirstErrorLetsWritebackErasePendingEdit(t *testing.T) {
	t.Parallel()

	const (
		entityA cardinal.EntityID = 1
		entityB cardinal.EntityID = 2
	)
	g := component.Vec2{} // zero gravity: a dynamic body stays put
	rt := NewRuntime(g, 1.0/60.0, 4, 0)

	box := func() ShapeSlice {
		return immutable.SliceOf(component.ColliderShape{
			ShapeType:    component.ShapeTypeBox,
			HalfExtents:  component.Vec2{X: 0.5, Y: 0.5},
			Density:      1,
			CategoryBits: 1,
			MaskBits:     ^uint64(0),
		})
	}
	dynamicBody := func(shapes ShapeSlice) component.PhysicsBody2D {
		return component.PhysicsBody2D{
			BodyType:        component.BodyTypeDynamic,
			Active:          true,
			Awake:           true,
			SleepingAllowed: true,
			GravityScale:    1,
			Shapes:          shapes,
		}
	}

	// Initial rebuild: both bodies at {50, 0} with valid box shapes.
	initial := []PhysicsRebuildEntry{
		{
			EntityID:    entityA,
			Transform:   component.Transform2D{Position: component.Vec2{X: 50, Y: 0}},
			PhysicsBody: dynamicBody(box()),
		},
		{
			EntityID:    entityB,
			Transform:   component.Transform2D{Position: component.Vec2{X: 50, Y: 0}},
			PhysicsBody: dynamicBody(box()),
		},
	}
	if err := rt.FullRebuildFromECS(g, initial); err != nil {
		t.Fatalf("FullRebuildFromECS: %v", err)
	}

	// Reconcile with A having empty shapes (fails) and B teleported to {100, 100}.
	entries := []PhysicsRebuildEntry{
		{
			EntityID:    entityA,
			Transform:   component.Transform2D{Position: component.Vec2{X: 50, Y: 0}},
			PhysicsBody: dynamicBody(immutable.SliceOf[component.ColliderShape]()),
		},
		{
			EntityID:    entityB,
			Transform:   component.Transform2D{Position: component.Vec2{X: 100, Y: 100}},
			PhysicsBody: dynamicBody(box()),
		},
	}

	err := rt.ReconcileFromECS(entries)

	// 1. ReconcileFromECS must return a non-nil error (A's shapes are empty).
	if err == nil {
		t.Fatalf("ReconcileFromECS: expected error for entity A with empty shapes, got nil")
	}
	if !strings.Contains(err.Error(), "collider has no shapes") {
		t.Fatalf("ReconcileFromECS: error should mention A's empty shapes, got: %v", err)
	}

	// 2. B's Box2D body must be at {100, 100} — reconcile continued past A's error.
	bodyB := rt.Bodies[entityB]
	posB := rt.World.BodyPosition(bodyB)
	if math.Abs(posB.X-100) > 1e-6 || math.Abs(posB.Y-100) > 1e-6 {
		t.Errorf("B's Box2D position after reconcile: got (%.6f, %.6f), want (100, 100) — "+
			"reconcile aborted before B (the bug)", posB.X, posB.Y)
	}

	// 3. B's shadow must reflect the teleport — it was applied to Box2D.
	shadowB, ok := rt.Shadow[entityB]
	if !ok {
		t.Fatalf("B's shadow missing after reconcile")
	}
	if math.Abs(shadowB.Transform.Position.X-100) > 1e-6 ||
		math.Abs(shadowB.Transform.Position.Y-100) > 1e-6 {
		t.Errorf("B's shadow transform after reconcile: got (%.6f, %.6f), want (100, 100)",
			shadowB.Transform.Position.X, shadowB.Transform.Position.Y)
	}

	// 4. A must be in reconcileFailed (so writeback skips it); B must not.
	if _, failed := rt.reconcileFailed[entityA]; !failed {
		t.Errorf("entity A should be in reconcileFailed (its reconcile failed)")
	}
	if _, failed := rt.reconcileFailed[entityB]; failed {
		t.Errorf("entity B should not be in reconcileFailed (its reconcile succeeded)")
	}

	// 5. After Step (zero gravity, zero velocity), B's body stays at {100, 100}.
	rt.Step()
	posB = rt.World.BodyPosition(bodyB)
	if math.Abs(posB.X-100) > 1e-6 || math.Abs(posB.Y-100) > 1e-6 {
		t.Errorf("B's Box2D position after step: got (%.6f, %.6f), want (100, 100)",
			posB.X, posB.Y)
	}

	// 6. Follow-up reconcile with A fixed: B's body stays at {100, 100} (no loss).
	//    PhysicsDiffers should report false for B (shadow matches ECS), so
	//    reconcileOneEntry no-ops and the teleport is permanent.
	fixedEntries := []PhysicsRebuildEntry{
		{
			EntityID:    entityA,
			Transform:   component.Transform2D{Position: component.Vec2{X: 50, Y: 0}},
			PhysicsBody: dynamicBody(box()),
		},
		{
			EntityID:    entityB,
			Transform:   component.Transform2D{Position: component.Vec2{X: 100, Y: 100}},
			PhysicsBody: dynamicBody(box()),
		},
	}
	if err := rt.ReconcileFromECS(fixedEntries); err != nil {
		t.Fatalf("follow-up ReconcileFromECS (A fixed): %v", err)
	}
	posB = rt.World.BodyPosition(bodyB)
	if math.Abs(posB.X-100) > 1e-6 || math.Abs(posB.Y-100) > 1e-6 {
		t.Errorf("B's Box2D position after follow-up reconcile: got (%.6f, %.6f), want (100, 100) — "+
			"the teleport was lost (the bug)", posB.X, posB.Y)
	}
	// reconcileFailed should be empty now (both entities reconciled successfully).
	if len(rt.reconcileFailed) != 0 {
		t.Errorf("reconcileFailed should be empty after successful reconcile, got %d entries: %v",
			len(rt.reconcileFailed), rt.reconcileFailed)
	}
}

// TestReconcileContinuesPastMultipleErrors verifies that ReconcileFromECS processes every entry
// even when multiple entries fail, and that the returned error is an aggregate of all failures.
func TestReconcileContinuesPastMultipleErrors(t *testing.T) {
	t.Parallel()

	g := component.Vec2{}
	rt := NewRuntime(g, 1.0/60.0, 4, 0)

	box := func() ShapeSlice {
		return immutable.SliceOf(component.ColliderShape{
			ShapeType:    component.ShapeTypeBox,
			HalfExtents:  component.Vec2{X: 0.5, Y: 0.5},
			Density:      1,
			CategoryBits: 1,
			MaskBits:     ^uint64(0),
		})
	}
	dynamicBody := func(shapes ShapeSlice) component.PhysicsBody2D {
		return component.PhysicsBody2D{
			BodyType:        component.BodyTypeDynamic,
			Active:          true,
			Awake:           true,
			SleepingAllowed: true,
			GravityScale:    1,
			Shapes:          shapes,
		}
	}
	emptyShapes := immutable.SliceOf[component.ColliderShape]()

	// Initial rebuild: three valid bodies.
	initial := []PhysicsRebuildEntry{
		{EntityID: 1, Transform: component.Transform2D{Position: component.Vec2{X: 0, Y: 0}},
			PhysicsBody: dynamicBody(box())},
		{EntityID: 2, Transform: component.Transform2D{Position: component.Vec2{X: 10, Y: 0}},
			PhysicsBody: dynamicBody(box())},
		{EntityID: 3, Transform: component.Transform2D{Position: component.Vec2{X: 20, Y: 0}},
			PhysicsBody: dynamicBody(box())},
	}
	if err := rt.FullRebuildFromECS(g, initial); err != nil {
		t.Fatalf("FullRebuildFromECS: %v", err)
	}

	// Reconcile: entities 1 and 3 fail (empty shapes), entity 2 is teleported.
	entries := []PhysicsRebuildEntry{
		{EntityID: 1, Transform: component.Transform2D{Position: component.Vec2{X: 0, Y: 0}},
			PhysicsBody: dynamicBody(emptyShapes)},
		{EntityID: 2, Transform: component.Transform2D{Position: component.Vec2{X: 100, Y: 0}},
			PhysicsBody: dynamicBody(box())},
		{EntityID: 3, Transform: component.Transform2D{Position: component.Vec2{X: 20, Y: 0}},
			PhysicsBody: dynamicBody(emptyShapes)},
	}
	err := rt.ReconcileFromECS(entries)
	if err == nil {
		t.Fatalf("expected error from entities 1 and 3, got nil")
	}

	// Entity 2 must be reconciled despite 1 failing before it.
	body2 := rt.Bodies[2]
	pos2 := rt.World.BodyPosition(body2)
	if math.Abs(pos2.X-100) > 1e-6 {
		t.Errorf("entity 2 position: got %.6f, want 100 — reconcile aborted before entity 2", pos2.X)
	}

	// Both failed entities must be in reconcileFailed.
	if _, failed := rt.reconcileFailed[1]; !failed {
		t.Errorf("entity 1 should be in reconcileFailed")
	}
	if _, failed := rt.reconcileFailed[3]; !failed {
		t.Errorf("entity 3 should be in reconcileFailed")
	}
	if _, failed := rt.reconcileFailed[2]; failed {
		t.Errorf("entity 2 should not be in reconcileFailed")
	}

	// The error must mention both failed entities (aggregate).
	errMsg := err.Error()
	if !strings.Contains(errMsg, "entity 1") {
		t.Errorf("error should mention entity 1, got: %v", err)
	}
	if !strings.Contains(errMsg, "entity 3") {
		t.Errorf("error should mention entity 3, got: %v", err)
	}
}
