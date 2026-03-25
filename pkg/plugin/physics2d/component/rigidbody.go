//nolint:recvcheck // UnmarshalJSON must be pointer receiver to support json.Unmarshal
package component

import (
	"fmt"

	"github.com/goccy/go-json"
)

// BodyType selects how the rigid body participates in the simulation.
type BodyType uint8

const (
	// BodyTypeStatic is immovable world geometry; zero velocity; does not respond to forces.
	BodyTypeStatic BodyType = iota + 1
	// BodyTypeDynamic is fully simulated: forces, collisions, and integration apply.
	BodyTypeDynamic
	// BodyTypeKinematic is moved by setting velocity from gameplay; Box2D integrates velocity
	// into position each step. Does not respond to forces but can push dynamic bodies on
	// contact. Post-step writeback keeps ECS in sync with Box2D's integrated position.
	BodyTypeKinematic
	// BodyTypeManual is for gameplay-driven entities that use Box2D only for contact detection.
	// Under the hood it creates a kinematic body, but post-step writeback is skipped: ECS owns
	// position and velocity, and the reconciler pushes ECS values into Box2D each tick.
	// Use this for characters, enemies, and other entities where gameplay code (input handling,
	// AI, pathfinding) computes position directly.
	//
	// Box2D collision rules apply: manual bodies generate contacts with dynamic bodies only,
	// not with static or other kinematic/manual bodies.
	BodyTypeManual
)

// Rigidbody2D holds simulation parameters for a rigid body.
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
// # Defaults
//
// Box2D defaults Active, Awake, and SleepingAllowed to true and GravityScale to 1. Use
// [NewRigidbody2D] to create a Rigidbody2D with these defaults set correctly. Bare struct
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
type Rigidbody2D struct {
	BodyType        BodyType `json:"body_type"`
	LinearDamping   float64  `json:"linear_damping"`
	AngularDamping  float64  `json:"angular_damping"`
	GravityScale    float64  `json:"gravity_scale"`
	Active          bool     `json:"active"`
	Awake           bool     `json:"awake"`
	SleepingAllowed bool     `json:"sleeping_allowed"`
	Bullet          bool     `json:"bullet"`
	FixedRotation   bool     `json:"fixed_rotation"`
}

// NewRigidbody2D returns a Rigidbody2D with the given body type and Box2D-compatible defaults:
// Active=true, Awake=true, SleepingAllowed=true, GravityScale=1.
func NewRigidbody2D(bodyType BodyType) Rigidbody2D {
	return Rigidbody2D{
		BodyType:        bodyType,
		GravityScale:    1,
		Active:          true,
		Awake:           true,
		SleepingAllowed: true,
	}
}

// UnmarshalJSON decodes a Rigidbody2D from JSON, applying Box2D-compatible defaults for
// fields missing from the payload. This handles old snapshots that predate the body flags
// (Active, Awake, SleepingAllowed default to true; GravityScale defaults to 1) while
// preserving explicitly serialized values including false.
func (r *Rigidbody2D) UnmarshalJSON(data []byte) error {
	type raw struct {
		BodyType        BodyType `json:"body_type"`
		LinearDamping   float64  `json:"linear_damping"`
		AngularDamping  float64  `json:"angular_damping"`
		GravityScale    *float64 `json:"gravity_scale"`
		Active          *bool    `json:"active"`
		Awake           *bool    `json:"awake"`
		SleepingAllowed *bool    `json:"sleeping_allowed"`
		Bullet          bool     `json:"bullet"`
		FixedRotation   bool     `json:"fixed_rotation"`
	}
	var aux raw
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*r = Rigidbody2D{
		BodyType:        aux.BodyType,
		LinearDamping:   aux.LinearDamping,
		AngularDamping:  aux.AngularDamping,
		GravityScale:    1,
		Active:          true,
		Awake:           true,
		SleepingAllowed: true,
		Bullet:          aux.Bullet,
		FixedRotation:   aux.FixedRotation,
	}
	if aux.GravityScale != nil {
		r.GravityScale = *aux.GravityScale
	}
	if aux.Active != nil {
		r.Active = *aux.Active
	}
	if aux.Awake != nil {
		r.Awake = *aux.Awake
	}
	if aux.SleepingAllowed != nil {
		r.SleepingAllowed = *aux.SleepingAllowed
	}
	return nil
}

// Name returns the ECS component name.
func (Rigidbody2D) Name() string { return "rigidbody_2d" }

// Validate guards against NaN/Inf in float fields and an invalid body type tag.
func (r Rigidbody2D) Validate() error {
	switch r.BodyType {
	case BodyTypeStatic, BodyTypeDynamic, BodyTypeKinematic, BodyTypeManual:
	default:
		return fmt.Errorf("rigidbody_2d.body_type: invalid value %d", r.BodyType)
	}
	if !isFinite(r.LinearDamping) {
		return fmt.Errorf("rigidbody_2d.linear_damping: must be finite, got %v", r.LinearDamping)
	}
	if !isFinite(r.AngularDamping) {
		return fmt.Errorf("rigidbody_2d.angular_damping: must be finite, got %v", r.AngularDamping)
	}
	if !isFinite(r.GravityScale) {
		return fmt.Errorf("rigidbody_2d.gravity_scale: must be finite, got %v", r.GravityScale)
	}
	return nil
}
