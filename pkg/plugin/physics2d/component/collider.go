package component

import (
	"errors"
	"fmt"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/immutable"
)

// MaxPolygonVertices is the most vertices a ShapeTypeConvexPolygon collider can carry. It is
// box2d.MaxPolygonVertices, the bound the engine itself compiles in, so ColliderShape.Vertices
// is sized to exactly what Box2D can accept and the two can never drift apart.
const MaxPolygonVertices = box2d.MaxPolygonVertices

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
	// ShapeTypeConvexPolygon uses the first VertexCount entries of Vertices as a convex polygon
	// in the shape's local frame.
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
	// ShapeTypeCapsule uses CapsuleCenter1, CapsuleCenter2, and Radius; fixture is a capsule
	// (two semicircles connected by a rectangle) in the shape's local frame.
	ShapeTypeCapsule
)

// ColliderShape is one child shape inside a compound PhysicsBody2D.
//
// Each entry has its own local transform, sensor flag, material, and collision filter (category, mask, group).
// Geometry fields are a tagged-union style: only the fields that match ShapeType are used.
//   - ShapeTypeCircle → Radius
//   - ShapeTypeBox → HalfExtents (half-width on X, half-height on Y, axis-aligned before LocalOffset/LocalRotation)
//   - ShapeTypeConvexPolygon → Vertices and VertexCount (convex polygon; see WithVertices)
//   - ShapeTypeStaticChain → ChainPoints (open polyline in local space)
//   - ShapeTypeStaticChainLoop → ChainPoints (closed loop in local space)
//   - ShapeTypeEdge → EdgeVertices (exactly 2 points in local space)
//   - ShapeTypeCapsule → CapsuleCenter1, CapsuleCenter2, Radius (two semicircles connected by a rectangle)
//
// A polygon has a bound Box2D fixes at MaxPolygonVertices, so Vertices is a fixed array stored
// inline and VertexCount says how many of its slots are live — the same shape box2d.Polygon uses.
// A chain has no such bound, so ChainPoints is an immutable.Slice: it travels as the same repeated
// field a []Vec2 would, but its backing array is unreachable, so the copy Ref.Get hands back cannot
// be used to write into the world's own column.
//
// ChainPoints has no encoding/json form — the wire format is protobuf, and a Slice keeps its
// storage private. Build a ColliderShape in Go, or restore one through UnmarshalWire.
type ColliderShape struct {
	ShapeType     ShapeType `json:"shape_type"`
	LocalOffset   Vec2      `json:"local_offset"`
	LocalRotation float64   `json:"local_rotation"`
	IsSensor      bool      `json:"is_sensor"`

	// Geometry (use fields matching ShapeType).
	Radius         float64                  `json:"radius,omitempty"`
	HalfExtents    Vec2                     `json:"half_extents,omitempty"`
	Vertices       [MaxPolygonVertices]Vec2 `json:"vertices,omitempty"`
	ChainPoints    immutable.Slice[Vec2]    `json:"chain_points,omitempty"`
	EdgeVertices   [2]Vec2                  `json:"edge_vertices,omitempty"`
	CapsuleCenter1 Vec2                     `json:"capsule_center1,omitempty"`
	CapsuleCenter2 Vec2                     `json:"capsule_center2,omitempty"`

	// Material and per-shape collision filtering (fixture-level in Box2D).
	Friction     float64 `json:"friction"`
	Restitution  float64 `json:"restitution"`
	Density      float64 `json:"density"`
	CategoryBits uint64  `json:"category_bits"`
	MaskBits     uint64  `json:"mask_bits"`
	GroupIndex   int32   `json:"group_index,omitempty"`

	// VertexCount says how many of Vertices' slots are live. It sits at the bottom, away from the
	// geometry it belongs to, because the generator numbers proto fields by declaration order and
	// the schema rule is append-only: putting it next to Vertices would renumber every field after
	// it and silently misread every ColliderShape in an existing snapshot.
	VertexCount int `json:"vertex_count,omitempty"`
}

// WithVertices returns a copy of s carrying vertices as its convex-polygon geometry, with
// VertexCount set to match. Slots past VertexCount are zeroed, so a shape reused with fewer
// vertices carries no leftovers from the longer one.
//
// It panics when given more than MaxPolygonVertices, matching how Box2D itself treats a polygon
// count it cannot hold: truncating instead would silently hand the solver a different shape than
// the caller described.
func (s ColliderShape) WithVertices(vertices ...Vec2) ColliderShape {
	if len(vertices) > MaxPolygonVertices {
		panic(fmt.Sprintf("physics2d: %d vertices exceeds MaxPolygonVertices (%d)",
			len(vertices), MaxPolygonVertices))
	}
	s.Vertices = [MaxPolygonVertices]Vec2{}
	copy(s.Vertices[:], vertices)
	s.VertexCount = len(vertices)
	return s
}

// PolygonVertices returns the live convex-polygon vertices, i.e. Vertices[:VertexCount]. It
// returns nil when VertexCount is out of range, which Validate reports as an error.
func (s ColliderShape) PolygonVertices() []Vec2 {
	if s.VertexCount < 0 || s.VertexCount > MaxPolygonVertices {
		return nil
	}
	return s.Vertices[:s.VertexCount]
}

// Validate checks for NaN/Inf in all float fields, a VertexCount within the polygon bound, and a
// valid ShapeType tag.
func (s ColliderShape) Validate() error {
	if err := s.validateScalars(); err != nil {
		return err
	}
	if err := s.validateGeometry(); err != nil {
		return err
	}
	switch s.ShapeType {
	case ShapeTypeCircle, ShapeTypeBox, ShapeTypeConvexPolygon, ShapeTypeStaticChain,
		ShapeTypeStaticChainLoop, ShapeTypeEdge, ShapeTypeCapsule:
	default:
		return fmt.Errorf("shape_type: unknown value %d", s.ShapeType)
	}
	return nil
}

// validateScalars checks the material and single-value geometry fields for NaN/Inf.
func (s ColliderShape) validateScalars() error {
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
	return nil
}

// validateGeometry checks every point a shape can carry, plus the polygon vertex count. The count
// is the one field with a bound rather than a finiteness rule: Vertices has MaxPolygonVertices
// slots, and a count past them describes vertices that are not there.
func (s ColliderShape) validateGeometry() error {
	if err := validateVec2("local_offset", s.LocalOffset); err != nil {
		return err
	}
	if err := validateVec2("half_extents", s.HalfExtents); err != nil {
		return err
	}
	if err := validateVec2("capsule_center1", s.CapsuleCenter1); err != nil {
		return err
	}
	if err := validateVec2("capsule_center2", s.CapsuleCenter2); err != nil {
		return err
	}
	if s.VertexCount < 0 || s.VertexCount > MaxPolygonVertices {
		return fmt.Errorf("vertex_count: must be between 0 and %d, got %d",
			MaxPolygonVertices, s.VertexCount)
	}
	// The whole array is checked rather than the live prefix: every slot travels on the wire, so a
	// NaN parked in an unused one is still a NaN a peer has to decode.
	for i, v := range s.Vertices {
		if err := validateVec2(fmt.Sprintf("vertices[%d]", i), v); err != nil {
			return err
		}
	}
	for i, v := range s.ChainPoints.All() {
		if err := validateVec2(fmt.Sprintf("chain_points[%d]", i), v); err != nil {
			return err
		}
	}
	for i, v := range s.EdgeVertices {
		if err := validateVec2(fmt.Sprintf("edge_vertices[%d]", i), v); err != nil {
			return err
		}
	}
	return nil
}
