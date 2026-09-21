package physics2d

import (
	"errors"
	"fmt"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	physicevent "github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
	physicsquery "github.com/argus-labs/world-engine/pkg/plugin/physics2d/query"
)

// -------------------------------------------------------------------------------------------------
// Bodies: the components an entity carries
// -------------------------------------------------------------------------------------------------
//
// An entity is simulated while it has all three. Build the body with NewPhysicsBody2D so the
// Box2D defaults (active, awake, gravity scale 1) are set; a bare literal is an inactive body.
//
//	row.Set(physics2d.Transform2D{Position: physics2d.Vec2{X: 1, Y: 10}})
//	row.Set(physics2d.Velocity2D{})
//	row.Set(physics2d.NewPhysicsBody2D(physics2d.BodyTypeDynamic, ball))
//
// A body's Shapes are ShapeRefs: which shape entity, where it sits on the body, and its
// collision filter (ref.Filter, ref.Group), which is per body so one shape serves every team.
// Tag them and edit by name (AddShape, ReplaceShape, RemoveShape) instead of by index.

// Components an entity needs to be simulated.
type (
	Transform2D   = component.Transform2D
	Velocity2D    = component.Velocity2D
	PhysicsBody2D = component.PhysicsBody2D
)

// Value types the components are built from.
type (
	Vec2     = component.Vec2
	BodyType = component.BodyType
	ShapeRef = component.ShapeRef
)

// Body kinds.
const (
	BodyTypeStatic    = component.BodyTypeStatic
	BodyTypeDynamic   = component.BodyTypeDynamic
	BodyTypeKinematic = component.BodyTypeKinematic
	BodyTypeManual    = component.BodyTypeManual
)

// NewPhysicsBody2D returns a PhysicsBody2D with Box2D-compatible defaults and the given shapes.
func NewPhysicsBody2D(bodyType BodyType, shapes ...ShapeRef) PhysicsBody2D {
	return component.NewPhysicsBody2D(bodyType, shapes...)
}

// -------------------------------------------------------------------------------------------------
// Shapes: describing one
// -------------------------------------------------------------------------------------------------
//
// A Shape is a plain value: one geometry plus material. Build it with a constructor and
// chain options onto it; nothing exists in the world until Shapes.Spawn. Collision filters
// are not part of a shape: they go on the ShapeRef a body holds.
//
//	ball := physics2d.Circle(0.5).Material(0.3, 0.1, 1)

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

// MaxPolygonVertices is Box2D's convex polygon vertex limit.
const MaxPolygonVertices = component.MaxPolygonVertices

// Shape is a shape's definition: geometry and material. Constructors carry Box2D's default
// material (solid, friction 0.6, restitution 0, density 1). The zero Shape has no geometry
// and fails Validate.
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

// Sensor sets whether the shape reports overlaps without ever colliding. Shapes are solid
// unless set.
func (d Shape) Sensor(on bool) Shape {
	d.s.Common.IsSensor = on
	return d
}

// Material sets friction, restitution and density.
func (d Shape) Material(friction, restitution, density float64) Shape {
	d.s.Common.Friction, d.s.Common.Restitution, d.s.Common.Density = friction, restitution, density
	return d
}

// Reshape returns geometry's shape carrying d's material. It is how a Fork
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
	if err := d.s.ValidateGeometry(); err != nil {
		return fmt.Errorf("physics2d: %s: %w", d.s.Kind, err)
	}
	return nil
}

// Kind reports the geometry kind, which says which of the geometry readers below apply.
func (d Shape) Kind() Kind { return d.s.Kind }

// Material, as the shape holds it.
func (d Shape) IsSensor() bool       { return d.s.Common.IsSensor }
func (d Shape) Friction() float64    { return d.s.Common.Friction }
func (d Shape) Restitution() float64 { return d.s.Common.Restitution }
func (d Shape) Density() float64     { return d.s.Common.Density }

// Geometry readers. Each applies to the kinds it names and returns zero for any other.

// Radius is a circle's or capsule's radius.
func (d Shape) Radius() float64 {
	if d.s.Kind == KindCapsule {
		return d.s.Capsule.Radius
	}
	return d.s.Circle.Radius // zero unless a circle
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
	if d.s.Kind == KindCapsule {
		return d.s.Capsule.A, d.s.Capsule.B
	}
	return d.s.Edge.A, d.s.Edge.B // zero unless an edge
}

