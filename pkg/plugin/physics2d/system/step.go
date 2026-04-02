package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physicscomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	physicevent "github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
)

// PhysicsStepSystemState steps the Box2D world and flushes contact/trigger system events.
type PhysicsStepSystemState struct {
	cardinal.BaseSystemState
	ContactBegin cardinal.WithSystemEventEmitter[physicevent.ContactBeginEvent]
	ContactEnd   cardinal.WithSystemEventEmitter[physicevent.ContactEndEvent]
	TriggerBegin cardinal.WithSystemEventEmitter[physicevent.TriggerBeginEvent]
	TriggerEnd   cardinal.WithSystemEventEmitter[physicevent.TriggerEndEvent]
	Singleton    physicsSingletonArchetype
}

type contactEmitterBridge struct {
	s *PhysicsStepSystemState
}

func (b contactEmitterBridge) EmitContactBegin(e physicevent.ContactBeginEvent) {
	b.s.ContactBegin.Emit(e)
}

func (b contactEmitterBridge) EmitContactEnd(e physicevent.ContactEndEvent) {
	b.s.ContactEnd.Emit(e)
}

func (b contactEmitterBridge) EmitTriggerBegin(e physicevent.TriggerBeginEvent) {
	b.s.TriggerBegin.Emit(e)
}

func (b contactEmitterBridge) EmitTriggerEnd(e physicevent.TriggerEndEvent) {
	b.s.TriggerEnd.Emit(e)
}

// PhysicsStepSystem runs World.Step and flushes contacts. Runs on cardinal.Update after PreUpdate reconcile.
// It syncs the ActiveContacts map between ECS (persisted) and the runtime (in-memory working copy).
func PhysicsStepSystem(state *PhysicsStepSystemState) {
	rt := internal.Runtime()
	if rt == nil || rt.World == nil {
		return
	}

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

	// After a restore or FullRebuildFromECS, rt.ActiveContacts is nil. Load from ECS.
	if singletonFound && rt.ActiveContacts == nil {
		rt.LoadActiveContactsFromComponent(acRef.Get())
	}

	cfg := stepConfig()
	internal.SetStepEmitter(contactEmitterBridge{s: state})
	rt.World.Step(cfg.FixedDT, cfg.VelocityIterations, cfg.PositionIterations)
	internal.FlushBufferedContacts()

	if singletonFound && rt.ActiveContactsDirty {
		acRef.Set(rt.ActiveContactsToComponent())
		rt.ActiveContactsDirty = false
	}
}
