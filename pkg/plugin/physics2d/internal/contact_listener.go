package internal

import (
	"sort"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
)

// ContactLifecycleKind distinguishes BeginContact vs EndContact.
type ContactLifecycleKind uint8

const (
	// ContactLifecycleBegin is emitted when two shapes start touching.
	ContactLifecycleBegin ContactLifecycleKind = iota
	// ContactLifecycleEnd is emitted when two shapes stop touching.
	ContactLifecycleEnd
)

// BufferedContactEvent is one contact record collected after a physics step for post-step
// consumption. It is not an engine/game event type — callers translate these records later.
//
// IsSensorContact is true if either shape is a sensor (overlap / trigger semantics).
// Sensors do not build collision manifolds, so NormalValid and PointValid are usually false
// for pure sensor overlaps.
//
// FilterA/FilterB are the shape filter bits at event time (category/mask/group), matching
// ECS per-shape bits so consumers can route without re-querying colliders.
type BufferedContactEvent struct {
	Kind               ContactLifecycleKind
	FilterA            event.FixtureFilterBits
	FilterB            event.FixtureFilterBits
	EntityA            cardinal.EntityID
	EntityB            cardinal.EntityID
	ShapeIndexA        int
	ShapeIndexB        int
	IsSensorContact    bool
	Normal             component.Vec2
	NormalValid        bool
	Point              component.Vec2
	PointValid         bool
	ManifoldPointCount int
}

// Step advances this runtime's Box2D world by FixedDT with SubStepCount solver sub-steps,
// then buffers the step's contact and sensor events into rt.BufferedContacts for the next
// FlushBufferedContacts. Event order matches the CGO bridge's drain order exactly:
// contact begins, contact ends, sensor begins, sensor ends.
func (rt *Runtime) Step() {
	if rt == nil || rt.World == nil {
		return
	}
	rt.World.Step(rt.FixedDT, rt.SubStepCount)
	rt.bufferContactEventsFromWorld()
}

// bufferContactEventsFromWorld drains the world's post-step contact/sensor event buffers
// into rt.BufferedContacts. Skipped entirely when contacts are suppressed (first step after
// rebuild), matching the CGO listener suppression.
func (rt *Runtime) bufferContactEventsFromWorld() {
	if rt.SuppressContactsStep {
		// The suppressed step is followed by a full diff against the persisted
		// ActiveContacts, which re-derives what ended; synthesized ends would
		// duplicate that, so drop them.
		rt.pendingEndEvents = rt.pendingEndEvents[:0]
		return
	}
	rt.BufferedContacts = rt.BufferedContacts[:0]

	// Ends synthesized while reconciling (bodies or fixtures destroyed while touching)
	// happened before this step, so they lead. Map iteration produced them in an
	// arbitrary order; sort so the emitted sequence is deterministic.
	if len(rt.pendingEndEvents) > 0 {
		sort.Slice(rt.pendingEndEvents, func(i, j int) bool {
			return lessBufferedContactEvent(rt.pendingEndEvents[i], rt.pendingEndEvents[j])
		})
		rt.BufferedContacts = append(rt.BufferedContacts, rt.pendingEndEvents...)
		rt.pendingEndEvents = rt.pendingEndEvents[:0]
	}

	w := rt.World
	contacts := w.ContactEvents()
	sensors := w.SensorEvents()

	// -- Begin events (contact) --
	for i := range contacts.BeginEvents {
		ev := &contacts.BeginEvents[i]
		buf := rt.makeBufferedEvent(ContactLifecycleBegin, ev.ShapeIDA, ev.ShapeIDB)
		if w.IsContactValid(ev.ContactID) {
			cd := w.ContactData(ev.ContactID)
			applyManifold(&buf, &cd.Manifold)
		}
		rt.BufferedContacts = append(rt.BufferedContacts, buf)
	}

	// -- End events (contact) --
	// Shapes referenced by end events may have been destroyed (see ContactEndTouchEvent);
	// skip those records since their identity can no longer be resolved.
	for i := range contacts.EndEvents {
		ev := &contacts.EndEvents[i]
		if !w.IsShapeValid(ev.ShapeIDA) || !w.IsShapeValid(ev.ShapeIDB) {
			continue
		}
		rt.BufferedContacts = append(rt.BufferedContacts,
			rt.makeBufferedEvent(ContactLifecycleEnd, ev.ShapeIDA, ev.ShapeIDB))
	}

	// -- Begin events (sensor) --
	for i := range sensors.BeginEvents {
		ev := &sensors.BeginEvents[i]
		rt.BufferedContacts = append(rt.BufferedContacts,
			rt.makeBufferedEvent(ContactLifecycleBegin, ev.SensorShapeID, ev.VisitorShapeID))
	}

	// -- End events (sensor) --
	// Skip sensor ends that reference destroyed shapes (same filter as the CGO bridge).
	for i := range sensors.EndEvents {
		ev := &sensors.EndEvents[i]
		if !w.IsShapeValid(ev.SensorShapeID) || !w.IsShapeValid(ev.VisitorShapeID) {
			continue
		}
		rt.BufferedContacts = append(rt.BufferedContacts,
			rt.makeBufferedEvent(ContactLifecycleEnd, ev.SensorShapeID, ev.VisitorShapeID))
	}
}

