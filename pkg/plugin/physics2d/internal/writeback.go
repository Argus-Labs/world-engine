package internal

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/cbridge"
)

// WritebackEntry holds the ECS refs needed to write Box2D results back to components.
type WritebackEntry struct {
	EntityID    cardinal.EntityID
	Transform   cardinal.Ref[component.Transform2D]
	Velocity    cardinal.Ref[component.Velocity2D]
	PhysicsBody cardinal.Ref[component.PhysicsBody2D]
}

// WritebackFromStepResults reads post-step positions, rotations, and velocities from the
// cbridge.Step body states and writes them into the corresponding ECS Transform2D and
// Velocity2D components. It also updates the shadow state so the next ReconcileFromECS tick
// sees no diff for these values.
//
// Writeback applies to dynamic and kinematic bodies only. Static bodies (BodyType==1) never
// move. Manual bodies (ECS BodyTypeManual==4) are skipped because ECS/gameplay code owns
// their position. Since both BodyTypeKinematic and BodyTypeManual map to Box2D kinematic
// bodies, the ECS body type is checked rather than the bridge body type.
func WritebackFromStepResults(states []cbridge.BodyState, entries []WritebackEntry) {
	rt := Runtime()
	if !cbridge.WorldExists() {
		return
	}

	// Build a lookup from entity ID to writeback entry for efficient matching.
	entryMap := make(map[cardinal.EntityID]*WritebackEntry, len(entries))
	for i := range entries {
		entryMap[entries[i].EntityID] = &entries[i]
	}

	for _, s := range states {
		entityID := cardinal.EntityID(s.EntityID)
		e, ok := entryMap[entityID]
		if !ok {
			continue
		}

		ecsBodyType := e.PhysicsBody.Get().BodyType
		if ecsBodyType == component.BodyTypeStatic || ecsBodyType == component.BodyTypeManual {
			continue
		}

		t := component.Transform2D{
			Position: component.Vec2{X: s.PX, Y: s.PY},
			Rotation: s.Angle,
		}
		v := component.Velocity2D{
			Linear:  component.Vec2{X: s.VX, Y: s.VY},
			Angular: s.AV,
		}

		e.Transform.Set(t)
		e.Velocity.Set(v)

		// Update shadow so ReconcileFromECS sees no diff for these fields next tick.
		if shadow, exists := rt.Shadow[entityID]; exists {
			shadow.Transform = t
			shadow.Velocity = v
			rt.Shadow[entityID] = shadow
		}
	}
}
