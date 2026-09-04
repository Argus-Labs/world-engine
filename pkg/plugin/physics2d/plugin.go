// Package physics2d is a Box2D-backed 2D physics plugin for Cardinal (pure-Go Box2D port in
// pkg/box2d). ECS components live in component; simulation and reconciliation systems are
// plugin-internal (internal/system) and are registered for you by Plugin.Register. All derived
// physics state is owned by the Plugin instance (see Plugin.Reset); the package holds no
// runtime state.
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
	physicssystem "github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/system"
	physicsquery "github.com/argus-labs/world-engine/pkg/plugin/physics2d/query"
	"github.com/rotisserie/eris"
)

// Re-export component types for callers that import the plugin root only.
type (
	Vec2                = component.Vec2
	BodyType            = component.BodyType
	ShapeType           = component.ShapeType
	ColliderShape       = component.ColliderShape
	ChainGeometry2D     = component.ChainGeometry2D
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
	// Workers is the number of workers the physics step may use
	// (box2d.WorldDef.WorkerCount). 0 means 1 (serial); negative values are
	// treated as 0 and values above box2d.MaxWorkers (64) are clamped down,
	// so any value is safe. The engine does not clamp to the core count:
	// whatever you set here is the partition width, so counts beyond the
	// host's cores just oversubscribe the Go scheduler. Simulation results
	// are byte-identical for every value — the engine's worker pool is
	// deterministic by construction — so this is purely a throughput knob
	// and never affects rollback, replay, or cross-machine agreement.
	//
	// Recommend setting Workers only for large scenes (hundreds or more
	// active bodies): small scenes run inline below the engine's per-stage
	// grain thresholds regardless of this value, and worker counts beyond
	// ~8 have diminishing returns.
	Workers int
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

	p.rt = internal.NewRuntime(p.config.Gravity, fixedDT, p.config.SubStepCount, p.config.Workers)
	p.rt.Reset()

	cardinal.RegisterSystem(world, physicssystem.NewInitPhysicsSystem(p.rt), cardinal.WithHook(cardinal.Init))
	cardinal.RegisterSystem(world, physicssystem.NewPhysicsPipelineSystem(p.rt), cardinal.WithHook(cardinal.PreUpdate))
}

// Engine returns the underlying pure-Go Box2D world, or nil when no world exists (before
// init or after Reset).
//
// It is a read-only escape hatch: use it for reads and queries the plugin does not expose
// directly (shape casts, custom query filters, sensor-only overlap, contact walks, body and
// shape inspection, debug draw). Pair it with BodyID / ShapeIDs to go from a Cardinal entity
// to the engine objects to inspect.
//
// Mutating the engine is NOT supported. The reconciler owns body and shape lifecycle and
// derives it from the ECS components each tick, so:
//   - changes to solver-owned state (body transform, velocity, shape geometry, filters,
//     body flags) are overwritten on the next reconcile;
//   - objects you create directly on the engine (bodies, shapes, joints) are untracked and are
//     destroyed by any rebuild: Reset, snapshot restore, or a structural component change.
//
// For changes that must persist, write the ECS components (Transform2D, Velocity2D,
// PhysicsBody2D); the reconciler pushes them into Box2D before each step.
//
// Do not cache the returned pointer across ticks: after Reset the old world is destroyed.
func (p *Plugin) Engine() *box2d.World {
	if p.rt == nil {
		return nil
	}
	return p.rt.World
}

// BodyID returns the Box2D body id backing entityID, and whether the entity currently has one.
// It reports false before the first reconcile creates the body, after the entity's body is
// destroyed, and whenever no world exists (before init or after Reset).
//
// The id is a read-only handle for use with Engine(): it is valid only until the next tick's
// reconcile, which may destroy and recreate the body. Look it up again each tick instead of
// caching it.
func (p *Plugin) BodyID(entityID cardinal.EntityID) (box2d.BodyID, bool) {
	if p.rt == nil || !p.rt.WorldExists() {
		return box2d.BodyID{}, false
	}
	return p.rt.BodyIDOf(entityID)
}

// ShapeIDs returns a copy of the Box2D shape ids backing entityID, indexed by collider slot
// (slot i is PhysicsBody2D.Shapes[i]), and whether the entity currently has any. Chain slots
// hold a null shape id because chains are tracked separately. The caller owns the returned
// slice; mutating it does not affect the plugin.
//
// It reports false under the same conditions as BodyID, and the ids carry the same lifetime:
// valid only until the next tick's reconcile.
func (p *Plugin) ShapeIDs(entityID cardinal.EntityID) ([]box2d.ShapeID, bool) {
	if p.rt == nil || !p.rt.WorldExists() {
		return nil, false
	}
	return p.rt.ShapeIDsOf(entityID)
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
