package internal

import (
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
)

// Shape cleanup.
//
// A shape entity lives exactly as long as some body names it. Nothing tracks that between
// ticks: after each reconcile pass the sweep reads which shapes are named right now and deletes
// every mirrored shape that is not. There is no count to keep in step with ECS and no queue to
// drain, so nothing gets out of step when a pass bails, a body drops out of the physics
// archetype, or the world is rebuilt.
//
// Named means named by a PhysicsBody2D component, whether or not its entity is a complete
// physics row (a body that dropped Transform2D or Velocity2D still holds its refs), plus the
// list a body's live fixtures were built from (its shadow) while a failing update has left
// them on an older list. A shape spawned this tick that no body picked up is swept this tick:
// spawn a shape in the same tick as the first body that uses it, or keep it in the store.
//
// The shape store is the other way a shape stays: a kept shape is held for as long as it is
// in the store, named by a body or not, and becomes an ordinary shape again on release.

// SyncKeptShapes refreshes the kept set from the shape store. The sweep reads the set, so a
// shape released this tick is swept this tick unless a body still names it.
func (rt *Runtime) SyncKeptShapes(kept immutable.Slice[component.KeptShape]) {
	clear(rt.keptShapes)
	for k := range kept.Values() {
		rt.keptShapes[k.Shape] = struct{}{}
	}
}

// SweepUnusedShapes destroys every mirrored shape entity that no body names and the store
// does not keep. entries are the bodies gathered this tick; holders is the search over every
// PhysicsBody2D, read only for the entities the gather missed. Ids are destroyed in ascending
// order via destroy (any of the caller's ECS searches), keeping the sweep deterministic.
func (rt *Runtime) SweepUnusedShapes(
	entries []PhysicsRebuildEntry, holders cardinal.SearchResult, destroy func(cardinal.EntityID) bool,
) {
	used, gathered := rt.sweepScratchSets()
	for i := range entries {
		gathered[entries[i].EntityID] = struct{}{}
		markNamed(used, entries[i].PhysicsBody.Shapes)
	}
	for _, sh := range rt.Shadow {
		markNamed(used, sh.PhysicsBody.Shapes)
	}
	for id := range rt.keptShapes {
		used[id] = struct{}{}
	}
	// Reading the component costs more per row than the gather did, so only the entities the
	// gather missed are read here: normally none.
	for row := range holders {
		if _, ok := gathered[row.ID()]; ok {
			continue
		}
		markNamed(used, row.Get[component.PhysicsBody2D]().Shapes)
	}

	unused := rt.sweepUnusedScratch[:0]
	for id := range rt.ShapeMirror {
		if _, ok := used[id]; !ok {
			unused = append(unused, id)
		}
	}
	slices.Sort(unused)
	for _, id := range unused {
		destroy(id)
		delete(rt.ShapeMirror, id)
	}
	rt.sweepUnusedScratch = unused[:0]
}

func markNamed(used map[cardinal.EntityID]struct{}, slots immutable.Slice[component.ShapeRef]) {
	for slot := range slots.Values() {
		used[slot.Shape] = struct{}{}
	}
}

// sweepScratchSets returns the used set then the gathered set, allocated once and cleared
// per call.
func (rt *Runtime) sweepScratchSets() (map[cardinal.EntityID]struct{}, map[cardinal.EntityID]struct{}) {
	if rt.sweepUsedScratch == nil {
		rt.sweepUsedScratch = make(map[cardinal.EntityID]struct{}, len(rt.ShapeMirror))
		rt.sweepGatheredScratch = make(map[cardinal.EntityID]struct{}, len(rt.KnownEntities))
	}
	clear(rt.sweepUsedScratch)
	clear(rt.sweepGatheredScratch)
	return rt.sweepUsedScratch, rt.sweepGatheredScratch
}
