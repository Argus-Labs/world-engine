//nolint:recvcheck // UnmarshalJSON must be pointer receiver to support json.Unmarshal
package component

import (
	"errors"
	"fmt"

	"github.com/goccy/go-json"
)

// PhysicsBody2D holds simulation parameters for a rigid body and its collider shapes.
//
// BodyType selects static vs dynamic vs kinematic behavior. LinearDamping and AngularDamping
// are simulation damping coefficients. GravityScale multiplies the world's gravity vector
// for this body; world gravity itself is runtime configuration, not a component field.
//
// # Body flags
//
// Active controls whether the body participates in the simulation at all. An inactive body
// has no contacts, no collisions, and is effectively removed from Box2D without destroying it.
// Set Active=false to temporarily disable an entity's physics (e.g. a dormant trap).
//
// Awake controls whether the body is currently awake in the simulation. Setting Awake=true
// wakes a sleeping body; Box2D may put it back to sleep on subsequent ticks if nothing
// disturbs it and SleepingAllowed is true. To keep a body permanently awake (e.g. a
// stationary kinematic sensor that must always generate contacts), set SleepingAllowed=false
// instead.
//
// SleepingAllowed controls whether Box2D is permitted to put the body to sleep when it comes
// to rest. When false, the body stays awake indefinitely. Use this for kinematic/manual
// bodies that are stationary but must still generate contacts (the common "sensor" pattern).
//
// Bullet enables continuous collision detection (CCD) for fast-moving dynamic bodies to
// prevent tunneling through thin geometry. Has a performance cost; only enable for
// projectiles or similarly fast objects.
//
// FixedRotation prevents the body from rotating in response to torques or collisions.
// Useful for top-down characters that should not spin.
//
// # Shapes
//
// Shapes holds the compound collider description. Cardinal allows one instance per component
// type per entity, so compound colliders are modeled as multiple ColliderShape entries.
// Shape identity (v1): index i in Shapes identifies fixture slot i.
//
// # Defaults
//
// Box2D defaults Active, Awake, and SleepingAllowed to true and GravityScale to 1. Use
// [NewPhysicsBody2D] to create a PhysicsBody2D with these defaults set correctly. Bare struct
// literals leave bool fields at false and GravityScale at 0, which produces an inactive,
// sleeping body with no gravity — almost never what you want.
//
// When deserializing from JSON (e.g. snapshot recovery), missing fields are defaulted to
// their Box2D values automatically via a custom UnmarshalJSON. Explicitly serialized false
// values are preserved exactly.
//
// Bullet and FixedRotation default to false (off), matching Box2D defaults.
//
// # Post-step writeback
//
// Writeback applies to dynamic and kinematic bodies. Static and manual bodies are
// not written back.
type PhysicsBody2D struct {
	BodyType        BodyType `json:"body_type"`
	LinearDamping   float64  `json:"linear_damping"`
	AngularDamping  float64  `json:"angular_damping"`
	GravityScale    float64  `json:"gravity_scale"`
	Active          bool     `json:"active"`
	Awake           bool     `json:"awake"`
	SleepingAllowed bool     `json:"sleeping_allowed"`
	Bullet          bool     `json:"bullet"`
	FixedRotation   bool     `json:"fixed_rotation"`

	Shapes []ColliderShape `json:"shapes"`
}

// NewPhysicsBody2D returns a PhysicsBody2D with the given body type, Box2D-compatible defaults
// (Active=true, Awake=true, SleepingAllowed=true, GravityScale=1), and the provided shapes.
func NewPhysicsBody2D(bodyType BodyType, shapes ...ColliderShape) PhysicsBody2D {
	return PhysicsBody2D{
		BodyType:        bodyType,
		GravityScale:    1,
		Active:          true,
		Awake:           true,
		SleepingAllowed: true,
		Shapes:          shapes,
	}
}

// UnmarshalJSON decodes a PhysicsBody2D from JSON, applying Box2D-compatible defaults for
// fields missing from the payload. This handles old snapshots that predate the body flags
// (Active, Awake, SleepingAllowed default to true; GravityScale defaults to 1) while
// preserving explicitly serialized values including false.
func (p *PhysicsBody2D) UnmarshalJSON(data []byte) error {
	type raw struct {
		BodyType        BodyType        `json:"body_type"`
		LinearDamping   float64         `json:"linear_damping"`
		AngularDamping  float64         `json:"angular_damping"`
		GravityScale    *float64        `json:"gravity_scale"`
		Active          *bool           `json:"active"`
		Awake           *bool           `json:"awake"`
		SleepingAllowed *bool           `json:"sleeping_allowed"`
		Bullet          bool            `json:"bullet"`
		FixedRotation   bool            `json:"fixed_rotation"`
		Shapes          []ColliderShape `json:"shapes"`
	}
	var aux raw
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*p = PhysicsBody2D{
		BodyType:        aux.BodyType,
		LinearDamping:   aux.LinearDamping,
		AngularDamping:  aux.AngularDamping,
		GravityScale:    1,
		Active:          true,
		Awake:           true,
		SleepingAllowed: true,
		Bullet:          aux.Bullet,
		FixedRotation:   aux.FixedRotation,
		Shapes:          aux.Shapes,
	}
	if aux.GravityScale != nil {
		p.GravityScale = *aux.GravityScale
	}
	if aux.Active != nil {
		p.Active = *aux.Active
	}
	if aux.Awake != nil {
		p.Awake = *aux.Awake
	}
	if aux.SleepingAllowed != nil {
		p.SleepingAllowed = *aux.SleepingAllowed
	}
	return nil
}

// Name returns the ECS component name.
func (PhysicsBody2D) Name() string { return "physics_body_2d" }

// Validate guards against NaN/Inf in float fields, an invalid body type tag, and invalid shapes.
func (p PhysicsBody2D) Validate() error {
	switch p.BodyType {
	case BodyTypeStatic, BodyTypeDynamic, BodyTypeKinematic, BodyTypeManual:
	default:
		return fmt.Errorf("physics_body_2d.body_type: invalid value %d", p.BodyType)
	}
	if !isFinite(p.LinearDamping) {
		return fmt.Errorf("physics_body_2d.linear_damping: must be finite, got %v", p.LinearDamping)
	}
	if !isFinite(p.AngularDamping) {
		return fmt.Errorf("physics_body_2d.angular_damping: must be finite, got %v", p.AngularDamping)
	}
	if !isFinite(p.GravityScale) {
		return fmt.Errorf("physics_body_2d.gravity_scale: must be finite, got %v", p.GravityScale)
	}
	if len(p.Shapes) == 0 {
		return errors.New("physics_body_2d.shapes: at least one ColliderShape is required")
	}
	for i := range p.Shapes {
		if err := p.Shapes[i].Validate(); err != nil {
			return fmt.Errorf("physics_body_2d.shapes[%d]: %w", i, err)
		}
	}
	return nil
}
