package internal

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/stretchr/testify/require"
)

// chainSegmentFixture is the per-segment state a mutable chain edit must reach.
type chainSegmentFixture struct {
	Friction    float64
	Restitution float64
	Filter      box2d.Filter
}

// twoChainEntry returns a static body whose slots are a box, then two open chains at
// different heights, so an edit to the second chain shows whether it landed on the right one.
func twoChainEntry(entityID cardinal.EntityID) PhysicsRebuildEntry {
	chain := func(y float64) component.ColliderShape {
		return component.ColliderShape{
			ShapeType: component.ShapeTypeStaticChain,
			ChainPoints: immutable.SliceOf(
				component.Vec2{X: -3}, component.Vec2{X: -2}, component.Vec2{X: 2}, component.Vec2{X: 3},
			),
			LocalOffset:  component.Vec2{Y: y},
			Friction:     0.6,
			CategoryBits: 1,
			MaskBits:     ^uint64(0),
		}
	}

	return PhysicsRebuildEntry{
		EntityID:    entityID,
		PhysicsBody: component.NewPhysicsBody2D(component.BodyTypeStatic, boxShape, chain(-2), chain(2)),
	}
}

// TestReconcileChainMaterialEditMatchesRebuild edits the friction, restitution, and filter of the
// second chain on a compound body in place, then checks every chain segment against a world
// rebuilt from the edited component. The first chain must keep its original values.
func TestReconcileChainMaterialEditMatchesRebuild(t *testing.T) {
	t.Parallel()

	const entityID = 1

	gravity := component.Vec2{}
	entry := twoChainEntry(entityID)

	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)
	require.NoError(t, rt.FullRebuildFromECS(gravity, []PhysicsRebuildEntry{entry}))

	before := chainSegmentFixtures(t, rt, entityID)

	edited := entry.PhysicsBody.Shapes.At(2)
	edited.Friction = 0.1
	edited.Restitution = 0.8
	edited.CategoryBits = 2
	edited.MaskBits = 4
	edited.GroupIndex = -3
	entry.PhysicsBody.Shapes.Set(2, edited)

	require.NoError(t, rt.ReconcileFromECS([]PhysicsRebuildEntry{entry}))

	rebuilt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)
	require.NoError(t, rebuilt.FullRebuildFromECS(gravity, []PhysicsRebuildEntry{entry}))

	got := chainSegmentFixtures(t, rt, entityID)
	want := chainSegmentFixtures(t, rebuilt, entityID)
	require.Equal(t, want, got, "in-place chain edit must match a rebuild from the edited component")
	require.Equal(t, before[0], got[0], "the unedited first chain keeps its original fixtures")
	require.NotEqual(t, before[1], got[1], "the edited second chain changed")
}

// TestReconcileChainEditFailsWithoutTrackedChain checks that an edit to a chain slot whose chain
// the runtime cannot resolve is reported as a failure instead of being skipped: the shadow must
// keep the old component so the edit is retried rather than recorded as applied.
func TestReconcileChainEditFailsWithoutTrackedChain(t *testing.T) {
	t.Parallel()

	const entityID = 1

	gravity := component.Vec2{}
	entry := twoChainEntry(entityID)

	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)
	require.NoError(t, rt.FullRebuildFromECS(gravity, []PhysicsRebuildEntry{entry}))

	shadow := rt.Shadow[entityID]
	rt.Chains[entityID] = rt.Chains[entityID][:1] // lose track of the second chain

	edited := entry.PhysicsBody.Shapes.At(2)
	edited.CategoryBits = 2
	entry.PhysicsBody.Shapes.Set(2, edited)

	require.ErrorContains(t, rt.ReconcileFromECS([]PhysicsRebuildEntry{entry}), "shapes[2]")
	require.Equal(t, shadow, rt.Shadow[entityID], "a failed edit must not advance the shadow")
	require.Contains(t, rt.reconcileFailed, cardinal.EntityID(entityID), "the entity is excluded from writeback")
}

// chainSegmentFixtures reads friction, restitution, and filter off every segment of each chain
// tracked for entityID, one inner slice per chain in slot order.
func chainSegmentFixtures(t *testing.T, rt *Runtime, entityID cardinal.EntityID) [][]chainSegmentFixture {
	t.Helper()

	chains := rt.Chains[entityID]
	require.Len(t, chains, 2, "the test body has two chain slots")

	out := make([][]chainSegmentFixture, len(chains))
	for i, chainID := range chains {
		segments := make([]box2d.ShapeID, rt.World.ChainSegmentCount(chainID))
		for _, sid := range segments[:rt.World.ChainSegments(chainID, segments)] {
			out[i] = append(out[i], chainSegmentFixture{
				Friction:    rt.World.ShapeFriction(sid),
				Restitution: rt.World.ShapeRestitution(sid),
				Filter:      rt.World.ShapeFilter(sid),
			})
		}
	}

	return out
}
