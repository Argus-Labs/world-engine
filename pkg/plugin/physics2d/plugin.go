// Package physics2d is a Box2D-backed 2D physics plugin for Cardinal (pure-Go Box2D port in
// pkg/box2d). ECS components live in component; simulation and reconciliation systems live in
// system. All derived physics state is owned by the Plugin instance (see Plugin.Reset); the
// package holds no runtime state.
//
// Usage:
//
//	world := cardinal.NewWorld(cardinal.WorldOptions{...})
//	physics := physics2d.NewPlugin(physics2d.Config{})
//	cardinal.RegisterPlugin(world, physics)
//	world.StartGame()
//
// Keep the *Plugin value: queries (Raycast, OverlapAABB, CircleSweep) and Reset are methods on it.
//
// Call Plugin.Reset from init/restore hooks when you rebuild the Cardinal world or after
// FromProto so the derived physics state matches ECS. Reset discards derived physics state; the
// next PhysicsPipelineSystem (PreUpdate) performs a full ECS->Box2D rebuild when it sees no
// live world.
package physics2d

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	physicevent "github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
	physicsquery "github.com/argus-labs/world-engine/pkg/plugin/physics2d/query"
	physicssystem "github.com/argus-labs/world-engine/pkg/plugin/physics2d/system"
	"github.com/rotisserie/eris"
)

// Re-export component types for callers that import the plugin root only.
type (
	Vec2                = component.Vec2
	BodyType            = component.BodyType
	ShapeType           = component.ShapeType
	ColliderShape       = component.ColliderShape
	PhysicsSingletonTag = component.PhysicsSingletonTag
	ActiveContacts      = component.ActiveContacts
	ContactPairEntry    = component.ContactPairEntry
)

// Components entities require to participate in physics simulation.
type (
	Transform2D   = component.Transform2D
	Velocity2D    = component.Velocity2D
	PhysicsBody2D = component.PhysicsBody2D
)

// Body kinds (PhysicsBody2D).
const (
	BodyTypeStatic    = component.BodyTypeStatic
	BodyTypeDynamic   = component.BodyTypeDynamic
	BodyTypeKinematic = component.BodyTypeKinematic
	BodyTypeManual    = component.BodyTypeManual
)

// Collider shape kinds (ColliderShape).
const (
	ShapeTypeCircle          = component.ShapeTypeCircle
	ShapeTypeBox             = component.ShapeTypeBox
	ShapeTypeConvexPolygon   = component.ShapeTypeConvexPolygon
	ShapeTypeStaticChain     = component.ShapeTypeStaticChain
	ShapeTypeStaticChainLoop = component.ShapeTypeStaticChainLoop
	ShapeTypeEdge            = component.ShapeTypeEdge
	ShapeTypeCapsule         = component.ShapeTypeCapsule
)

// Contact / trigger system events (implement ecs.SystemEvent; register with WithSystemEventEmitter).
type (
	FixtureFilterBits   = physicevent.FixtureFilterBits
	ContactEventPayload = physicevent.ContactEventPayload
	ContactBeginEvent   = physicevent.ContactBeginEvent
	ContactEndEvent     = physicevent.ContactEndEvent
	TriggerBeginEvent   = physicevent.TriggerBeginEvent
	TriggerEndEvent     = physicevent.TriggerEndEvent
	ContactEventEmitter = physicevent.ContactEventEmitter
)

// Query API (v1): raycast, AABB overlap, circle sweep.
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

// Config holds plugin options for simulation and stepping.
type Config struct {
	// Gravity is applied to the Box2D world (world gravity vector).
	Gravity Vec2
	// TickRate is simulation steps per second: each Cardinal tick steps the physics world by 1/TickRate.
	// Match cardinal.WorldOptions.TickRate so simulated time advances one tick of wall-clock intent per tick.
	// Zero or negative defaults to 60 (same as historical FixedDT 1/60).
	TickRate float64
	// SubStepCount is the number of sub-steps per physics step. Zero defaults to 4.
	SubStepCount int
}

