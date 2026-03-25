package event

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// FixtureFilterBits is the Box2D collision filter for one fixture at contact time. It matches
// ECS ColliderShape CategoryBits/MaskBits and the fixture’s GroupIndex (non-zero group rules
// override category/mask in Box2D).
type FixtureFilterBits struct {
	CategoryBits uint16 `json:"category_bits"`
	MaskBits     uint16 `json:"mask_bits"`
	GroupIndex   int16  `json:"group_index"`
}

// ContactEventPayload is shared data for all flushed contact/trigger events.
// Entity/fixture ordering and how filter/manifold fields are filled can differ between a
// normal step (live Box2D callbacks) and recovery after a world rebuild; see
// internal/contact_flush.go.
type ContactEventPayload struct {
	FilterA     FixtureFilterBits `json:"filter_a"`
	FilterB     FixtureFilterBits `json:"filter_b"`
	EntityA     cardinal.EntityID `json:"entity_a"`
	EntityB     cardinal.EntityID `json:"entity_b"`
	ShapeIndexA int               `json:"shape_index_a"`
	ShapeIndexB int               `json:"shape_index_b"`
	Normal      component.Vec2    `json:"normal"`
	NormalValid bool              `json:"normal_valid"`
	Point       component.Vec2    `json:"point"`
	PointValid  bool              `json:"point_valid"`
}

// ContactBeginEvent is emitted after the physics step when two non-sensor fixtures begin touching.
// Normal and Point are populated when the collision manifold has at least one point (see NormalValid / PointValid).
type ContactBeginEvent struct {
	ContactEventPayload
}

func (ContactBeginEvent) Name() string { return "physics2d_contact_begin" }

// ContactEndEvent is emitted after the physics step when two non-sensor fixtures stop touching.
// Manifold data is usually unavailable for EndContact; NormalValid/PointValid are typically false.
type ContactEndEvent struct {
	ContactEventPayload
}

func (ContactEndEvent) Name() string { return "physics2d_contact_end" }

// TriggerBeginEvent is emitted after the physics step when an overlap involving at least one sensor begins.
type TriggerBeginEvent struct {
	ContactEventPayload
}

func (TriggerBeginEvent) Name() string { return "physics2d_trigger_begin" }

// TriggerEndEvent is emitted after the physics step when an overlap involving at least one sensor ends.
type TriggerEndEvent struct {
	ContactEventPayload
}

func (TriggerEndEvent) Name() string { return "physics2d_trigger_end" }

// ContactEventEmitter is the per-step sink for flushed physics contact/trigger events. The physics
// step driver assigns an implementation (PhysicsRuntime.Emitter) before World.Step and calls
// FlushBufferedContacts after the step.
type ContactEventEmitter interface {
	EmitContactBegin(ContactBeginEvent)
	EmitContactEnd(ContactEndEvent)
	EmitTriggerBegin(TriggerBeginEvent)
	EmitTriggerEnd(TriggerEndEvent)
}
