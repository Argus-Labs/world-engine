package internal

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// WritebackEntry holds the ECS refs needed to write Box2D results back to components.
type WritebackEntry struct {
	EntityID    cardinal.EntityID
	Transform   cardinal.Ref[component.Transform2D]
	Velocity    cardinal.Ref[component.Velocity2D]
	PhysicsBody cardinal.Ref[component.PhysicsBody2D]
}

// WritebackFromStepResults reads post-step positions, rotations, and velocities from the
// Box2D world and writes them into the corresponding ECS Transform2D and Velocity2D
// components. It also updates the shadow state so the next ReconcileFromECS tick sees no
// diff for these values.
//
// Iteration is driven by entries (ECS iteration order, EntityID-sorted), never by the
// runtime's Go maps, so the write order is deterministic.
//
// Writeback applies to dynamic and kinematic bodies only. Static bodies never move.
// Manual bodies (ECS BodyTypeManual) are skipped because ECS/gameplay code owns their
// position. Since both BodyTypeKinematic and BodyTypeManual map to Box2D kinematic bodies,
// the ECS body type is checked rather than the Box2D body type.
func (rt *Runtime) WritebackFromStepResults(entries []WritebackEntry) {
	if rt.World == nil {
		return
	}

	for i := range entries {
		e := &entries[i]
		bodyID, ok := rt.Bodies[e.EntityID]
		if !ok {
			continue
		}

		ecsBodyType := e.PhysicsBody.Get().BodyType
		if ecsBodyType == component.BodyTypeStatic || ecsBodyType == component.BodyTypeManual {
			continue
		}

		pos := rt.World.BodyPosition(bodyID)
		angle := box2d.RotGetAngle(rt.World.BodyRotation(bodyID))
		lv := rt.World.BodyLinearVelocity(bodyID)
		av := rt.World.BodyAngularVelocity(bodyID)

		t := component.Transform2D{
			Position: component.Vec2{X: pos.X, Y: pos.Y},
			Rotation: angle,
		}
		v := component.Velocity2D{
			Linear:  component.Vec2{X: lv.X, Y: lv.Y},
			Angular: av,
		}

		e.Transform.Set(t)
		e.Velocity.Set(v)

		// Update shadow so ReconcileFromECS sees no diff for these fields next tick.
		if shadow, exists := rt.Shadow[e.EntityID]; exists {
			shadow.Transform = t
			shadow.Velocity = v
			rt.Shadow[e.EntityID] = shadow
		}
	}
}
