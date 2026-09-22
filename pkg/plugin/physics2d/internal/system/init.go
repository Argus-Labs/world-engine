package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
	physicscomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
)

// physicsBodyRow matches entities that participate in 2D physics (ECS authoritative).
type physicsBodyRow struct {
	Transform   cardinal.WithComponent[physicscomp.Transform2D]
	Velocity    cardinal.WithComponent[physicscomp.Velocity2D]
	PhysicsBody cardinal.WithComponent[physicscomp.PhysicsBody2D]
}

// gatherRebuildEntries collects physics archetype rows for reconcile/rebuild, appending into
// dst (reset to length 0 first) so callers reuse one buffer instead of re-growing a fresh slice
// every tick. Both callers pass Runtime.RebuildEntriesScratch() and hand the result back through
// Runtime.KeepRebuildEntriesScratch, so the init gather starts from whatever capacity is already
// there and the steady-state gather inherits the capacity init built.
func gatherRebuildEntries(dst []internal.PhysicsRebuildEntry,
	iter cardinal.SearchResult,
) []internal.PhysicsRebuildEntry {
	entries := dst[:0]
	for row := range iter {
		eid := row.ID()
		entries = append(entries, internal.PhysicsRebuildEntry{
			EntityID:    eid,
			Transform:   row.Get[physicscomp.Transform2D](),
			Velocity:    row.Get[physicscomp.Velocity2D](),
			PhysicsBody: row.Get[physicscomp.PhysicsBody2D](),
		})
	}
	return entries
}

// shapeHolderRow matches every entity carrying PhysicsBody2D, complete physics row or not.
// The shape sweep reads it for the entities the physics gather missed: a body that dropped
// Transform2D or Velocity2D still names its shapes in this component.
type shapeHolderRow struct {
	PhysicsBody cardinal.WithComponent[physicscomp.PhysicsBody2D]
}

// physicsSingletonSearch is the Exact query for the plugin singleton (ActiveContacts).
type physicsSingletonSearch = cardinal.Exact[struct {
	Tag            cardinal.WithComponent[physicscomp.PhysicsSingletonTag]
	ActiveContacts cardinal.WithComponent[physicscomp.ActiveContacts]
}]

// InitPhysicsSystemState runs once at world init: FullRebuildFromECS from current ECS entities.
type InitPhysicsSystemState struct {
	cardinal.BaseSystemState
	Bodies    cardinal.Contains[physicsBodyRow]
	Holders   cardinal.Contains[shapeHolderRow]
	Circles   cardinal.Contains[circleShapeRow]
	Boxes     cardinal.Contains[boxShapeRow]
	Polygons  cardinal.Contains[polygonShapeRow]
	Chains    cardinal.Contains[chainShapeRow]
	Edges     cardinal.Contains[edgeShapeRow]
	Capsules  cardinal.Contains[capsuleShapeRow]
	Singleton physicsSingletonSearch
}

func (s *InitPhysicsSystemState) shapes() shapeSearches {
	return shapeSearches{&s.Circles, &s.Boxes, &s.Polygons, &s.Chains, &s.Edges, &s.Capsules}
}

// NewInitPhysicsSystem returns the Init-hook system bound to rt. The system creates the
// singleton entity (if absent), syncs shape entities, then builds the Box2D world and bodies
// from ECS.
func NewInitPhysicsSystem(rt *internal.Runtime) func(*InitPhysicsSystemState) {
	return func(state *InitPhysicsSystemState) {
		ensurePhysicsSingleton(&state.Singleton)
		syncShapes(rt, state.shapes())

		entries := rt.KeepRebuildEntriesScratch(
			gatherRebuildEntries(rt.RebuildEntriesScratch(), state.Bodies.Iter()))
		if err := rt.FullRebuildFromECS(rt.Gravity, entries); err != nil {
			// Log, as the tick path does. A failed rebuild leaves no bodies, so the first
			// tick's reconcile builds the good ones and keeps logging the bad one.
			state.Logger().Error().Err(err).Msg("physics2d: FullRebuildFromECS failed at init")
		}
		// The sweep keeps no state, so it runs here as it does every tick: a shape no body
		// names at init is gone before the first tick, not one tick later.
		rt.SweepUnusedShapes(entries, state.Holders.Iter(),
			func(id cardinal.EntityID) bool { return state.Entity(id).Destroy() })
	}
}

