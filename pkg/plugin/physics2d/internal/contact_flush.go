package internal

import (
	"sort"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
)

// FlushBufferedContacts turns buffered contact records into physics2d events via the
// runtime emitter, then clears the buffer and nils Emitter.
//
// On normal ticks, every Begin adds to rt.ActiveContacts and every End removes from it.
// After applying the step buffer, sustained overlaps are refreshed from the live contact
// list so IsSensor and filter bits stay aligned if a shape toggles sensor/solid or filters
// change while the pair remains touching.
//
// On the first step after a rebuild (SuppressContactsStep was true), the buffer is empty
// because the listener suppressed callbacks. Instead, we diff rt.ActiveContacts (loaded from
// the persisted ECS component) against the live contact list:
//   - Pairs in live but not in the map -> emit Begin, add to map
//   - Pairs in the map but not in live -> emit End, remove from map
//   - Pairs in both -> no event (game already knows)
//
// If NoPersistedActiveContactsBaseline is set (missing singleton on a suppressed step), the
// suppressed flush adopts live contacts into the map without emitting events so one-shot
// Begin handlers do not all fire spuriously; the flag is cleared.
func (rt *Runtime) FlushBufferedContacts() {
	stepHadEmitter := rt.Emitter != nil
	wasSuppressed := rt.SuppressContactsStep
	defer func() {
		rt.BufferedContacts = rt.BufferedContacts[:0]
		rt.Emitter = nil
		// End one-shot listener suppression only when this flush was paired with a real step emitter.
		if stepHadEmitter {
			rt.SuppressContactsStep = false
		}
	}()

	em := rt.Emitter
	if em == nil {
		return
	}

	if rt.ActiveContacts == nil {
		rt.ActiveContacts = make(map[ContactPairKey]ContactPairInfo)
	}

	// First step after rebuild: listener wrote nothing; reconcile map vs live contacts instead of draining the buffer.
	if wasSuppressed && stepHadEmitter {
		// No ECS baseline: seed map from live contacts only; do not emit Begins for every overlap.
		if rt.NoPersistedActiveContactsBaseline {
			rt.NoPersistedActiveContactsBaseline = false
			rt.adoptLiveContactsWithoutEmit()
			return
		}
		rt.diffActiveContactsAfterRebuild(em)
		return
	}

	// Normal step: apply Begin/End buffer, then refresh metadata for pairs still touching.
	for _, buf := range rt.BufferedContacts {
		key := normalizeContactPairKey(buf.EntityA, buf.ShapeIndexA, buf.EntityB, buf.ShapeIndexB)
		switch buf.Kind {
		case ContactLifecycleBegin:
			rt.ActiveContacts[key] = contactInfoNormalizedFromBuffered(buf, key)
			rt.ActiveContactsDirty = true
		case ContactLifecycleEnd:
			delete(rt.ActiveContacts, key)
			rt.ActiveContactsDirty = true
		}
		flushOneBufferedContact(em, buf)
	}
	rt.refreshActiveContactsFromLive()
}

// adoptLiveContactsWithoutEmit replaces the in-memory map with the current live touching pairs
// and does not emit system events (no persisted baseline when the singleton entity is missing).
func (rt *Runtime) adoptLiveContactsWithoutEmit() {
	if rt.World == nil {
		return
	}
	// Gathering straight into ActiveContacts (which gatherLiveContactsInto clears) replaces it
	// wholesale, which is exactly the intent here.
	rt.gatherLiveContactsInto(rt.ActiveContacts)
	if len(rt.ActiveContacts) > 0 {
		rt.ActiveContactsDirty = true
	}
}

