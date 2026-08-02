package internal

import (
	"sort"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
)

// ContactPairKey identifies a unique fixture-pair contact. Always normalized so that
// (EntityA, ShapeIndexA) < (EntityB, ShapeIndexB) lexicographically.
type ContactPairKey struct {
	EntityA     cardinal.EntityID
	ShapeIndexA int
	EntityB     cardinal.EntityID
	ShapeIndexB int
}

// ContactPairInfo stores metadata for an active contact pair. FilterA/FilterB correspond to
// (EntityA, ShapeIndexA) and (EntityB, ShapeIndexB) after normalization. Manifold fields are
// best-effort from the last live sample (not serialized to snapshots).
type ContactPairInfo struct {
	IsSensor           bool
	FilterA            event.FixtureFilterBits
	FilterB            event.FixtureFilterBits
	Normal             component.Vec2
	NormalValid        bool
	Point              component.Vec2
	PointValid         bool
	ManifoldPointCount int
}

// Runtime owns derived physics state for one Cardinal world instance. ECS remains
// authoritative; this struct is disposable and rebuilt from components when needed.
//
// One Runtime belongs to exactly one physics2d.Plugin instance. The pure-Go Box2D backend is
// per-instance state, so multiple live Runtimes in one process simulate independently.
type Runtime struct {
	// World is the pure-Go Box2D world owned by this runtime. Nil until the first
	// FullRebuildFromECS and after Reset.
	World *box2d.World

	// Bodies maps Cardinal entity ids to their Box2D body ids in World.
	Bodies map[cardinal.EntityID]box2d.BodyID

	// Shapes maps entity ids to per-collider-slot Box2D shape ids: slot i corresponds to
	// ColliderShape index i. Chain slots hold a null ShapeID (chains are tracked in Chains)
	// so per-shape mutable setters skip them, matching the CGO bridge behavior.
	Shapes map[cardinal.EntityID][]box2d.ShapeID

	// Chains maps entity ids to the chain shapes created for chain-type collider slots.
	Chains map[cardinal.EntityID][]box2d.ChainID

	// Gravity is the world gravity vector applied on world creation and on rebuild.
	Gravity component.Vec2

	// FixedDT is the simulated time advanced by one physics step, in seconds.
	FixedDT float64

	// SubStepCount is the number of solver sub-steps per physics step.
	SubStepCount int

	// Workers is the worker count handed to box2d.WorldDef.WorkerCount on world
	// creation (0 = serial). Simulation results are byte-identical for every
	// value, so it never participates in rebuild/restore determinism.
	Workers int

	// KnownEntities tracks which Cardinal entities have bodies in the Box2D world.
	KnownEntities map[cardinal.EntityID]struct{}

	// Shadow holds per-entity reconciler snapshots (diff against ECS each tick).
	Shadow map[cardinal.EntityID]ShadowState

	// BufferedContacts collects contact events from the physics step for post-step flush.
	BufferedContacts []BufferedContactEvent

	// pendingEndEvents holds End events synthesized during reconcile, when a body or its
	// fixtures are destroyed while touching. They are produced before the step but must
	// survive until the step's own events are buffered, so they cannot live in
	// BufferedContacts (which is reset per step). See PruneActiveContactsInvolvingEntity.
	pendingEndEvents []BufferedContactEvent

	// Emitter is the current tick's contact flush sink, set by the step driver before Step
	// and cleared in FlushBufferedContacts. Nil means skip emitting for this flush.
	Emitter event.ContactEventEmitter

	// SuppressContactsStep, when true, skips emitting contact/trigger begin/end for this step
	// (e.g. first step after restore).
	SuppressContactsStep bool

	// ActiveContacts is the in-memory working copy of which Begin events have been emitted
	// without a matching End. nil means "not yet loaded from ECS" (e.g. after Reset);
	// the step system populates it from the persisted ActiveContacts component on first access.
	ActiveContacts map[ContactPairKey]ContactPairInfo

	// ActiveContactsDirty is set when ActiveContacts was mutated during the current flush.
	// The step system checks this to decide whether to Set() the ECS component.
	ActiveContactsDirty bool

	// NoPersistedActiveContactsBaseline, when true, the next suppressed contact flush seeds
	// ActiveContacts from the live contact list without emitting Begin/End (physics singleton entity missing).
	NoPersistedActiveContactsBaseline bool

	// castScratch and overlapScratch are reusable query callback contexts. Box2D
	// callbacks take their context as `any`, so a stack local would be heap
	// allocated on every Raycast/CircleSweep/OverlapAABB call. See query.go.
	castScratch    castHit
	overlapScratch overlapCollector

	// reconcileSortScratch backs the sorted entries clone in ReconcileFromECS, reused
	// across ticks so the per-tick clone does not allocate. Only valid within one call.
	reconcileSortScratch []PhysicsRebuildEntry

	// liveContactsScratch, liveIDsScratch, and contactDataScratch are reused by
	// gatherLiveContacts, which runs up to twice per tick. The map returned by
	// gatherLiveContacts aliases liveContactsScratch and is invalidated by the next call.
	liveContactsScratch map[ContactPairKey]ContactPairInfo
	liveIDsScratch      []cardinal.EntityID
	contactDataScratch  []box2d.ContactData
}

