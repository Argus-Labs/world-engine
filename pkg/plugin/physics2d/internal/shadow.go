package internal

import (
	"slices"

	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// ShadowState is a snapshot of the ECS physics components last applied to Box2D for one
// entity. The slot slice is cloned so in-place component edits do not corrupt the snapshot.
// Shape contents are not shadowed: shapes live on their own entities and the runtime's
// ShapeMirror tracks their changes (see SyncShapes).
type ShadowState struct {
	Transform   component.Transform2D
	Velocity    component.Velocity2D
	PhysicsBody component.PhysicsBody2D
}

// NewShadowState returns a shadow snapshot with the slot slice cloned.
func NewShadowState(
	t component.Transform2D,
	v component.Velocity2D,
	pb component.PhysicsBody2D,
) ShadowState {
	shadow := pb
	shadow.Shapes = slices.Clone(pb.Shapes)
	return ShadowState{
		Transform:   t,
		Velocity:    v,
		PhysicsBody: shadow,
	}
}

// TransformDiffers reports whether the live transform differs from the shadow.
func (s ShadowState) TransformDiffers(t component.Transform2D) bool {
	return !vec2Equal(s.Transform.Position, t.Position) || s.Transform.Rotation != t.Rotation
}

// VelocityDiffers reports whether the live velocity differs from the shadow.
func (s ShadowState) VelocityDiffers(v component.Velocity2D) bool {
	return !vec2Equal(s.Velocity.Linear, v.Linear) || s.Velocity.Angular != v.Angular
}

// BodyParamsDiffer reports whether rigidbody simulation parameters differ from the shadow.
func (s ShadowState) BodyParamsDiffer(p component.PhysicsBody2D) bool {
	return s.PhysicsBody.BodyType != p.BodyType ||
		s.PhysicsBody.LinearDamping != p.LinearDamping ||
		s.PhysicsBody.AngularDamping != p.AngularDamping ||
		s.PhysicsBody.GravityScale != p.GravityScale ||
		s.PhysicsBody.Active != p.Active ||
		s.PhysicsBody.Awake != p.Awake ||
		s.PhysicsBody.SleepingAllowed != p.SleepingAllowed ||
		s.PhysicsBody.Bullet != p.Bullet ||
		s.PhysicsBody.FixedRotation != p.FixedRotation
}

// ShapesDiffer reports whether the slot list changed: count, order, shape entity id, or local
// transform. Changes inside a shape entity are detected by the runtime's mirror instead.
func (s ShadowState) ShapesDiffer(p component.PhysicsBody2D) bool {
	return !slices.Equal(s.PhysicsBody.Shapes, p.Shapes)
}

// PhysicsDiffers is true if any reconciled field differs from the given live components.
func (s ShadowState) PhysicsDiffers(
	t component.Transform2D,
	v component.Velocity2D,
	p component.PhysicsBody2D,
) bool {
	return s.TransformDiffers(t) ||
		s.VelocityDiffers(v) ||
		s.BodyParamsDiffer(p) ||
		s.ShapesDiffer(p)
}

func vec2Equal(a, b component.Vec2) bool {
	return a.X == b.X && a.Y == b.Y
}
