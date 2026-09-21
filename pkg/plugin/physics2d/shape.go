package physics2d

import (
	"errors"
	"fmt"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
)

// A shape is an entity: a material/filter component plus exactly one geometry component.
// Bodies reference it from a ShapeRef, so any number of bodies share one shape.
//
// Games never touch those components. This package does not export them, and the Shapes
// search hands out and accepts plain Shape values instead of entity handles. Declare one
// Shapes field on the system state, then spawn, read or fork through it:
//
//	type SpawnState struct {
//	    cardinal.BaseSystemState
//	    Shapes physics2d.Shapes
//	    Balls  cardinal.Exact[ballRow]
//	}
//
//	func Spawn(state *SpawnState) {
//	    ball, err := state.Shapes.Spawn(physics2d.Circle(0.5).Material(0.3, 0.1, 1))
//	    if err != nil {
//	        return // the definition is unusable; nothing was created
//	    }
//	    row := state.Balls.Create()
//	    row.Set(physics2d.NewPhysicsBody2D(physics2d.BodyTypeDynamic, ball))
//	}
//
// Spawn returns a ShapeRef; put it on more bodies to share the shape. A shape lives exactly
// as long as some body names it, so spawn it in the same tick as its first body and keep
// Shape values, not refs, for shapes you will need later.
//
// A shape is never edited in place: to change one, Fork it (a copy with your edit applied)
// and point the body at the result. That can only ever affect the bodies you re-point, so a
// shape shared by other bodies stays as it was. To change many bodies, Fork once and re-point
// each of them at the same new ref.

// Kind is a shape's geometry kind.
type Kind = internal.ShapeKind

// The six geometry kinds.
const (
	KindCircle  = internal.ShapeKindCircle
	KindBox     = internal.ShapeKindBox
	KindPolygon = internal.ShapeKindPolygon
	KindChain   = internal.ShapeKindChain
	KindEdge    = internal.ShapeKindEdge
	KindCapsule = internal.ShapeKindCapsule
)

// Shape is a shape's definition: geometry, material and filter. It is a plain value with no
// tie to any entity; constructors carry Box2D's default material (solid, friction 0.6,
// restitution 0, density 1, category 1, mask all). The zero Shape has no geometry and
// fails Validate.
type Shape struct {
	s internal.ResolvedShape
}

func newShape[G internal.Geometry](geom G) Shape {
	return Shape{s: internal.Resolve(component.DefaultShapeCommon(), geom)}
}

// Circle is a circle of radius, centred on the ref's local offset.
func Circle(radius float64) Shape {
	return newShape(component.CircleGeom{Radius: radius})
}

// Box is an axis-aligned box with the given half extents, before the ref's offset and rotation.
func Box(halfWidth, halfHeight float64) Shape {
	return newShape(component.BoxGeom{HalfExtents: Vec2{X: halfWidth, Y: halfHeight}})
}

// Polygon is a convex polygon of 3..MaxPolygonVertices vertices in shape space. More vertices
// than that fail Validate, and so Spawn.
func Polygon(vertices ...Vec2) Shape {
	var g component.PolygonGeom
	g.Count = uint8(min(len(vertices), 255))
	copy(g.Vertices[:], vertices)
	return newShape(g)
}

// Chain is an open polyline through points (copied) in shape space. Static or kinematic
// bodies only.
func Chain(points ...Vec2) Shape {
	return newShape(component.ChainGeom{Points: immutable.SliceOf(points...)})
}

// ChainLoop is a closed polyline through points (copied) in shape space. Static or kinematic
// bodies only.
func ChainLoop(points ...Vec2) Shape {
	return newShape(component.ChainGeom{Points: immutable.SliceOf(points...), Loop: true})
}

// Edge is a single segment from a to b in shape space.
func Edge(a, b Vec2) Shape {
	return newShape(component.EdgeGeom{A: a, B: b})
}

// Capsule is the segment from a to b inflated by radius.
func Capsule(a, b Vec2, radius float64) Shape {
	return newShape(component.CapsuleGeom{A: a, B: b, Radius: radius})
}

