package internal

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
)

// WritebackEntry holds the entity needed to write Box2D results back to components.
type WritebackEntry struct {
	Entity cardinal.Entity
}

// WritebackFromStepResults reads post-step positions, rotations, velocities, and awake state
// from the Box2D world and writes the ones that changed into the corresponding ECS
// Transform2D, Velocity2D, and PhysicsBody2D components. It also updates the shadow state so
// the next ReconcileFromECS tick sees no diff for these values.
//
// Iteration is driven by entries (ECS iteration order, EntityID-sorted), never by the
// runtime's Go maps, so the write order is deterministic.
//
// Writeback applies to dynamic and kinematic bodies only. Static bodies never move.
// Manual bodies (ECS BodyTypeManual) are skipped because ECS/gameplay code owns their
// position. Since both BodyTypeKinematic and BodyTypeManual map to Box2D kinematic bodies,
// the ECS body type is checked rather than the Box2D body type.
//
// Awake is mirrored back on change so the component tracks solver sleep state instead of
// freezing at its creation value. Disabled bodies are skipped: IsBodyAwake reads false for
// them, which is disablement, not sleep.
func (rt *Runtime) WritebackFromStepResults(entries []WritebackEntry) {
	if rt.World == nil {
		return
	}

	for i := range entries {
		e := &entries[i]
		bodyID, ok := rt.Bodies[e.Entity.ID()]
		if !ok {
			continue
		}

		pb := e.Entity.Get[component.PhysicsBody2D]()
		if pb.BodyType == component.BodyTypeStatic || pb.BodyType == component.BodyTypeManual {
			continue
		}

		pos := rt.World.BodyPosition(bodyID)
		angle := box2d.RotGetAngle(rt.World.BodyRotation(bodyID))
		lv := rt.World.BodyLinearVelocity(bodyID)
		av := rt.World.BodyAngularVelocity(bodyID)
		awake := rt.World.IsBodyAwake(bodyID)

		t := component.Transform2D{
			Position: component.Vec2{X: pos.X, Y: pos.Y},
			Rotation: angle,
		}
		v := component.Velocity2D{
			Linear:  component.Vec2{X: lv.X, Y: lv.Y},
			Angular: av,
		}

		// The shadow holds what ECS has (reconcile refreshed it before the step), so a value
		// equal to it is unchanged and the write is skipped. Downstream change detection then
		// sees a write only when something moved; a sleeping body writes nothing.
		shadow, hasShadow := rt.Shadow[e.Entity.ID()]
		if !hasShadow || shadow.TransformDiffers(t) {
			e.Entity.Set(t)
		}
		if !hasShadow || shadow.VelocityDiffers(v) {
			e.Entity.Set(v)
		}
		if pb.Active && pb.Awake != awake {
			pb.Awake = awake
			e.Entity.Set(pb)
		}
		if hasShadow {
			shadow.Transform = t
			shadow.Velocity = v
			if pb.Active {
				shadow.PhysicsBody.Awake = awake
			}
			rt.Shadow[e.Entity.ID()] = shadow
		}
	}
}
