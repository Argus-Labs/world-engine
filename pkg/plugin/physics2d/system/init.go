package system

import (
	"github.com/ByteArena/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physicscomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
)

// physicsArchetype matches entities that participate in 2D physics (ECS authoritative).
type physicsArchetype struct {
	Transform cardinal.Ref[physicscomp.Transform2D]
	Velocity  cardinal.Ref[physicscomp.Velocity2D]
	Rigidbody cardinal.Ref[physicscomp.Rigidbody2D]
	Collider  cardinal.Ref[physicscomp.Collider2D]
}

// gatherRebuildEntries collects physics archetype rows for reconcile/rebuild.
// Initial slice capacity is fixed at 64 (arbitrary): small for tiny worlds, may reallocate when
// many physics entities match; a future hint or search count could size this if profiling warrants it.
func gatherRebuildEntries(iter cardinal.SearchResult[cardinal.EntityID, physicsArchetype],
) []internal.PhysicsRebuildEntry {
	entries := make([]internal.PhysicsRebuildEntry, 0, 64)
	for eid, row := range iter {
		entries = append(entries, internal.PhysicsRebuildEntry{
			EntityID:  eid,
			Transform: row.Transform.Get(),
			Velocity:  row.Velocity.Get(),
			Rigidbody: row.Rigidbody.Get(),
			Collider:  row.Collider.Get(),
		})
	}
	return entries
}

// physicsSingletonArchetype is the entity holding plugin-owned state (ActiveContacts).
type physicsSingletonArchetype = cardinal.Exact[struct {
	Tag            cardinal.Ref[physicscomp.PhysicsSingletonTag]
	ActiveContacts cardinal.Ref[physicscomp.ActiveContacts]
}]

// InitPhysicsSystemState runs once at world init: FullRebuildFromECS from current ECS entities.
type InitPhysicsSystemState struct {
	cardinal.BaseSystemState
	cardinal.Contains[physicsArchetype]
	Singleton physicsSingletonArchetype
}

// InitPhysicsSystem creates the singleton entity (if absent), then builds the Box2D world
// and bodies from ECS. Runs on cardinal.Init.
func InitPhysicsSystem(state *InitPhysicsSystemState) {
	ensurePhysicsSingleton(&state.Singleton)

	cfg := stepConfig()
	g := box2d.MakeB2Vec2(cfg.Gravity.X, cfg.Gravity.Y)
	entries := gatherRebuildEntries(state.Iter())
	if err := internal.FullRebuildFromECS(g, entries); err != nil {
		state.Logger().Error().Err(err).Msg("physics2d: FullRebuildFromECS failed")
	}
}
