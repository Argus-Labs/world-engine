package component

import (
	"errors"
	"fmt"

	"github.com/argus-labs/world-engine/pkg/immutable"
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
// disturbs it and SleepingAllowed is true. The plugin mirrors solver sleep state back into
// this field after each step, and wakes the body when you change another param without
// touching Awake. A rebuild (snapshot restore, Plugin.Reset) wakes bodies that had active
// contacts; others keep their Awake value.
//
// SleepingAllowed controls whether Box2D is permitted to put the body to sleep when it comes
// to rest. When false, the body stays awake indefinitely.
//
// Note: Box2D v3 processes sensors in a separate overlap pass that runs regardless of body
// sleep state (only disabled bodies are skipped). Sensor contacts are not affected by sleeping.
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
// Shapes lists the body's shapes. Each [ShapeRef] names a shape entity (which carries
// [ShapeCommon] plus one geometry component) and places it in body space. Index i is
// fixture i, the index contact events and query hits report. The list is an immutable.Slice:
// derive a new one (With, Append, Sub) and Set the body to change it. Or tag the shapes and
// edit them by name: AddShape, ReplaceShape and RemoveShape return the changed body and fail
// when the tag is already used, or missing, as the case may be; ShapeIndex and ShapeTag go
// between a tag and the index events report.
//
// # Defaults
//
// Box2D defaults Active, Awake, and SleepingAllowed to true and GravityScale to 1. Use
// [NewPhysicsBody2D] to create a PhysicsBody2D with these defaults set correctly. Bare struct
// literals leave bool fields at false and GravityScale at 0, which produces an inactive,
// sleeping body with no gravity — almost never what you want.
//
// When deserializing from JSON, missing fields are defaulted to their Box2D values automatically
// via a custom UnmarshalJSON. Explicitly serialized false values are preserved exactly. Snapshots
// go through MarshalWire/UnmarshalWire (protobuf) rather than JSON: JSON is for hand-written
// payloads and for logging, not for round-tripping a body the world is holding. (A Slice field such
// as Shapes or ChainGeom.Points encodes as a plain JSON array — see immutable.Slice.)
//
// Bullet and FixedRotation default to false (off), matching Box2D defaults.
//
// # Post-step writeback
//
// Writeback applies to dynamic and kinematic bodies: position, rotation, velocity and Awake.
// Static and manual bodies are not written back.
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

	Shapes immutable.Slice[ShapeRef] `json:"shapes"`
}

// NewPhysicsBody2D returns a PhysicsBody2D with the given body type, Box2D-compatible defaults
// (Active=true, Awake=true, SleepingAllowed=true, GravityScale=1), and the provided shapes.
func NewPhysicsBody2D(bodyType BodyType, shapes ...ShapeRef) PhysicsBody2D {
	return PhysicsBody2D{
		BodyType:        bodyType,
		GravityScale:    1,
		Active:          true,
		Awake:           true,
		SleepingAllowed: true,
		Shapes:          immutable.SliceOf(shapes...),
	}
}

// UnmarshalJSON decodes a PhysicsBody2D from JSON, applying Box2D-compatible defaults for
// fields missing from the payload. This handles old snapshots that predate the body flags
// (Active, Awake, SleepingAllowed default to true; GravityScale defaults to 1) while
// preserving explicitly serialized values including false.
func (p *PhysicsBody2D) UnmarshalJSON(data []byte) error {
	type raw struct {
		BodyType        BodyType   `json:"body_type"`
		LinearDamping   float64    `json:"linear_damping"`
		AngularDamping  float64    `json:"angular_damping"`
		GravityScale    *float64   `json:"gravity_scale"`
		Active          *bool      `json:"active"`
		Awake           *bool      `json:"awake"`
		SleepingAllowed *bool      `json:"sleeping_allowed"`
		Bullet          bool       `json:"bullet"`
		FixedRotation   bool       `json:"fixed_rotation"`
		Shapes          []ShapeRef `json:"shapes"`
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
		Shapes:          immutable.SliceOf(aux.Shapes...),
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
	if p.Shapes.Len() == 0 {
		return errors.New("physics_body_2d.shapes: at least one shape is required")
	}
	for i, s := range p.Shapes.All() {
		if err := s.Validate(); err != nil {
			return fmt.Errorf("physics_body_2d.shapes[%d]: %w", i, err)
		}
		if s.Tag != "" && p.ShapeIndex(s.Tag) != i {
			return fmt.Errorf("physics_body_2d.shapes[%d]: tag %q is already used by another shape", i, s.Tag)
		}
	}
	return nil
}

// ShapeIndex returns the index of the shape tagged tag, or -1. That index is the one contact
// events and query hits report for it.
func (p PhysicsBody2D) ShapeIndex(tag string) int {
	if tag == "" {
		return -1
	}
	return p.Shapes.IndexFunc(func(s ShapeRef) bool { return s.Tag == tag })
}

// ShapeTag returns the tag of shape i, or "" when i is out of range or the shape is untagged.
func (p PhysicsBody2D) ShapeTag(i int) string {
	if i < 0 || i >= p.Shapes.Len() {
		return ""
	}
	return p.Shapes.At(i).Tag
}

// AddShape appends shape under tag and returns the changed body. It fails when tag is already
// used on this body. An empty tag adds an untagged shape. Set the returned body to apply it.
func (p PhysicsBody2D) AddShape(tag string, shape ShapeRef) (PhysicsBody2D, error) {
	if p.ShapeIndex(tag) >= 0 {
		return p, fmt.Errorf("physics_body_2d: shape tag %q is already used", tag)
	}
	shape.Tag = tag
	p.Shapes = p.Shapes.Append(shape)
	return p, nil
}

// ReplaceShape swaps the shape tagged tag for shape, at the same index, and returns the
// changed body. Keeping the index keeps the fixture, so a same-geometry replacement updates
// in place. It fails when no shape has that tag. Set the returned body to apply it; the
// receiver, and the component it was read from, are untouched.
func (p PhysicsBody2D) ReplaceShape(tag string, shape ShapeRef) (PhysicsBody2D, error) {
	i := p.ShapeIndex(tag)
	if i < 0 {
		return p, fmt.Errorf("physics_body_2d: no shape tagged %q", tag)
	}
	shape.Tag = tag
	// With writes through its array, which a body read from ECS shares with the stored
	// component. Copy first so only the returned body changes.
	p.Shapes = immutable.Collect(p.Shapes.Values()).With(i, shape)
	return p, nil
}

// RemoveShape drops the shape tagged tag and returns the changed body. Shapes after it move
// down one index. It fails when no shape has that tag. Set the returned body to apply it; the
// receiver, and the component it was read from, are untouched.
func (p PhysicsBody2D) RemoveShape(tag string) (PhysicsBody2D, error) {
	i := p.ShapeIndex(tag)
	if i < 0 {
		return p, fmt.Errorf("physics_body_2d: no shape tagged %q", tag)
	}
	// Without shifts and zero-fills through its array; see ReplaceShape.
	p.Shapes = immutable.Collect(p.Shapes.Values()).Without(i)
	return p, nil
}
