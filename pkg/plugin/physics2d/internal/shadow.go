package internal

import (
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// ShadowState is a deep snapshot of the ECS physics components last applied to Box2D for one
// entity. It must not share slice backing with live ECS data so in-place component edits do
// not corrupt the snapshot.
type ShadowState struct {
	Transform component.Transform2D
	Velocity  component.Velocity2D
	Rigidbody component.Rigidbody2D
	Collider  component.Collider2D
}

// NewShadowState returns a shadow snapshot with a deep-copied collider (Shapes and per-shape
// Vertices / ChainPoints are cloned).
func NewShadowState(
	t component.Transform2D,
	v component.Velocity2D,
	r component.Rigidbody2D,
	c component.Collider2D,
) ShadowState {
	return ShadowState{
		Transform: t,
		Velocity:  v,
		Rigidbody: r,
		Collider:  DeepCopyCollider2D(c),
	}
}

// DeepCopyCollider2D clones the compound collider, including each shape’s slice geometry.
func DeepCopyCollider2D(c component.Collider2D) component.Collider2D {
	out := make([]component.ColliderShape, len(c.Shapes))
	for i := range c.Shapes {
		out[i] = deepCopyColliderShape(c.Shapes[i])
	}
	return component.Collider2D{Shapes: out}
}

func deepCopyColliderShape(s component.ColliderShape) component.ColliderShape {
	return component.ColliderShape{
		ShapeType:     s.ShapeType,
		LocalOffset:   s.LocalOffset,
		LocalRotation: s.LocalRotation,
		IsSensor:      s.IsSensor,
		Radius:        s.Radius,
		HalfExtents:   s.HalfExtents,
		Vertices:      cloneVec2Slice(s.Vertices),
		ChainPoints:   cloneVec2Slice(s.ChainPoints),
		Friction:      s.Friction,
		Restitution:   s.Restitution,
		Density:       s.Density,
		CategoryBits:  s.CategoryBits,
		MaskBits:      s.MaskBits,
		GroupIndex:    s.GroupIndex,
	}
}

func cloneVec2Slice(src []component.Vec2) []component.Vec2 {
	if src == nil {
		return nil
	}
	if len(src) == 0 {
		return make([]component.Vec2, 0)
	}
	out := make([]component.Vec2, len(src))
	copy(out, src)
	return out
}

// TransformDiffers reports whether the live transform differs from the shadow.
func (s ShadowState) TransformDiffers(t component.Transform2D) bool {
	return !vec2Equal(s.Transform.Position, t.Position) || s.Transform.Rotation != t.Rotation
}

// VelocityDiffers reports whether the live velocity differs from the shadow.
func (s ShadowState) VelocityDiffers(v component.Velocity2D) bool {
	return !vec2Equal(s.Velocity.Linear, v.Linear) || s.Velocity.Angular != v.Angular
}

// RigidbodyDiffers reports whether rigidbody simulation parameters differ from the shadow.
func (s ShadowState) RigidbodyDiffers(r component.Rigidbody2D) bool {
	return s.Rigidbody.BodyType != r.BodyType ||
		s.Rigidbody.LinearDamping != r.LinearDamping ||
		s.Rigidbody.AngularDamping != r.AngularDamping ||
		s.Rigidbody.GravityScale != r.GravityScale
}

// ColliderDiffers reports deep differences in compound collider data: shape count/order (topology),
// per-shape type, transform, sensor, geometry, friction/restitution/density, and filter (category, mask, group).
func (s ShadowState) ColliderDiffers(c component.Collider2D) bool {
	return !Collider2DDeepEqual(s.Collider, c)
}

// PhysicsDiffers is true if any reconciled field differs from the given live components.
func (s ShadowState) PhysicsDiffers(
	t component.Transform2D,
	v component.Velocity2D,
	r component.Rigidbody2D,
	c component.Collider2D,
) bool {
	return s.TransformDiffers(t) ||
		s.VelocityDiffers(v) ||
		s.RigidbodyDiffers(r) ||
		s.ColliderDiffers(c)
}

// Collider2DDeepEqual compares compound colliders including shape order and slice geometry.
func Collider2DDeepEqual(a, b component.Collider2D) bool {
	if len(a.Shapes) != len(b.Shapes) {
		return false
	}
	for i := range a.Shapes {
		if !colliderShapeDeepEqual(a.Shapes[i], b.Shapes[i]) {
			return false
		}
	}
	return true
}

func colliderShapeDeepEqual(a, b component.ColliderShape) bool {
	if a.ShapeType != b.ShapeType ||
		!vec2Equal(a.LocalOffset, b.LocalOffset) ||
		a.LocalRotation != b.LocalRotation ||
		a.IsSensor != b.IsSensor ||
		a.Radius != b.Radius ||
		!vec2Equal(a.HalfExtents, b.HalfExtents) ||
		a.Friction != b.Friction ||
		a.Restitution != b.Restitution ||
		a.Density != b.Density ||
		a.CategoryBits != b.CategoryBits ||
		a.MaskBits != b.MaskBits ||
		a.GroupIndex != b.GroupIndex {
		return false
	}
	return vec2SliceEqual(a.Vertices, b.Vertices) && vec2SliceEqual(a.ChainPoints, b.ChainPoints)
}

// Collider2DStructuralEqual reports whether two colliders match for Box2D fixture shape
// definition: shape count/order (topology) and, per index, shape type, local transform, and
// geometry. Differences confined to sensor flag, friction, restitution, density, or filter
// (category, mask, group) are not structural and can be applied with fixture setters.
func Collider2DStructuralEqual(a, b component.Collider2D) bool {
	if len(a.Shapes) != len(b.Shapes) {
		return false
	}
	for i := range a.Shapes {
		if !colliderShapeStructuralEqual(a.Shapes[i], b.Shapes[i]) {
			return false
		}
	}
	return true
}

func colliderShapeStructuralEqual(a, b component.ColliderShape) bool {
	return a.ShapeType == b.ShapeType &&
		vec2Equal(a.LocalOffset, b.LocalOffset) &&
		a.LocalRotation == b.LocalRotation &&
		a.Radius == b.Radius &&
		vec2Equal(a.HalfExtents, b.HalfExtents) &&
		vec2SliceEqual(a.Vertices, b.Vertices) &&
		vec2SliceEqual(a.ChainPoints, b.ChainPoints)
}

// ColliderShapeMutableFieldsEqual compares per-shape fields that Box2D can update without
// recreating the fixture shape.
func ColliderShapeMutableFieldsEqual(a, b component.ColliderShape) bool {
	return a.IsSensor == b.IsSensor &&
		a.Friction == b.Friction &&
		a.Restitution == b.Restitution &&
		a.Density == b.Density &&
		a.CategoryBits == b.CategoryBits &&
		a.MaskBits == b.MaskBits &&
		a.GroupIndex == b.GroupIndex
}

func vec2Equal(a, b component.Vec2) bool {
	return a.X == b.X && a.Y == b.Y
}

func vec2SliceEqual(a, b []component.Vec2) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !vec2Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}
