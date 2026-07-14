package internal

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/event"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/cbridge"
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

// SetBufferedContactsFromStep converts cbridge.ContactEvent results from Step into
// BufferedContactEvents and stores them in rt.BufferedContacts for the next flush.
// Called by the pipeline between cbridge.Step and FlushBufferedContacts.
func SetBufferedContactsFromStep(events []cbridge.ContactEvent) {
	rt := Runtime()
	if rt == nil {
		return
	}
	// Skip buffering if contacts are suppressed (first step after rebuild).
	if rt.SuppressContactsStep {
		return
	}
	rt.BufferedContacts = rt.BufferedContacts[:0]
	for _, c := range events {
		var kind ContactLifecycleKind
		if c.Kind == cbridge.ContactEnd {
			kind = ContactLifecycleEnd
		} else {
			kind = ContactLifecycleBegin
		}
		rt.BufferedContacts = append(rt.BufferedContacts, BufferedContactEvent{
			Kind: kind,
			FilterA: event.FixtureFilterBits{
				CategoryBits: c.CatA,
				MaskBits:     c.MaskA,
				GroupIndex:   c.GroupA,
			},
			FilterB: event.FixtureFilterBits{
				CategoryBits: c.CatB,
				MaskBits:     c.MaskB,
				GroupIndex:   c.GroupB,
			},
			EntityA:            cardinal.EntityID(c.EntityA),
			EntityB:            cardinal.EntityID(c.EntityB),
			ShapeIndexA:        c.ShapeIndexA,
			ShapeIndexB:        c.ShapeIndexB,
			IsSensorContact:    c.IsSensor,
			Normal:             component.Vec2{X: c.NormalX, Y: c.NormalY},
			NormalValid:        c.NormalValid,
			Point:              component.Vec2{X: c.PointX, Y: c.PointY},
			PointValid:         c.PointValid,
			ManifoldPointCount: c.ManifoldPointCount,
		})
	}
}
