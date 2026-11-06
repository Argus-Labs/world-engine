package internal

import (
	"fmt"
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/stretchr/testify/require"
)

// boxShape is a valid unit box collider shared across the reconcile-error tests.
var boxShape = component.ColliderShape{
	ShapeType:    component.ShapeTypeBox,
	HalfExtents:  component.Vec2{X: 0.5, Y: 0.5},
	Density:      1,
	CategoryBits: 1,
	MaskBits:     ^uint64(0),
}

// boxBody returns a dynamic body with the shared box shape and Box2D-compatible defaults
// (Active/Awake/SleepingAllowed true, GravityScale 1), via component.NewPhysicsBody2D.
func boxBody() component.PhysicsBody2D {
	return component.NewPhysicsBody2D(component.BodyTypeDynamic, boxShape)
}

// physicsArchetype is the component set for a physics entity: Transform2D, Velocity2D, PhysicsBody2D.
type physicsArchetype struct {
	Transform   component.Transform2D
	Velocity    component.Velocity2D
	PhysicsBody component.PhysicsBody2D
}

// runOnceSystem runs a single closure as a Cardinal system, so component mutations run on the
// world's system goroutine (Entity.Set panics outside a step).
type runOnceSystem struct {
	run func(*cardinal.World)
}

func (s *runOnceSystem) Run(w *cardinal.World) { s.run(w) }

// rebuildEntriesFromECS reads the authoritative physics components off each entity in handle order.
func rebuildEntriesFromECS(entities ...cardinal.Entity) []PhysicsRebuildEntry {
	out := make([]PhysicsRebuildEntry, 0, len(entities))
	for _, e := range entities {
		out = append(out, PhysicsRebuildEntry{
			EntityID:    e.ID(),
			Transform:   e.Get[component.Transform2D](),
			Velocity:    e.Get[component.Velocity2D](),
			PhysicsBody: e.Get[component.PhysicsBody2D](),
		})
	}
	return out
}

// writebackEntries builds the writeback entry list for the given entities.
func writebackEntries(entities ...cardinal.Entity) []WritebackEntry {
	out := make([]WritebackEntry, 0, len(entities))
	for _, e := range entities {
		out = append(out, WritebackEntry{Entity: e})
	}
	return out
}

// requireXY compares a coordinate pair against the expected values within epsilon, labeling
// the assertion with a phase so a failure names which pipeline stage it came from.
func requireXY(t *testing.T, phase string, x, y, wantX, wantY float64) {
	t.Helper()
	require.InDelta(t, wantX, x, 1e-6, "%s: X = %.6f, want %.6f", phase, x, wantX)
	require.InDelta(t, wantY, y, 1e-6, "%s: Y = %.6f, want %.6f", phase, y, wantY)
}

