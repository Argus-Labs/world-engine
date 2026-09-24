package component

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/goccy/go-json"
)

// ShapeKind says which geometry a Shape carries.
type ShapeKind uint8

// Shape kinds. Zero is no geometry, which Validate rejects.
const (
	ShapeKindCircle ShapeKind = iota + 1
	ShapeKindBox
	ShapeKindPolygon
	ShapeKindChain
	ShapeKindChainLoop
	ShapeKindEdge
	ShapeKindCapsule
)

// MaxPolygonVertices is Box2D's convex polygon vertex limit, the bound the engine compiles in.
const MaxPolygonVertices = box2d.MaxPolygonVertices

// MinChainPoints is the fewest points Box2D accepts for a chain.
const MinChainPoints = 4

// IsChain reports whether the kind is a chain, open or closed.
func (k ShapeKind) IsChain() bool { return k == ShapeKindChain || k == ShapeKindChainLoop }

func (k ShapeKind) String() string {
	switch k {
	case ShapeKindCircle:
		return "circle"
	case ShapeKindBox:
		return "box"
	case ShapeKindPolygon:
		return "polygon"
	case ShapeKindChain:
		return "chain"
	case ShapeKindChainLoop:
		return "chain loop"
	case ShapeKindEdge:
		return "edge"
	case ShapeKindCapsule:
		return "capsule"
	}
	return "no geometry"
}

// Shape is one collider on a body: its geometry, where it sits on the body, what it is made
// of and what it collides with. It lives in PhysicsBody2D.Shapes and goes wherever the body
// goes: there is no shape to share, keep alive or clean up. Index i in that list is fixture
// i, the index contact events and query hits report.
//
// A constructor (Circle, Box, Polygon, Chain, ChainLoop, Edge or Capsule) or Reshape sets the
// geometry, and getters read it: what its numbers mean depends on the kind. Everything else
// is a plain field, which At, Filter, Group, Sensor and Material also set in one expression.
// A bare struct literal has no geometry and fails Validate.
type Shape struct {
	// Geometry is exported only until the wire generator reads unexported fields; treat it
	// as private. JSON writes its fields inline, see MarshalJSON.
	Geometry ShapeGeometry `json:"-"`

	// Placement in body space; rotation is in radians, CCW.
	LocalOffset   Vec2    `json:"local_offset"`
	LocalRotation float64 `json:"local_rotation"`

	// Material. The constructors set Box2D's defaults: friction 0.6, restitution 0, density 1.
	Friction    float64 `json:"friction"`
	Restitution float64 `json:"restitution"`
	Density     float64 `json:"density"`

	// Collision filter, plain Box2D semantics: two shapes collide when each one's category
	// overlaps the other's mask, so a zero mask collides with nothing. A matching non-zero
	// GroupIndex decides instead: positive always collides, negative never. The constructors
	// set category 1 and mask all.
	CategoryBits uint64 `json:"category_bits"`
	MaskBits     uint64 `json:"mask_bits"`
	GroupIndex   int32  `json:"group_index,omitempty"`

	// IsSensor reports overlaps instead of colliding; chains cannot be sensors. Last, so it
	// packs beside GroupIndex.
	IsSensor bool `json:"is_sensor,omitempty"`
}

// ShapeGeometry is a shape's geometry: one union for every kind, so a circle carries no
// vertex array. Radius: circle, capsule. A: a box's half extents, or an edge's and a
// capsule's first endpoint, with B the second. Points: polygon vertices or chain points.
type ShapeGeometry struct {
	Kind   ShapeKind             `json:"kind"`
	Radius float64               `json:"radius,omitempty"`
	A      Vec2                  `json:"a,omitempty"`
	B      Vec2                  `json:"b,omitempty"`
	Points immutable.Slice[Vec2] `json:"points,omitempty"`
}

// -------------------------------------------------------------------------------------------------
// Constructors
// -------------------------------------------------------------------------------------------------

