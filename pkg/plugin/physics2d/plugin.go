// Package physics2d is a Box2D-backed 2D physics plugin for Cardinal. ECS components live in
// component; simulation and reconciliation systems live in system; package-level runtime state
// is owned here (see Runtime, ResetRuntime).
//
// Usage:
//
//	world := cardinal.NewWorld(cardinal.WorldOptions{...})
//	cardinal.RegisterPlugin(world, physics2d.NewPlugin(physics2d.Config{}))
//	world.StartGame()
//
// Call ResetRuntime from init/restore hooks when you rebuild the Cardinal world or after
// FromProto so the derived physics state matches ECS.
//
// ResetRuntime (this package) discards Box2D state; the next PreUpdate reconcile performs
// a full ECS→Box2D rebuild when it sees a nil world.
package physics2d

import (
	"github.com/ByteArena/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	physicevent "github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
	physicsquery "github.com/argus-labs/world-engine/pkg/plugin/physics2d/query"
	physicssystem "github.com/argus-labs/world-engine/pkg/plugin/physics2d/system"
)

// Re-export component types for callers that import the plugin root only.
type (
	Vec2                = component.Vec2
	Transform2D         = component.Transform2D
	Velocity2D          = component.Velocity2D
	BodyType            = component.BodyType
	Rigidbody2D         = component.Rigidbody2D
	ShapeType           = component.ShapeType
	ColliderShape       = component.ColliderShape
	Collider2D          = component.Collider2D
	PhysicsSingletonTag = component.PhysicsSingletonTag
	ActiveContacts      = component.ActiveContacts
	ContactPairEntry    = component.ContactPairEntry
)

// Body kinds (Rigidbody2D).
const (
	BodyTypeStatic    = component.BodyTypeStatic
	BodyTypeDynamic   = component.BodyTypeDynamic
	BodyTypeKinematic = component.BodyTypeKinematic
	BodyTypeManual    = component.BodyTypeManual
)

