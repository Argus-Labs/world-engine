package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physicevent "github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
	physicscomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
)

// PhysicsPipelineSystemState runs the full physics pipeline atomically: reconcile -> step -> writeback.
// Combining all three phases into a single system guarantees the scheduler cannot interleave game
// code between them.
type PhysicsPipelineSystemState struct {
	cardinal.BaseSystemState
	Bodies       cardinal.Contains[physicsBodyRow]
	Holders      cardinal.Contains[shapeHolderRow]
	Circles      cardinal.Contains[circleShapeRow]
	Boxes        cardinal.Contains[boxShapeRow]
	Polygons     cardinal.Contains[polygonShapeRow]
	Chains       cardinal.Contains[chainShapeRow]
	Edges        cardinal.Contains[edgeShapeRow]
	Capsules     cardinal.Contains[capsuleShapeRow]
	Singleton    internal.SingletonSearch
	ContactBegin cardinal.WithSystemEventEmitter[physicevent.ContactBeginEvent]
	ContactEnd   cardinal.WithSystemEventEmitter[physicevent.ContactEndEvent]
	TriggerBegin cardinal.WithSystemEventEmitter[physicevent.TriggerBeginEvent]
	TriggerEnd   cardinal.WithSystemEventEmitter[physicevent.TriggerEndEvent]
}

func (s *PhysicsPipelineSystemState) shapes() shapeSearches {
	return shapeSearches{&s.Circles, &s.Boxes, &s.Polygons, &s.Chains, &s.Edges, &s.Capsules}
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

// loadContactBaseline locates the physics singleton entity and
// seeds the runtime's contact-dedupe baseline from it when the runtime has
// none (e.g. right after a snapshot restore or Reset).
func loadContactBaseline(
	rt *internal.Runtime, state *PhysicsPipelineSystemState,
) (cardinal.Entity, bool) {
	var acRef cardinal.Entity
	singletonFound := false
	for row := range state.Singleton.Iter() {
		acRef = row
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
func NewPhysicsPipelineSystem(rt *internal.Runtime) func(*PhysicsPipelineSystemState) {
	// Both per-tick gather buffers live on the Runtime, not in this closure: they hold
	// component values and entity handles, so Reset() has to be able to drop them and each
	// gather has to clear the tail past its new length (the Keep* helpers do that). Systems
	// for one world run sequentially, so the runtime-owned scratch is never shared.
	return func(state *PhysicsPipelineSystemState) {
		// --- 1. Reconcile (ECS -> Box2D) ---
		singleton := internal.EnsureSingleton(&state.Singleton)
		rt.SyncKeptShapes(singleton.Get[physicscomp.ShapeStore]().Kept)
		// Shapes before bodies: attaches below resolve slots through the shape mirror.
		syncShapes(rt, state.shapes())
		entries := rt.KeepRebuildEntriesScratch(
			gatherRebuildEntries(rt.RebuildEntriesScratch(), state.Bodies.Iter()))

		// Entity binds any id regardless of the searches' components, so it is the plain
		// "destroy entity" call the sweep needs.
		destroyEntity := func(id cardinal.EntityID) bool { return state.Entity(id).Destroy() }

		if !rt.WorldExists() {
			if err := rt.FullRebuildFromECS(rt.Gravity, entries); err != nil {
				state.Logger().Error().Err(err).Msg("physics2d: FullRebuildFromECS failed (nil world recovery)")
			}
			// The sweep reads what is named right now, so it runs on this path too: a shape
			// no rebuilt body names is gone on the rebuild tick, not one tick later.
			rt.SweepUnusedShapes(entries, state.Holders.Iter(), destroyEntity)
			return
		}
		if err := rt.ReconcileFromECS(entries); err != nil {
			state.Logger().Error().Err(err).Msg("physics2d: ReconcileFromECS failed")
		}
		rt.SweepUnusedShapes(entries, state.Holders.Iter(), destroyEntity)

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
		wb := rt.WritebackScratch()
		for row := range state.Bodies.Iter() {
			wb = append(wb, internal.WritebackEntry{
				Entity: row,
			})
		}
		rt.WritebackFromStepResults(rt.KeepWritebackScratch(wb))
	}
}
