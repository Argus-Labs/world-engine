package component

import (
	"fmt"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

// Shape constructors are the intended way to build a ColliderShape: each sets the tag and only
// its own geometry, with Box2D's default material (friction 0.6, restitution 0, density 1) and
// filter (category 1, mask all). Chain the option methods to adjust. Validate rejects geometry
// that belongs to another shape type, so a shape can never carry two variants at once.

func newShape(t ShapeType) ColliderShape {
	return ColliderShape{
		ShapeType:    t,
		Friction:     0.6,
		Density:      1,
		CategoryBits: 1,
		MaskBits:     ^uint64(0),
	}
}

// Circle is a circle of radius, centred on the shape's local offset.
func Circle(radius float64) ColliderShape {
	s := newShape(ShapeTypeCircle)
	s.Radius = radius
	return s
}

// Box is an axis-aligned box with the given half extents, before local offset and rotation.
func Box(halfWidth, halfHeight float64) ColliderShape {
	s := newShape(ShapeTypeBox)
	s.HalfExtents = Vec2{X: halfWidth, Y: halfHeight}
	return s
}

// Polygon is a convex polygon of 3..8 vertices in shape-local space. The vertices are copied.
func Polygon(vertices ...Vec2) ColliderShape {
	s := newShape(ShapeTypeConvexPolygon)
	s.Vertices = append([]Vec2(nil), vertices...)
	return s
}

// Chain is an open polyline whose points live on the ChainGeometry2D entity geometry.
func Chain(geometry cardinal.EntityID) ColliderShape {
	s := newShape(ShapeTypeStaticChain)
	s.ChainGeometry = geometry
	return s
}

// ChainLoop is a closed polyline whose points live on the ChainGeometry2D entity geometry.
func ChainLoop(geometry cardinal.EntityID) ColliderShape {
	s := newShape(ShapeTypeStaticChainLoop)
	s.ChainGeometry = geometry
	return s
}

// Edge is a single segment from a to b in shape-local space.
func Edge(a, b Vec2) ColliderShape {
	s := newShape(ShapeTypeEdge)
	s.EdgeVertices = [2]Vec2{a, b}
	return s
}

// Capsule is a segment from a to b inflated by radius.
func Capsule(a, b Vec2, radius float64) ColliderShape {
	s := newShape(ShapeTypeCapsule)
	s.CapsuleCenter1, s.CapsuleCenter2, s.Radius = a, b, radius
	return s
}

// At places the shape at offset and rotation (radians) in body space.
func (s ColliderShape) At(offset Vec2, rotation float64) ColliderShape {
	s.LocalOffset, s.LocalRotation = offset, rotation
	return s
}

// AsSensor makes the shape report overlaps without ever colliding.
func (s ColliderShape) AsSensor() ColliderShape {
	s.IsSensor = true
	return s
}

// Material sets friction, restitution and density.
func (s ColliderShape) Material(friction, restitution, density float64) ColliderShape {
	s.Friction, s.Restitution, s.Density = friction, restitution, density
	return s
}

// Filter sets the collision category and mask bits.
func (s ColliderShape) Filter(category, mask uint64) ColliderShape {
	s.CategoryBits, s.MaskBits = category, mask
	return s
}

// Group sets the Box2D group index: shapes sharing a positive index always collide, a
// negative one never.
func (s ColliderShape) Group(index int32) ColliderShape {
	s.GroupIndex = index
	return s
}

// Geometry field groups, as a bitmask, so Validate can reject a union holding a variant's
// fields under another variant's tag.
const (
	geomRadius uint8 = 1 << iota
	geomHalfExtents
	geomVertices
	geomChainGeometry
	geomEdgeVertices
	geomCapsuleCenters
)

func (s ColliderShape) geometrySet() uint8 {
	var set uint8
	if s.Radius != 0 {
		set |= geomRadius
	}
	if s.HalfExtents != (Vec2{}) {
		set |= geomHalfExtents
	}
	if len(s.Vertices) > 0 {
		set |= geomVertices
	}
	if s.ChainGeometry != 0 {
		set |= geomChainGeometry
	}
	if s.EdgeVertices != ([2]Vec2{}) {
		set |= geomEdgeVertices
	}
	if s.CapsuleCenter1 != (Vec2{}) || s.CapsuleCenter2 != (Vec2{}) {
		set |= geomCapsuleCenters
	}
	return set
}

// allowedGeometry reports the geometry groups a shape type reads, and false for an unknown tag.
func allowedGeometry(t ShapeType) (uint8, bool) {
	switch t {
	case ShapeTypeCircle:
		return geomRadius, true
	case ShapeTypeBox:
		return geomHalfExtents, true
	case ShapeTypeConvexPolygon:
		return geomVertices, true
	case ShapeTypeStaticChain, ShapeTypeStaticChainLoop:
		return geomChainGeometry, true
	case ShapeTypeEdge:
		return geomEdgeVertices, true
	case ShapeTypeCapsule:
		return geomCapsuleCenters | geomRadius, true
	default:
		return 0, false
	}
}

func geometryName(bit uint8) string {
	switch {
	case bit&geomRadius != 0:
		return "radius"
	case bit&geomHalfExtents != 0:
		return "half_extents"
	case bit&geomVertices != 0:
		return "vertices"
	case bit&geomChainGeometry != 0:
		return "chain_geometry"
	case bit&geomEdgeVertices != 0:
		return "edge_vertices"
	default:
		return "capsule_centers"
	}
}

// validateVariant checks the tag is known and no other variant's geometry is set.
func (s ColliderShape) validateVariant() error {
	allowed, ok := allowedGeometry(s.ShapeType)
	if !ok {
		return fmt.Errorf("shape_type: unknown value %d", s.ShapeType)
	}
	if stray := s.geometrySet() &^ allowed; stray != 0 {
		return fmt.Errorf("%s: not used by shape_type %d", geometryName(stray), s.ShapeType)
	}
	return nil
}
