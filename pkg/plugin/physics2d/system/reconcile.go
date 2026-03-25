package system

import (
	"github.com/ByteArena/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
)

// ReconcilePhysicsSystemState syncs ECS → Box2D each tick before the step system.
type ReconcilePhysicsSystemState struct {
	cardinal.BaseSystemState
	cardinal.Contains[physicsArchetype]
	Singleton physicsSingletonArchetype
}

// ReconcilePhysicsSystem applies incremental ECS changes to Box2D. Runs on cardinal.PreUpdate.
// If the runtime has no world (e.g. after ResetRuntime), performs FullRebuildFromECS like Init.
func ReconcilePhysicsSystem(state *ReconcilePhysicsSystemState) {
	rt := internal.Runtime()
	if rt == nil {
		return
	}
	ensurePhysicsSingleton(&state.Singleton)
	entries := gatherRebuildEntries(state.Iter())
	cfg := stepConfig()
	g := box2d.MakeB2Vec2(cfg.Gravity.X, cfg.Gravity.Y)
	if rt.World == nil {
		if err := internal.FullRebuildFromECS(g, entries); err != nil {
			state.Logger().Error().Err(err).Msg("physics2d: FullRebuildFromECS failed (nil world recovery)")
		}
		return
	}
	if err := internal.ReconcileFromECS(entries); err != nil {
		state.Logger().Error().Err(err).Msg("physics2d: ReconcileFromECS failed")
	}
}