// TestReconcileErrorDoesNotSkipLaterEntities exercises the full reconcile -> step -> writeback
// pipeline with real ECS handles. A failing entity must not abort the loop, and writeback must
// skip it so its pending ECS edit survives for the next reconcile attempt.
//
// Scenario: entity A (lower EntityID) is given an invalid collider (no shapes) plus a pending
// transform/velocity edit; entity B (higher EntityID) is teleported. After reconcile -> step ->
// writeback, A's pending edit and old shadow must survive (writeback skipped A) and B's teleport
// must stick. Repairing A's collider and rerunning the pipeline resumes normal writeback for A.
func TestReconcileErrorDoesNotSkipLaterEntities(t *testing.T) {
	t.Parallel()

	gravity := component.Vec2{} // zero gravity: a dynamic body with zero velocity stays put
	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)
	pendingTransformA := component.Transform2D{Position: component.Vec2{X: 50, Y: 7}}
	pendingVelocityA := component.Velocity2D{Linear: component.Vec2{X: 3, Y: 0}}

	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		w.RegisterComponent[component.Transform2D]()
		w.RegisterComponent[component.Velocity2D]()
		w.RegisterComponent[component.PhysicsBody2D]()
	})

	var entA, entB cardinal.Entity
	w.RunSystem(&runOnceSystem{func(w *cardinal.World) {
		// Spawn A (lower ID) and B (higher ID) at {50, 0} with valid box bodies.
		entA = w.Create[physicsArchetype]()
		entA.Set(component.Transform2D{Position: component.Vec2{X: 50, Y: 0}})
		entA.Set(component.Velocity2D{})
		entA.Set(boxBody())

		entB = w.Create[physicsArchetype]()
		entB.Set(component.Transform2D{Position: component.Vec2{X: 50, Y: 0}})
		entB.Set(component.Velocity2D{})
		entB.Set(boxBody())
	}})

	idA, idB := entA.ID(), entB.ID()
	require.Less(t, idA, idB, "entity A must sort before entity B so A is reconciled first")

	w.RunSystem(&runOnceSystem{func(w *cardinal.World) {
		// Initial rebuild from ECS.
		require.NoError(t, rt.FullRebuildFromECS(gravity, rebuildEntriesFromECS(entA, entB)))

		// A: invalid collider (no shapes) plus a pending transform/velocity edit.
		entA.Set(pendingTransformA)
		entA.Set(pendingVelocityA)
		entA.Set(component.NewPhysicsBody2D(component.BodyTypeDynamic)) // empty shapes -> reconcile fails
		// B: teleport.
		entB.Set(component.Transform2D{Position: component.Vec2{X: 100, Y: 100}})

		// reconcile -> step -> writeback
		require.Error(t, rt.ReconcileFromECS(rebuildEntriesFromECS(entA, entB)),
			"reconcile should fail for A's empty shapes")

		// Reconcile continued past A's error: B's Box2D body was teleported.
		posB := rt.World.BodyPosition(rt.Bodies[idB])
		requireXY(t, "B body after reconcile", posB.X, posB.Y, 100, 100)

		rt.Step()
		rt.WritebackFromStepResults(writebackEntries(entA, entB))

		// A's pending edit must survive: writeback skipped it.
		require.Equal(t, pendingTransformA, entA.Get[component.Transform2D](),
			"A's pending transform must survive (writeback skipped A)")
		require.Equal(t, pendingVelocityA, entA.Get[component.Velocity2D](),
			"A's pending velocity must survive (writeback skipped A)")
		// A's shadow must be the OLD shadow (not synced onto the stomped value).
		shadowA := rt.Shadow[idA]
		require.Equal(t, component.Vec2{X: 50, Y: 0}, shadowA.Transform.Position,
			"A's shadow must stay at the old position")

		// B's teleport must stick through writeback.
		transformB := entB.Get[component.Transform2D]()
		requireXY(t, "B transform after writeback", transformB.Position.X, transformB.Position.Y, 100, 100)
		shadowB := rt.Shadow[idB]
		requireXY(t, "B shadow after writeback", shadowB.Transform.Position.X, shadowB.Transform.Position.Y, 100, 100)

		// Repair A: valid collider again, keep the pending edit.
		entA.Set(boxBody())
		require.NoError(t, rt.ReconcileFromECS(rebuildEntriesFromECS(entA, entB)),
			"follow-up reconcile with A repaired should succeed")
		rt.Step()
		rt.WritebackFromStepResults(writebackEntries(entA, entB))

		// A's edit was applied and writeback resumed: A moved by velocity * dt.
		transformA := entA.Get[component.Transform2D]()
		requireXY(t, "A transform after repair",
			transformA.Position.X, transformA.Position.Y,
			50+pendingVelocityA.Linear.X*defaultFixedDT, 7)
		require.Equal(t, pendingVelocityA, entA.Get[component.Velocity2D](),
			"A's velocity after writeback resumed")
		// B stays put.
		transformB = entB.Get[component.Transform2D]()
		requireXY(t, "B transform after repair", transformB.Position.X, transformB.Position.Y, 100, 100)
		// reconcileFailed is now empty (both reconciled successfully).
		require.Empty(t, rt.reconcileFailed, "reconcileFailed should be empty after successful reconcile")
	}})
}

