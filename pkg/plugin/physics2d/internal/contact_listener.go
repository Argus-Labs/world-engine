package internal

import (
	"github.com/ByteArena/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/query"
)

// ContactLifecycleKind distinguishes BeginContact vs EndContact from Box2D.
type ContactLifecycleKind uint8

const (
	// ContactLifecycleBegin is emitted when two fixtures start touching.
	ContactLifecycleBegin ContactLifecycleKind = iota
	// ContactLifecycleEnd is emitted when two fixtures stop touching.
	ContactLifecycleEnd
)

// BufferedContactEvent is one contact callback recorded during World.Step for post-step
// consumption. It is not an engine/game event type — callers translate these records later.
//
// Fixture A/B order matches Box2D’s contact fixture ordering. ShapeIndexA/B come from
// FixtureUserData (ECS collider shape index), not Box2D child indices.
//
// IsSensorContact is true if either fixture is a sensor (overlap / trigger semantics).
// Sensors do not build collision manifolds in this Box2D port, so NormalValid and PointValid
// are usually false for pure sensor overlaps.
//
// FilterA/FilterB are the Box2D fixture filters at callback time (category/mask/group), matching
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

// physicsContactListener implements box2d.B2ContactListenerInterface and appends to
// PhysicsRuntime.BufferedContacts during World.Step.
type physicsContactListener struct{}

// RegisterPhysicsContactListener installs the package contact listener on w. Idempotent;
// call after creating the world (e.g. from FullRebuildFromECS) and safe on an existing world.
func RegisterPhysicsContactListener(w *box2d.B2World) {
	if w == nil {
		return
	}
	w.SetContactListener(physicsContactListener{})
}

// ClearStepContactBuffer clears pending contact records before World.Step so each tick’s buffer
// only contains callbacks from that step.
func ClearStepContactBuffer() {
	if rt := Runtime(); rt != nil {
		rt.BufferedContacts = rt.BufferedContacts[:0]
	}
}

func (physicsContactListener) BeginContact(contact box2d.B2ContactInterface) {
	bufferContactEvent(ContactLifecycleBegin, contact)
}

func (physicsContactListener) EndContact(contact box2d.B2ContactInterface) {
	bufferContactEvent(ContactLifecycleEnd, contact)
}

func (physicsContactListener) PreSolve(_ box2d.B2ContactInterface, _ box2d.B2Manifold) {
}

func (physicsContactListener) PostSolve(_ box2d.B2ContactInterface, _ *box2d.B2ContactImpulse) {
}

// bufferContactEvent appends one BufferedContactEvent for FlushBufferedContacts after World.Step.
// It preserves Box2D fixture A/B order, fixture filters, entity/shape indices from user data, and
// manifold data when present. No-op if Runtime is nil, contacts are suppressed, fixtures are
// missing, or user data does not encode plugin fixtures.
func bufferContactEvent(kind ContactLifecycleKind, contact box2d.B2ContactInterface) {
	rt := Runtime()
	if rt == nil || rt.SuppressContactsStep {
		return
	}
	if contact == nil {
		return
	}
	fa := contact.GetFixtureA()
	fb := contact.GetFixtureB()
	if fa == nil || fb == nil {
		return
	}
	entityA, shapeIndexA, okA := query.FixtureUserDataFrom(fa.GetUserData())
	entityB, shapeIndexB, okB := query.FixtureUserDataFrom(fb.GetUserData())
	if !okA || !okB {
		return
	}
	fda := fa.GetFilterData()
	fdb := fb.GetFilterData()
	ev := BufferedContactEvent{
		Kind: kind,
		FilterA: event.FixtureFilterBits{
			CategoryBits: fda.CategoryBits,
			MaskBits:     fda.MaskBits,
			GroupIndex:   fda.GroupIndex,
		},
		FilterB: event.FixtureFilterBits{
			CategoryBits: fdb.CategoryBits,
			MaskBits:     fdb.MaskBits,
			GroupIndex:   fdb.GroupIndex,
		},
		EntityA:         entityA,
		EntityB:         entityB,
		ShapeIndexA:     shapeIndexA,
		ShapeIndexB:     shapeIndexB,
		IsSensorContact: fa.IsSensor() || fb.IsSensor(),
	}
	if m := contact.GetManifold(); m != nil {
		ev.ManifoldPointCount = m.PointCount
		if m.PointCount > 0 {
			wm := box2d.MakeB2WorldManifold()
			contact.GetWorldManifold(&wm)
			ev.Normal = component.Vec2{X: wm.Normal.X, Y: wm.Normal.Y}
			ev.NormalValid = true
			ev.Point = component.Vec2{X: wm.Points[0].X, Y: wm.Points[0].Y}
			ev.PointValid = true
		}
	}
	rt.BufferedContacts = append(rt.BufferedContacts, ev)
}
