package internal

import (
	"cmp"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
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

// noteDeclared records the slot list a body entity holds this tick, adjusting the counts
// by what changed since the last time.
// The list is copied: With, Without and Filter write into the array they derive from, so a
// declaration that aliased the component would rewrite itself under the next edit and hide
// the change.
func (rt *Runtime) noteDeclared(entityID cardinal.EntityID, slots immutable.Slice[component.ShapeRef]) {
	prev, seen := rt.declaredSlots[entityID]
	if seen && immutable.Equal(prev, slots) {
		return
	}
	if seen {
		rt.unrefSlots(prev)
	}
	rt.refSlots(slots)
	rt.declaredSlots[entityID] = immutable.Collect(slots.Values())
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

func (rt *Runtime) refSlots(slots immutable.Slice[component.ShapeRef]) {
	for slot := range slots.Values() {
		rt.shapeRefs[slot.Shape]++
	}
}

// unrefSlots releases one reference per slot. A shape whose count reaches zero is forgotten
// and queued for SweepUnusedShapes.
func (rt *Runtime) unrefSlots(slots immutable.Slice[component.ShapeRef]) {
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

// rebuildShapeRefs recomputes the declared lists and counts after a full rebuild. Every
// mirrored shape is a sweep candidate again; the counts decide.
//
// Rebuilt rows are not the whole story. An entity can hold a PhysicsBody2D without being a
// complete physics row, and the refs in that component are as real as any other's, so its list
// is carried over rather than dropped. Without that, a rebuild would release the shapes of
// every entity that is between states and let their ids be reused under refs still in use.
func (rt *Runtime) rebuildShapeRefs(
	entries []PhysicsRebuildEntry, stillHoldsBody func(cardinal.EntityID) bool,
) {
	carried := rt.carryOverDeclared(entries, stillHoldsBody)
	clear(rt.declaredSlots)
	clear(rt.shapeRefs)
	rt.shapeSweepScratch = rt.shapeSweepScratch[:0]
	for i := range entries {
		rt.noteDeclared(entries[i].EntityID, entries[i].PhysicsBody.Shapes)
	}
	for i := range carried {
		rt.noteDeclared(carried[i].EntityID, carried[i].Shapes)
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
	if len(queued) == 0 || rt.shapeRefsStale {
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

// carryOverDeclared returns the declared lists that must survive a rebuild: entities that are
// not among the rebuilt rows but still hold a PhysicsBody2D naming their shapes. Allocates,
// which is fine here: rebuilds are Reset, restore and nil-world recovery, not the steady state.
func (rt *Runtime) carryOverDeclared(
	entries []PhysicsRebuildEntry, stillHoldsBody func(cardinal.EntityID) bool,
) []shapeHolder {
	var carried []shapeHolder
	for id, slots := range rt.declaredSlots {
		if slices.ContainsFunc(entries, func(e PhysicsRebuildEntry) bool { return e.EntityID == id }) {
			continue
		}
		if stillHoldsBody(id) {
			carried = append(carried, shapeHolder{EntityID: id, Shapes: slots})
		}
	}
	slices.SortFunc(carried, func(a, b shapeHolder) int { return cmp.Compare(a.EntityID, b.EntityID) })
	return carried
}
