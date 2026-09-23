package internal

import (
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
)

// ShadowState is a snapshot of the ECS physics components last applied to Box2D for one
// entity. It must not share backing storage with live ECS data: With, Without and Filter write
// into the array they derive from, so a system doing Shapes.With(i, shape), or editing a
// chain's Points, would otherwise edit the shadow too and the change would never reach Box2D.
type ShadowState struct {
	Transform   component.Transform2D
	Velocity    component.Velocity2D
	PhysicsBody component.PhysicsBody2D
}

// NewShadowState returns a shadow snapshot of the three components, with its own copy of
// the shape list and of every shape's points.
func NewShadowState(
	t component.Transform2D,
	v component.Velocity2D,
	pb component.PhysicsBody2D,
) ShadowState {
	pb.Shapes = immutable.Collect(func(yield func(component.Shape) bool) {
		for s := range pb.Shapes.Values() {
			if !yield(s.Copy()) {
				return
			}
		}
	})
	return ShadowState{
		Transform:   t,
		Velocity:    v,
		PhysicsBody: pb,
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

// ShapesDiffer reports whether any shape changed: count, order or any field, points included.
func (s ShadowState) ShapesDiffer(p component.PhysicsBody2D) bool {
	return !s.PhysicsBody.Shapes.EqualFunc(p.Shapes, component.Shape.Equal)
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
