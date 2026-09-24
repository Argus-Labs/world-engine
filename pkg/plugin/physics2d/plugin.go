// Package physics2d is a Box2D-backed 2D physics plugin for Cardinal, on the pure-Go Box2D
// port in pkg/box2d.
//
// Install it once per world and keep the *Plugin: queries and Reset are methods on it.
//
//	physics := physics2d.NewPlugin(physics2d.Config{Gravity: physics2d.Vec2{Y: -9.8}})
//	w.RegisterPlugin(physics)
//
// An entity is simulated while it carries Transform2D, Velocity2D and PhysicsBody2D. A body
// carries its shapes as values in its own list. Everything a system touches is in api.go, in
// the order a game meets it: bodies, shapes, queries, contact events. This file is the plugin
// instance and the raw-engine escape hatch.
//
// The simulation state (the Box2D world, bodies, fixtures) is derived from ECS every tick and
// owned by the Plugin instance. It is never snapshotted; call Reset after a restore and the
// next tick rebuilds it from the components.
package physics2d

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
	physicssystem "github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/system"
	"github.com/rotisserie/eris"
)

// Config holds plugin options for simulation and stepping.
type Config struct {
	// Gravity is the world gravity vector.
	Gravity Vec2
	// TickRate is simulation steps per second; each Cardinal tick steps the world by 1/TickRate.
	// Match cardinal.WorldOptions.TickRate. Zero or negative defaults to 60.
	TickRate float64
	// SubStepCount is the number of sub-steps per step. Zero defaults to 4.
	SubStepCount int
	// Workers is the number of workers a step may use; 0 means serial, and any value is safe
	// (clamped to box2d.MaxWorkers). Results are byte-identical for every value, so this is a
	// throughput knob only. Worth setting for scenes of hundreds of active bodies; see the
	// README for the trade-offs.
	Workers int
}

// Plugin implements cardinal.Plugin. It owns the derived physics state for the world it is
// registered with; separate instances simulate independently.
type Plugin struct {
	config Config
	rt     *internal.Runtime
}

var _ cardinal.Plugin = (*Plugin)(nil)

// NewPlugin builds a physics2d plugin instance.
func NewPlugin(config Config) *Plugin {
	return &Plugin{config: config}
}

// Register implements cardinal.Plugin: it creates this instance's runtime and registers the
// simulation systems. Registering the same instance twice panics.
func (p *Plugin) Register(w *cardinal.World) {
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

	w.RegisterSystem(physicssystem.NewInitPhysicsSystem(p.rt), cardinal.WithHook(cardinal.Init))
	w.RegisterSystem(physicssystem.NewPhysicsPipelineSystem(p.rt), cardinal.WithHook(cardinal.PreUpdate))
}

// Reset drops all derived physics state; ECS components are untouched. The next tick rebuilds
// the world from them. Call it after a snapshot restore. A no-op before Register.
func (p *Plugin) Reset() {
	if p.rt == nil {
		return
	}
	p.rt.Reset()
}

// -------------------------------------------------------------------------------------------------
// Escape hatch: the raw Box2D world
// -------------------------------------------------------------------------------------------------
//
// Engine hands out the underlying world for reads the built-in queries do not cover; BodyID and
// ShapeIDs map an entity to the engine objects behind it. All three are read-only: the
// reconciler derives the world from ECS every tick, so engine-side edits are overwritten and
// engine-created objects are destroyed on the next rebuild. Ids and the world pointer are valid
// until the next tick's reconcile; look them up again each tick. The README has the rules.

// Engine returns the Box2D world, or nil when none exists (before init or after Reset).
func (p *Plugin) Engine() *box2d.World {
	if p.rt == nil {
		return nil
	}
	return p.rt.World
}

// BodyID returns the Box2D body backing entityID, and false when it has none: before the
// first reconcile, after the body is destroyed, or while no world exists.
func (p *Plugin) BodyID(entityID cardinal.EntityID) (box2d.BodyID, bool) {
	if p.rt == nil || !p.rt.WorldExists() {
		return box2d.BodyID{}, false
	}
	return p.rt.BodyIDOf(entityID)
}

// ShapeIDs returns a copy of the Box2D shape ids backing entityID, indexed like
// PhysicsBody2D.Shapes, and false under the same conditions as BodyID. Chain shapes hold a
// null id because chains are tracked separately.
func (p *Plugin) ShapeIDs(entityID cardinal.EntityID) ([]box2d.ShapeID, bool) {
	if p.rt == nil || !p.rt.WorldExists() {
		return nil, false
	}
	return p.rt.ShapeIDsOf(entityID)
}