// defaultFixedDT is the step size used when a non-positive FixedDT is supplied (60 Hz).
const defaultFixedDT = 1.0 / 60.0

// defaultSubStepCount is the Box2D v3 friendly solver sub-step count used when a non-positive
// SubStepCount is supplied.
const defaultSubStepCount = 4

// NewRuntime returns an empty runtime with the given simulation parameters. Maps are
// initialized; Emitter is nil. Non-positive fixedDT or subSteps fall back to 60 Hz / 4 sub-steps.
// workers is the box2d.WorldDef.WorkerCount for worlds this runtime creates (0 = serial;
// results are byte-identical for every value).
// SuppressContactsStep is true so the next armed simulation step does not record contact
// begin/end; the following FlushBufferedContacts clears suppression when that flush is
// paired with an emitter (see contact_flush.go).
// ActiveContacts is nil, signaling "load from ECS on next step".
func NewRuntime(gravity component.Vec2, fixedDT float64, subSteps, workers int) *Runtime {
	if fixedDT <= 0 {
		fixedDT = defaultFixedDT
	}
	if subSteps <= 0 {
		subSteps = defaultSubStepCount
	}
	// Clamp like the other knobs: box2d.NewWorld panics outside
	// [0, MaxWorkers], and a plugin misconfiguration must not crash the
	// first tick's world rebuild.
	if workers < 0 {
		workers = 0
	}
	if workers > box2d.MaxWorkers {
		workers = box2d.MaxWorkers
	}
	return &Runtime{
		Gravity:              gravity,
		FixedDT:              fixedDT,
		SubStepCount:         subSteps,
		Workers:              workers,
		Bodies:               make(map[cardinal.EntityID]box2d.BodyID),
		Shapes:               make(map[cardinal.EntityID][]box2d.ShapeID),
		Chains:               make(map[cardinal.EntityID][]box2d.ChainID),
		KnownEntities:        make(map[cardinal.EntityID]struct{}),
		Shadow:               make(map[cardinal.EntityID]ShadowState),
		BufferedContacts:     make([]BufferedContactEvent, 0),
		SuppressContactsStep: true,
		ActiveContacts:       nil,
	}
}

// Reset drops all derived physics state on this runtime, returning it to the state of a freshly
// constructed Runtime. If a Box2D world exists, it is destroyed first. Simulation parameters
// (Gravity, FixedDT, SubStepCount, Workers) are preserved.
func (rt *Runtime) Reset() {
	if rt.World != nil {
		rt.World.Destroy()
		rt.World = nil
	}
	rt.Bodies = make(map[cardinal.EntityID]box2d.BodyID)
	rt.Shapes = make(map[cardinal.EntityID][]box2d.ShapeID)
	rt.Chains = make(map[cardinal.EntityID][]box2d.ChainID)
	rt.KnownEntities = make(map[cardinal.EntityID]struct{})
	rt.Shadow = make(map[cardinal.EntityID]ShadowState)
	rt.BufferedContacts = make([]BufferedContactEvent, 0)
	rt.pendingEndEvents = nil
	rt.Emitter = nil
	rt.SuppressContactsStep = true
	rt.ActiveContacts = nil
	rt.ActiveContactsDirty = false
	rt.NoPersistedActiveContactsBaseline = false
	rt.reconcileSortScratch = nil
	rt.liveContactsScratch = nil
	rt.liveIDsScratch = nil
	rt.contactDataScratch = nil
}

// WorldExists reports whether this runtime's Box2D world has been created and is alive.
func (rt *Runtime) WorldExists() bool {
	return rt.World != nil
}

// BodyIDOf returns the Box2D body id tracked for entityID and whether one exists.
// Read-only accessor for the plugin's public lookup API.
func (rt *Runtime) BodyIDOf(entityID cardinal.EntityID) (box2d.BodyID, bool) {
	bodyID, ok := rt.Bodies[entityID]
	return bodyID, ok
}

// ShapeIDsOf returns a copy of the per-collider-slot shape ids tracked for entityID and whether
// any exist. The copy keeps callers from mutating the runtime's own slice.
func (rt *Runtime) ShapeIDsOf(entityID cardinal.EntityID) ([]box2d.ShapeID, bool) {
	shapes, ok := rt.Shapes[entityID]
	if !ok {
		return nil, false
	}
	out := make([]box2d.ShapeID, len(shapes))
	copy(out, shapes)
	return out, true
}

