package component

import "errors"

// Vec2 is a 2D vector in world space, used by physics-facing components and APIs.
type Vec2 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Transform2D is the authoritative world-space pose of an entity.
//
// Position is the world-space origin of the body transform. Rotation is the world-space
// orientation in radians (see package doc for handedness and sign).
type Transform2D struct {
	Position Vec2    `json:"position"`
	Rotation float64 `json:"rotation"`
}

// Name returns the ECS component name.
func (Transform2D) Name() string { return "transform_2d" }

// Validate checks that the transform uses finite scalars only.
func (t Transform2D) Validate() error {
	if err := validateVec2("transform_2d.position", t.Position); err != nil {
		return err
	}
	if !isFinite(t.Rotation) {
		return errors.New("transform_2d.rotation: must be finite")
	}
	return nil
}

// Velocity2D is the authoritative linear and angular motion state.
//
// Linear is world-space linear velocity. Angular is angular velocity in radians per second
// about the axis perpendicular to the XY plane (required for rotating rigid bodies and
// oriented colliders).
type Velocity2D struct {
	Linear  Vec2    `json:"linear"`
	Angular float64 `json:"angular"`
}

// Name returns the ECS component name.
func (Velocity2D) Name() string { return "velocity_2d" }

// Validate checks that linear and angular velocity are finite.
func (v Velocity2D) Validate() error {
	if err := validateVec2("velocity_2d.linear", v.Linear); err != nil {
		return err
	}
	if !isFinite(v.Angular) {
		return errors.New("velocity_2d.angular: must be finite")
	}
	return nil
}