func newShape(g ShapeGeometry) Shape {
	return Shape{Geometry: g, Friction: 0.6, Density: 1, CategoryBits: 1, MaskBits: ^uint64(0)}
}

// Circle is a circle of radius, centred on the shape's local offset.
func Circle(radius float64) Shape {
	return newShape(ShapeGeometry{Kind: ShapeKindCircle, Radius: radius})
}

// Box is an axis-aligned box with the given half extents, before the local offset and rotation.
func Box(halfWidth, halfHeight float64) Shape {
	return newShape(ShapeGeometry{Kind: ShapeKindBox, A: Vec2{X: halfWidth, Y: halfHeight}})
}

// Polygon is a convex polygon of 3..MaxPolygonVertices vertices (copied) in shape space. Any
// other count, or points with no hull, fails Validate.
func Polygon(vertices ...Vec2) Shape {
	return newShape(ShapeGeometry{Kind: ShapeKindPolygon, Points: immutable.SliceOf(vertices...)})
}

// Chain is an open polyline through points (copied) in shape space, at least MinChainPoints
// of them. Static or kinematic bodies only.
func Chain(points ...Vec2) Shape {
	return newShape(ShapeGeometry{Kind: ShapeKindChain, Points: immutable.SliceOf(points...)})
}

// ChainLoop is Chain closed back to its first point.
func ChainLoop(points ...Vec2) Shape {
	return newShape(ShapeGeometry{Kind: ShapeKindChainLoop, Points: immutable.SliceOf(points...)})
}

// Edge is a single segment from a to b in shape space. Static or kinematic bodies only.
func Edge(a, b Vec2) Shape {
	return newShape(ShapeGeometry{Kind: ShapeKindEdge, A: a, B: b})
}

// Capsule is the segment from a to b inflated by radius.
func Capsule(a, b Vec2, radius float64) Shape {
	return newShape(ShapeGeometry{Kind: ShapeKindCapsule, Radius: radius, A: a, B: b})
}

// -------------------------------------------------------------------------------------------------
// Options: each returns the changed shape
// -------------------------------------------------------------------------------------------------

// At places the shape at offset and rotation (radians, CCW) in body space.
func (s Shape) At(offset Vec2, rotation float64) Shape {
	s.LocalOffset, s.LocalRotation = offset, rotation
	return s
}

// Filter sets the collision category and mask bits.
func (s Shape) Filter(category, mask uint64) Shape {
	s.CategoryBits, s.MaskBits = category, mask
	return s
}

// Group sets the Box2D group index; see the field.
func (s Shape) Group(index int32) Shape {
	s.GroupIndex = index
	return s
}

// Sensor makes the shape report overlaps instead of colliding. Chains cannot be sensors.
func (s Shape) Sensor(on bool) Shape {
	s.IsSensor = on
	return s
}

// Material sets friction, restitution and density.
func (s Shape) Material(friction, restitution, density float64) Shape {
	s.Friction, s.Restitution, s.Density = friction, restitution, density
	return s
}

// Reshape takes the geometry of geometry, a constructor result such as Circle(2), and keeps
// this shape's placement, material, filter and sensor flag.
func (s Shape) Reshape(geometry Shape) Shape {
	s.Geometry = geometry.Geometry
	return s
}

// -------------------------------------------------------------------------------------------------
// Geometry getters: each answers for its own kinds and reads zero or nil for the rest
// -------------------------------------------------------------------------------------------------

// Kind is the geometry kind.
func (s Shape) Kind() ShapeKind { return s.Geometry.Kind }

// Radius is a circle's or capsule's radius.
func (s Shape) Radius() float64 {
	if k := s.Geometry.Kind; k == ShapeKindCircle || k == ShapeKindCapsule {
		return s.Geometry.Radius
	}
	return 0
}

// HalfExtents is a box's half width and half height.
func (s Shape) HalfExtents() Vec2 {
	if s.Geometry.Kind == ShapeKindBox {
		return s.Geometry.A
	}
	return Vec2{}
}