// AsSensor makes the shape report overlaps without ever colliding.
func (d Shape) AsSensor() Shape {
	d.s.Common.IsSensor = true
	return d
}

// Material sets friction, restitution and density.
func (d Shape) Material(friction, restitution, density float64) Shape {
	d.s.Common.Friction, d.s.Common.Restitution, d.s.Common.Density = friction, restitution, density
	return d
}

// Filter sets the collision category and mask bits.
func (d Shape) Filter(category, mask uint64) Shape {
	d.s.Common.CategoryBits, d.s.Common.MaskBits = category, mask
	return d
}

// Group sets the Box2D group index: shapes sharing a positive index always collide, a
// negative one never.
func (d Shape) Group(index int32) Shape {
	d.s.Common.GroupIndex = index
	return d
}

// Reshape returns geometry's shape carrying d's material and filter. It is how a Fork
// changes geometry: build the new geometry with a constructor and keep the old material.
func (d Shape) Reshape(geometry Shape) Shape {
	geometry.s.Common = d.s.Common
	return geometry
}

// Validate reports why Box2D could never build this shape, or nil. Spawn runs it first.
func (d Shape) Validate() error {
	if d.s.Kind == 0 {
		return errors.New("physics2d: shape has no geometry")
	}
	if err := d.s.Common.Validate(); err != nil {
		return fmt.Errorf("physics2d: shape material: %w", err)
	}
	if err := d.s.Validate(); err != nil {
		return fmt.Errorf("physics2d: %s: %w", d.s.Kind, err)
	}
	return nil
}

// Kind reports the geometry kind, which says which of the geometry readers below apply.
func (d Shape) Kind() Kind { return d.s.Kind }

// Material and filter, as the shape holds them.
func (d Shape) IsSensor() bool       { return d.s.Common.IsSensor }
func (d Shape) Friction() float64    { return d.s.Common.Friction }
func (d Shape) Restitution() float64 { return d.s.Common.Restitution }
func (d Shape) Density() float64     { return d.s.Common.Density }
func (d Shape) Category() uint64     { return d.s.Common.CategoryBits }
func (d Shape) Mask() uint64         { return d.s.Common.MaskBits }
func (d Shape) GroupIndex() int32    { return d.s.Common.GroupIndex }

// Geometry readers. Each applies to the kinds it names and returns zero for any other.

// Radius is a circle's or capsule's radius.
func (d Shape) Radius() float64 {
	switch d.s.Kind {
	case KindCircle:
		return d.s.Circle.Radius
	case KindCapsule:
		return d.s.Capsule.Radius
	}
	return 0
}

// HalfExtents is a box's half width and half height.
func (d Shape) HalfExtents() Vec2 { return d.s.Box.HalfExtents }

// Vertices is a copy of a polygon's vertices.
func (d Shape) Vertices() []Vec2 {
	if d.s.Kind != KindPolygon {
		return nil
	}
	return slices.Clone(d.s.Polygon.Vertices[:min(int(d.s.Polygon.Count), MaxPolygonVertices)])
}

// Points is a copy of a chain's points.
func (d Shape) Points() []Vec2 { return slices.Collect(d.s.Chain.Points.Values()) }

// Loop reports whether a chain closes back to its first point.
func (d Shape) Loop() bool { return d.s.Chain.Loop }

// Endpoints returns an edge's or capsule's endpoints, A then B.
func (d Shape) Endpoints() (Vec2, Vec2) {
	switch d.s.Kind {
	case KindEdge:
		return d.s.Edge.A, d.s.Edge.B
	case KindCapsule:
		return d.s.Capsule.A, d.s.Capsule.B
	}
	return Vec2{}, Vec2{}
}

