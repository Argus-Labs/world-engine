package internal

import (
	"sort"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/cbridge"
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

// PhysicsRuntime owns derived physics state for one Cardinal world instance. ECS remains
// authoritative; this struct is disposable and rebuilt from components when needed.
type PhysicsRuntime struct {
	// KnownEntities tracks which Cardinal entities have bodies in the C-side Box2D world.
	KnownEntities map[cardinal.EntityID]struct{}

	// Shadow holds per-entity reconciler snapshots (diff against ECS each tick).
	Shadow map[cardinal.EntityID]ShadowState

	// BufferedContacts collects contact events from cbridge.Step for post-step flush.
	BufferedContacts []BufferedContactEvent

	// Emitter is the current tick's contact flush sink, set by the step driver before Step
	// and cleared in FlushBufferedContacts. Nil means skip emitting for this flush.
	Emitter event.ContactEventEmitter

	// SuppressContactsStep, when true, skips emitting contact/trigger begin/end for this step
	// (e.g. first step after restore).
	SuppressContactsStep bool

	// ActiveContacts is the in-memory working copy of which Begin events have been emitted
	// without a matching End. nil means "not yet loaded from ECS" (e.g. after ResetRuntime);
	// the step system populates it from the persisted ActiveContacts component on first access.
	ActiveContacts map[ContactPairKey]ContactPairInfo

	// ActiveContactsDirty is set when ActiveContacts was mutated during the current flush.
	// The step system checks this to decide whether to Set() the ECS component.
	ActiveContactsDirty bool

	// NoPersistedActiveContactsBaseline, when true, the next suppressed contact flush seeds
	// ActiveContacts from the live contact list without emitting Begin/End (physics singleton entity missing).
	NoPersistedActiveContactsBaseline bool
}

//nolint:gochecknoglobals // Package-scoped runtime singleton.
var runtime *PhysicsRuntime

// NewPhysicsRuntime returns an empty runtime. Maps are initialized; Emitter is nil.
// SuppressContactsStep is true so the next armed simulation step does not record contact
// begin/end; the following FlushBufferedContacts clears suppression when that flush is
// paired with an emitter (see contact_flush.go).
// ActiveContacts is nil, signaling "load from ECS on next step".
func NewPhysicsRuntime() *PhysicsRuntime {
	return &PhysicsRuntime{
		KnownEntities:        make(map[cardinal.EntityID]struct{}),
		Shadow:               make(map[cardinal.EntityID]ShadowState),
		BufferedContacts:     make([]BufferedContactEvent, 0),
		SuppressContactsStep: true,
		ActiveContacts:       nil,
	}
}

// ResetRuntime replaces the package runtime with a fresh PhysicsRuntime.
// If a C-side world exists, it is destroyed first.
func ResetRuntime() {
	if cbridge.WorldExists() {
		cbridge.DestroyWorld()
	}
	runtime = NewPhysicsRuntime()
}

// Runtime returns the current package-scoped physics runtime. It does not create one lazily:
// callers must invoke ResetRuntime first; otherwise this returns nil.
func Runtime() *PhysicsRuntime {
	return runtime
}

// WorldExists reports whether the C-side Box2D world has been created and is alive.
func (rt *PhysicsRuntime) WorldExists() bool {
	return cbridge.WorldExists()
}

// PruneActiveContactsInvolvingEntity removes every active-contact key that references entityID.
// Call when that entity's body is destroyed or its fixtures are structurally replaced so
// end-of-tick persistence and the next suppressed diff do not retain stale pair keys.
func (rt *PhysicsRuntime) PruneActiveContactsInvolvingEntity(entityID cardinal.EntityID) {
	if len(rt.ActiveContacts) == 0 {
		return
	}
	for k := range rt.ActiveContacts {
		if k.EntityA == entityID || k.EntityB == entityID {
			delete(rt.ActiveContacts, k)
			rt.ActiveContactsDirty = true
		}
	}
}

// LoadActiveContactsFromComponent populates the in-memory working map from the persisted
// ECS component. Called by the step system after a restore when ActiveContacts is nil.
func (rt *PhysicsRuntime) LoadActiveContactsFromComponent(ac component.ActiveContacts) {
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
func (rt *PhysicsRuntime) ActiveContactsToComponent() component.ActiveContacts {
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
