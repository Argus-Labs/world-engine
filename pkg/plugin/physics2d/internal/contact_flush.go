package internal

import (
	"maps"
	"sort"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/cbridge"
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
func FlushBufferedContacts() {
	rt := Runtime()
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
			adoptLiveContactsWithoutEmit(rt)
			return
		}
		diffActiveContactsAfterRebuild(rt, em)
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
	refreshActiveContactsFromLive(rt)
}

// adoptLiveContactsWithoutEmit replaces the in-memory map with the current live touching pairs
// and does not emit system events (no persisted baseline when the singleton entity is missing).
func adoptLiveContactsWithoutEmit(rt *PhysicsRuntime) {
	if !cbridge.WorldExists() {
		return
	}
	live := gatherLiveContacts()
	clear(rt.ActiveContacts)
	maps.Copy(rt.ActiveContacts, live)
	if len(live) > 0 {
		rt.ActiveContactsDirty = true
	}
}

// diffActiveContactsAfterRebuild walks the live contact list and diffs against the persisted
// ActiveContacts map. Emits Begin for genuinely new overlaps and End for contacts that no
// longer exist in the simulation. Events are sorted for deterministic ordering.
func diffActiveContactsAfterRebuild(rt *PhysicsRuntime, em event.ContactEventEmitter) {
	if !cbridge.WorldExists() {
		return
	}

	liveContacts := gatherLiveContacts()

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

// gatherLiveContacts calls cbridge.GatherLiveContacts and converts the results into the
// normalized ContactPairKey -> ContactPairInfo map.
func gatherLiveContacts() map[ContactPairKey]ContactPairInfo {
	result := make(map[ContactPairKey]ContactPairInfo)
	liveEvents := cbridge.GatherLiveContacts()
	for _, c := range liveEvents {
		entityA := cardinal.EntityID(c.EntityA)
		entityB := cardinal.EntityID(c.EntityB)
		shapeIndexA := c.ShapeIndexA
		shapeIndexB := c.ShapeIndexB

		key := normalizeContactPairKey(entityA, shapeIndexA, entityB, shapeIndexB)
		info := ContactPairInfo{IsSensor: c.IsSensor}

		fda := event.FixtureFilterBits{
			CategoryBits: c.CatA,
			MaskBits:     c.MaskA,
			GroupIndex:   c.GroupA,
		}
		fdb := event.FixtureFilterBits{
			CategoryBits: c.CatB,
			MaskBits:     c.MaskB,
			GroupIndex:   c.GroupB,
		}
		if entityA == key.EntityA && shapeIndexA == key.ShapeIndexA {
			info.FilterA = fda
			info.FilterB = fdb
		} else {
			info.FilterA = fdb
			info.FilterB = fda
		}

		if c.NormalValid {
			info.Normal = component.Vec2{X: c.NormalX, Y: c.NormalY}
			info.NormalValid = true
		}
		if c.PointValid {
			info.Point = component.Vec2{X: c.PointX, Y: c.PointY}
			info.PointValid = true
		}
		info.ManifoldPointCount = c.ManifoldPointCount

		result[key] = info
	}
	return result
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
func refreshActiveContactsFromLive(rt *PhysicsRuntime) {
	if !cbridge.WorldExists() || len(rt.ActiveContacts) == 0 {
		return
	}
	live := gatherLiveContacts()
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
		uint64(a.EntityA), a.ShapeIndexA, uint64(a.EntityB), a.ShapeIndexB,
		uint64(b.EntityA), b.ShapeIndexA, uint64(b.EntityB), b.ShapeIndexB,
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
func SetStepEmitter(emitter event.ContactEventEmitter) {
	if rt := Runtime(); rt != nil {
		rt.Emitter = emitter
	}
}