// TestReconcileContinuesPastMultipleErrors verifies that ReconcileFromECS processes every entry
// even when multiple entries fail, and that the returned error is an aggregate of all failures.
func TestReconcileContinuesPastMultipleErrors(t *testing.T) {
	t.Parallel()

	gravity := component.Vec2{}
	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)

	// Initial rebuild: three valid bodies.
	initial := []PhysicsRebuildEntry{
		{EntityID: 1, Transform: component.Transform2D{Position: component.Vec2{X: 0, Y: 0}},
			PhysicsBody: boxBody()},
		{EntityID: 2, Transform: component.Transform2D{Position: component.Vec2{X: 10, Y: 0}},
			PhysicsBody: boxBody()},
		{EntityID: 3, Transform: component.Transform2D{Position: component.Vec2{X: 20, Y: 0}},
			PhysicsBody: boxBody()},
	}
	require.NoError(t, rt.FullRebuildFromECS(gravity, initial))

	// Reconcile: entities 1 and 3 fail (no shapes), entity 2 is teleported.
	emptyBody := component.NewPhysicsBody2D(component.BodyTypeDynamic)
	entries := []PhysicsRebuildEntry{
		{EntityID: 1, Transform: component.Transform2D{Position: component.Vec2{X: 0, Y: 0}},
			PhysicsBody: emptyBody},
		{EntityID: 2, Transform: component.Transform2D{Position: component.Vec2{X: 100, Y: 0}},
			PhysicsBody: boxBody()},
		{EntityID: 3, Transform: component.Transform2D{Position: component.Vec2{X: 20, Y: 0}},
			PhysicsBody: emptyBody},
	}
	err := rt.ReconcileFromECS(entries)
	require.Error(t, err, "expected error from entities 1 and 3")

	// Entity 2 must be reconciled despite 1 failing before it.
	pos2 := rt.World.BodyPosition(rt.Bodies[2])
	requireXY(t, "entity 2 after reconcile", pos2.X, pos2.Y, 100, 0)

	// Both failed entities must be in reconcileFailed; entity 2 must not.
	_, failed1 := rt.reconcileFailed[cardinal.EntityID(1)]
	require.True(t, failed1, "entity 1 should be in reconcileFailed")
	_, failed3 := rt.reconcileFailed[cardinal.EntityID(3)]
	require.True(t, failed3, "entity 3 should be in reconcileFailed")
	_, failed2 := rt.reconcileFailed[cardinal.EntityID(2)]
	require.False(t, failed2, "entity 2 should not be in reconcileFailed")

	// The aggregate error must mention both failed entities.
	require.Contains(t, err.Error(), "entity 1", "error should mention entity 1: %v", err)
	require.Contains(t, err.Error(), "entity 3", "error should mention entity 3: %v", err)
}

// TestReconcileCreatePathErrorIncludesEntityID verifies that a create-path failure (a new entity
// with no existing body) wraps its error with the entity ID, so joined errors identify which
// entity needs repairing. Existing-body errors already include the ID via reconcileOneEntry.
func TestReconcileCreatePathErrorIncludesEntityID(t *testing.T) {
	t.Parallel()

	gravity := component.Vec2{}
	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)

	// Seed a live world with one valid body so ReconcileFromECS has a world to run against.
	require.NoError(t, rt.FullRebuildFromECS(gravity, []PhysicsRebuildEntry{
		{EntityID: 1, Transform: component.Transform2D{Position: component.Vec2{X: 0, Y: 0}},
			PhysicsBody: boxBody()},
	}))

	// A new entity (no existing body) with invalid velocity fails in the create path
	// (CreateBody -> velocity.Validate). The error must identify the entity.
	newID := cardinal.EntityID(99)
	entries := []PhysicsRebuildEntry{
		{EntityID: newID, Transform: component.Transform2D{Position: component.Vec2{X: 0, Y: 0}},
			Velocity:    component.Velocity2D{Linear: component.Vec2{X: math.NaN()}},
			PhysicsBody: boxBody()},
	}
	err := rt.ReconcileFromECS(entries)
	require.Error(t, err)
	require.Contains(t, err.Error(), fmt.Sprintf("entity %d", newID),
		"create-path error should include the entity id: %v", err)
	require.Contains(t, err.Error(), "velocity", "create-path error should name the failing field: %v", err)

	// The failed entity must be in reconcileFailed so writeback skips it.
	_, failed := rt.reconcileFailed[newID]
	require.True(t, failed, "entity %d should be in reconcileFailed", newID)
}
