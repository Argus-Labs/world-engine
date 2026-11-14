package internal

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
	"github.com/stretchr/testify/require"
)

// recordingEmitter captures every flushed contact/trigger event for assertions. It is the
// in-package stand-in for the contactEmitterBridge the pipeline system binds to the runtime.
type recordingEmitter struct {
	triggerBegins []event.TriggerBeginEvent
	triggerEnds   []event.TriggerEndEvent
	contactBegins []event.ContactBeginEvent
	contactEnds   []event.ContactEndEvent
}

func (e *recordingEmitter) EmitTriggerBegin(ev event.TriggerBeginEvent) {
	e.triggerBegins = append(e.triggerBegins, ev)
}
func (e *recordingEmitter) EmitTriggerEnd(ev event.TriggerEndEvent) {
	e.triggerEnds = append(e.triggerEnds, ev)
}
func (e *recordingEmitter) EmitContactBegin(ev event.ContactBeginEvent) {
	e.contactBegins = append(e.contactBegins, ev)
}
func (e *recordingEmitter) EmitContactEnd(ev event.ContactEndEvent) {
	e.contactEnds = append(e.contactEnds, ev)
}

func (e *recordingEmitter) total() int {
	return len(e.triggerBegins) + len(e.triggerEnds) + len(e.contactBegins) + len(e.contactEnds)
}

// sensorVisitorPair builds the persisted ActiveContacts baseline for a single sensor (entity
// sa, shape index saIdx) <-> visitor (entity sb, shape index sbIdx) overlap, the way a real
// TriggerBegin would have left it in the map. The pair is normalized so EntityA < EntityB.
func sensorVisitorPair(sa cardinal.EntityID, saIdx int, sb cardinal.EntityID, sbIdx int,
) component.ActiveContacts {
	if sb < sa || (sb == sa && sbIdx < saIdx) {
		sa, sb = sb, sa
		saIdx, sbIdx = sbIdx, saIdx
	}
	return component.ActiveContacts{Pairs: immutable.SliceOf(component.ContactPairEntry{
		EntityA:             sa,
		ShapeIndexA:         saIdx,
		EntityB:             sb,
		ShapeIndexB:         sbIdx,
		IsSensor:            true,
		FilterACategoryBits: 1,
		FilterAMaskBits:     ^uint64(0),
		FilterBCategoryBits: 1,
		FilterBMaskBits:     ^uint64(0),
	})}
}

// sensorBox returns a static sensor box (half-extents hx,hy) with a category/mask pair that
// overlaps the visitor below. Density is zero: static bodies carry no mass.
func sensorBox(hx, hy float64) component.PhysicsBody2D {
	return component.NewPhysicsBody2D(component.BodyTypeStatic, component.ColliderShape{
		ShapeType:    component.ShapeTypeBox,
		HalfExtents:  component.Vec2{X: hx, Y: hy},
		IsSensor:     true,
		Friction:     0.6,
		Density:      0,
		CategoryBits: 1,
		MaskBits:     ^uint64(0),
	})
}

// dynamicVisitor returns a dynamic circle (radius r) that collides on category 1 / mask all.
// The test worlds use zero gravity so the visitor stays put wherever the rebuild placed it.
// SleepingAllowed is disabled so the overlap state stays observable across steps without
// relying on sleep invariants.
func dynamicVisitor(r float64) component.PhysicsBody2D {
	pb := component.NewPhysicsBody2D(component.BodyTypeDynamic, component.ColliderShape{
		ShapeType:    component.ShapeTypeCircle,
		Radius:       r,
		Friction:     0.3,
		Density:      1,
		CategoryBits: 1,
		MaskBits:     ^uint64(0),
	})
	pb.SleepingAllowed = false
	return pb
}

// at returns a rebuild entry for a body at (x,y) with zero velocity.
func at(id cardinal.EntityID, x, y float64, pb component.PhysicsBody2D) PhysicsRebuildEntry {
	return PhysicsRebuildEntry{
		EntityID:    id,
		Transform:   component.Transform2D{Position: component.Vec2{X: x, Y: y}},
		PhysicsBody: pb,
	}
}

