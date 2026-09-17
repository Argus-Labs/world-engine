package internal

import (
	"fmt"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// Restore determinism: the same snapshot restored twice in the same configuration must
// simulate to the same world, bit for bit.
//
// This is the axis the rest of the suite does not cover. The other determinism gates
// compare across worker counts or against a committed fixture; none runs one configuration
// repeatedly and compares it with itself, which is the only thing that catches a Go map
// iterated for its side effects.
//
// The scene is deliberately a clump of sleeping crates: a restored sleeper is its own solver
// set, so the order the post-rebuild wake visits them fixes their index in the awake set,
// and that index reaches the move array, contact creation order, and constraint colouring.
// Before wakePersistedContactEntities sorted its wake, 30 identical restores of this scene
// produced 6 distinct worlds.

const (
	restoreCrateCount = 14
	restoreCrate0     = cardinal.EntityID(100)
	restoreGravity    = -10.0
)

// Shape entity ids for the restore scene: the ground box and the crate box, mirrored into
// the runtime by restoreShapeMirror as the pipeline's SyncShapes would.
const (
	restoreGroundShape = cardinal.EntityID(2)
	restoreCrateShape  = cardinal.EntityID(3)
)

// restoreShapeMirror loads the two box shapes the scene references into rt.ShapeMirror.
func restoreShapeMirror(rt *Runtime) {
	box := func(hw, hh float64) ResolvedShape {
		return Resolve(component.ShapeCommon{
			Density: 1, Friction: 0.6, CategoryBits: 1, MaskBits: ^uint64(0),
		}, component.BoxGeom{HalfExtents: component.Vec2{X: hw, Y: hh}})
	}
	rt.ShapeMirror[restoreGroundShape] = box(40, 1)
	rt.ShapeMirror[restoreCrateShape] = box(0.5, 0.5)
}

// restoreSnapshotEntries is a ground plane plus crates that settled and fell asleep,
// as a snapshot would hold them (Awake=false, mirrored from the solver).
func restoreSnapshotEntries() []PhysicsRebuildEntry {
	slots := func(shape cardinal.EntityID) immutable.Slice[component.ShapeSlot] {
		return immutable.SliceOf(component.ShapeSlot{Shape: shape})
	}
	out := []PhysicsRebuildEntry{{
		EntityID:  1,
		Transform: component.Transform2D{Position: component.Vec2{Y: -1}},
		PhysicsBody: component.PhysicsBody2D{
			BodyType: component.BodyTypeStatic, Active: true, Awake: true,
			SleepingAllowed: true, GravityScale: 1, Shapes: slots(restoreGroundShape),
		},
	}}
	for i := range restoreCrateCount {
		out = append(out, PhysicsRebuildEntry{
			EntityID: restoreCrate0 + cardinal.EntityID(i),
			Transform: component.Transform2D{
				Position: component.Vec2{X: float64(i)*0.35 - 0.4, Y: 0.5 + float64(i)*1.05},
				Rotation: float64(i) * 0.11,
			},
			PhysicsBody: component.PhysicsBody2D{
				BodyType: component.BodyTypeDynamic, Active: true,
				Awake:           false,
				SleepingAllowed: true, GravityScale: 1, Shapes: slots(restoreCrateShape),
			},
		})
	}
	return out
}

// restoreBaseline is the persisted ActiveContacts component: each crate touching the next.
func restoreBaseline() component.ActiveContacts {
	pairs := make([]component.ContactPairEntry, 0, restoreCrateCount-1)
	for i := 0; i+1 < restoreCrateCount; i++ {
		pairs = append(pairs, component.ContactPairEntry{
			EntityA: restoreCrate0 + cardinal.EntityID(i), EntityB: restoreCrate0 + cardinal.EntityID(i+1),
		})
	}
	return component.ActiveContacts{Pairs: immutable.SliceOf(pairs...)}
}

// restoreAndFingerprint runs the real restore path — FullRebuildFromECS,
// LoadActiveContactsFromComponent, then steps — and returns every crate's exact pose.
func restoreAndFingerprint(t *testing.T, steps int) string {
	t.Helper()
	g := component.Vec2{Y: restoreGravity}
	rt := NewRuntime(g, 1.0/60.0, 4, 0)
	restoreShapeMirror(rt)
	if err := rt.FullRebuildFromECS(g, restoreSnapshotEntries()); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	rt.LoadActiveContactsFromComponent(restoreBaseline())

	for range steps {
		rt.Step() // the first call runs wakePersistedContactEntities
	}

	var fp strings.Builder
	for i := range restoreCrateCount {
		bodyID := rt.Bodies[restoreCrate0+cardinal.EntityID(i)]
		p := rt.World.BodyPosition(bodyID)
		r := rt.World.BodyRotation(bodyID)
		fmt.Fprintf(&fp, "%.17g,%.17g,%.17g,%.17g;", p.X, p.Y, r.C, r.S)
	}
	return fp.String()
}

func TestRestoreIsDeterministicAcrossRepeatedRuns(t *testing.T) {
	t.Parallel()
	const runs, steps = 24, 900

	seen := map[string]int{}
	for range runs {
		seen[restoreAndFingerprint(t, steps)]++
	}
	if len(seen) != 1 {
		t.Errorf("%d identical restores produced %d distinct worlds; restore is not deterministic",
			runs, len(seen))
		for fp, n := range seen {
			t.Logf("  %2d run(s): %s", n, fp[:min(120, len(fp))])
		}
	}
}
