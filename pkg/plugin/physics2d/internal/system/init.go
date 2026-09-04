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
	Singleton physicsSingletonSearch
}

// NewInitPhysicsSystem returns the Init-hook system bound to rt. The system creates the
// singleton entity (if absent), then builds the Box2D world and bodies from ECS.
func NewInitPhysicsSystem(rt *internal.Runtime) func(*InitPhysicsSystemState) {
	return func(state *InitPhysicsSystemState) {
		ensurePhysicsSingleton(&state.Singleton)

		entries := rt.KeepRebuildEntriesScratch(
			gatherRebuildEntries(rt.RebuildEntriesScratch(), state.Bodies.Iter()))
		if err := rt.FullRebuildFromECS(rt.Gravity, entries); err != nil {
			panic(eris.Wrap(err, "physics2d: FullRebuildFromECS failed"))
		}
	}
}