// shapeSearch is the Cardinal search behind Shapes: every entity carrying ShapeCommon,
// whatever its geometry. It is embedded under this unexported name so the raw search is not
// reachable from another package, and the sealed methods below shadow every exported method
// it would otherwise promote. Nothing here hands out an entity handle, and nothing here
// deletes: the plugin removes a shape itself once no body names it.
type shapeSearch = cardinal.Contains[struct {
	Common cardinal.WithComponent[component.ShapeCommon]
}]

// Shapes is the API over shape entities. Declare one on a system state; Cardinal wires it
// up when the system registers.
//
// A shape is shared by every body whose ref names it, so nothing here changes a shape in
// place: Fork hands back a new shape, and only the bodies you point at it change.
type Shapes struct {
	shapeSearch
}

// Spawn validates def, creates a shape entity from it, and returns a ref to that entity at
// the body origin. Chain At on the ref to place it.
//
// A definition that fails validation creates nothing and reports the reason. Checking here is
// what keeps a shape entity from existing in a state no body could ever attach: the reconciler
// would otherwise reject that body once per tick, with nothing left to point at the line that
// built it.
func (s *Shapes) Spawn(def Shape) (ShapeRef, error) {
	if err := def.Validate(); err != nil {
		return ShapeRef{}, err
	}
	row := s.shapeSearch.Create()
	row.Set(def.s.Common)
	switch def.s.Kind {
	case KindCircle:
		row.Set(def.s.Circle)
	case KindBox:
		row.Set(def.s.Box)
	case KindPolygon:
		row.Set(def.s.Polygon)
	case KindChain:
		row.Set(def.s.Chain)
	case KindEdge:
		row.Set(def.s.Edge)
	case KindCapsule:
		row.Set(def.s.Capsule)
	}
	return component.Ref(row.ID()), nil
}

// Read returns a copy of the shape behind ref, and false when no shape is behind it.
func (s *Shapes) Read(ref ShapeRef) (Shape, bool) {
	row, err := s.shapeSearch.GetByID(ref.Shape)
	if err != nil {
		return Shape{}, false
	}
	common := row.Get[component.ShapeCommon]()
	switch {
	case row.Has[component.CircleGeom]():
		return Shape{s: internal.Resolve(common, row.Get[component.CircleGeom]())}, true
	case row.Has[component.BoxGeom]():
		return Shape{s: internal.Resolve(common, row.Get[component.BoxGeom]())}, true
	case row.Has[component.PolygonGeom]():
		return Shape{s: internal.Resolve(common, row.Get[component.PolygonGeom]())}, true
	case row.Has[component.ChainGeom]():
		return Shape{s: internal.Resolve(common, row.Get[component.ChainGeom]())}, true
	case row.Has[component.EdgeGeom]():
		return Shape{s: internal.Resolve(common, row.Get[component.EdgeGeom]())}, true
	case row.Has[component.CapsuleGeom]():
		return Shape{s: internal.Resolve(common, row.Get[component.CapsuleGeom]())}, true
	}
	return Shape{}, false
}

// Fork spawns a copy of the shape behind ref with edit applied, and returns a ref to the
// copy at the same offset and rotation. The shape behind ref is untouched, so bodies still
// using it keep what they had; point the bodies that should change at the returned ref.
// It fails when no shape is behind ref or the edited shape does not validate.
func (s *Shapes) Fork(ref ShapeRef, edit func(Shape) Shape) (ShapeRef, error) {
	def, ok := s.Read(ref)
	if !ok {
		return ShapeRef{}, fmt.Errorf("physics2d: no shape behind ref (entity %d)", ref.Shape)
	}
	forked, err := s.Spawn(edit(def))
	if err != nil {
		return ShapeRef{}, err
	}
	return forked.At(ref.LocalOffset, ref.LocalRotation), nil
}

// sealed cannot be named outside this package, so the methods taking it cannot be called.
type sealed struct{}

// Not part of the API. Go promotes the embedded search's Create, GetByID and Iter, each of
// which hands out an entity handle; these shadow them with an uncallable signature.
func (*Shapes) Create(sealed)  {}
func (*Shapes) GetByID(sealed) {}
func (*Shapes) Iter(sealed)    {}