// diffActiveContactsAfterRebuild walks the live contact list and diffs against the persisted
// ActiveContacts map. Emits Begin for genuinely new overlaps and End for contacts that no
// longer exist in the simulation. Events are sorted for deterministic ordering.
func (rt *Runtime) diffActiveContactsAfterRebuild(em event.ContactEventEmitter) {
	if rt.World == nil {
		return
	}

	liveContacts := rt.liveContacts()
	rt.gatherLiveContactsInto(liveContacts)

	var events []BufferedContactEvent

	// New overlaps: in live but not in persisted map -> Begin.
	for key, info := range liveContacts {
		if _, exists := rt.ActiveContacts[key]; !exists {
			events = append(events, makeContactEvent(ContactLifecycleBegin, key, info))
			rt.ActiveContacts[key] = info
			rt.ActiveContactsDirty = true
		}
	}

	// Gone overlaps: in persisted map but not in live -> End.
	for key, info := range rt.ActiveContacts {
		if _, exists := liveContacts[key]; !exists {
			events = append(events, makeContactEvent(ContactLifecycleEnd, key, info))
			delete(rt.ActiveContacts, key)
			rt.ActiveContactsDirty = true
		}
	}

	sort.Slice(events, func(i, j int) bool {
		return lessBufferedContactEvent(events[i], events[j])
	})

	for _, ev := range events {
		flushOneBufferedContact(em, ev)
	}
}

// gatherLiveContactsInto fills dst (cleared first) with the currently touching contact pairs,
// by walking every tracked body and reading its touching contact data from the world. Pairs are
// normalized and deduplicated: Box2D reports each contact from both endpoints. Replaces the CGO
// bridge's bridge_gather_live_contacts.
//
// Body iteration order is rt.Bodies map order, which is deliberate: the result does not depend
// on it. World.BodyContactData fills ShapeIDA/ShapeIDB and the manifold from the contact record
// itself, not from the endpoint that asked, so both endpoints of a pair produce a byte-identical
// ContactPairInfo and it cannot matter which one is recorded first. Every consumer of dst is
// order-free as well (a keyed lookup, or a total sort of the derived events).
func (rt *Runtime) gatherLiveContactsInto(dst map[ContactPairKey]ContactPairInfo) {
	clear(dst)
	w := rt.World

	rt.beginContactGather()

	buf := rt.contactDataScratch
	for _, bodyID := range rt.Bodies {
		capacity := w.BodyContactCapacity(bodyID)
		if capacity == 0 {
			continue
		}
		if cap(buf) < capacity {
			buf = make([]box2d.ContactData, capacity)
		}
		n := w.BodyContactData(bodyID, buf[:capacity])
		rt.collectBodyContacts(buf[:n], dst)
	}
	rt.contactDataScratch = buf
}

// liveContacts returns the reusable live-contacts map for callers that need a gather target
// separate from ActiveContacts. gatherLiveContactsInto clears it.
func (rt *Runtime) liveContacts() map[ContactPairKey]ContactPairInfo {
	if rt.liveContactsScratch == nil {
		rt.liveContactsScratch = make(map[ContactPairKey]ContactPairInfo)
	}
	return rt.liveContactsScratch
}

// beginContactGather advances the stamp epoch used by markContactSeen. Wrapping back to 0 would
// make stale stamps compare equal, so that one epoch is spent clearing instead.
func (rt *Runtime) beginContactGather() {
	rt.gatherEpoch++
	if rt.gatherEpoch == 0 {
		clear(rt.seenContactsScratch)
		rt.gatherEpoch = 1
	}
}

// markContactSeen reports whether this contact was already folded into the current gather, and
// records it if not. Both endpoints of a pair report the same contact, so this discards the
// second one before it pays for two shapeIdentity resolutions (three world lookups each).
func (rt *Runtime) markContactSeen(id box2d.ContactID) bool {
	index := int(box2d.PackContactID(id)[0])
	if index >= len(rt.seenContactsScratch) {
		// Grow through append so the backing array grows geometrically. Sizing
		// it to exactly index+1 would reallocate once per contact in a scene
		// that first sees its contact indices in ascending order.
		rt.seenContactsScratch = append(rt.seenContactsScratch,
			make([]uint32, index+1-len(rt.seenContactsScratch))...)
	}
	if rt.seenContactsScratch[index] == rt.gatherEpoch {
		return true
	}
	rt.seenContactsScratch[index] = rt.gatherEpoch
	return false
}