// resetRebuildAndDiff runs the full post-rebuild contact pipeline against a freshly rebuilt
// world: FullRebuildFromECS, load the given persisted baseline, suppressed Step, then the
// FlushBufferedContacts diff. It returns the emitter that recorded the diff's events.
//
// This mirrors the real pipeline sequence (system/pipeline.go): loadContactBaseline ->
// SetStepEmitter -> Step -> FlushBufferedContacts, with the world rebuilt from a snapshot
// restore or Plugin.Reset. The world is built with zero gravity so the visitor stays put
// wherever the rebuild placed it; every test caller exercises a static or zero-gravity scene.
func resetRebuildAndDiff(t *testing.T, entries []PhysicsRebuildEntry,
	baseline component.ActiveContacts,
) (*Runtime, *recordingEmitter) {
	t.Helper()
	gravity := component.Vec2{}
	rt := NewRuntime(gravity, defaultFixedDT, defaultSubStepCount, 0)
	require.NoError(t, rt.FullRebuildFromECS(gravity, entries))
	rt.LoadActiveContactsFromComponent(baseline)

	em := &recordingEmitter{}
	rt.SetStepEmitter(em)
	rt.Step()
	rt.FlushBufferedContacts()
	return rt, em
}

// sensorVisitorOverlapLive reports whether the sensor shape on sensorEntity still overlaps the
// visitor shape on visitorEntity in the live world, via the sensor's own overlap list
// (World.ShapeSensorData). This is the API the fix re-derives state from, so it is also the API
// the assertions check.
//
//nolint:unparam // every suite uses entities 1 and 2; params stay generic for future multi-sensor suites.
func sensorVisitorOverlapLive(t *testing.T, rt *Runtime, sensorEntity, visitorEntity cardinal.EntityID) bool {
	t.Helper()
	sensorShape := rt.Shapes[sensorEntity][0]
	capacity := rt.World.ShapeSensorCapacity(sensorShape)
	if capacity == 0 {
		return false
	}
	buf := make([]box2d.ShapeID, capacity)
	n := rt.World.ShapeSensorData(sensorShape, buf)
	for i := range n {
		// Resolve the visitor overlap back to its entity/shape-index and compare identity,
		// so a generation mismatch (destroyed+recreated shape) does not alias the visitor.
		ent, idx := rt.shapeIdentity(buf[i])
		if ent == visitorEntity && idx == 0 {
			return true
		}
	}
	return false
}

// --- The regression test for the reported bug ---

// TestDiffAfterRebuildSustainedSensorOverlapEmitsNoEvents is the direct regression test for the
// spurious-TriggerEnd-after-rebuild bug. A sensor pair that is persisted in the ActiveContacts
// baseline and still overlapping after the rebuild must produce no events on the post-rebuild
// diff tick, and the pair must stay in the baseline. Before the fix, the diff's End loop treated
// every persisted sensor pair as "gone" (sensor pairs are structurally absent from the live
// solid contact list) and emitted a TriggerEnd while the overlap persisted, desynchronizing any
// game code that latches trigger state on Begin/End.
func TestDiffAfterRebuildSustainedSensorOverlapEmitsNoEvents(t *testing.T) {
	t.Parallel()

	const sensorEntity, visitorEntity cardinal.EntityID = 1, 2

	entries := []PhysicsRebuildEntry{
		at(sensorEntity, 0, 0, sensorBox(3, 3)),
		at(visitorEntity, 0, 0, dynamicVisitor(0.5)), // co-located, overlapping
	}
	baseline := sensorVisitorPair(sensorEntity, 0, visitorEntity, 0)

	rt, em := resetRebuildAndDiff(t, entries, baseline)

	// The overlap demonstrably persists after the suppressed step + diff.
	require.True(t, sensorVisitorOverlapLive(t, rt, sensorEntity, visitorEntity),
		"visitor must still overlap the sensor after the rebuild diff")

	// No events whatsoever: sustained overlap, game already knows.
	require.Empty(t, em.triggerEnds, "sustained sensor overlap must not emit a spurious TriggerEnd")
	require.Empty(t, em.triggerBegins, "sustained sensor overlap must not replay a TriggerBegin")
	require.Zero(t, em.total(), "sustained sensor overlap must emit no contact/trigger events")

	// The pair stays in the baseline; nothing was deleted.
	require.Len(t, rt.ActiveContacts, 1, "sustained sensor pair must remain in ActiveContacts")
	key := ContactPairKey{EntityA: sensorEntity, ShapeIndexA: 0, EntityB: visitorEntity, ShapeIndexB: 0}
	info, ok := rt.ActiveContacts[key]
	require.True(t, ok, "the exact sensor/visitor pair key must be retained")
	require.True(t, info.IsSensor, "retained pair must still be flagged sensor")

	// Subsequent normal steps keep the overlap sustained: no TriggerBegin/End ever fires,
	// and the pair never leaves the baseline. This is the "lost until re-enter" half of the
	// bug: before the fix the pair was deleted, so no begin could ever fire again while the
	// entity stayed inside.
	for range 5 {
		em2 := &recordingEmitter{}
		rt.SetStepEmitter(em2)
		rt.Step()
		rt.FlushBufferedContacts()
		require.Zero(t, em2.total(), "no events on sustained sensor ticks after rebuild")
		require.True(t, sensorVisitorOverlapLive(t, rt, sensorEntity, visitorEntity),
			"overlap must persist across post-rebuild ticks")
	}
	require.Len(t, rt.ActiveContacts, 1, "pair must still be in ActiveContacts after post-rebuild ticks")
}

