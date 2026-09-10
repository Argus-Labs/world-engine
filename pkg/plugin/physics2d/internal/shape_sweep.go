package internal

import (
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// Shape cleanup.
//
// Shape entities are shared, so no single body may delete one. The runtime counts, per shape
// entity, how many body entities name it in a slot (shapeRefs). The counts come from what each
// body declares in ECS (declaredSlots), not from what Box2D accepted: a body whose attach
// fails still holds its shapes, so the retry on the next tick still has them to point at.
//
// A shape gets a count entry the first time a body declares it. When its count reaches zero the
// entry stays (the shape is "armed": it was used once) and the id is queued. SweepUnusedShapes
// destroys the queued shapes still at zero after the whole reconcile pass, so a shape moving
// between two bodies in one tick is never deleted. A shape spawned but never used has no entry
// and is never touched, and after a full rebuild (Reset, restore) the counts start from the
// bodies that exist then, so anything unreferenced at that moment counts as never used.

// noteDeclared records the slot list a body entity declares this tick, adjusting the counts by
// what changed since the last list it declared. Declaring the same list again costs one compare.
func (rt *Runtime) noteDeclared(entityID cardinal.EntityID, slots immutable.Slice[component.ShapeSlot]) {
	prev, seen := rt.declaredSlots[entityID]
	if seen && immutable.Equal(prev, slots) {
		return
	}
	if seen {
		rt.unrefSlots(prev)
	}
	rt.refSlots(slots)
	rt.declaredSlots[entityID] = slots
}

// forgetDeclared drops a body entity's slot list, releasing its references. Called when the
// entity leaves ECS, never when its body merely fails to attach.
func (rt *Runtime) forgetDeclared(entityID cardinal.EntityID) {
	prev, seen := rt.declaredSlots[entityID]
	if !seen {
		return
	}
	rt.unrefSlots(prev)
	delete(rt.declaredSlots, entityID)
}

func (rt *Runtime) refSlots(slots immutable.Slice[component.ShapeSlot]) {
	for slot := range slots.Values() {
		rt.shapeRefs[slot.Shape]++
	}
}

// unrefSlots releases one reference per slot. A count that reaches zero stays in the map (the
// shape is "armed": it was used once) and the id is queued for SweepUnusedShapes.
func (rt *Runtime) unrefSlots(slots immutable.Slice[component.ShapeSlot]) {
	for slot := range slots.Values() {
		n, ok := rt.shapeRefs[slot.Shape]
		if !ok {
			continue
		}
		n--
		if n <= 0 {
			n = 0
			rt.shapeSweepScratch = append(rt.shapeSweepScratch, slot.Shape)
		}
		rt.shapeRefs[slot.Shape] = n
	}
}

// rebuildShapeRefs recomputes the declared lists and counts from entries, after a full rebuild.
func (rt *Runtime) rebuildShapeRefs(entries []PhysicsRebuildEntry) {
	clear(rt.declaredSlots)
	clear(rt.shapeRefs)
	rt.shapeSweepScratch = rt.shapeSweepScratch[:0]
	for i := range entries {
		rt.noteDeclared(entries[i].EntityID, entries[i].PhysicsBody.Shapes)
	}
}

// SweepUnusedShapes destroys every shape entity whose last body reference went away during
// this tick's reconcile. Ids are destroyed in ascending order via destroy (any of the caller's
// ECS searches), keeping the sweep deterministic. Shapes referenced again before the sweep
// are kept.
func (rt *Runtime) SweepUnusedShapes(destroy func(cardinal.EntityID) bool) {
	queued := rt.shapeSweepScratch
	if len(queued) == 0 {
		return
	}
	slices.Sort(queued)
	for _, id := range queued {
		n, armed := rt.shapeRefs[id]
		if !armed || n > 0 {
			continue // re-referenced this tick, or already swept (queued twice)
		}
		destroy(id)
		delete(rt.shapeRefs, id)
		delete(rt.ShapeMirror, id)
	}
	rt.shapeSweepScratch = queued[:0]
}
