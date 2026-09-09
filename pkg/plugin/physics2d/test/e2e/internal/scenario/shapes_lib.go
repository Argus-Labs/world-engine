// Package scenario holds the individual physics2d test scenarios. Each exported
// constructor returns one harness.Scenario; register.go lists them in run order.
//
// Every scenario is written in lane-local coordinates — the harness offsets each
// scenario into its own slice of the world so bodies from different scenarios can
// never collide with, or be found by queries belonging to, another scenario.
package scenario

import (
	"fmt"

	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	physcomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
)

// Filter bits used by scenarios that do not care about collision filtering.
const (
	catAll  = ^uint64(0)
	maskAll = ^uint64(0)
)

// Default material values. Restitution 0 and friction 0.3 keep drops from
// bouncing or sliding unless a scenario deliberately asks for it.
const (
	defaultDensity     = 1.0
	defaultFriction    = 0.3
	defaultRestitution = 0.0
)

// vec is shorthand for a physics vector.
func vec(x, y float64) physics.Vec2 { return physics.Vec2{X: x, Y: y} }

// ShapeSpec is a shape ready to spawn: the plugin's ShapeCommon, where the slot
// sits on the body, and a kind-specific spawner. A shape is an entity in the
// plugin, so nothing is created until Spawn runs inside a system; the helpers
// below build specs, the modifiers adjust them, and body spawns them.
type ShapeSpec struct {
	Common   physcomp.ShapeCommon
	Offset   physics.Vec2
	Rotation float64
	spawn    func(c *harness.Ctx, common physcomp.ShapeCommon) physics.ShapeSlot
}

// Spawn creates the shape entity and returns its slot, placed at the spec's
// offset and rotation.
func (s ShapeSpec) Spawn(c *harness.Ctx) physics.ShapeSlot {
	return s.spawn(c, s.Common).At(s.Offset, s.Rotation)
}

// baseCommon is the default material with an all-layers filter.
func baseCommon() physcomp.ShapeCommon {
	return physcomp.ShapeCommon{
		Density:      defaultDensity,
		Friction:     defaultFriction,
		Restitution:  defaultRestitution,
		CategoryBits: catAll,
		MaskBits:     maskAll,
	}
}

// spec wraps one geometry component as a spawnable spec with the default material.
func spec[G physics.Geometry](geom G) ShapeSpec {
	return ShapeSpec{
		Common: baseCommon(),
		spawn: func(c *harness.Ctx, common physcomp.ShapeCommon) physics.ShapeSlot {
			return harness.Shape(c, physics.ShapeDef[G]{Common: common, Geom: geom})
		},
	}
}

// circle builds a circle collider of the given radius.
func circle(radius float64) ShapeSpec {
	return spec(physics.Circle(radius).Geom)
}

// box builds an axis-aligned box collider from half-extents.
func box(halfWidth, halfHeight float64) ShapeSpec {
	return spec(physics.Box(halfWidth, halfHeight).Geom)
}

// Box is box for callers outside the package.
func Box(halfWidth, halfHeight float64) ShapeSpec { return box(halfWidth, halfHeight) }

// polygon builds a convex polygon collider. Box2D welds and hulls the points, so
// they need not be given in a particular winding order.
func polygon(vertices ...physics.Vec2) ShapeSpec {
	return spec(physics.Polygon(vertices...).Geom)
}

// capsule builds a capsule collider between two local centers.
func capsule(c1, c2 physics.Vec2, radius float64) ShapeSpec {
	return spec(physics.Capsule(c1, c2, radius).Geom)
}

// chain builds an open static chain collider through the given points.
func chain(points ...physics.Vec2) ShapeSpec {
	return spec(physics.Chain(points...).Geom)
}

// chainLoop builds a closed static chain collider; the last point joins the first.
func chainLoop(points ...physics.Vec2) ShapeSpec {
	return spec(physics.ChainLoop(points...).Geom)
}

// edge builds a single static line-segment collider.
func edge(a, b physics.Vec2) ShapeSpec {
	return spec(physics.Edge(a, b).Geom)
}

// -----------------------------------------------------------------------------
// Collider modifiers — chainable so scenario code reads as one expression.
// -----------------------------------------------------------------------------

func withDensity(s ShapeSpec, density float64) ShapeSpec {
	s.Common.Density = density
	return s
}

func withFriction(s ShapeSpec, friction float64) ShapeSpec {
	s.Common.Friction = friction
	return s
}

func withRestitution(s ShapeSpec, restitution float64) ShapeSpec {
	s.Common.Restitution = restitution
	return s
}