// Endpoints are an edge's or capsule's endpoints in shape space.
func (s Shape) Endpoints() (Vec2, Vec2) {
	if k := s.Geometry.Kind; k == ShapeKindEdge || k == ShapeKindCapsule {
		return s.Geometry.A, s.Geometry.B
	}
	return Vec2{}, Vec2{}
}

// Vertices is a copy of a polygon's vertices in shape space.
func (s Shape) Vertices() []Vec2 {
	if s.Geometry.Kind != ShapeKindPolygon {
		return nil
	}
	return slices.Collect(s.Geometry.Points.Values())
}

// Points is a copy of a chain's points in shape space.
func (s Shape) Points() []Vec2 {
	if !s.Geometry.Kind.IsChain() {
		return nil
	}
	return slices.Collect(s.Geometry.Points.Values())
}

// -------------------------------------------------------------------------------------------------
// Validation
// -------------------------------------------------------------------------------------------------

// Validate reports why Box2D could never build this shape, or nil. Every float is checked
// for NaN/Inf, the unused geometry fields too: the reconciler compares shapes by value, and a
// NaN anywhere would make a shape unequal to itself and rebuild its fixture every tick.
func (s Shape) Validate() error {
	g := &s.Geometry
	if err := validateVec2("local_offset", s.LocalOffset); err != nil {
		return err
	}
	if !isFinite(s.LocalRotation) {
		return errors.New("local_rotation: must be finite")
	}
	for _, f := range [...]struct {
		name string
		v    float64
	}{{"friction", s.Friction}, {"restitution", s.Restitution}, {"density", s.Density}, {"radius", g.Radius}} {
		if !isFinite(f.v) {
			return fmt.Errorf("%s: must be finite, got %v", f.name, f.v)
		}
		if f.v < 0 {
			return fmt.Errorf("%s: must not be negative, got %v", f.name, f.v)
		}
	}
	for _, v := range [...]struct {
		name string
		v    Vec2
	}{{"a", g.A}, {"b", g.B}} {
		if err := validateVec2(v.name, v.v); err != nil {
			return err
		}
	}
	for i, p := range g.Points.All() {
		if !isFinite(p.X) || !isFinite(p.Y) {
			return fmt.Errorf("points[%d]: must be finite (got %v, %v)", i, p.X, p.Y)
		}
	}
	return s.validateGeometry()
}

// validateGeometry checks the fields the Kind uses; finiteness is already established.
func (s Shape) validateGeometry() error {
	g := &s.Geometry
	switch g.Kind {
	case ShapeKindCircle:
		if g.Radius <= 0 {
			return fmt.Errorf("radius: must be positive, got %v", g.Radius)
		}
	case ShapeKindBox:
		if g.A.X <= 0 || g.A.Y <= 0 {
			return fmt.Errorf("a: a box's half extents must be positive, got (%v, %v)", g.A.X, g.A.Y)
		}
	case ShapeKindPolygon:
		n := g.Points.Len()
		if n < 3 || n > MaxPolygonVertices {
			return fmt.Errorf("points: a polygon needs 3..%d vertices, got %d", MaxPolygonVertices, n)
		}
		// The hull Box2D builds at attach: points within its slop weld and near-collinear
		// ones drop, so a flat or crushed polygon would build no fixture.
		var pts [MaxPolygonVertices]box2d.Vec2
		for i, p := range g.Points.All() {
			pts[i] = box2d.Vec2{X: p.X, Y: p.Y}
		}
		if box2d.ComputeHull(pts[:n]).Count < 3 {
			return errors.New("points: no convex hull (points coincide or lie on one line)")
		}
	case ShapeKindChain, ShapeKindChainLoop:
		if n := g.Points.Len(); n < MinChainPoints {
			return fmt.Errorf("points: a chain needs at least %d, got %d", MinChainPoints, n)
		}
		if s.IsSensor {
			// Box2D builds chain segments from one def with no sensor flag: a sensor chain
			// would come out solid and quietly block what it was meant to watch.
			return errors.New("is_sensor: a chain cannot be a sensor")
		}
	case ShapeKindEdge:
		return validateSegment(g.A, g.B)
	case ShapeKindCapsule:
		if g.Radius <= 0 {
			return fmt.Errorf("radius: must be positive, got %v", g.Radius)
		}
		return validateSegment(g.A, g.B)
	default:
		return errors.New("kind: no geometry (build the shape with Circle, Box, Polygon, Chain, Edge or Capsule)")
	}
	return nil
}

