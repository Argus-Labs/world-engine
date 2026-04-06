package system

import (
	"github.com/ByteArena/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physicscomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	physicevent "github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
)

// PhysicsPipelineSystemState runs the full physics pipeline atomically: reconcile → step → writeback.
// Combining all three phases into a single system guarantees the scheduler cannot interleave game
// code between them.
type PhysicsPipelineSystemState struct {
	cardinal.BaseSystemState
	Bodies       cardinal.Contains[physicsBodyRow]
	Singleton    physicsSingletonSearch
	ContactBegin cardinal.WithSystemEventEmitter[physicevent.ContactBeginEvent]
	ContactEnd   cardinal.WithSystemEventEmitter[physicevent.ContactEndEvent]
	TriggerBegin cardinal.WithSystemEventEmitter[physicevent.TriggerBeginEvent]
	TriggerEnd   cardinal.WithSystemEventEmitter[physicevent.TriggerEndEvent]
}

type contactEmitterBridge struct {
	s *PhysicsPipelineSystemState
}

func (b contactEmitterBridge) EmitContactBegin(e physicevent.ContactBeginEvent) {
	b.s.ContactBegin.Emit(e)
}
func (b contactEmitterBridge) EmitContactEnd(e physicevent.ContactEndEvent) { b.s.ContactEnd.Emit(e) }
func (b contactEmitterBridge) EmitTriggerBegin(e physicevent.TriggerBeginEvent) {
	b.s.TriggerBegin.Emit(e)
}
func (b contactEmitterBridge) EmitTriggerEnd(e physicevent.TriggerEndEvent) { b.s.TriggerEnd.Emit(e) }

// PhysicsPipelineSystem runs the full physics pipeline as one atomic unit on cardinal.Update:
//  1. Reconcile: sync ECS → Box2D (create/update/destroy bodies from component changes)
//  2. Step: advance Box2D simulation, flush contact/trigger events
//  3. Writeback: sync Box2D → ECS (write post-step positions/velocities back to components)
func PhysicsPipelineSystem(state *PhysicsPipelineSystemState) {
	rt := internal.Runtime()

	// --- 1. Reconcile (ECS → Box2D) ---
	ensurePhysicsSingleton(&state.Singleton)
	entries := gatherRebuildEntries(state.Bodies.Iter())
	cfg := stepConfig()
	g := box2d.MakeB2Vec2(cfg.Gravity.X, cfg.Gravity.Y)

	if rt.World == nil {
		if err := internal.FullRebuildFromECS(g, entries); err != nil {
			state.Logger().Error().Err(err).Msg("physics2d: FullRebuildFromECS failed (nil world recovery)")
		}
		return
	}
	if err := internal.ReconcileFromECS(entries); err != nil {
		state.Logger().Error().Err(err).Msg("physics2d: ReconcileFromECS failed")
	}

	// --- 2. Step + flush contacts ---
	var acRef cardinal.Ref[physicscomp.ActiveContacts]
	singletonFound := false
	for _, row := range state.Singleton.Iter() {
		acRef = row.ActiveContacts
		singletonFound = true
		break
	}

	if !singletonFound {
		state.Logger().Error().Msg("physics2d: physics singleton entity missing; contact dedupe has no persisted baseline")
		if rt.SuppressContactsStep {
			rt.NoPersistedActiveContactsBaseline = true
		}
	}

	if singletonFound && rt.ActiveContacts == nil {
		rt.LoadActiveContactsFromComponent(acRef.Get())
	}

	internal.SetStepEmitter(contactEmitterBridge{s: state})
	rt.World.Step(cfg.FixedDT, cfg.VelocityIterations, cfg.PositionIterations)
	internal.FlushBufferedContacts()

	if singletonFound && rt.ActiveContactsDirty {
		acRef.Set(rt.ActiveContactsToComponent())
		rt.ActiveContactsDirty = false
	}

	// --- 3. Writeback (Box2D → ECS) ---
	wbEntries := make([]internal.WritebackEntry, 0, len(rt.Bodies))
	for eid, row := range state.Bodies.Iter() {
		wbEntries = append(wbEntries, internal.WritebackEntry{
			EntityID:    eid,
			Transform:   row.Transform,
			Velocity:    row.Velocity,
			PhysicsBody: row.PhysicsBody,
		})
	}
	internal.WritebackFromBox2D(wbEntries)
}
