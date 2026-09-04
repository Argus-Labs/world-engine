// Package scenario holds the individual physics2d test scenarios. Each exported
// constructor returns one harness.Scenario; register.go lists them in run order.
//
// Every scenario is written in lane-local coordinates — the harness offsets each
// scenario into its own slice of the world so bodies from different scenarios can
// never collide with, or be found by queries belonging to, another scenario.
package scenario

import (
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	physcomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
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

// base returns a collider with the default material and an all-layers filter.
func base(shapeType physics.ShapeType) physics.ColliderShape {
	return physics.ColliderShape{
		ShapeType:    shapeType,
		Density:      defaultDensity,
		Friction:     defaultFriction,
		Restitution:  defaultRestitution,
		CategoryBits: catAll,
		MaskBits:     maskAll,
	}
}

// circle builds a circle collider of the given radius.
func circle(radius float64) physics.ColliderShape {
	s := base(physics.ShapeTypeCircle)
	s.Radius = radius
	return s
}

// box builds an axis-aligned box collider from half-extents.
func box(halfWidth, halfHeight float64) physics.ColliderShape {
	s := base(physics.ShapeTypeBox)
	s.HalfExtents = vec(halfWidth, halfHeight)
	return s
}

// polygon builds a convex polygon collider. Box2D welds and hulls the points, so
// they need not be given in a particular winding order.
func polygon(vertices ...physics.Vec2) physics.ColliderShape {
	s := base(physics.ShapeTypeConvexPolygon)
	s.Vertices = vertices
	return s
}

// capsule builds a capsule collider between two local centers.
func capsule(c1, c2 physics.Vec2, radius float64) physics.ColliderShape {
	s := base(physics.ShapeTypeCapsule)
	s.CapsuleCenter1 = c1
	s.CapsuleCenter2 = c2
	s.Radius = radius
	return s
}

// chain builds an open static chain collider through the given points.
func chain(points ...physics.Vec2) physics.ColliderShape {
	s := base(physics.ShapeTypeStaticChain)
	s.ChainPoints = points
	return s
}

// chainLoop builds a closed static chain collider; the last point joins the first.
func chainLoop(points ...physics.Vec2) physics.ColliderShape {
	s := base(physics.ShapeTypeStaticChainLoop)
	s.ChainPoints = points
	return s
}

// edge builds a single static line-segment collider.
func edge(a, b physics.Vec2) physics.ColliderShape {
	s := base(physics.ShapeTypeEdge)
	s.EdgeVertices = [2]physics.Vec2{a, b}
	return s
}

// -----------------------------------------------------------------------------
// Collider modifiers — chainable so scenario code reads as one expression.
// -----------------------------------------------------------------------------

func withDensity(s physics.ColliderShape, density float64) physics.ColliderShape {
	s.Density = density
	return s
}

func withFriction(s physics.ColliderShape, friction float64) physics.ColliderShape {
	s.Friction = friction
	return s
}

func withRestitution(s physics.ColliderShape, restitution float64) physics.ColliderShape {
	s.Restitution = restitution
	return s
}

func withFilter(s physics.ColliderShape, category, mask uint64, group int32) physics.ColliderShape {
	s.CategoryBits = category
	s.MaskBits = mask
	s.GroupIndex = group
	return s
}

func asSensor(s physics.ColliderShape) physics.ColliderShape {
	s.IsSensor = true
	return s
}

func atOffset(s physics.ColliderShape, x, y float64) physics.ColliderShape {
	s.LocalOffset = vec(x, y)
	return s
}

func rotatedBy(s physics.ColliderShape, radians float64) physics.ColliderShape {
	s.LocalRotation = radians
	return s
}

// -----------------------------------------------------------------------------
// Body builders
// -----------------------------------------------------------------------------

// body builds a PhysicsBody2D through the plugin constructor, which is the only
// way to get Box2D's real defaults (Active, Awake, SleepingAllowed true and
// GravityScale 1). A bare struct literal produces a disabled, sleeping,
// gravity-less body — see the "defaults" scenario.
func body(kind physics.BodyType, shapes ...physics.ColliderShape) physics.PhysicsBody2D {
	return physcomp.NewPhysicsBody2D(kind, shapes...)
}

// ground returns a static box body wide enough to catch anything dropped on it,
// with its top surface at y=0.
func ground(halfWidth float64) physics.PhysicsBody2D {
	return body(physics.BodyTypeStatic, withFriction(box(halfWidth, 1.0), 0.6))
}

// groundY is the Y a ground(...) body must be spawned at for its top to sit at y=0.
const groundY = -1.0
