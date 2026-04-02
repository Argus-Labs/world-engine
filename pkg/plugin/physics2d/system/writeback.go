package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
)

// WritebackPhysicsSystemState writes Box2D simulation results back to ECS after World.Step.
type WritebackPhysicsSystemState struct {
	cardinal.BaseSystemState
	cardinal.Contains[physicsArchetype]
}

// WritebackPhysicsSystem reads post-step positions/velocities from Box2D bodies and writes
// them into Transform2D and Velocity2D components. Static bodies and entities with
// KinematicAuthority are skipped. Shadow state is updated so the next reconcile sees no diff.
// Runs on cardinal.PostUpdate after PhysicsStepSystem.
func WritebackPhysicsSystem(state *WritebackPhysicsSystemState) {
	rt := internal.Runtime()
	if rt == nil || rt.World == nil {
		return
	}

	entries := make([]internal.WritebackEntry, 0, len(rt.Bodies))
	for eid, row := range state.Iter() {
		entries = append(entries, internal.WritebackEntry{
			EntityID:  eid,
			Transform: row.Transform,
			Velocity:  row.Velocity,
			Rigidbody: row.Rigidbody,
		})
	}

	internal.WritebackFromBox2D(entries)
}
