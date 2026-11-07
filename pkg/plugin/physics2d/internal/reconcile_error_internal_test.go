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
// world's system goroutine. This adapter follows the system-goroutine contract: Entity.Set
// panics for absent entities or unregistered components, but does not enforce an active-step
// check, so callers must keep mutations inside the closure.
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

// registerPhysicsWorld registers the three physics components on a test world. Shared by the
// reconcile-error tests so each does not repeat the same registration block.
func registerPhysicsWorld(w *cardinal.World) {
	w.RegisterComponent[component.Transform2D]()
	w.RegisterComponent[component.Velocity2D]()
	w.RegisterComponent[component.PhysicsBody2D]()
}

// spawnBoxAt creates a dynamic box entity at pos (via boxBody) and returns it. Must be called
// from within a system on the world's system goroutine; explicit call order preserves EntityID
// assignment order.
func spawnBoxAt(w *cardinal.World, pos component.Vec2) cardinal.Entity {
	e := w.Create[physicsArchetype]()
	e.Set(component.Transform2D{Position: pos})
	e.Set(component.Velocity2D{})
	e.Set(boxBody())
	return e
}

// seedSingleBox spawns one dynamic box entity at the origin and returns it. Shared one-body
// seed for the lifecycle and policy tests.
func seedSingleBox(w *cardinal.World) cardinal.Entity {
	return spawnBoxAt(w, component.Vec2{})
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
		registerPhysicsWorld(w)
	})

	var entA, entB cardinal.Entity
	w.RunSystem(&runOnceSystem{func(w *cardinal.World) {
		// Spawn A (lower ID) and B (higher ID) at {50, 0}, in explicit creation order.
		entA = spawnBoxAt(w, component.Vec2{X: 50, Y: 0})
		entB = spawnBoxAt(w, component.Vec2{X: 50, Y: 0})
	}})

	idA, idB := entA.ID(), entB.ID()
	require.Less(t, idA, idB, "entity A must sort before entity B so A is reconciled first")

	w.RunSystem(&runOnceSystem{func(*cardinal.World) {
		require.NoError(t, rt.FullRebuildFromECS(gravity, rebuildEntriesFromECS(entA, entB)))

		// A: invalid collider (no shapes) plus a pending transform/velocity edit.
		entA.Set(pendingTransformA)
		entA.Set(pendingVelocityA)
		entA.Set(component.NewPhysicsBody2D(component.BodyTypeDynamic)) // empty shapes -> reconcile fails
		// B: teleport.
		entB.Set(component.Transform2D{Position: component.Vec2{X: 100, Y: 100}})

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
		wantShadowA := NewShadowState(
			component.Transform2D{Position: component.Vec2{X: 50, Y: 0}},
			component.Velocity2D{},
			boxBody(),
		)
		require.Equal(t, wantShadowA, rt.Shadow[idA], "A's shadow must stay at the old state")

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
		transformB = entB.Get[component.Transform2D]()
		requireXY(t, "B transform after repair", transformB.Position.X, transformB.Position.Y, 100, 100)
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

	// The aggregate error must mention both failed entities (colon-delimited so the
	// assertion cannot also pass for entity 10 / 30).
	require.Contains(t, err.Error(), "entity 1:", "error should mention entity 1: %v", err)
	require.Contains(t, err.Error(), "entity 3:", "error should mention entity 3: %v", err)
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

	// The failed entity must be in reconcileFailed. A failed create leaves no body so
	// writeback already skips it via the Bodies lookup this tick, but the membership
	// keeps writeback skipping it on a later tick once a body exists, until a successful
	// reconcile clears the set.
	_, failed := rt.reconcileFailed[newID]
	require.True(t, failed, "entity %d should be in reconcileFailed", newID)
}

// TestReconcileFailureFreezesECSWhileBodyFalls pins the failed-body policy under nonzero gravity:
// when a body parameter edit fails validation, the existing Box2D body keeps simulating (falling
// under gravity) while the entity's ECS Transform/Velocity stay frozen (writeback skips it), and
// the frozen ECS state jumps to the fallen position once the input is repaired.
//
// It also drives reconcile -> step -> writeback in the same phase order as PhysicsPipelineSystem.Run
// across several ticks. PhysicsPipelineSystem.Run itself lives in the internal/system subpackage,
// which imports this package, so it cannot be reached from here without an import cycle; a full
// plugin-level pipeline test belongs in the external test/ package.
func TestReconcileFailureFreezesECSWhileBodyFalls(t *testing.T) {
	t.Parallel()

	gravity := component.Vec2{X: 0, Y: -10}
	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)

	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		registerPhysicsWorld(w)
	})

	var entA cardinal.Entity
	w.RunSystem(&runOnceSystem{func(w *cardinal.World) {
		entA = seedSingleBox(w)
	}})

	idA := entA.ID()

	w.RunSystem(&runOnceSystem{func(*cardinal.World) {
		require.NoError(t, rt.FullRebuildFromECS(gravity, rebuildEntriesFromECS(entA)))

		// A: invalid damping (NaN) fails reconcile validation. The existing body is left
		// untouched, so it keeps falling under gravity while ECS stays frozen.
		invalidDamping := boxBody()
		invalidDamping.LinearDamping = math.NaN()
		entA.Set(invalidDamping)

		// Drive reconcile -> step -> writeback in pipeline phase order across several ticks.
		for range 3 {
			require.Error(t, rt.ReconcileFromECS(rebuildEntriesFromECS(entA)),
				"reconcile should fail each tick while damping is NaN")
			rt.Step()
			rt.WritebackFromStepResults(writebackEntries(entA))
			require.InDelta(t, 0, entA.Get[component.Transform2D]().Position.Y, 1e-9,
				"A's ECS transform must stay frozen while reconcile fails")
			require.Equal(t, component.Velocity2D{}, entA.Get[component.Velocity2D](),
				"A's ECS velocity must stay frozen while reconcile fails")
		}
		// The Box2D body kept falling under gravity; A is still flagged failed.
		bodyY := rt.World.BodyPosition(rt.Bodies[idA]).Y
		require.Less(t, bodyY, 0.0, "A's Box2D body should have fallen under gravity")
		_, failed := rt.reconcileFailed[idA]
		require.True(t, failed, "A should still be in reconcileFailed while damping is invalid")

		// Repair A: valid damping. Reconcile succeeds, the body keeps its fallen position,
		// and the next writeback jumps ECS to that fallen position.
		entA.Set(boxBody())
		require.NoError(t, rt.ReconcileFromECS(rebuildEntriesFromECS(entA)),
			"reconcile should succeed once damping is repaired")
		rt.Step()
		rt.WritebackFromStepResults(writebackEntries(entA))

		// ECS jumps to the fallen position (allowing this next step's further fall), rather
		// than only Y < 0 which one step from spawn would satisfy.
		ecsY := entA.Get[component.Transform2D]().Position.Y
		require.InDelta(t, bodyY, ecsY, 0.5,
			"A's ECS transform should jump to the fallen Box2D position after repair")
		require.Empty(t, rt.reconcileFailed, "reconcileFailed should be empty after a successful reconcile")
	}})
}

