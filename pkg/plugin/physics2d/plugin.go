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
// ResetRuntime (this package) discards derived physics state; the next PreUpdate reconcile
// performs a full ECS->Box2D rebuild when it sees no world on the C side.
package physics2d

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	physicevent "github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/cbridge"
	physicsquery "github.com/argus-labs/world-engine/pkg/plugin/physics2d/query"
	physicssystem "github.com/argus-labs/world-engine/pkg/plugin/physics2d/system"
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

// WorldID returns the Box2D v3 world id packed as a uint32.
// Returns 0 when no world exists (before init or after ResetRuntime).
// Customers can use this with their own CGO code to call any Box2D v3 function
// via b2LoadWorldId(). This enables custom queries, joints, or any Box2D feature
// not directly exposed by the plugin.
func WorldID() uint32 {
	return cbridge.GetWorldID()
}

// ResetRuntime drops all derived physics simulation state (no world, no bodies, empty maps).
// ECS components are unchanged. The next PhysicsPipelineSystem (PreUpdate) runs
// FullRebuildFromECS from current physics entities, same as recovering after snapshot restore.
func ResetRuntime() {
	internal.ResetRuntime()
}

// Raycast casts a ray along the segment from req.Origin to req.End and returns the closest hit.
// Requires an initialized physics runtime with a C-side world (e.g. after FullRebuildFromECS).
// A zero-length segment returns Hit=false. When Filter is nil, all category/mask pairs match and
// sensors are skipped (same as Filter{CategoryBits: ^uint64(0), MaskBits: ^uint64(0), IncludeSensors: false}).
func Raycast(req RaycastRequest) RaycastResult {
	if !cbridge.WorldExists() {
		return RaycastResult{}
	}
	return physicsquery.Raycast(req)
}

// OverlapAABB returns distinct (entity, shape index) pairs whose shapes overlap the world-space AABB.
func OverlapAABB(req AABBOverlapRequest) AABBOverlapResult {
	if !cbridge.WorldExists() {
		return AABBOverlapResult{}
	}
	return physicsquery.OverlapAABB(req)
}

// CircleSweep sweeps a circle from req.Start to req.End and returns the earliest TOI hit along that segment.
func CircleSweep(req CircleSweepRequest) CircleSweepResult {
	if !cbridge.WorldExists() {
		return CircleSweepResult{}
	}
	return physicsquery.CircleSweep(req)
}

// FlushBufferedContacts emits all contact/trigger events from the last physics step via the
// ContactEventEmitter set by SetStepContactEmitter, then clears the buffer and the emitter slot.
// If an emitter was set for that step, one-shot contact suppression (after reset/rebuild) ends here.
// Ordering matches C-side callback order for that step only; see internal.FlushBufferedContacts.
func FlushBufferedContacts() {
	internal.FlushBufferedContacts()
}

// SetStepContactEmitter assigns the sink for the next physics step's flushed contact events.
func SetStepContactEmitter(e ContactEventEmitter) {
	internal.SetStepEmitter(e)
}

// Config holds plugin options for simulation and stepping.
type Config struct {
	// Gravity is applied to the Box2D world (world gravity vector).
	Gravity Vec2
	// TickRate is simulation steps per second: each Cardinal tick calls cbridge.Step(1/TickRate, sub-steps).
	// Match cardinal.WorldOptions.TickRate so simulated time advances one tick of wall-clock intent per tick.
	// Zero or negative defaults to 60 (same as historical FixedDT 1/60).
	TickRate float64
	// SubStepCount is the number of sub-steps per physics step. Zero defaults to 4.
	SubStepCount int
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
		Gravity:      p.config.Gravity,
		FixedDT:      fixedDT,
		SubStepCount: p.config.SubStepCount,
	})

	cardinal.RegisterSystem(world, physicssystem.InitPhysicsSystem, cardinal.WithHook(cardinal.Init))
	cardinal.RegisterSystem(world, physicssystem.PhysicsPipelineSystem, cardinal.WithHook(cardinal.PreUpdate))
}