// -------------------------------------------------------------------------------------------------
// Shapes: spawning and editing them
// -------------------------------------------------------------------------------------------------
//
// A spawned shape is an entity that any number of bodies share by ShapeRef. Spawn de-duplicates:
// an equal definition gets a ref to the live shape that already matches, so bodies built from
// the same definition share one entity without passing refs around, whatever their filters. Declare one Shapes field
// on the system state; Cardinal wires it when the system registers.
//
//	type SpawnState struct {
//	    cardinal.BaseSystemState
//	    Shapes physics2d.Shapes
//	}
//
//	ball, err := state.Shapes.Spawn(physics2d.Circle(0.5))
//	body, err := physics2d.NewPhysicsBody2D(physics2d.BodyTypeDynamic).AddShape("hull", ball)
//
// A shape lives exactly as long as some body names it, so spawn it in the same tick as its
// first body and keep Shape values, not refs, for shapes you will need later. A shape is never
// edited in place: Fork one to get a changed copy, and point the bodies that should change at
// it. There is no delete; the plugin removes a shape once no body names it.

// shapeSearch is the Cardinal search behind Shapes: every entity carrying ShapeCommon. It is
// embedded under this unexported name so the raw search is unreachable from other packages,
// and the sealed methods below shadow what it would promote.
type shapeSearch = cardinal.Contains[struct {
	Common cardinal.WithComponent[component.ShapeCommon]
}]

// Shapes is the API over shape entities, declared as a field on a system state.
type Shapes struct {
	shapeSearch
}

// Spawn returns a ref, at the body origin, to a shape entity matching def: the live one with
// exactly the same geometry and material when there is one, else a new one. Chain At
// on the ref to place it. A definition that fails validation creates nothing and reports why.
func (s *Shapes) Spawn(def Shape) (ShapeRef, error) {
	if err := def.Validate(); err != nil {
		return ShapeRef{}, err
	}
	if ref, ok := s.find(def); ok {
		return ref, nil
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

// find returns a ref to a live shape equal to def. Material is compared first, since it is
// one component read and rules out most shapes.
func (s *Shapes) find(def Shape) (ShapeRef, bool) {
	for row := range s.shapeSearch.Iter() {
		if row.Get[component.ShapeCommon]() != def.s.Common {
			continue
		}
		ref := component.Ref(row.ID())
		if got, ok := s.Read(ref); ok && got.s.Equal(def.s) {
			return ref, true
		}
	}
	return ShapeRef{}, false
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

// Fork spawns a copy of the shape behind ref with edit applied, and returns a ref to the copy
// at the same offset and rotation. The original is untouched, so bodies still on it keep what
// they had. It fails when no shape is behind ref or the edited shape does not validate.
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

// -------------------------------------------------------------------------------------------------
// Queries
// -------------------------------------------------------------------------------------------------
//
// All three are methods on the *Plugin and return an empty result while no world exists
// (before the first tick, or right after Reset). A nil Filter matches every category and
// skips sensors.

// Query requests and results.
type (
	Filter             = physicsquery.Filter
	RaycastRequest     = physicsquery.RaycastRequest
	RaycastResult      = physicsquery.RaycastResult
	AABBOverlapRequest = physicsquery.AABBOverlapRequest
	AABBOverlapHit     = physicsquery.AABBOverlapHit
	AABBOverlapResult  = physicsquery.AABBOverlapResult
	CircleSweepRequest = physicsquery.CircleSweepRequest
	CircleSweepResult  = physicsquery.CircleSweepResult
)

// Raycast casts the segment from req.Origin to req.End and returns the closest hit. A
// zero-length segment returns Hit=false.
func (p *Plugin) Raycast(req RaycastRequest) RaycastResult {
	if p.rt == nil || !p.rt.WorldExists() {
		return RaycastResult{}
	}
	return p.rt.Raycast(req)
}

// OverlapAABB returns the distinct (entity, shape index) pairs whose shapes overlap the
// world-space box.
func (p *Plugin) OverlapAABB(req AABBOverlapRequest) AABBOverlapResult {
	if p.rt == nil || !p.rt.WorldExists() {
		return AABBOverlapResult{}
	}
	return p.rt.OverlapAABB(req)
}

// CircleSweep sweeps a circle from req.Start to req.End and returns the earliest hit.
func (p *Plugin) CircleSweep(req CircleSweepRequest) CircleSweepResult {
	if p.rt == nil || !p.rt.WorldExists() {
		return CircleSweepResult{}
	}
	return p.rt.CircleSweep(req)
}

// -------------------------------------------------------------------------------------------------
// Contact events
// -------------------------------------------------------------------------------------------------
//
// The plugin emits these on Cardinal's system-event bus each tick; receive them with a
// cardinal.WithSystemEventReceiver field. Each carries both entities and both shape indices;
// PhysicsBody2D.ShapeTag turns an index into the tag you gave the shape.

// Contact and trigger events, and the payload they share.
type (
	ContactEventPayload = physicevent.ContactEventPayload
	FixtureFilterBits   = physicevent.FixtureFilterBits
	ContactBeginEvent   = physicevent.ContactBeginEvent
	ContactEndEvent     = physicevent.ContactEndEvent
	TriggerBeginEvent   = physicevent.TriggerBeginEvent
	TriggerEndEvent     = physicevent.TriggerEndEvent
)
