package physics2d

import (
	"iter"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
)

// A shape is an entity: a material/filter component plus exactly one geometry component.
// Bodies reference it from a ShapeSlot, so any number of bodies share one shape.
//
// Games never touch those components directly. This package does not re-export them, and
// the shape searches below hand out and accept plain ShapeDef values instead of refs. Declare
// one search per geometry kind on the system state, then spawn, read, edit or clone through
// it:
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
// Spawn returns the slot; put it on more bodies to share the shape. A shape lives exactly as
// long as some body names it, so spawn it in the same tick as its first body and keep
// definitions, not slots, for shapes you will need later.
//
// A shape is never edited in place: to change one, Fork it (a copy with your edit applied)
// and point the body's slot at the result. That can only ever affect the bodies you re-point,
// so a shape shared by other bodies stays as it was. To change many bodies, Fork once and
// re-point each of them at the same new slot.

// Geometry is the constraint satisfied by the six geometry kinds. Games see them through the
// per-kind definitions (CircleDef, BoxDef, ...) rather than by name.
type Geometry = internal.Geometry

// ShapeDef is a shape's definition: material and filter in Common, plus one geometry. It is a
// plain value; constructors fill Common with Box2D's defaults (solid, friction 0.6,
// restitution 0, density 1, category 1, mask all).
type ShapeDef[G Geometry] struct {
	Common component.ShapeCommon
	Geom   G
}

// Per-kind definitions, so a game can name what a search hands back without naming the
// geometry component behind it.
type (
	CircleDef  = ShapeDef[component.CircleGeom]
	BoxDef     = ShapeDef[component.BoxGeom]
	PolygonDef = ShapeDef[component.PolygonGeom]
	ChainDef   = ShapeDef[component.ChainGeom]
	EdgeDef    = ShapeDef[component.EdgeGeom]
	CapsuleDef = ShapeDef[component.CapsuleGeom]
)

func newShapeDef[G Geometry](geom G) ShapeDef[G] {
	return ShapeDef[G]{Common: component.DefaultShapeCommon(), Geom: geom}
}

// Circle is a circle of radius, centred on the slot's local offset.
func Circle(radius float64) CircleDef {
	return newShapeDef(component.CircleGeom{Radius: radius})
}

// Box is an axis-aligned box with the given half extents, before the slot's offset and rotation.
func Box(halfWidth, halfHeight float64) BoxDef {
	return newShapeDef(component.BoxGeom{HalfExtents: Vec2{X: halfWidth, Y: halfHeight}})
}

// Polygon is a convex polygon of 3..MaxPolygonVertices vertices in shape space. More vertices
// than that fail validation at attach.
func Polygon(vertices ...Vec2) PolygonDef {
	var g component.PolygonGeom
	g.Count = uint8(min(len(vertices), 255)) //nolint:gosec // clamped above
	copy(g.Vertices[:], vertices)
	return newShapeDef(g)
}

// Chain is an open polyline through points (copied) in shape space. Static or kinematic
// bodies only.
func Chain(points ...Vec2) ChainDef {
	return newShapeDef(component.ChainGeom{Points: immutable.SliceOf(points...)})
}

// ChainLoop is a closed polyline through points (copied) in shape space. Static or kinematic
// bodies only.
func ChainLoop(points ...Vec2) ChainDef {
	return newShapeDef(component.ChainGeom{Points: immutable.SliceOf(points...), Loop: true})
}

// Edge is a single segment from a to b in shape space.
func Edge(a, b Vec2) EdgeDef {
	return newShapeDef(component.EdgeGeom{A: a, B: b})
}