// PruneActiveContactsInvolvingEntity removes every active-contact key that references entityID.
// Call when that entity's body is destroyed or its fixtures are structurally replaced so
// end-of-tick persistence and the next suppressed diff do not retain stale pair keys.
//
// Each pruned pair was touching, so its contact genuinely ends here. The engine does report
// end-touch events for a destroyed body, but by the time they are drained the shapes are gone
// and the records can no longer be resolved to entities, so those events are dropped (see
// bufferContactEventsFromWorld). Synthesize the End from the persisted pair metadata instead
// and hold it until the next flush; without this a consumer that latches state on Begin (an
// "is grounded" flag, say) would never be told the contact ended.
func (rt *Runtime) PruneActiveContactsInvolvingEntity(entityID cardinal.EntityID) {
	if len(rt.ActiveContacts) == 0 {
		return
	}
	for k, info := range rt.ActiveContacts {
		if k.EntityA == entityID || k.EntityB == entityID {
			rt.pendingEndEvents = append(rt.pendingEndEvents, makeContactEvent(ContactLifecycleEnd, k, info))
			delete(rt.ActiveContacts, k)
			rt.ActiveContactsDirty = true
		}
	}
}

// LoadActiveContactsFromComponent populates the in-memory working map from the persisted
// ECS component. Called by the step system after a restore when ActiveContacts is nil.
func (rt *Runtime) LoadActiveContactsFromComponent(ac component.ActiveContacts) {
	rt.ActiveContacts = make(map[ContactPairKey]ContactPairInfo, len(ac.Pairs))
	for _, p := range ac.Pairs {
		key := ContactPairKey{
			EntityA:     p.EntityA,
			ShapeIndexA: p.ShapeIndexA,
			EntityB:     p.EntityB,
			ShapeIndexB: p.ShapeIndexB,
		}
		rt.ActiveContacts[key] = ContactPairInfo{
			IsSensor: p.IsSensor,
			FilterA: event.FixtureFilterBits{
				CategoryBits: p.FilterACategoryBits,
				MaskBits:     p.FilterAMaskBits,
				GroupIndex:   p.FilterAGroupIndex,
			},
			FilterB: event.FixtureFilterBits{
				CategoryBits: p.FilterBCategoryBits,
				MaskBits:     p.FilterBMaskBits,
				GroupIndex:   p.FilterBGroupIndex,
			},
		}
	}
	rt.ActiveContactsDirty = false
}

// ActiveContactsToComponent converts the working map to the ECS component format (sorted
// slice for deterministic snapshots).
func (rt *Runtime) ActiveContactsToComponent() component.ActiveContacts {
	if rt.ActiveContacts == nil {
		return component.ActiveContacts{}
	}
	pairs := make([]component.ContactPairEntry, 0, len(rt.ActiveContacts))
	for key, info := range rt.ActiveContacts {
		pairs = append(pairs, component.ContactPairEntry{
			EntityA:             key.EntityA,
			ShapeIndexA:         key.ShapeIndexA,
			EntityB:             key.EntityB,
			ShapeIndexB:         key.ShapeIndexB,
			IsSensor:            info.IsSensor,
			FilterACategoryBits: info.FilterA.CategoryBits,
			FilterAMaskBits:     info.FilterA.MaskBits,
			FilterAGroupIndex:   info.FilterA.GroupIndex,
			FilterBCategoryBits: info.FilterB.CategoryBits,
			FilterBMaskBits:     info.FilterB.MaskBits,
			FilterBGroupIndex:   info.FilterB.GroupIndex,
		})
	}
	sortContactPairEntries(pairs)
	return component.ActiveContacts{Pairs: pairs}
}

// sortContactPairEntries sorts by (EntityA, ShapeIndexA, EntityB, ShapeIndexB) for
// deterministic serialization.
func sortContactPairEntries(pairs []component.ContactPairEntry) {
	sort.Slice(pairs, func(i, j int) bool {
		return lessContactPairEntry(pairs[i], pairs[j])
	})
}

// lessContactPairEntry reports whether a should sort before b. Order matches
// lessContactPairByEndpoints on the four endpoint fields only (filters/sensor are ignored);
// used by sortContactPairEntries so ActiveContacts JSON snapshots are stable across map iteration.
func lessContactPairEntry(a, b component.ContactPairEntry) bool {
	return lessContactPairByEndpoints(
		a.EntityA, a.ShapeIndexA, a.EntityB, a.ShapeIndexB,
		b.EntityA, b.ShapeIndexA, b.EntityB, b.ShapeIndexB,
	)
}

// lessContactPairByEndpoints compares (entityA, shapeIndexA, entityB, shapeIndexB) lexicographically.
func lessContactPairByEndpoints(
	aEA cardinal.EntityID, aSA int, aEB cardinal.EntityID, aSB int,
	bEA cardinal.EntityID, bSA int, bEB cardinal.EntityID, bSB int,
) bool {
	if aEA != bEA {
		return aEA < bEA
	}
	if aSA != bSA {
		return aSA < bSA
	}
	if aEB != bEB {
		return aEB < bEB
	}
	return aSB < bSB
}