// Collider shape kinds (ColliderShape).
const (
	ShapeTypeCircle        = component.ShapeTypeCircle
	ShapeTypeBox           = component.ShapeTypeBox
	ShapeTypeConvexPolygon = component.ShapeTypeConvexPolygon
	ShapeTypeStaticChain   = component.ShapeTypeStaticChain
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

// PhysicsWorld returns the active Box2D world for custom queries and advanced use, or nil if
// ResetRuntime has not run or the world has not been created yet (e.g. before FullRebuildFromECS).
//
// Prefer read-only access (raycasts, overlaps, inspection). Creating, destroying, or moving
// bodies/fixtures outside the plugin’s ECS reconciliation path can desync simulation from ECS.
// Call from the same tick/thread context as other physics2d entry points.
func PhysicsWorld() *box2d.B2World {
	rt := internal.Runtime()
	if rt == nil {
		return nil
	}
	return rt.World
}

// Body returns the underlying Box2D body for a Cardinal entity, or nil if the entity has no
// physics body (not yet created, or the entity lacks physics components).
//
// # Safe operations (read-only queries)
//
// Use the returned body for inspecting simulation state that is not exposed through ECS
// components:
//
//   - Position/velocity inspection: GetPosition, GetAngle, GetLinearVelocity, GetAngularVelocity
//   - Mass queries: GetMass, GetInertia, GetMassData
//   - Contact iteration: GetContactList (walk the contact edge linked list)
//   - Fixture inspection: GetFixtureList, GetFixtureCount
//   - State checks: IsAwake, IsActive, IsBullet, IsFixedRotation, IsSleepingAllowed
//   - World queries from body: GetWorld
//
// # Unsafe operations (will be overwritten)
//
// The plugin's reconciler (PreUpdate) and writeback (PostUpdate) systems own the following
// body state. Calling these setters directly is ineffective — the values will be overwritten
// by the next tick's reconcile or writeback pass. Use the ECS components instead:
//
//   - SetTransform, SetLinearVelocity, SetAngularVelocity → modify [Transform2D], [Velocity2D]
//   - SetType, SetLinearDamping, SetAngularDamping, SetGravityScale → modify [Rigidbody2D]
//   - SetActive, SetAwake, SetSleepingAllowed, SetBullet, SetFixedRotation → modify [Rigidbody2D]
//
// # Forbidden operations (will desync or crash)
//
// Do not create or destroy bodies/fixtures through the raw pointer. The plugin manages body
// and fixture lifecycle from ECS components ([Rigidbody2D], [Collider2D]). Bypassing this
// corrupts the internal body map, shadow state, and active-contact tracking.
//
// # Lifecycle
//
// The returned pointer is valid for the current tick only. After a ResetRuntime or
// FullRebuildFromECS (e.g. crash recovery), all body pointers are invalidated — the plugin
// destroys and recreates them from ECS state. Do not cache the pointer across ticks.
func Body(entityID cardinal.EntityID) *box2d.B2Body {
	rt := internal.Runtime()
	if rt == nil {
		return nil
	}
	return rt.Bodies[entityID]
}

// ResetRuntime drops all Box2D simulation state (no world, no bodies, empty maps).
// ECS components are unchanged. The next ReconcilePhysicsSystem (PreUpdate) runs
// FullRebuildFromECS from current physics entities, same as recovering after snapshot restore.
func ResetRuntime() {
	internal.ResetRuntime()
}

// Raycast casts a ray along the segment from req.Origin to req.End and returns the closest hit.
// Requires an initialized physics runtime with a Box2D world (e.g. after FullRebuildFromECS).
// A zero-length segment returns Hit=false. When Filter is nil, all category/mask pairs match and
// sensors are skipped (same as Filter{CategoryBits: 0xFFFF, MaskBits: 0xFFFF, IncludeSensors: false}).
func Raycast(req RaycastRequest) RaycastResult {
	rt := internal.Runtime()
	if rt == nil || rt.World == nil {
		return RaycastResult{}
	}
	return physicsquery.Raycast(rt.World, req)
}

// OverlapAABB returns distinct (entity, shape index) pairs whose fixtures overlap the world-space AABB.
func OverlapAABB(req AABBOverlapRequest) AABBOverlapResult {
	rt := internal.Runtime()
	if rt == nil || rt.World == nil {
		return AABBOverlapResult{}
	}
	return physicsquery.OverlapAABB(rt.World, req)
}

// CircleSweep sweeps a circle from req.Start to req.End and returns the earliest TOI hit along that segment.
func CircleSweep(req CircleSweepRequest) CircleSweepResult {
	rt := internal.Runtime()
	if rt == nil || rt.World == nil {
		return CircleSweepResult{}
	}
	return physicsquery.CircleSweep(rt.World, req)
}

// FlushBufferedContacts emits all contact/trigger events from the last Box2D step via the
// ContactEventEmitter set by SetStepContactEmitter, then clears the buffer and the emitter slot.
// If an emitter was set for that step, one-shot contact suppression (after reset/rebuild) ends here.
// Ordering matches Box2D callback order for that step only; see internal.FlushBufferedContacts.
func FlushBufferedContacts() {
	internal.FlushBufferedContacts()
}

// SetStepContactEmitter assigns the sink for the next physics step’s flushed contact events.
func SetStepContactEmitter(e ContactEventEmitter) {
	internal.SetStepEmitter(e)
}

// Config holds plugin options for simulation and stepping.
type Config struct {
	// Gravity is applied to the Box2D world (world gravity vector).
	Gravity Vec2
	// TickRate is simulation steps per second: each Cardinal tick calls Box2D World.Step(1/TickRate, ...).
	// Match cardinal.WorldOptions.TickRate so simulated time advances one tick of wall-clock intent per tick.
	// Zero or negative defaults to 60 (same as historical FixedDT 1/60).
	TickRate float64
	// VelocityIterations is Box2D velocity solver iterations. Zero defaults to 8.
	VelocityIterations int
	// PositionIterations is Box2D position solver iterations. Zero defaults to 3.
	PositionIterations int
}

// Plugin implements cardinal.Plugin for the physics2d package.
type Plugin struct {
	config Config
}

var _ cardinal.Plugin = (*Plugin)(nil)

// NewPlugin builds a physics2d plugin instance.
func NewPlugin(config Config) *Plugin {
	return &Plugin{config: config}
}

// Register implements cardinal.Plugin: resets runtime state and registers systems.
func (p *Plugin) Register(world *cardinal.World) {
	internal.ResetRuntime()

	tickRate := p.config.TickRate
	if tickRate <= 0 {
		tickRate = 60
	}
	fixedDT := 1.0 / tickRate

	physicssystem.SetRuntimeConfig(physicssystem.RuntimeConfig{
		Gravity:            p.config.Gravity,
		FixedDT:            fixedDT,
		VelocityIterations: p.config.VelocityIterations,
		PositionIterations: p.config.PositionIterations,
	})

	cardinal.RegisterSystem(world, physicssystem.InitPhysicsSystem, cardinal.WithHook(cardinal.Init))
	cardinal.RegisterSystem(world, physicssystem.ReconcilePhysicsSystem, cardinal.WithHook(cardinal.PreUpdate))
	cardinal.RegisterSystem(world, physicssystem.PhysicsStepSystem, cardinal.WithHook(cardinal.Update))
	cardinal.RegisterSystem(world, physicssystem.WritebackPhysicsSystem, cardinal.WithHook(cardinal.PostUpdate))
}
