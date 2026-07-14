package internal

import (
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// ShadowState is a deep snapshot of the ECS physics components last applied to Box2D for one
// entity. It must not share slice backing with live ECS data so in-place component edits do
// not corrupt the snapshot.
type ShadowState struct {
	Transform   component.Transform2D
	Velocity    component.Velocity2D
	PhysicsBody component.PhysicsBody2D
}

// NewShadowState returns a shadow snapshot with deep-copied shapes (Shapes and per-shape
// Vertices / ChainPoints are cloned).
func NewShadowState(
	t component.Transform2D,
	v component.Velocity2D,
	pb component.PhysicsBody2D,
) ShadowState {
	shadow := pb
	shadow.Shapes = deepCopyShapes(pb.Shapes)
	return ShadowState{
		Transform:   t,
		Velocity:    v,
		PhysicsBody: shadow,
	}
}

// deepCopyShapes clones the shapes slice, including each shape's slice geometry.
func deepCopyShapes(shapes []component.ColliderShape) []component.ColliderShape {
	out := make([]component.ColliderShape, len(shapes))
	for i := range shapes {
		out[i] = deepCopyColliderShape(shapes[i])
	}
	return out
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
		EdgeVertices:  s.EdgeVertices,
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

// ShapesDiffer reports deep differences in compound collider data: shape count/order (topology),
// per-shape type, transform, sensor, geometry, friction/restitution/density, and filter (category, mask, group).
func (s ShadowState) ShapesDiffer(p component.PhysicsBody2D) bool {
	return !shapesDeepEqual(s.PhysicsBody.Shapes, p.Shapes)
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

// shapesDeepEqual compares shape slices including shape order and slice geometry.
func shapesDeepEqual(a, b []component.ColliderShape) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !colliderShapeDeepEqual(a[i], b[i]) {
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
	return vec2SliceEqual(a.Vertices, b.Vertices) &&
		vec2SliceEqual(a.ChainPoints, b.ChainPoints) &&
		a.EdgeVertices == b.EdgeVertices
}

// ShapesStructuralEqual reports whether two shape slices match for Box2D fixture shape
// definition: shape count/order (topology) and, per index, shape type, local transform, and
// geometry. Differences confined to sensor flag, friction, restitution, density, or filter
// (category, mask, group) are not structural and can be applied with fixture setters.
func ShapesStructuralEqual(a, b []component.ColliderShape) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !colliderShapeStructuralEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func colliderShapeStructuralEqual(a, b component.ColliderShape) bool {
	return a.ShapeType == b.ShapeType &&
		a.IsSensor == b.IsSensor && // Box2D v3: isSensor is immutable after creation
		vec2Equal(a.LocalOffset, b.LocalOffset) &&
		a.LocalRotation == b.LocalRotation &&
		a.Radius == b.Radius &&
		vec2Equal(a.HalfExtents, b.HalfExtents) &&
		vec2SliceEqual(a.Vertices, b.Vertices) &&
		vec2SliceEqual(a.ChainPoints, b.ChainPoints) &&
		a.EdgeVertices == b.EdgeVertices
}

// ColliderShapeMutableFieldsEqual compares per-shape fields that Box2D can update without
// recreating the fixture shape.
func ColliderShapeMutableFieldsEqual(a, b component.ColliderShape) bool {
	return a.Friction == b.Friction &&
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