// TestDiffAfterRebuildGoneSensorOverlapEmitsTriggerEnd verifies the fix does not over-correct: a
// persisted sensor pair whose overlap is genuinely gone after the rebuild (the visitor was moved
// out of the sensor, or the sensor no longer overlaps) still synthesizes a TriggerEnd and is
// removed from the baseline. Before the fix this case happened to pass (every sensor pair was
// ended), so this test pins that the query-based fix handles the gone case correctly.
func TestDiffAfterRebuildGoneSensorOverlapEmitsTriggerEnd(t *testing.T) {
	t.Parallel()

	const sensorEntity, visitorEntity cardinal.EntityID = 1, 2

	// Visitor rebuilt far outside the sensor (box spans [-3,3]^2; (40,40) is well clear).
	entries := []PhysicsRebuildEntry{
		at(sensorEntity, 0, 0, sensorBox(3, 3)),
		at(visitorEntity, 40, 40, dynamicVisitor(0.5)),
	}
	baseline := sensorVisitorPair(sensorEntity, 0, visitorEntity, 0)

	rt, em := resetRebuildAndDiff(t, entries, baseline)

	// The overlap is genuinely gone.
	require.False(t, sensorVisitorOverlapLive(t, rt, sensorEntity, visitorEntity),
		"visitor must not overlap the sensor after the rebuild")

	// Exactly one TriggerEnd for the right pair, no other events.
	require.Len(t, em.triggerEnds, 1, "gone sensor overlap must emit exactly one TriggerEnd")
	require.Empty(t, em.triggerBegins, "gone sensor overlap must not emit a TriggerBegin")
	require.Empty(t, em.contactEnds, "no solid ContactEnd expected")
	require.Empty(t, em.contactBegins, "no solid ContactBegin expected")
	ev := em.triggerEnds[0]
	require.Equal(t, sensorEntity, ev.EntityA)
	require.Equal(t, visitorEntity, ev.EntityB)
	require.Equal(t, 0, ev.ShapeIndexA)
	require.Equal(t, 0, ev.ShapeIndexB)

	// The pair is removed from the baseline.
	require.Empty(t, rt.ActiveContacts, "gone sensor pair must be removed from ActiveContacts")
	require.True(t, rt.ActiveContactsDirty, "dirty flag must be set so the ECS component is rewritten")
}

// TestDiffAfterRebuildDoesNotEmitBeginForNewSensorOverlap pins the build-time suppression that
// the fix must preserve. When the baseline is empty (a fresh build / restore whose persisted
// contacts were empty) but a sensor overlap exists after the rebuild, the diff must NOT
// synthesize a TriggerBegin: the suppressed step is precisely the one-shot suppression of
// co-located overlaps (see adoptLiveContactsWithoutEmit and bufferContactEventsFromWorld).
// The fix only re-derives sensor state for the End decision; it must not start emitting Begins
// for sensor pairs the game never saw a Begin for.
func TestDiffAfterRebuildDoesNotEmitBeginForNewSensorOverlap(t *testing.T) {
	t.Parallel()

	const sensorEntity, visitorEntity cardinal.EntityID = 1, 2

	entries := []PhysicsRebuildEntry{
		at(sensorEntity, 0, 0, sensorBox(3, 3)),
		at(visitorEntity, 0, 0, dynamicVisitor(0.5)), // overlapping, but baseline empty
	}
	emptyBaseline := component.ActiveContacts{}

	rt, em := resetRebuildAndDiff(t, entries, emptyBaseline)

	require.True(t, sensorVisitorOverlapLive(t, rt, sensorEntity, visitorEntity),
		"visitor overlaps the sensor after the rebuild")
	require.Zero(t, em.total(),
		"a sensor overlap absent from the baseline must be suppressed (no TriggerBegin)")
	require.Empty(t, rt.ActiveContacts,
		"a suppressed sensor overlap must not enter ActiveContacts")
}