// collectBodyContacts normalizes and records one body's touching contact data into result,
// skipping pairs already recorded from the other endpoint.
func (rt *Runtime) collectBodyContacts(contacts []box2d.ContactData, result map[ContactPairKey]ContactPairInfo) {
	w := rt.World
	for j := range contacts {
		cd := &contacts[j]
		if rt.markContactSeen(cd.ContactID) {
			continue
		}

		entityA, shapeIndexA := rt.shapeIdentity(cd.ShapeIDA)
		entityB, shapeIndexB := rt.shapeIdentity(cd.ShapeIDB)

		key := normalizeContactPairKey(entityA, shapeIndexA, entityB, shapeIndexB)
		// One contact maps to exactly one normalized pair key, so the stamp above already
		// covers this; the map check is what makes a stamp FALSE NEGATIVE free (both endpoints
		// build a byte-identical ContactPairInfo, so a re-record would be a no-op). It does
		// NOT cover the other direction: a false-positive stamp hits the `continue` at the
		// markContactSeen call above and the pair is dropped silently — a spurious End for a
		// still-touching persisted pair, a missing Begin for a new one, stale sensor/filter
		// metadata for the rest. What holds that side is the ContactID layout — PackContactID
		// slot 0 is the contact's 1-based dense index (index1), pinned by
		// TestOracleContactID_CReference in pkg/box2d.
		if _, seen := result[key]; seen {
			continue
		}
		info := ContactPairInfo{
			IsSensor: w.IsShapeSensor(cd.ShapeIDA) || w.IsShapeSensor(cd.ShapeIDB),
		}

		fda := toFixtureFilterBits(w.ShapeFilter(cd.ShapeIDA))
		fdb := toFixtureFilterBits(w.ShapeFilter(cd.ShapeIDB))
		if entityA == key.EntityA && shapeIndexA == key.ShapeIndexA {
			info.FilterA = fda
			info.FilterB = fdb
		} else {
			info.FilterA = fdb
			info.FilterB = fda
		}

		if cd.Manifold.PointCount > 0 {
			info.Normal = component.Vec2{X: cd.Manifold.Normal.X, Y: cd.Manifold.Normal.Y}
			info.NormalValid = true
			info.Point = component.Vec2{
				X: cd.Manifold.Points[0].AnchorA.X,
				Y: cd.Manifold.Points[0].AnchorA.Y,
			}
			info.PointValid = true
			info.ManifoldPointCount = cd.Manifold.PointCount
		}

		result[key] = info
	}
}

// normalizeContactPairKey returns a stable map key: the lexicographically smaller (entity, shapeIndex) pair is A.
func normalizeContactPairKey(entityA cardinal.EntityID, shapeIndexA int, entityB cardinal.EntityID, shapeIndexB int,
) ContactPairKey {
	if entityA < entityB || (entityA == entityB && shapeIndexA <= shapeIndexB) {
		return ContactPairKey{EntityA: entityA, ShapeIndexA: shapeIndexA, EntityB: entityB, ShapeIndexB: shapeIndexB}
	}
	return ContactPairKey{EntityA: entityB, ShapeIndexA: shapeIndexB, EntityB: entityA, ShapeIndexB: shapeIndexA}
}

// contactInfoNormalizedFromBuffered maps buffer order into normalized ContactPairKey
// field order (FilterA matches key.EntityA's shape).
func contactInfoNormalizedFromBuffered(buf BufferedContactEvent, key ContactPairKey) ContactPairInfo {
	info := ContactPairInfo{
		IsSensor:           buf.IsSensorContact,
		Normal:             buf.Normal,
		NormalValid:        buf.NormalValid,
		Point:              buf.Point,
		PointValid:         buf.PointValid,
		ManifoldPointCount: buf.ManifoldPointCount,
	}
	if buf.EntityA == key.EntityA && buf.ShapeIndexA == key.ShapeIndexA {
		info.FilterA = buf.FilterA
		info.FilterB = buf.FilterB
	} else {
		info.FilterA = buf.FilterB
		info.FilterB = buf.FilterA
	}
	return info
}

