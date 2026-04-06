package component

import (
	"errors"
	"fmt"
)

// ShapeType selects which geometry fields in ColliderShape are valid.
//
// Callers must set ShapeType consistently with the populated geometry fields.
// Box2D validates geometry internally (convexity, vertex count, welding) and will panic
// on invalid input. This is caught during development.
type ShapeType uint8

const (
	// ShapeTypeCircle uses Radius; fixture is a circle in the shape's local frame.
	ShapeTypeCircle ShapeType = iota + 1
	// ShapeTypeBox uses HalfExtents (half-width, half-height) for an axis-aligned box in the
	// shape's local frame before applying LocalOffset/LocalRotation.
	ShapeTypeBox
	// ShapeTypeConvexPolygon uses Vertices as a convex polygon in the shape's local frame.
	ShapeTypeConvexPolygon
	// ShapeTypeStaticChain uses ChainPoints for open chain segments (static or kinematic
	// bodies only; not for dynamic bodies which require mass).
	ShapeTypeStaticChain
	// ShapeTypeStaticChainLoop uses ChainPoints for closed chain loops (static or kinematic
	// bodies only; not for dynamic bodies). Unlike ShapeTypeStaticChain, the last vertex
	// automatically connects back to the first, creating a sealed boundary.
	ShapeTypeStaticChainLoop
	// ShapeTypeEdge uses EdgeVertices (exactly 2 points) for a single line segment
	// (static or kinematic bodies only). Lighter than a 2-point chain for isolated barriers
	// or triggers.
	ShapeTypeEdge
)

// ColliderShape is one child shape inside a compound PhysicsBody2D.
//
// Each entry has its own local transform, sensor flag, material, and collision filter (category, mask, group).
// Geometry fields are a tagged-union style: only the fields that match ShapeType are used.
//   - ShapeTypeCircle → Radius
//   - ShapeTypeBox → HalfExtents (half-width on X, half-height on Y, axis-aligned before LocalOffset/LocalRotation)
//   - ShapeTypeConvexPolygon → Vertices (convex polygon, respect backend limits)
//   - ShapeTypeStaticChain → ChainPoints (open polyline in local space)
//   - ShapeTypeStaticChainLoop → ChainPoints (closed loop in local space)
//   - ShapeTypeEdge → EdgeVertices (exactly 2 points in local space)
type ColliderShape struct {
	ShapeType     ShapeType `json:"shape_type"`
	LocalOffset   Vec2      `json:"local_offset"`
	LocalRotation float64   `json:"local_rotation"`
	IsSensor      bool      `json:"is_sensor"`

	// Geometry (use fields matching ShapeType).
	Radius       float64 `json:"radius,omitempty"`
	HalfExtents  Vec2    `json:"half_extents,omitempty"`
	Vertices     []Vec2  `json:"vertices,omitempty"`
	ChainPoints  []Vec2  `json:"chain_points,omitempty"`
	EdgeVertices [2]Vec2 `json:"edge_vertices,omitempty"`

	// Material and per-shape collision filtering (fixture-level in Box2D).
	Friction     float64 `json:"friction"`
	Restitution  float64 `json:"restitution"`
	Density      float64 `json:"density"`
	CategoryBits uint16  `json:"category_bits"`
	MaskBits     uint16  `json:"mask_bits"`
	GroupIndex   int16   `json:"group_index,omitempty"`
}

// Validate checks for NaN/Inf in all float fields and a valid ShapeType tag.
func (s ColliderShape) Validate() error {
	if err := validateVec2("local_offset", s.LocalOffset); err != nil {
		return err
	}
	if !isFinite(s.LocalRotation) {
		return errors.New("local_rotation: must be finite")
	}
	if !isFinite(s.Friction) {
		return fmt.Errorf("friction: must be finite, got %v", s.Friction)
	}
	if !isFinite(s.Restitution) {
		return fmt.Errorf("restitution: must be finite, got %v", s.Restitution)
	}
	if !isFinite(s.Density) {
		return fmt.Errorf("density: must be finite, got %v", s.Density)
	}
	if !isFinite(s.Radius) {
		return fmt.Errorf("radius: must be finite, got %v", s.Radius)
	}
	if err := validateVec2("half_extents", s.HalfExtents); err != nil {
		return err
	}
	for i, v := range s.Vertices {
		if err := validateVec2(fmt.Sprintf("vertices[%d]", i), v); err != nil {
			return err
		}
	}
	for i, v := range s.ChainPoints {
		if err := validateVec2(fmt.Sprintf("chain_points[%d]", i), v); err != nil {
			return err
		}
	}
	for i, v := range s.EdgeVertices {
		if err := validateVec2(fmt.Sprintf("edge_vertices[%d]", i), v); err != nil {
			return err
		}
	}

	switch s.ShapeType {
	case ShapeTypeCircle, ShapeTypeBox, ShapeTypeConvexPolygon, ShapeTypeStaticChain,
		ShapeTypeStaticChainLoop, ShapeTypeEdge:
	default:
		return fmt.Errorf("shape_type: unknown value %d", s.ShapeType)
	}
	return nil
}
