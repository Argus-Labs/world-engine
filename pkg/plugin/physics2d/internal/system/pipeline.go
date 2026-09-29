package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physicscomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	physicevent "github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
)

// PhysicsPipelineSystem runs the full physics pipeline atomically: reconcile -> step -> writeback.
// Combining all three phases into a single system guarantees the scheduler cannot interleave game
// code between them.
type PhysicsPipelineSystem struct {
	rt *internal.Runtime
}

// contactEmitterBridge forwards flushed contact/trigger events to the world's system event queues.
// Plugin.Register registers the four event types, so EmitSystemEvent cannot hit an unregistered one.
type contactEmitterBridge struct {
	w *cardinal.World
}

func (b contactEmitterBridge) EmitContactBegin(e physicevent.ContactBeginEvent) {
	b.w.EmitSystemEvent(e)
}
func (b contactEmitterBridge) EmitContactEnd(e physicevent.ContactEndEvent) { b.w.EmitSystemEvent(e) }
func (b contactEmitterBridge) EmitTriggerBegin(e physicevent.TriggerBeginEvent) {
	b.w.EmitSystemEvent(e)
}
func (b contactEmitterBridge) EmitTriggerEnd(e physicevent.TriggerEndEvent) { b.w.EmitSystemEvent(e) }

// loadContactBaseline locates the physics singleton entity and
// seeds the runtime's contact-dedupe baseline from it when the runtime has
// none (e.g. right after a snapshot restore or Reset).
func loadContactBaseline(
	rt *internal.Runtime, w *cardinal.World, singleton cardinal.Search,
) (cardinal.Entity, bool) {
	var acRef cardinal.Entity
	singletonFound := false
	for row := range singleton.Iter() {
		acRef = row
		singletonFound = true
		break
	}

	if !singletonFound {
		w.Logger().Error().
			Msg("physics2d: physics singleton entity missing; contact dedupe has no persisted baseline")
		if rt.SuppressContactsStep {
			rt.NoPersistedActiveContactsBaseline = true
		}
		return acRef, false
	}

	if rt.ActiveContacts == nil {
		rt.LoadActiveContactsFromComponent(acRef.Get[physicscomp.ActiveContacts]())
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
func NewPhysicsPipelineSystem(rt *internal.Runtime) *PhysicsPipelineSystem {
	return &PhysicsPipelineSystem{rt: rt}
}

func (s *PhysicsPipelineSystem) Run(w *cardinal.World) {
	rt := s.rt
	// Gather buffers stay on Runtime so Reset can drop their component values and entity handles.
	// Keep* clears the unused tail. Systems in one world use this scratch sequentially.
	// --- 1. Reconcile (ECS -> Box2D) ---
	singleton := w.Exact[physicsSingletonRow]()
	ensurePhysicsSingleton(singleton)
	bodies := w.Contains[physicsBodyRow]()
	entries := rt.KeepRebuildEntriesScratch(
		gatherRebuildEntries(rt.RebuildEntriesScratch(), bodies.Iter()))

	if !rt.WorldExists() {
		if err := rt.FullRebuildFromECS(rt.Gravity, entries); err != nil {
			w.Logger().Error().Err(err).Msg("physics2d: FullRebuildFromECS failed (nil world recovery)")
		}
		return
	}
	if err := rt.ReconcileFromECS(entries); err != nil {
		w.Logger().Error().Err(err).Msg("physics2d: ReconcileFromECS failed")
	}

	// --- 2. Step + flush contacts ---
	acRef, singletonFound := loadContactBaseline(rt, w, singleton)

	rt.SetStepEmitter(contactEmitterBridge{w: w})
	rt.Step()
	rt.FlushBufferedContacts()

	if singletonFound && rt.ActiveContactsDirty {
		acRef.Set(rt.ActiveContactsToComponent())
		rt.ActiveContactsDirty = false
	}

	// --- 3. Writeback (Box2D -> ECS) ---
	wb := rt.WritebackScratch()
	for row := range bodies.Iter() {
		wb = append(wb, internal.WritebackEntry{
			Entity: row,
		})
	}
	rt.WritebackFromStepResults(rt.KeepWritebackScratch(wb))
}
