package physics2d

import (
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
)

// A shape is an entity: ShapeCommon (sensor flag, material, filter) plus exactly one geometry
// component. Bodies reference it from a ShapeSlot, so any number of bodies share one shape.
//
// Build one with a constructor, adjust it with the option methods, then Spawn it through a
// ShapeSearch on your system state:
//
//	type SpawnState struct {
//	    cardinal.BaseSystemState
//	    Circles physics2d.CircleShapes
//	    Balls   cardinal.Exact[ballRow]
//	}
//
//	func Spawn(state *SpawnState) {
//	    ball := physics2d.Circle(0.5).Material(0.3, 0.1, 1).Spawn(&state.Circles)
//	    _, row := state.Balls.Create()
//	    row.Body.Set(physics2d.NewPhysicsBody2D(physics2d.BodyTypeDynamic, ball))
//	}
//
// Spawn returns the slot; keep it (or its Shape id) to put the same shape on more bodies.

// Geometry is the constraint satisfied by the geometry components: CircleGeom, BoxGeom,
// PolygonGeom, ChainGeom, EdgeGeom and CapsuleGeom.
type Geometry = internal.Geometry

// ShapeRow is what a shape search yields per entity: its ShapeCommon and geometry refs.
type ShapeRow[G Geometry] = internal.ShapeRow[G]

// ShapeSearch is an Exact search over shape entities of one geometry kind. Declare one on a
// system state per kind you spawn or edit; Cardinal wires it up when the system registers.
type ShapeSearch[G Geometry] struct {
	cardinal.Exact[internal.ShapeRow[G]]
}

// Per-kind shape searches for system states.
type (
	CircleShapes  = ShapeSearch[CircleGeom]
	BoxShapes     = ShapeSearch[BoxGeom]
	PolygonShapes = ShapeSearch[PolygonGeom]
	ChainShapes   = ShapeSearch[ChainGeom]
	EdgeShapes    = ShapeSearch[EdgeGeom]
	CapsuleShapes = ShapeSearch[CapsuleGeom]
)

// ShapeDef is a shape ready to spawn: ShapeCommon plus one geometry component. Constructors
// fill Common with Box2D's defaults (solid, friction 0.6, restitution 0, density 1,
// category 1, mask all).
type ShapeDef[G Geometry] struct {
	Common ShapeCommon
	Geom   G
}

func newShapeDef[G Geometry](geom G) ShapeDef[G] {
	return ShapeDef[G]{Common: component.DefaultShapeCommon(), Geom: geom}
}

// Circle is a circle of radius, centred on the slot's local offset.
func Circle(radius float64) ShapeDef[CircleGeom] {
	return newShapeDef(CircleGeom{Radius: radius})
}

// Box is an axis-aligned box with the given half extents, before the slot's offset and rotation.
func Box(halfWidth, halfHeight float64) ShapeDef[BoxGeom] {
	return newShapeDef(BoxGeom{HalfExtents: Vec2{X: halfWidth, Y: halfHeight}})
}

// Polygon is a convex polygon of 3..MaxPolygonVertices vertices in shape space. More vertices
// than that fail validation at attach.
func Polygon(vertices ...Vec2) ShapeDef[PolygonGeom] {
	var g PolygonGeom
	g.Count = uint8(min(len(vertices), 255)) //nolint:gosec // clamped above
	copy(g.Vertices[:], vertices)
	return newShapeDef(g)
}

// Chain is an open polyline through points (copied) in shape space. Static or kinematic
// bodies only.
func Chain(points ...Vec2) ShapeDef[ChainGeom] {
	return newShapeDef(ChainGeom{Points: slices.Clone(points)})
}

// ChainLoop is a closed polyline through points (copied) in shape space. Static or kinematic
// bodies only.
func ChainLoop(points ...Vec2) ShapeDef[ChainGeom] {
	return newShapeDef(ChainGeom{Points: slices.Clone(points), Loop: true})
}

// Edge is a single segment from a to b in shape space.
func Edge(a, b Vec2) ShapeDef[EdgeGeom] {
	return newShapeDef(EdgeGeom{A: a, B: b})
}

// Capsule is the segment from a to b inflated by radius.
func Capsule(a, b Vec2, radius float64) ShapeDef[CapsuleGeom] {
	return newShapeDef(CapsuleGeom{A: a, B: b, Radius: radius})
}

// AsSensor makes the shape report overlaps without ever colliding.
func (d ShapeDef[G]) AsSensor() ShapeDef[G] {
	d.Common.IsSensor = true
	return d
}

// Material sets friction, restitution and density.
func (d ShapeDef[G]) Material(friction, restitution, density float64) ShapeDef[G] {
	d.Common.Friction, d.Common.Restitution, d.Common.Density = friction, restitution, density
	return d
}

// Filter sets the collision category and mask bits.
func (d ShapeDef[G]) Filter(category, mask uint64) ShapeDef[G] {
	d.Common.CategoryBits, d.Common.MaskBits = category, mask
	return d
}

// Group sets the Box2D group index: shapes sharing a positive index always collide, a
// negative one never.
func (d ShapeDef[G]) Group(index int32) ShapeDef[G] {
	d.Common.GroupIndex = index
	return d
}

// Spawn creates the shape entity through shapes and returns a slot referencing it at the
// body origin. Chain At on the slot to place it.
func (d ShapeDef[G]) Spawn(shapes *ShapeSearch[G]) ShapeSlot {
	id, row := shapes.Create()
	row.Common.Set(d.Common)
	row.Geom.Set(d.Geom)
	return component.Slot(id)
}

// Slot references an existing shape entity at the body origin. Chain At to place it.
func Slot(shape cardinal.EntityID) ShapeSlot { return component.Slot(shape) }