// TestReconcileFailureRetainsInvalidMotion covers the existing-body case where the pending edit
// itself is invalid: a NaN Transform fails reconcile validation, so the body keeps its old Box2D
// state and keeps simulating while ECS retains the NaN (writeback skips the entity). The NaN is
// not replaced with finite solver state until the input is repaired, pinning that the protection
// applies to invalid pending motion edits too.
func TestReconcileFailureRetainsInvalidMotion(t *testing.T) {
	t.Parallel()

	gravity := component.Vec2{X: 0, Y: -10}
	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)

	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		registerPhysicsWorld(w)
	})

	var entA cardinal.Entity
	w.RunSystem(&runOnceSystem{func(w *cardinal.World) {
		entA = seedSingleBox(w)
	}})

	w.RunSystem(&runOnceSystem{func(*cardinal.World) {
		require.NoError(t, rt.FullRebuildFromECS(gravity, rebuildEntriesFromECS(entA)))

		// A: invalid transform (NaN position) fails reconcile validation. The existing
		// body keeps its old transform and keeps falling while ECS retains the NaN.
		entA.Set(component.Transform2D{Position: component.Vec2{X: math.NaN(), Y: 0}})

		require.Error(t, rt.ReconcileFromECS(rebuildEntriesFromECS(entA)),
			"reconcile should fail while transform is NaN")
		rt.Step()
		rt.WritebackFromStepResults(writebackEntries(entA))

		// ECS retains the NaN edit; writeback skipped A.
		require.True(t, math.IsNaN(entA.Get[component.Transform2D]().Position.X),
			"A's NaN transform must stay in ECS while reconcile fails")
		_, failed := rt.reconcileFailed[entA.ID()]
		require.True(t, failed, "A should be in reconcileFailed while transform is invalid")

		// The Box2D body kept its old transform and fell under gravity.
		require.Less(t, rt.World.BodyPosition(rt.Bodies[entA.ID()]).Y, 0.0,
			"A's Box2D body should keep falling under gravity")

		// Repair A: valid transform. Reconcile succeeds and writeback resumes.
		entA.Set(component.Transform2D{Position: component.Vec2{}})
		require.NoError(t, rt.ReconcileFromECS(rebuildEntriesFromECS(entA)),
			"reconcile should succeed once transform is repaired")
		rt.Step()
		rt.WritebackFromStepResults(writebackEntries(entA))

		require.False(t, math.IsNaN(entA.Get[component.Transform2D]().Position.X),
			"A's ECS transform should be finite after repair")
		require.Empty(t, rt.reconcileFailed, "reconcileFailed should be empty after a successful reconcile")
	}})
}