// TestDiffAfterRebuildSustainedSolidOverlapEmitsNoEvents is a sanity guard for the solid End
// branch, which the fix must leave unchanged: a persisted solid contact pair that still touches
// after the rebuild produces no events and stays in the baseline. Solid pairs are the ones the
// diff was originally designed for, so this confirms the sensor special-case did not regress it.
func TestDiffAfterRebuildSustainedSolidOverlapEmitsNoEvents(t *testing.T) {
	t.Parallel()

	const floorEntity, boxEntity cardinal.EntityID = 1, 2

	floor := component.NewPhysicsBody2D(component.BodyTypeStatic, component.ColliderShape{
		ShapeType:    component.ShapeTypeBox,
		HalfExtents:  component.Vec2{X: 4, Y: 1},
		Friction:     0.6,
		Density:      0,
		CategoryBits: 1,
		MaskBits:     ^uint64(0),
	})
	box := component.NewPhysicsBody2D(component.BodyTypeDynamic, component.ColliderShape{
		ShapeType:    component.ShapeTypeBox,
		HalfExtents:  component.Vec2{X: 0.5, Y: 0.5},
		Friction:     0.6,
		Density:      1,
		CategoryBits: 1,
		MaskBits:     ^uint64(0),
	})
	// Floor top at y=0 (center -1, half 1); box resting on it (center 0.5, half 0.5).
	entries := []PhysicsRebuildEntry{
		at(floorEntity, 0, -1, floor),
		at(boxEntity, 0, 0.5, box),
	}
	pairs := []component.ContactPairEntry{{
		EntityA: floorEntity, ShapeIndexA: 0, EntityB: boxEntity, ShapeIndexB: 0,
		FilterACategoryBits: 1, FilterAMaskBits: ^uint64(0),
		FilterBCategoryBits: 1, FilterBMaskBits: ^uint64(0),
	}}
	baseline := component.ActiveContacts{Pairs: immutable.SliceOf(pairs...)}

	rt, em := resetRebuildAndDiff(t, entries, baseline)

	require.Zero(t, em.total(), "sustained solid contact must emit no events after rebuild")
	require.Len(t, rt.ActiveContacts, 1, "sustained solid pair must remain in ActiveContacts")
}

// TestDiffAfterRebuildGoneSolidOverlapEmitsContactEnd is the matching solid sanity guard for the
// gone branch: a persisted solid pair that no longer touches after the rebuild synthesizes a
// ContactEnd and is removed. Confirms the sensor special-case did not regress solid ends.
func TestDiffAfterRebuildGoneSolidOverlapEmitsContactEnd(t *testing.T) {
	t.Parallel()

	const floorEntity, boxEntity cardinal.EntityID = 1, 2

	floor := component.NewPhysicsBody2D(component.BodyTypeStatic, component.ColliderShape{
		ShapeType:    component.ShapeTypeBox,
		HalfExtents:  component.Vec2{X: 4, Y: 1},
		Friction:     0.6,
		Density:      0,
		CategoryBits: 1,
		MaskBits:     ^uint64(0),
	})
	box := component.NewPhysicsBody2D(component.BodyTypeDynamic, component.ColliderShape{
		ShapeType:    component.ShapeTypeBox,
		HalfExtents:  component.Vec2{X: 0.5, Y: 0.5},
		Friction:     0.6,
		Density:      1,
		CategoryBits: 1,
		MaskBits:     ^uint64(0),
	})
	// Floor at y=-1, box far away at y=50: not touching.
	entries := []PhysicsRebuildEntry{
		at(floorEntity, 0, -1, floor),
		at(boxEntity, 0, 50, box),
	}
	pairs := []component.ContactPairEntry{{
		EntityA: floorEntity, ShapeIndexA: 0, EntityB: boxEntity, ShapeIndexB: 0,
		FilterACategoryBits: 1, FilterAMaskBits: ^uint64(0),
		FilterBCategoryBits: 1, FilterBMaskBits: ^uint64(0),
	}}
	baseline := component.ActiveContacts{Pairs: immutable.SliceOf(pairs...)}

	rt, em := resetRebuildAndDiff(t, entries, baseline)

	require.Len(t, em.contactEnds, 1, "gone solid pair must emit a ContactEnd")
	require.Empty(t, em.triggerEnds, "solid pair must not emit a TriggerEnd")
	require.Empty(t, rt.ActiveContacts, "gone solid pair must be removed from ActiveContacts")
}
