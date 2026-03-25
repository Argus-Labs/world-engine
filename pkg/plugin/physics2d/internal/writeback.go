package internal

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// WritebackEntry holds the ECS refs needed to write Box2D results back to components.
type WritebackEntry struct {
	EntityID  cardinal.EntityID
	Transform cardinal.Ref[component.Transform2D]
	Velocity  cardinal.Ref[component.Velocity2D]
	Rigidbody cardinal.Ref[component.Rigidbody2D]
}

// WritebackFromBox2D reads post-step positions, rotations, and velocities from Box2D bodies
// and writes them into the corresponding ECS Transform2D and Velocity2D components. It also
// updates the shadow state so the next ReconcileFromECS tick sees no diff for these values.
//
// Writeback applies to dynamic and kinematic bodies only. Static bodies never move. Manual
// bodies (BodyTypeManual) are skipped because ECS/gameplay code owns their position. Since
// both BodyTypeKinematic and BodyTypeManual map to Box2D kinematic bodies, the ECS body type
// is checked rather than body.GetType().
func WritebackFromBox2D(entries []WritebackEntry) {
	rt := Runtime()
	if rt.World == nil {
		return
	}

	for _, e := range entries {
		body, ok := rt.Bodies[e.EntityID]
		if !ok || body == nil {
			continue
		}

		ecsBodyType := e.Rigidbody.Get().BodyType
		if ecsBodyType == component.BodyTypeStatic || ecsBodyType == component.BodyTypeManual {
			continue
		}

		pos := body.GetPosition()
		angle := body.GetAngle()
		linVel := body.GetLinearVelocity()
		angVel := body.GetAngularVelocity()

		t := component.Transform2D{
			Position: component.Vec2{X: pos.X, Y: pos.Y},
			Rotation: angle,
		}
		v := component.Velocity2D{
			Linear:  component.Vec2{X: linVel.X, Y: linVel.Y},
			Angular: angVel,
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