// TestReconcileFailedClearedByRebuildAndReset covers the failure-set lifecycle contracts: after a
// failure populates reconcileFailed, FullRebuildFromECS and Reset both clear it so a subsequent
// step+writeback resumes writing back to the entity instead of skipping it.
func TestReconcileFailedClearedByRebuildAndReset(t *testing.T) {
	t.Parallel()

	gravity := component.Vec2{X: 0, Y: -10}
	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)

	w := cardinal.NewTestWorld(t, func(w *cardinal.World) {
		registerPhysicsWorld(w)
	})

	var entA cardinal.Entity
	w.RunSystem(&runOnceSystem{func(w *cardinal.World) {
		entA = seedSingleBox(w)
	}})

	w.RunSystem(&runOnceSystem{func(*cardinal.World) {
		// Initial rebuild, then fail reconcile for A (no shapes).
		require.NoError(t, rt.FullRebuildFromECS(gravity, rebuildEntriesFromECS(entA)))
		entA.Set(component.NewPhysicsBody2D(component.BodyTypeDynamic)) // empty shapes -> fails
		require.Error(t, rt.ReconcileFromECS(rebuildEntriesFromECS(entA)))
		_, failed := rt.reconcileFailed[entA.ID()]
		require.True(t, failed, "A should be in reconcileFailed after the failed reconcile")

		// FullRebuildFromECS clears reconcileFailed and rebuilds the body; writeback resumes.
		entA.Set(boxBody())
		require.NoError(t, rt.FullRebuildFromECS(gravity, rebuildEntriesFromECS(entA)))
		require.Empty(t, rt.reconcileFailed, "reconcileFailed should be cleared by FullRebuildFromECS")
		rt.Step()
		rt.WritebackFromStepResults(writebackEntries(entA))
		require.Less(t, entA.Get[component.Transform2D]().Position.Y, 0.0,
			"writeback should resume after rebuild (A falls under gravity)")

		// Re-introduce a failure, then Reset clears reconcileFailed (and drops the world).
		entA.Set(component.NewPhysicsBody2D(component.BodyTypeDynamic))
		require.Error(t, rt.ReconcileFromECS(rebuildEntriesFromECS(entA)))
		_, failed = rt.reconcileFailed[entA.ID()]
		require.True(t, failed, "A should be in reconcileFailed again after the second failure")
		rt.Reset()
		require.Empty(t, rt.reconcileFailed, "reconcileFailed should be cleared by Reset")

		// After Reset, a fresh rebuild + step + writeback resumes writing back to A.
		entA.Set(boxBody())
		require.NoError(t, rt.FullRebuildFromECS(gravity, rebuildEntriesFromECS(entA)))
		require.Empty(t, rt.reconcileFailed, "reconcileFailed should stay empty after the fresh rebuild")
		rt.Step()
		rt.WritebackFromStepResults(writebackEntries(entA))
		require.Less(t, entA.Get[component.Transform2D]().Position.Y, 0.0,
			"writeback should resume after Reset + rebuild (A falls under gravity)")
	}})
}

// TestReconcileFailedRetainedOnDuplicateIDReturn verifies that a duplicate-ID early return does
// not clear the prior reconcileFailed set. The duplicate-ID validation returns before the clear
// at the start of the entry loop, so entities that failed an earlier reconcile remain skipped.
func TestReconcileFailedRetainedOnDuplicateIDReturn(t *testing.T) {
	t.Parallel()

	gravity := component.Vec2{}
	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)

	// Seed a live world with one valid body.
	require.NoError(t, rt.FullRebuildFromECS(gravity, []PhysicsRebuildEntry{
		{EntityID: 1, Transform: component.Transform2D{Position: component.Vec2{X: 0, Y: 0}},
			PhysicsBody: boxBody()},
	}))

	// Fail reconcile for entity 1 (no shapes) so it lands in reconcileFailed.
	emptyBody := component.NewPhysicsBody2D(component.BodyTypeDynamic)
	require.Error(t, rt.ReconcileFromECS([]PhysicsRebuildEntry{
		{EntityID: 1, Transform: component.Transform2D{Position: component.Vec2{X: 0, Y: 0}},
			PhysicsBody: emptyBody},
	}))
	_, failedBefore := rt.reconcileFailed[cardinal.EntityID(1)]
	require.True(t, failedBefore, "entity 1 should be in reconcileFailed after the failed reconcile")

	// A duplicate-ID reconcile returns before the clear, so the prior failure is retained.
	dupErr := rt.ReconcileFromECS([]PhysicsRebuildEntry{
		{EntityID: 1, Transform: component.Transform2D{Position: component.Vec2{X: 0, Y: 0}},
			PhysicsBody: boxBody()},
		{EntityID: 1, Transform: component.Transform2D{Position: component.Vec2{X: 5, Y: 0}},
			PhysicsBody: boxBody()},
	})
	require.Error(t, dupErr, "duplicate IDs should be rejected")
	require.Contains(t, dupErr.Error(), "duplicate entity_id 1")

	_, failedAfter := rt.reconcileFailed[cardinal.EntityID(1)]
	require.True(t, failedAfter, "entity 1 should remain in reconcileFailed after the duplicate-ID return")
}
