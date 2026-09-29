package internal

import (
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// ShapeSlice is the compound collider list carried by PhysicsBody2D, named here so the
// reconciler's signatures read as shapes rather than as their generic spelling.
type ShapeSlice = immutable.Slice[component.ColliderShape]

// ShadowState is a deep snapshot of the ECS physics components last applied to Box2D for one
// entity. It must not share backing storage with live ECS data so in-place component edits do
// not corrupt the snapshot.
//
// An immutable.Slice is not that protection on its own: it hides its array from indexing, but
// its derivations write straight through it, so a system that does Shapes.With(0, sh) edits the
// very array a shadow taken earlier would still be reading. Hence the copies below.
type ShadowState struct {
	Transform   component.Transform2D
	Velocity    component.Velocity2D
	PhysicsBody component.PhysicsBody2D
}

// NewShadowState returns a shadow snapshot with deep-copied shapes (Shapes and per-shape
// ChainPoints are cloned; Vertices is a fixed array and copies with the struct).
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

// deepCopyShapes clones the shape list, including each shape's chain geometry. immutable.Map
// allocates a fresh array rather than deriving in place, which is what makes the result
// independent of the component the caller passed in.
func deepCopyShapes(shapes ShapeSlice) ShapeSlice {
	return immutable.Map(shapes, deepCopyColliderShape)
}

func deepCopyColliderShape(s component.ColliderShape) component.ColliderShape {
	// ChainPoints is the only field that still points at shared storage; everything else,
	// Vertices included, is a value that copied with the struct.
	s.ChainPoints = immutable.Collect(s.ChainPoints.Values())
	return s
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

// shapesDeepEqual compares shape lists including shape order and chain geometry.
func shapesDeepEqual(a, b ShapeSlice) bool {
	return a.EqualFunc(b, colliderShapeDeepEqual)
}

func colliderShapeDeepEqual(a, b component.ColliderShape) bool {
	if a.ShapeType != b.ShapeType ||
		!vec2Equal(a.LocalOffset, b.LocalOffset) ||
		a.LocalRotation != b.LocalRotation ||
		a.IsSensor != b.IsSensor ||
		a.Radius != b.Radius ||
		!vec2Equal(a.HalfExtents, b.HalfExtents) ||
		!vec2Equal(a.CapsuleCenter1, b.CapsuleCenter1) ||
		!vec2Equal(a.CapsuleCenter2, b.CapsuleCenter2) ||
		a.Friction != b.Friction ||
		a.Restitution != b.Restitution ||
		a.Density != b.Density ||
		a.CategoryBits != b.CategoryBits ||
		a.MaskBits != b.MaskBits ||
		a.GroupIndex != b.GroupIndex {
		return false
	}
	return polygonVerticesEqual(a, b) &&
		immutable.Equal(a.ChainPoints, b.ChainPoints) &&
		a.EdgeVertices == b.EdgeVertices
}

// polygonVerticesEqual compares the live polygon prefix, Vertices[:VertexCount], and nothing past
// it. Slots beyond the count are not geometry — a shape narrowed from four vertices to three still
// carries whatever the fourth held — so comparing the whole array would report a fixture change
// that Box2D would not see. The count is clamped because this runs before Validate on the
// reconcile path, where an out-of-range count is still possible.
func polygonVerticesEqual(a, b component.ColliderShape) bool {
	if a.VertexCount != b.VertexCount {
		return false
	}
	for i := range min(a.VertexCount, component.MaxPolygonVertices) {
		if !vec2Equal(a.Vertices[i], b.Vertices[i]) {
			return false
		}
	}
	return true
}

// ShapesStructuralEqual reports whether two shape lists match for Box2D fixture shape
// definition: shape count/order (topology) and, per index, shape type, local transform, and
// geometry. Differences confined to sensor flag, friction, restitution, density, or filter
// (category, mask, group) are not structural and can be applied with fixture setters.
func ShapesStructuralEqual(a, b ShapeSlice) bool {
	return a.EqualFunc(b, colliderShapeStructuralEqual)
}

func colliderShapeStructuralEqual(a, b component.ColliderShape) bool {
	return a.ShapeType == b.ShapeType &&
		a.IsSensor == b.IsSensor && // Box2D v3: isSensor is immutable after creation
		vec2Equal(a.LocalOffset, b.LocalOffset) &&
		a.LocalRotation == b.LocalRotation &&
		a.Radius == b.Radius &&
		vec2Equal(a.HalfExtents, b.HalfExtents) &&
		vec2Equal(a.CapsuleCenter1, b.CapsuleCenter1) &&
		vec2Equal(a.CapsuleCenter2, b.CapsuleCenter2) &&
		polygonVerticesEqual(a, b) &&
		immutable.Equal(a.ChainPoints, b.ChainPoints) &&
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
