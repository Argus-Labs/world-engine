package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physicscomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	physicevent "github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
)

// PhysicsPipelineSystemState runs the full physics pipeline atomically: reconcile -> step -> writeback.
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

// loadContactBaseline locates the physics singleton's ActiveContacts ref and
// seeds the runtime's contact-dedupe baseline from it when the runtime has
// none (e.g. right after a snapshot restore or Reset).
func loadContactBaseline(
	rt *internal.Runtime, state *PhysicsPipelineSystemState,
) (cardinal.Ref[physicscomp.ActiveContacts], bool) {
	var acRef cardinal.Ref[physicscomp.ActiveContacts]
	singletonFound := false
	for _, row := range state.Singleton.Iter() {
		acRef = row.ActiveContacts
		singletonFound = true
		break
	}

	if !singletonFound {
		state.Logger().Error().
			Msg("physics2d: physics singleton entity missing; contact dedupe has no persisted baseline")
		if rt.SuppressContactsStep {
			rt.NoPersistedActiveContactsBaseline = true
		}
		return acRef, false
	}

	if rt.ActiveContacts == nil {
		rt.LoadActiveContactsFromComponent(acRef.Get())
	}
	return acRef, true
}

// NewPhysicsPipelineSystem returns the full physics pipeline as one atomic unit, bound to rt.
// The plugin registers it on cardinal.PreUpdate so simulation and writeback finish before
// cardinal.Update game logic in the same tick, while contact/trigger system events remain
// visible until the tick ends.
//
// Phases:
//  1. Reconcile: sync ECS -> Box2D (create/update/destroy bodies from component changes)
//  2. Step: advance physics simulation, buffer contact/trigger events
//  3. Writeback: sync Box2D -> ECS (write post-step positions/velocities back to components)
func NewPhysicsPipelineSystem(rt *internal.Runtime) func(*PhysicsPipelineSystemState) {
	return func(state *PhysicsPipelineSystemState) {
		// --- 1. Reconcile (ECS -> Box2D) ---
		ensurePhysicsSingleton(&state.Singleton)
		entries := gatherRebuildEntries(state.Bodies.Iter())

		if !rt.WorldExists() {
			if err := rt.FullRebuildFromECS(rt.Gravity, entries); err != nil {
				state.Logger().Error().Err(err).Msg("physics2d: FullRebuildFromECS failed (nil world recovery)")
			}
			return
		}
		if err := rt.ReconcileFromECS(entries); err != nil {
			state.Logger().Error().Err(err).Msg("physics2d: ReconcileFromECS failed")
		}

		// --- 2. Step + flush contacts ---
		acRef, singletonFound := loadContactBaseline(rt, state)

		rt.SetStepEmitter(contactEmitterBridge{s: state})
		rt.Step()
		rt.FlushBufferedContacts()

		if singletonFound && rt.ActiveContactsDirty {
			acRef.Set(rt.ActiveContactsToComponent())
			rt.ActiveContactsDirty = false
		}

		// --- 3. Writeback (Box2D -> ECS) ---
		wbEntries := make([]internal.WritebackEntry, 0, len(rt.Shadow))
		for eid, row := range state.Bodies.Iter() {
			wbEntries = append(wbEntries, internal.WritebackEntry{
				EntityID:    eid,
				Transform:   row.Transform,
				Velocity:    row.Velocity,
				PhysicsBody: row.PhysicsBody,
			})
		}
		rt.WritebackFromStepResults(wbEntries)
	}
}