// Capsule is the segment from a to b inflated by radius.
func Capsule(a, b Vec2, radius float64) CapsuleDef {
	return newShapeDef(component.CapsuleGeom{A: a, B: b, Radius: radius})
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
// body origin. Chain At on the slot to place it. Same as shapes.Create(d).
func (d ShapeDef[G]) Spawn(shapes *ShapeSearch[G]) ShapeSlot { return shapes.Create(d) }

// Slot references an existing shape entity at the body origin. Chain At to place it.
func Slot(shape cardinal.EntityID) ShapeSlot { return component.Slot(shape) }

// exactOf is the Cardinal search behind a shape search. It is embedded under this unexported
// name so the raw search is not reachable from another package, and the methods below shadow
// every exported method it would otherwise promote. Nothing here hands out a Ref, and nothing
// here deletes: the plugin removes a shape itself once no body names it.
type exactOf[G Geometry] = cardinal.Exact[internal.ShapeRow[G]]

// ShapeSearch is the API over the shape entities of one geometry kind. Declare one on a
// system state per kind you spawn or edit; Cardinal wires it up when the system registers.
//
// A shape is shared by every body whose slot names it, so nothing here changes a shape in
// place: Fork and Clone hand back a new shape, and only the bodies you point at it change.
type ShapeSearch[G Geometry] struct {
	exactOf[G]
}

// Per-kind shape searches for system states.
type (
	CircleShapes  = ShapeSearch[component.CircleGeom]
	BoxShapes     = ShapeSearch[component.BoxGeom]
	PolygonShapes = ShapeSearch[component.PolygonGeom]
	ChainShapes   = ShapeSearch[component.ChainGeom]
	EdgeShapes    = ShapeSearch[component.EdgeGeom]
	CapsuleShapes = ShapeSearch[component.CapsuleGeom]
)

// Create spawns def as a new shape entity and returns a slot referencing it at the body
// origin.
func (s *ShapeSearch[G]) Create(def ShapeDef[G]) ShapeSlot {
	id, row := s.exactOf.Create()
	row.Common.Set(def.Common)
	row.Geom.Set(def.Geom)
	return component.Slot(id)
}

// GetByID is Read by entity id (the id a slot carries in Shape). It exists to shadow the
// embedded Cardinal search's method of the same name; prefer Read.
func (s *ShapeSearch[G]) GetByID(id cardinal.EntityID) (ShapeDef[G], bool) {
	row, err := s.exactOf.GetByID(id)
	if err != nil {
		return ShapeDef[G]{}, false
	}
	return ShapeDef[G]{Common: row.Common.Get(), Geom: row.Geom.Get()}, true
}

// Read returns a copy of the definition of the shape behind slot, and false when no shape of
// this kind is behind it.
func (s *ShapeSearch[G]) Read(slot ShapeSlot) (ShapeDef[G], bool) {
	return s.GetByID(slot.Shape)
}

// Fork spawns a copy of the shape behind slot with edit applied, and returns a slot for the
// copy at the same offset and rotation. The shape behind slot is untouched, so bodies still
// using it keep what they had; point the bodies that should change at the returned slot.
// Reports false when no shape of this kind is behind slot.
func (s *ShapeSearch[G]) Fork(slot ShapeSlot, edit func(*ShapeDef[G])) (ShapeSlot, bool) {
	def, ok := s.GetByID(slot.Shape)
	if !ok {
		return ShapeSlot{}, false
	}
	edit(&def)
	return s.Create(def).At(slot.LocalOffset, slot.LocalRotation), true
}

// Clone is Fork with no edit: a copy of the shape behind slot, for a body that should stop
// sharing it before it diverges later.
func (s *ShapeSearch[G]) Clone(slot ShapeSlot) (ShapeSlot, bool) {
	return s.Fork(slot, func(*ShapeDef[G]) {})
}

// noDestroy cannot be named outside this package, which is the point: it makes Destroy below
// impossible to call.
type noDestroy struct{}

// Destroy is not part of the API: the plugin deletes a shape once no body names it. The method
// exists only because Go promotes the embedded search's Destroy, which would delete a shape out
// from under every other body sharing it. Declaring it here shadows that one, and its parameter
// type makes any call from outside this package a compile error.
func (*ShapeSearch[G]) Destroy(noDestroy) {}

// Iter yields every shape entity of this kind with a copy of its definition.
func (s *ShapeSearch[G]) Iter() iter.Seq2[cardinal.EntityID, ShapeDef[G]] {
	return func(yield func(cardinal.EntityID, ShapeDef[G]) bool) {
		for id, row := range s.exactOf.Iter() {
			if !yield(id, ShapeDef[G]{Common: row.Common.Get(), Geom: row.Geom.Get()}) {
				return
			}
		}
	}
}