// makeBufferedEvent fills entity/shape identity, sensor flag, and filter bits for a shape
// pair, mirroring the CGO bridge's fill_contact_event (without manifold data).
func (rt *Runtime) makeBufferedEvent(
	kind ContactLifecycleKind,
	sidA, sidB box2d.ShapeID,
) BufferedContactEvent {
	w := rt.World
	entityA, shapeIndexA := rt.shapeIdentity(sidA)
	entityB, shapeIndexB := rt.shapeIdentity(sidB)
	filterA := w.ShapeFilter(sidA)
	filterB := w.ShapeFilter(sidB)
	return BufferedContactEvent{
		Kind:            kind,
		FilterA:         toFixtureFilterBits(filterA),
		FilterB:         toFixtureFilterBits(filterB),
		EntityA:         entityA,
		EntityB:         entityB,
		ShapeIndexA:     shapeIndexA,
		ShapeIndexB:     shapeIndexB,
		IsSensorContact: w.IsShapeSensor(sidA) || w.IsShapeSensor(sidB),
	}
}

// applyManifold copies contact-begin manifold data into the buffered event (normal, first
// contact point, point count), mirroring the CGO bridge.
func applyManifold(buf *BufferedContactEvent, m *box2d.Manifold) {
	if m.PointCount == 0 {
		return
	}
	buf.Normal = component.Vec2{X: m.Normal.X, Y: m.Normal.Y}
	buf.NormalValid = true
	buf.Point = component.Vec2{X: m.Points[0].AnchorA.X, Y: m.Points[0].AnchorA.Y}
	buf.PointValid = true
	buf.ManifoldPointCount = m.PointCount
}

// shapeIdentity resolves a Box2D shape id to its (entity, collider shape index) pair via the
// body/shape user data written at creation time.
func (rt *Runtime) shapeIdentity(sid box2d.ShapeID) (cardinal.EntityID, int) {
	w := rt.World
	bodyID := w.ShapeBody(sid)
	entityID := cardinal.EntityID(uint32(w.BodyUserData(bodyID))) //nolint:gosec // packed from uint32 entity id
	shapeIndex := int(uint32(w.ShapeUserData(sid)))               //nolint:gosec // packed from small shape index
	return entityID, shapeIndex
}

// toFixtureFilterBits converts a Box2D shape filter to the event filter bits type.
func toFixtureFilterBits(f box2d.Filter) event.FixtureFilterBits {
	return event.FixtureFilterBits{
		CategoryBits: f.CategoryBits,
		MaskBits:     f.MaskBits,
		GroupIndex:   int32(f.GroupIndex), //nolint:gosec // group indexes are small
	}
}