func withFilter(s ShapeSpec, category, mask uint64, group int32) ShapeSpec {
	s.Common.CategoryBits = category
	s.Common.MaskBits = mask
	s.Common.GroupIndex = group
	return s
}

func asSensor(s ShapeSpec) ShapeSpec {
	s.Common.IsSensor = true
	return s
}

func atOffset(s ShapeSpec, x, y float64) ShapeSpec {
	s.Offset = vec(x, y)
	return s
}

func rotatedBy(s ShapeSpec, radians float64) ShapeSpec {
	s.Rotation = radians
	return s
}

// -----------------------------------------------------------------------------
// Body builders
// -----------------------------------------------------------------------------

// body spawns every shape as its own entity and builds a PhysicsBody2D through
// the plugin constructor, which is the only way to get Box2D's real defaults
// (Active, Awake, SleepingAllowed true and GravityScale 1). A bare struct
// literal produces a disabled, sleeping, gravity-less body — see the "defaults"
// scenario.
func body(c *harness.Ctx, kind physics.BodyType, shapes ...ShapeSpec) physics.PhysicsBody2D {
	slots := make([]physics.ShapeSlot, len(shapes))
	for i, s := range shapes {
		slots[i] = s.Spawn(c)
	}
	return physcomp.NewPhysicsBody2D(kind, slots...)
}

// Body is body for callers outside the package.
func Body(c *harness.Ctx, kind physics.BodyType, shapes ...ShapeSpec) physics.PhysicsBody2D {
	return body(c, kind, shapes...)
}

// ground returns a static box body wide enough to catch anything dropped on it,
// with its top surface at y=0.
func ground(c *harness.Ctx, halfWidth float64) physics.PhysicsBody2D {
	return body(c, physics.BodyTypeStatic, withFriction(box(halfWidth, 1.0), 0.6))
}

// groundY is the Y a ground(...) body must be spawned at for its top to sit at y=0.
const groundY = -1.0

// ShapeKind names one geometry kind, for tests that enumerate shape kinds rather
// than describe geometry. The plugin has no such tag: a shape entity's kind is
// whichever geometry component it carries.
type ShapeKind uint8

// The seven geometry kinds.
const (
	KindCircle ShapeKind = iota
	KindBox
	KindPolygon
	KindChain
	KindChainLoop
	KindEdge
	KindCapsule
)

// AllShapeKinds lists every kind in a stable order.
func AllShapeKinds() []ShapeKind {
	return []ShapeKind{KindCircle, KindBox, KindPolygon, KindChain, KindChainLoop, KindEdge, KindCapsule}
}

// LineKinds lists the kinds that, per the component docs, belong on static or
// kinematic bodies only.
func LineKinds() []ShapeKind { return []ShapeKind{KindEdge, KindChain, KindChainLoop} }

// IsLine reports whether the kind is one of LineKinds.
func (k ShapeKind) IsLine() bool {
	return k == KindChain || k == KindChainLoop || k == KindEdge
}

func (k ShapeKind) String() string {
	switch k {
	case KindCircle:
		return "circle"
	case KindBox:
		return "box"
	case KindPolygon:
		return "polygon"
	case KindChain:
		return "chain"
	case KindChainLoop:
		return "chainloop"
	case KindEdge:
		return "edge"
	case KindCapsule:
		return "capsule"
	default:
		return fmt.Sprintf("kind(%d)", uint8(k))
	}
}

// SampleShape returns a small, valid collider of the given kind, for tests that
// enumerate shape kinds rather than describe geometry. Chains use four points,
// the engine's minimum.
func SampleShape(kind ShapeKind) ShapeSpec {
	switch kind {
	case KindCircle:
		return circle(0.5)
	case KindBox:
		return box(0.5, 0.5)
	case KindPolygon:
		return polygon(vec(-0.5, -0.5), vec(0.5, -0.5), vec(0, 0.6))
	case KindChain:
		return chain(vec(-1, 0), vec(-0.3, 0.2), vec(0.3, 0.2), vec(1, 0))
	case KindChainLoop:
		return chainLoop(vec(-1, -1), vec(1, -1), vec(1, 1), vec(-1, 1))
	case KindEdge:
		return edge(vec(-1, 0), vec(1, 0))
	case KindCapsule:
		return capsule(vec(0, -0.5), vec(0, 0.5), 0.3)
	default:
		panic(fmt.Sprintf("SampleShape: unknown shape kind %v", kind))
	}
}
