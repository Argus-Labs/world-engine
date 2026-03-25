package component

import "fmt"

// BodyType selects how the rigid body participates in the simulation.
type BodyType uint8

const (
	// BodyTypeStatic is immovable world geometry; zero velocity; does not respond to forces.
	BodyTypeStatic BodyType = iota + 1
	// BodyTypeDynamic is fully simulated: forces, collisions, and integration apply.
	BodyTypeDynamic
	// BodyTypeKinematic is moved by setting velocity/transform from gameplay; does not respond
	// to forces like a dynamic body but can affect dynamic bodies on contact.
	BodyTypeKinematic
)

// Rigidbody2D holds simulation parameters for a rigid body.
//
// BodyType selects static vs dynamic vs kinematic behavior. LinearDamping and AngularDamping
// are simulation damping coefficients. GravityScale multiplies the world's gravity vector
// for this body; world gravity itself is runtime configuration, not a component field.
type Rigidbody2D struct {
	BodyType       BodyType `json:"body_type"`
	LinearDamping  float64  `json:"linear_damping"`
	AngularDamping float64  `json:"angular_damping"`
	GravityScale   float64  `json:"gravity_scale"`
}

// Name returns the ECS component name.
func (Rigidbody2D) Name() string { return "rigidbody_2d" }

// Validate guards against NaN/Inf in float fields and an invalid body type tag.
func (r Rigidbody2D) Validate() error {
	switch r.BodyType {
	case BodyTypeStatic, BodyTypeDynamic, BodyTypeKinematic:
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
