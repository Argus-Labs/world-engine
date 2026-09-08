package component

import (
	"fmt"

	"github.com/argus-labs/world-engine/pkg/immutable"
)

// Geometry components: a shape entity carries [ShapeCommon] plus exactly one of these. The
// component present is the shape's kind; there is no type tag. All coordinates are in shape
// space, before the slot's LocalOffset and LocalRotation.

// CircleGeom is a circle centred on the slot's local offset.
type CircleGeom struct {
	Radius float64 `json:"radius"`
}

// Name returns the ECS component name.
func (CircleGeom) Name() string { return "circle_geom_2d" }

// Validate checks Radius for NaN/Inf.
func (g CircleGeom) Validate() error {
	if !isFinite(g.Radius) {
		return fmt.Errorf("radius: must be finite, got %v", g.Radius)
	}
	return nil
}

// BoxGeom is an axis-aligned box given by half extents (half-width on X, half-height on Y).
type BoxGeom struct {
	HalfExtents Vec2 `json:"half_extents"`
}

// Name returns the ECS component name.
func (BoxGeom) Name() string { return "box_geom_2d" }

// Validate checks HalfExtents for NaN/Inf.
func (g BoxGeom) Validate() error {
	return validateVec2("half_extents", g.HalfExtents)
}

// MaxPolygonVertices is Box2D's convex polygon vertex limit. PolygonGeom stores exactly that
// many slots; Count says how many are used.
const MaxPolygonVertices = 8

// PolygonGeom is a convex polygon of 3..MaxPolygonVertices vertices.
type PolygonGeom struct {
	Vertices [MaxPolygonVertices]Vec2 `json:"vertices"`
	Count    uint8                    `json:"count"`
}

// Name returns the ECS component name.
func (PolygonGeom) Name() string { return "polygon_geom_2d" }

// Validate checks the vertex count and the used vertices for NaN/Inf.
func (g PolygonGeom) Validate() error {
	if g.Count < 3 || g.Count > MaxPolygonVertices {
		return fmt.Errorf("count: must be 3..%d, got %d", MaxPolygonVertices, g.Count)
	}
	for i := range int(g.Count) {
		if err := validateVec2(fmt.Sprintf("vertices[%d]", i), g.Vertices[i]); err != nil {
			return err
		}
	}
	return nil
}

// ChainGeom is a polyline. Loop closes the last point back to the first. Static or kinematic
// bodies only (chains have no mass).
//
// Points are fixed once a body uses the shape: the plugin copies them when it first sees the
// entity and never re-reads them, so a long polyline costs nothing per tick. To change
// terrain, spawn a new chain shape and point the slot at it. Box2D requires at least 4 points
// and enforces that at fixture creation, like the other kinds' geometry rules.
type ChainGeom struct {
	Points immutable.Slice[Vec2] `json:"points"`
	Loop   bool                  `json:"loop"`
}

// Name returns the ECS component name.
func (ChainGeom) Name() string { return "chain_geom_2d" }

// Validate checks every point for NaN/Inf.
func (g ChainGeom) Validate() error {
	for i, v := range g.Points.All() {
		if err := validateVec2(fmt.Sprintf("points[%d]", i), v); err != nil {
			return err
		}
	}
	return nil
}

// EdgeGeom is a single segment from A to B. Static or kinematic bodies only.
type EdgeGeom struct {
	A Vec2 `json:"a"`
	B Vec2 `json:"b"`
}

// Name returns the ECS component name.
func (EdgeGeom) Name() string { return "edge_geom_2d" }

// Validate checks both endpoints for NaN/Inf.
func (g EdgeGeom) Validate() error {
	if err := validateVec2("a", g.A); err != nil {
		return err
	}
	return validateVec2("b", g.B)
}

// CapsuleGeom is the segment from A to B inflated by Radius.
type CapsuleGeom struct {
	A      Vec2    `json:"a"`
	B      Vec2    `json:"b"`
	Radius float64 `json:"radius"`
}

// Name returns the ECS component name.
func (CapsuleGeom) Name() string { return "capsule_geom_2d" }

// Validate checks the endpoints and radius for NaN/Inf.
func (g CapsuleGeom) Validate() error {
	if err := validateVec2("a", g.A); err != nil {
		return err
	}
	if err := validateVec2("b", g.B); err != nil {
		return err
	}
	if !isFinite(g.Radius) {
		return fmt.Errorf("radius: must be finite, got %v", g.Radius)
	}
	return nil
}
