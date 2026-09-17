package physics2d

import (
	"fmt"
	"iter"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
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
//	    ball, err := physics2d.Circle(0.5).Material(0.3, 0.1, 1).Spawn(&state.Circles)
//	    if err != nil {
//	        return // the definition is unusable; nothing was created
//	    }
//	    row := state.Balls.Create()
//	    row.Set(physics2d.NewPhysicsBody2D(physics2d.BodyTypeDynamic, ball))
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
	common component.ShapeCommon
	geom   G
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
	return ShapeDef[G]{common: component.DefaultShapeCommon(), geom: geom}
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
	g.Count = uint8(min(len(vertices), 255))
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
	d.common.IsSensor = true
	return d
}

// Material sets friction, restitution and density.
func (d ShapeDef[G]) Material(friction, restitution, density float64) ShapeDef[G] {
	d.common.Friction, d.common.Restitution, d.common.Density = friction, restitution, density
	return d
}

// Filter sets the collision category and mask bits.
func (d ShapeDef[G]) Filter(category, mask uint64) ShapeDef[G] {
	d.common.CategoryBits, d.common.MaskBits = category, mask
	return d
}

// Group sets the Box2D group index: shapes sharing a positive index always collide, a
// negative one never.
func (d ShapeDef[G]) Group(index int32) ShapeDef[G] {
	d.common.GroupIndex = index
	return d
}

// Spawn validates the definition, creates the shape entity through shapes, and returns a slot
// referencing it at the body origin. Chain At on the slot to place it.
//
// A definition that fails validation creates nothing and reports the reason. Checking here is
// what keeps a shape entity from existing in a state no body could ever attach: the reconciler
// would otherwise reject that body once per tick, with nothing left to point at the line that
// built it.
func (d ShapeDef[G]) Spawn(shapes *ShapeSearch[G]) (ShapeSlot, error) {
	if err := d.Validate(); err != nil {
		return ShapeSlot{}, err
	}
	return shapes.Create(d), nil
}

// Validate reports why Box2D could never build this definition, or nil. Spawn runs it first.
func (d ShapeDef[G]) Validate() error {
	if err := d.common.Validate(); err != nil {
		return fmt.Errorf("physics2d: shape material: %w", err)
	}
	if err := d.geom.Validate(); err != nil {
		return fmt.Errorf("physics2d: %s: %w", d.geom.Name(), err)
	}
	return nil
}

// Material and filter, as the definition holds them.
func (d ShapeDef[G]) IsSensor() bool       { return d.common.IsSensor }
func (d ShapeDef[G]) Friction() float64    { return d.common.Friction }
func (d ShapeDef[G]) Restitution() float64 { return d.common.Restitution }
func (d ShapeDef[G]) Density() float64     { return d.common.Density }
func (d ShapeDef[G]) Category() uint64     { return d.common.CategoryBits }
func (d ShapeDef[G]) Mask() uint64         { return d.common.MaskBits }
func (d ShapeDef[G]) GroupIndex() int32    { return d.common.GroupIndex }

// CopyMaterial returns to carrying from's material and filter. It is how a Fork changes
// geometry: build the new shape with a constructor, then keep the old material.
func CopyMaterial[G, H Geometry](from ShapeDef[G], to ShapeDef[H]) ShapeDef[H] {
	to.common = from.common
	return to
}

// Geometry, per kind. Definitions are opaque so a game never holds a shape component; these
// read the numbers back out.
func RadiusOf(d CircleDef) float64 { return d.geom.Radius }
func HalfExtentsOf(d BoxDef) Vec2  { return d.geom.HalfExtents }
func VerticesOf(d PolygonDef) []Vec2 {
	return slices.Clone(d.geom.Vertices[:min(int(d.geom.Count), MaxPolygonVertices)])
}
func PointsOf(d ChainDef) []Vec2 { return slices.Collect(d.geom.Points.Values()) }
func IsLoop(d ChainDef) bool     { return d.geom.Loop }

// EndpointsOf returns an edge's endpoints, A then B.
func EndpointsOf(d EdgeDef) (Vec2, Vec2) { return d.geom.A, d.geom.B }

// CapsuleOf returns a capsule's endpoints, A then B, and its radius.
func CapsuleOf(d CapsuleDef) (Vec2, Vec2, float64) {
	return d.geom.A, d.geom.B, d.geom.Radius
}

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
	row := s.exactOf.Create()
	row.Set(def.common)
	row.Set(def.geom)
	return component.Slot(row.ID())
}

// GetByID is Read by entity id (the id a slot carries in Shape). It exists to shadow the
// embedded Cardinal search's method of the same name; prefer Read.
func (s *ShapeSearch[G]) GetByID(id cardinal.EntityID) (ShapeDef[G], bool) {
	row, err := s.exactOf.GetByID(id)
	if err != nil {
		return ShapeDef[G]{}, false
	}
	return ShapeDef[G]{common: row.Get[component.ShapeCommon](), geom: row.Get[G]()}, true
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
func (s *ShapeSearch[G]) Fork(slot ShapeSlot, edit func(ShapeDef[G]) ShapeDef[G]) (ShapeSlot, bool) {
	def, ok := s.GetByID(slot.Shape)
	if !ok {
		return ShapeSlot{}, false
	}
	def = edit(def)
	return s.Create(def).At(slot.LocalOffset, slot.LocalRotation), true
}

// Clone is Fork with no edit: a copy of the shape behind slot, for a body that should stop
// sharing it before it diverges later.
func (s *ShapeSearch[G]) Clone(slot ShapeSlot) (ShapeSlot, bool) {
	return s.Fork(slot, func(d ShapeDef[G]) ShapeDef[G] { return d })
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
		for row := range s.exactOf.Iter() {
			if !yield(row.ID(), ShapeDef[G]{common: row.Get[component.ShapeCommon](), geom: row.Get[G]()}) {
				return
			}
		}
	}
}