// refreshActiveContactsFromLive overwrites each ActiveContacts entry that still exists in the
// live contact list with the latest sensor/filter snapshot. Marks the ECS component dirty when
// those fields change.
func (rt *Runtime) refreshActiveContactsFromLive() {
	if rt.World == nil || len(rt.ActiveContacts) == 0 {
		return
	}
	live := rt.liveContacts()
	rt.gatherLiveContactsInto(live)
	for k, prev := range rt.ActiveContacts {
		li, ok := live[k]
		if !ok {
			continue
		}
		rt.ActiveContacts[k] = li
		if contactPairInfoPersistedFieldsDiffer(prev, li) {
			rt.ActiveContactsDirty = true
		}
	}
}

// contactPairInfoPersistedFieldsDiffer is true when sensor or filter bits differ between two snapshots of same pair.
func contactPairInfoPersistedFieldsDiffer(a, b ContactPairInfo) bool {
	return a.IsSensor != b.IsSensor || a.FilterA != b.FilterA || a.FilterB != b.FilterB
}

// makeContactEvent builds a BufferedContactEvent for diffActiveContactsAfterRebuild using normalized key and live info.
func makeContactEvent(kind ContactLifecycleKind, key ContactPairKey, info ContactPairInfo) BufferedContactEvent {
	return BufferedContactEvent{
		Kind:               kind,
		FilterA:            info.FilterA,
		FilterB:            info.FilterB,
		EntityA:            key.EntityA,
		EntityB:            key.EntityB,
		ShapeIndexA:        key.ShapeIndexA,
		ShapeIndexB:        key.ShapeIndexB,
		IsSensorContact:    info.IsSensor,
		Normal:             info.Normal,
		NormalValid:        info.NormalValid,
		Point:              info.Point,
		PointValid:         info.PointValid,
		ManifoldPointCount: info.ManifoldPointCount,
	}
}

// lessBufferedContactEvent orders events for stable diff output: Begin before End, then by normalized pair endpoints.
func lessBufferedContactEvent(a, b BufferedContactEvent) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return lessContactPairByEndpoints(
		a.EntityA, a.ShapeIndexA, a.EntityB, a.ShapeIndexB,
		b.EntityA, b.ShapeIndexA, b.EntityB, b.ShapeIndexB,
	)
}

// flushOneBufferedContact maps one buffered record to TriggerBegin/End or ContactBegin/End on em.
func flushOneBufferedContact(em event.ContactEventEmitter, buf BufferedContactEvent) {
	payload := event.ContactEventPayload{
		FilterA:     buf.FilterA,
		FilterB:     buf.FilterB,
		EntityA:     buf.EntityA,
		EntityB:     buf.EntityB,
		ShapeIndexA: buf.ShapeIndexA,
		ShapeIndexB: buf.ShapeIndexB,
		Normal:      buf.Normal,
		NormalValid: buf.NormalValid,
		Point:       buf.Point,
		PointValid:  buf.PointValid,
	}
	if buf.IsSensorContact {
		switch buf.Kind {
		case ContactLifecycleBegin:
			em.EmitTriggerBegin(event.TriggerBeginEvent{ContactEventPayload: payload})
		case ContactLifecycleEnd:
			em.EmitTriggerEnd(event.TriggerEndEvent{ContactEventPayload: payload})
		}
		return
	}
	switch buf.Kind {
	case ContactLifecycleBegin:
		em.EmitContactBegin(event.ContactBeginEvent{ContactEventPayload: payload})
	case ContactLifecycleEnd:
		em.EmitContactEnd(event.ContactEndEvent{ContactEventPayload: payload})
	}
}

// SetStepEmitter stores the contact event sink for the upcoming simulation step. The step driver
// should set this before the step and call FlushBufferedContacts after the step.
func (rt *Runtime) SetStepEmitter(emitter event.ContactEventEmitter) {
	if rt == nil {
		return
	}
	rt.Emitter = emitter
}
