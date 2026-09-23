package physics2d

import (
	physicevent "github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
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
// A body's Shapes are Shape values: geometry, placement on the body, material and collision
// filter, all in one. They belong to the body. Index i is fixture i, the index contact events
// and query hits report.

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
)

// Body kinds.
const (
	BodyTypeStatic    = component.BodyTypeStatic
	BodyTypeDynamic   = component.BodyTypeDynamic
	BodyTypeKinematic = component.BodyTypeKinematic
	BodyTypeManual    = component.BodyTypeManual
)

// NewPhysicsBody2D returns a PhysicsBody2D with Box2D-compatible defaults and the given shapes.
func NewPhysicsBody2D(bodyType BodyType, shapes ...Shape) PhysicsBody2D {
	return component.NewPhysicsBody2D(bodyType, shapes...)
}

// -------------------------------------------------------------------------------------------------
// Shapes
// -------------------------------------------------------------------------------------------------
//
// A Shape is a plain value the body carries: one geometry, where it sits on the body, its
// material and its collision filter. Build it with a constructor, chain options onto it, and
// put it in NewPhysicsBody2D or append it to a body's Shapes. It belongs to that body: nothing
// is shared between bodies, and there is nothing to keep alive or clean up.
//
//	ball := physics2d.Circle(0.5).Material(0.3, 0.1, 1).Filter(0x0001, 0xFFFF)
//	row.Set(physics2d.NewPhysicsBody2D(physics2d.BodyTypeDynamic, ball))
//
// Placement, material and filter are plain fields, so ball.Friction = 0.9 works as well as
// the options. The geometry is not: what its numbers mean depends on the kind. Reshape swaps
// it, and the getters Kind, Radius, HalfExtents, Endpoints, Vertices and Points read it. Each
// getter reads zero or nil on a kind it does not belong to.

// Shape is one collider on a body. Its Geometry field is exported only until the wire
// generator reads unexported fields: use the constructors, Reshape and the getters instead.
type Shape = component.Shape

// ShapeKind is a shape's geometry kind.
type ShapeKind = component.ShapeKind

// The geometry kinds.
const (
	ShapeKindCircle    = component.ShapeKindCircle
	ShapeKindBox       = component.ShapeKindBox
	ShapeKindPolygon   = component.ShapeKindPolygon
	ShapeKindChain     = component.ShapeKindChain
	ShapeKindChainLoop = component.ShapeKindChainLoop
	ShapeKindEdge      = component.ShapeKindEdge
	ShapeKindCapsule   = component.ShapeKindCapsule
)

// MaxPolygonVertices is Box2D's convex polygon vertex limit.
const MaxPolygonVertices = component.MaxPolygonVertices

// MinChainPoints is Box2D's chain minimum required points for a valid chain.
const MinChainPoints = component.MinChainPoints

// Constructors. Each carries Box2D's default material (solid, friction 0.6, restitution 0,
// density 1) and filter (category 1, mask all); chain At, Filter, Group, Sensor and Material
// onto the result. A shape Box2D could never build fails Validate, and with it the body.

// Circle is a circle of radius, centred on the shape's local offset.
func Circle(radius float64) Shape { return component.Circle(radius) }

// Box is an axis-aligned box with the given half extents, before the local offset and rotation.
func Box(halfWidth, halfHeight float64) Shape { return component.Box(halfWidth, halfHeight) }

// Polygon is a convex polygon of 3..MaxPolygonVertices vertices (copied) in shape space.
func Polygon(vertices ...Vec2) Shape { return component.Polygon(vertices...) }

// Chain is an open polyline through points (copied), at least MinChainPoints of them. Static
// or kinematic bodies only.
func Chain(points ...Vec2) Shape { return component.Chain(points...) }

// ChainLoop is Chain closed back to its first point.
func ChainLoop(points ...Vec2) Shape { return component.ChainLoop(points...) }

// Edge is a single segment from a to b in shape space. Static or kinematic bodies only.
func Edge(a, b Vec2) Shape { return component.Edge(a, b) }

// Capsule is the segment from a to b inflated by radius.
func Capsule(a, b Vec2, radius float64) Shape { return component.Capsule(a, b, radius) }

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
// cardinal.WithSystemEventReceiver field. Each carries both entities, both shape indices and
// both shapes' filters, so a handler can tell what hit what by category bits.

// Contact and trigger events, and the payload they share.
type (
	ContactEventPayload = physicevent.ContactEventPayload
	FixtureFilterBits   = physicevent.FixtureFilterBits
	ContactBeginEvent   = physicevent.ContactBeginEvent
	ContactEndEvent     = physicevent.ContactEndEvent
	TriggerBeginEvent   = physicevent.TriggerBeginEvent
	TriggerEndEvent     = physicevent.TriggerEndEvent
)

// -------------------------------------------------------------------------------------------------
// Active contacts
// -------------------------------------------------------------------------------------------------
//
// The polled view of the same contacts. After each step the plugin writes every pair touching
// or overlapping to the ActiveContacts component on its singleton entity, sorted by (EntityA,
// ShapeIndexA, EntityB, ShapeIndexB), each entry with both shapes' filter bits. Read it with an
// Exact search on the two components; see the README.

// The plugin's singleton entity and the contact list it carries.
type (
	PhysicsSingletonTag = component.PhysicsSingletonTag
	ActiveContacts      = component.ActiveContacts
	ContactPairEntry    = component.ContactPairEntry
)
