package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physicscomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
	"github.com/rotisserie/eris"
)

// physicsBodyRow matches entities that participate in 2D physics (ECS authoritative).
type physicsBodyRow struct {
	Transform   cardinal.Ref[physicscomp.Transform2D]
	Velocity    cardinal.Ref[physicscomp.Velocity2D]
	PhysicsBody cardinal.Ref[physicscomp.PhysicsBody2D]
}

// gatherRebuildEntries collects physics archetype rows for reconcile/rebuild, appending into
// dst (reset to length 0 first) so callers reuse one buffer instead of re-growing a fresh slice
// every tick. Both callers pass Runtime.RebuildEntriesScratch() and hand the result back through
// Runtime.KeepRebuildEntriesScratch, so the init gather starts from whatever capacity is already
// there and the steady-state gather inherits the capacity init built.
func gatherRebuildEntries(dst []internal.PhysicsRebuildEntry,
	iter cardinal.SearchResult[cardinal.EntityID, physicsBodyRow],
) []internal.PhysicsRebuildEntry {
	entries := dst[:0]
	for eid, row := range iter {
		entries = append(entries, internal.PhysicsRebuildEntry{
			EntityID:    eid,
			Transform:   row.Transform.Get(),
			Velocity:    row.Velocity.Get(),
			PhysicsBody: row.PhysicsBody.Get(),
		})
	}
	return entries
}

// physicsSingletonSearch is the Exact query for the plugin singleton (ActiveContacts).
type physicsSingletonSearch = cardinal.Exact[struct {
	Tag            cardinal.Ref[physicscomp.PhysicsSingletonTag]
	ActiveContacts cardinal.Ref[physicscomp.ActiveContacts]
}]

// InitPhysicsSystemState runs once at world init: FullRebuildFromECS from current ECS entities.
type InitPhysicsSystemState struct {
	cardinal.BaseSystemState
	Bodies    cardinal.Contains[physicsBodyRow]
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
		if err := syncShapes(rt, state.shapes()); err != nil {
			state.Logger().Error().Err(err).Msg("physics2d: shape entity sync")
		}

		entries := rt.KeepRebuildEntriesScratch(
			gatherRebuildEntries(rt.RebuildEntriesScratch(), state.Bodies.Iter()))
		if err := rt.FullRebuildFromECS(rt.Gravity, entries); err != nil {
			panic(eris.Wrap(err, "physics2d: FullRebuildFromECS failed"))
		}
	}
}

// Shape rows, one per geometry kind. Spelled out rather than instantiated from
// internal.ShapeRow so the wire generator, which discovers components through concrete
// cardinal.Ref fields, sees every geometry component.
type (
	circleShapeRow struct {
		Common cardinal.Ref[physicscomp.ShapeCommon]
		Geom   cardinal.Ref[physicscomp.CircleGeom]
	}
	boxShapeRow struct {
		Common cardinal.Ref[physicscomp.ShapeCommon]
		Geom   cardinal.Ref[physicscomp.BoxGeom]
	}
	polygonShapeRow struct {
		Common cardinal.Ref[physicscomp.ShapeCommon]
		Geom   cardinal.Ref[physicscomp.PolygonGeom]
	}
	chainShapeRow struct {
		Common cardinal.Ref[physicscomp.ShapeCommon]
		Geom   cardinal.Ref[physicscomp.ChainGeom]
	}
	edgeShapeRow struct {
		Common cardinal.Ref[physicscomp.ShapeCommon]
		Geom   cardinal.Ref[physicscomp.EdgeGeom]
	}
	capsuleShapeRow struct {
		Common cardinal.Ref[physicscomp.ShapeCommon]
		Geom   cardinal.Ref[physicscomp.CapsuleGeom]
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
// bodies rebuild or reconcile so slots can resolve. The returned error reports a shape entity
// carrying two geometry components; the mirror is still updated with the first one gathered.
func syncShapes(rt *internal.Runtime, s shapeSearches) error {
	entries := rt.ShapeEntriesScratch()
	for eid, row := range s.Circles.Iter() {
		entries = append(entries, shapeEntry(eid, row.Common, row.Geom))
	}
	for eid, row := range s.Boxes.Iter() {
		entries = append(entries, shapeEntry(eid, row.Common, row.Geom))
	}
	for eid, row := range s.Polygons.Iter() {
		entries = append(entries, shapeEntry(eid, row.Common, row.Geom))
	}
	for eid, row := range s.Chains.Iter() {
		entries = append(entries, shapeEntry(eid, row.Common, row.Geom))
	}
	for eid, row := range s.Edges.Iter() {
		entries = append(entries, shapeEntry(eid, row.Common, row.Geom))
	}
	for eid, row := range s.Capsules.Iter() {
		entries = append(entries, shapeEntry(eid, row.Common, row.Geom))
	}
	return rt.SyncShapes(rt.KeepShapeEntriesScratch(entries))
}

// shapeEntry reads one shape entity's two components into a mirror entry.
func shapeEntry[G internal.Geometry](
	eid cardinal.EntityID,
	common cardinal.Ref[physicscomp.ShapeCommon],
	geom cardinal.Ref[G],
) internal.ShapeEntry {
	return internal.ShapeEntry{EntityID: eid, Shape: internal.Resolve(common.Get(), geom.Get())}
}