// Shape rows, one per geometry kind. Spelled out rather than instantiated from
// internal.ShapeRow so the wire generator, which discovers components through concrete
// cardinal.WithComponent fields, sees every geometry component.
type (
	circleShapeRow struct {
		Common cardinal.WithComponent[physicscomp.ShapeCommon]
		Geom   cardinal.WithComponent[physicscomp.CircleGeom]
	}
	boxShapeRow struct {
		Common cardinal.WithComponent[physicscomp.ShapeCommon]
		Geom   cardinal.WithComponent[physicscomp.BoxGeom]
	}
	polygonShapeRow struct {
		Common cardinal.WithComponent[physicscomp.ShapeCommon]
		Geom   cardinal.WithComponent[physicscomp.PolygonGeom]
	}
	chainShapeRow struct {
		Common cardinal.WithComponent[physicscomp.ShapeCommon]
		Geom   cardinal.WithComponent[physicscomp.ChainGeom]
	}
	edgeShapeRow struct {
		Common cardinal.WithComponent[physicscomp.ShapeCommon]
		Geom   cardinal.WithComponent[physicscomp.EdgeGeom]
	}
	capsuleShapeRow struct {
		Common cardinal.WithComponent[physicscomp.ShapeCommon]
		Geom   cardinal.WithComponent[physicscomp.CapsuleGeom]
	}
)

// shapeSearches is a view over the six per-kind shape searches. Cardinal wires only the
// top-level fields of a system state, so each state declares the searches itself and hands
// them over through this view.
type shapeSearches struct {
	Circles  *cardinal.Contains[circleShapeRow]
	Boxes    *cardinal.Contains[boxShapeRow]
	Polygons *cardinal.Contains[polygonShapeRow]
	Chains   *cardinal.Contains[chainShapeRow]
	Edges    *cardinal.Contains[edgeShapeRow]
	Capsules *cardinal.Contains[capsuleShapeRow]
}

// syncShapes refreshes the runtime's shape mirror from every shape entity. Must run before
// bodies rebuild or reconcile so slots can resolve.
func syncShapes(rt *internal.Runtime, s shapeSearches) {
	entries := rt.ShapeEntriesScratch()
	for row := range s.Circles.Iter() {
		entries = append(entries, shapeEntry[physicscomp.CircleGeom](row))
	}
	for row := range s.Boxes.Iter() {
		entries = append(entries, shapeEntry[physicscomp.BoxGeom](row))
	}
	for row := range s.Polygons.Iter() {
		entries = append(entries, shapeEntry[physicscomp.PolygonGeom](row))
	}
	for row := range s.Chains.Iter() {
		entries = append(entries, shapeEntry[physicscomp.ChainGeom](row))
	}
	for row := range s.Edges.Iter() {
		entries = append(entries, shapeEntry[physicscomp.EdgeGeom](row))
	}
	for row := range s.Capsules.Iter() {
		entries = append(entries, shapeEntry[physicscomp.CapsuleGeom](row))
	}
	rt.SyncShapes(rt.KeepShapeEntriesScratch(entries))
}

// shapeEntry reads one shape entity's two components into a mirror entry.
func shapeEntry[G internal.Geometry](row cardinal.Entity) internal.ShapeEntry {
	return internal.ShapeEntry{
		EntityID: row.ID(),
		Shape:    internal.Resolve(row.Get[physicscomp.ShapeCommon](), row.Get[G]()),
	}
}