// validateSegment checks that two endpoints are far enough apart for Box2D, which refuses any
// segment at or under LinearSlop: a capsule comes back as a null shape id, and an edge trips an
// assertion that kills the process in an asserts build. Either way the fixture is lost, so the
// plugin refuses the shape instead. LinearSlop is read here rather than copied, because
// box2d.SetLengthUnitsPerMeter moves it.
func validateSegment(a, b Vec2) error {
	dx, dy := b.X-a.X, b.Y-a.Y
	if dx*dx+dy*dy <= box2d.LinearSlop*box2d.LinearSlop {
		return fmt.Errorf("a and b: must be more than %v apart, got %v",
			box2d.LinearSlop, math.Sqrt(dx*dx+dy*dy))
	}
	return nil
}

// -------------------------------------------------------------------------------------------------
// Comparison and copying, for the reconciler
// -------------------------------------------------------------------------------------------------

// Equal reports whether every field matches, points included.
func (s Shape) Equal(o Shape) bool {
	return s.StructuralEqual(o) &&
		s.Friction == o.Friction && s.Restitution == o.Restitution && s.Density == o.Density &&
		s.CategoryBits == o.CategoryBits && s.MaskBits == o.MaskBits && s.GroupIndex == o.GroupIndex
}

// StructuralEqual reports whether the two would build the same Box2D fixture: same kind,
// geometry, placement and sensor flag, which Box2D cannot toggle in place. Material and
// filter are not structural; they are applied to a live fixture.
func (s Shape) StructuralEqual(o Shape) bool {
	a, b := &s.Geometry, &o.Geometry
	return a.Kind == b.Kind &&
		s.IsSensor == o.IsSensor &&
		s.LocalOffset == o.LocalOffset && s.LocalRotation == o.LocalRotation &&
		a.Radius == b.Radius && a.A == b.A && a.B == b.B &&
		immutable.Equal(a.Points, b.Points)
}

// Copy returns the shape with its own points array. Slice derivations write through the
// array they derive from, so a copy the game cannot reach is what the reconciler keeps.
func (s Shape) Copy() Shape {
	s.Geometry.Points = immutable.Collect(s.Geometry.Points.Values())
	return s
}

// -------------------------------------------------------------------------------------------------
// JSON: one flat object, the geometry's fields beside the rest
// -------------------------------------------------------------------------------------------------

// shapeFields is Shape without its methods, so encoding it does not call MarshalJSON again.
type shapeFields Shape

// flatShape is a Shape as JSON: embedding lifts the geometry's fields to the top level.
type flatShape struct {
	ShapeGeometry
	shapeFields
}

// MarshalJSON encodes the shape as one flat object.
func (s Shape) MarshalJSON() ([]byte, error) {
	return json.Marshal(flatShape{ShapeGeometry: s.Geometry, shapeFields: shapeFields(s)})
}

// UnmarshalJSON decodes a shape, filling a missing material or filter with the constructors'
// defaults so a hand-written {"kind": 1, "radius": 0.5} is a normal solid circle. An explicit
// zero is kept.
func (s *Shape) UnmarshalJSON(data []byte) error {
	aux := flatShape{shapeFields: shapeFields(newShape(ShapeGeometry{}))}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*s = Shape(aux.shapeFields)
	s.Geometry = aux.ShapeGeometry
	return nil
}
