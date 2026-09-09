package internal

import (
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// Shape cleanup.
//
// A shape entity lives exactly as long as some body names it in a slot. The runtime keeps,
// per body entity, the slot list it last saw in ECS (declared) and a reference count per shape
// entity derived from those lists. The counts follow what bodies declare, not what Box2D
// accepted, so a body that fails to attach still holds its shapes.
//
// After each reconcile pass, every shape entity that no body names is deleted. That includes a
// shape spawned this tick that no body picked up: spawn a shape in the same tick as the first
// body that uses it. A shape that moves between two bodies in one tick is never deleted,
// because the counts are settled before the sweep runs. After a full rebuild (Reset, restore)
// the counts start over from the bodies that exist then, and the same rule applies.

// noteDeclared records the slot list a body entity declares this tick, adjusting the counts
// by what changed since the last time.
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

// forgetDeclared drops a body entity's slot list, releasing its references.
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

// unrefSlots releases one reference per slot. A shape whose count reaches zero is forgotten
// and queued for SweepUnusedShapes.
func (rt *Runtime) unrefSlots(slots immutable.Slice[component.ShapeSlot]) {
	for slot := range slots.Values() {
		n := rt.shapeRefs[slot.Shape] - 1
		if n > 0 {
			rt.shapeRefs[slot.Shape] = n
			continue
		}
		delete(rt.shapeRefs, slot.Shape)
		rt.shapeSweepScratch = append(rt.shapeSweepScratch, slot.Shape)
	}
}

// rebuildShapeRefs recomputes the declared lists and counts from entries, after a full
// rebuild. Every mirrored shape is a sweep candidate again; the counts decide.
func (rt *Runtime) rebuildShapeRefs(entries []PhysicsRebuildEntry) {
	clear(rt.declaredSlots)
	clear(rt.shapeRefs)
	rt.shapeSweepScratch = rt.shapeSweepScratch[:0]
	for i := range entries {
		rt.noteDeclared(entries[i].EntityID, entries[i].PhysicsBody.Shapes)
	}
	for id := range rt.ShapeMirror {
		rt.shapeSweepScratch = append(rt.shapeSweepScratch, id)
	}
}

// SweepUnusedShapes destroys every queued shape entity that no body names. Candidates are
// shapes that lost their last reference this tick and shapes first seen this tick; ids are
// destroyed in ascending order via destroy (any of the caller's ECS searches), keeping the
// sweep deterministic. A candidate that is referenced again by sweep time is kept.
func (rt *Runtime) SweepUnusedShapes(destroy func(cardinal.EntityID) bool) {
	queued := rt.shapeSweepScratch
	if len(queued) == 0 {
		return
	}
	slices.Sort(queued)
	queued = slices.Compact(queued)
	for _, id := range queued {
		if _, used := rt.shapeRefs[id]; used {
			continue
		}
		if _, exists := rt.ShapeMirror[id]; !exists {
			continue // already gone (deleted by the game, or swept last tick)
		}
		destroy(id)
		delete(rt.ShapeMirror, id)
	}
	rt.shapeSweepScratch = queued[:0]
}