// Plugin implements cardinal.Plugin for the physics2d package. It owns the derived physics
// state (the runtime, including the pure-Go Box2D world) for the world it is registered with.
// Multiple Plugin instances in one process simulate fully independently.
type Plugin struct {
	config Config
	rt     *internal.Runtime
}

var _ cardinal.Plugin = (*Plugin)(nil)

// NewPlugin builds a physics2d plugin instance.
func NewPlugin(config Config) *Plugin {
	return &Plugin{config: config}
}

// Register implements cardinal.Plugin: creates this instance's runtime state and registers
// systems. Registering the same Plugin instance twice panics.
func (p *Plugin) Register(world *cardinal.World) {
	if p.rt != nil {
		panic(eris.New("physics2d: Plugin.Register called twice on the same instance; " +
			"create a separate plugin instance per world"))
	}

	tickRate := p.config.TickRate
	if tickRate <= 0 {
		tickRate = 60
	}
	fixedDT := 1.0 / tickRate

	p.rt = internal.NewRuntime(p.config.Gravity, fixedDT, p.config.SubStepCount)
	p.rt.Reset()

	cardinal.RegisterSystem(world, physicssystem.NewInitPhysicsSystem(p.rt), cardinal.WithHook(cardinal.Init))
	cardinal.RegisterSystem(world, physicssystem.NewPhysicsPipelineSystem(p.rt), cardinal.WithHook(cardinal.PreUpdate))
}

// Engine returns the underlying pure-Go Box2D world, or nil when no world exists (before
// init or after Reset). Use it for custom read-only queries or any Box2D feature not
// directly exposed by the plugin.
//
// Caveats: reads and queries are safe between ticks. Mutating solver-owned state (body
// transforms, velocities, shapes, filters) bypasses the plugin's ECS shadow copy, so the
// reconciler may overwrite or duplicate those changes on the next tick. Objects created
// directly on the engine (bodies, shapes, joints) are not tracked in ECS and are lost on
// Reset or on any restore-triggered rebuild.
func (p *Plugin) Engine() *box2d.World {
	if p.rt == nil {
		return nil
	}
	return p.rt.World
}

// Reset drops all derived physics simulation state (no world, no bodies, empty maps).
// ECS components are unchanged. The next PhysicsPipelineSystem (PreUpdate) runs
// FullRebuildFromECS from current physics entities, same as recovering after snapshot restore.
// It is a no-op on a plugin that has not been registered yet.
func (p *Plugin) Reset() {
	if p.rt == nil {
		return
	}
	p.rt.Reset()
}

// Raycast casts a ray along the segment from req.Origin to req.End and returns the closest hit.
// Requires an initialized physics runtime with a live world (e.g. after FullRebuildFromECS).
// A zero-length segment returns Hit=false. When Filter is nil, all category/mask pairs match and
// sensors are skipped (same as Filter{CategoryBits: ^uint64(0), MaskBits: ^uint64(0), IncludeSensors: false}).
func (p *Plugin) Raycast(req RaycastRequest) RaycastResult {
	if p.rt == nil || !p.rt.WorldExists() {
		return RaycastResult{}
	}
	return p.rt.Raycast(req)
}

// OverlapAABB returns distinct (entity, shape index) pairs whose shapes overlap the world-space AABB.
func (p *Plugin) OverlapAABB(req AABBOverlapRequest) AABBOverlapResult {
	if p.rt == nil || !p.rt.WorldExists() {
		return AABBOverlapResult{}
	}
	return p.rt.OverlapAABB(req)
}

// CircleSweep sweeps a circle from req.Start to req.End and returns the earliest TOI hit along that segment.
func (p *Plugin) CircleSweep(req CircleSweepRequest) CircleSweepResult {
	if p.rt == nil || !p.rt.WorldExists() {
		return CircleSweepResult{}
	}
	return p.rt.CircleSweep(req)
}
