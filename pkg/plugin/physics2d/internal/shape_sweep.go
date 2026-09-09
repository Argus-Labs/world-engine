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
// entity, how many attached bodies reference it (shapeRefs). The counts are derived from the
// shadows: every shadow write goes through setShadow / dropShadow, which adjust the counts by
// the slots that changed, so the counts are exact in steady state and cost nothing on ticks
// where no body changes its slots.
//
// A shape gets a count entry the first time a body uses it. When its count reaches zero it is
// queued, and SweepUnusedShapes destroys the queued shapes still at zero after the whole
// reconcile pass — so a shape moving between two bodies in one tick is never deleted. A shape
// that was spawned but never used has no entry and is never touched, and after a full rebuild
// (Reset, restore) the counts start from the bodies that exist then, so anything unreferenced
// at that moment counts as never used and is kept.
//
// One known gap: a body whose attach fails has no shadow, so its shapes are not counted.

// setShadow stores the shadow for entityID, adjusting shape reference counts by the slots that
// changed since the shadow it replaces.
func (rt *Runtime) setShadow(entityID cardinal.EntityID, s ShadowState) {
	if prev, ok := rt.Shadow[entityID]; ok {
		if !immutable.Equal(prev.PhysicsBody.Shapes, s.PhysicsBody.Shapes) {
			rt.unrefSlots(prev.PhysicsBody.Shapes)
			rt.refSlots(s.PhysicsBody.Shapes)
		}
	} else {
		rt.refSlots(s.PhysicsBody.Shapes)
	}
	rt.Shadow[entityID] = s
}

// dropShadow removes entityID's shadow, releasing its slots' references.
func (rt *Runtime) dropShadow(entityID cardinal.EntityID) {
	prev, ok := rt.Shadow[entityID]
	if !ok {
		return
	}
	rt.unrefSlots(prev.PhysicsBody.Shapes)
	delete(rt.Shadow, entityID)
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

// rebuildShapeRefs recomputes the counts from every shadow, after a full rebuild.
func (rt *Runtime) rebuildShapeRefs() {
	clear(rt.shapeRefs)
	rt.shapeSweepScratch = rt.shapeSweepScratch[:0]
	for _, s := range rt.Shadow {
		rt.refSlots(s.PhysicsBody.Shapes)
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
