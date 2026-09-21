package component

import "fmt"

// ShapeCommon is the part of a shape every geometry kind shares: sensor flag and material.
// The collision filter is not here: it is per body, on the [ShapeRef]. A shape entity carries
// exactly one ShapeCommon and exactly one geometry component ([CircleGeom], [BoxGeom],
// [PolygonGeom], [ChainGeom], [EdgeGeom] or [CapsuleGeom]). Bodies reference the entity from
// their [ShapeRef]s.
//
// Games change a shape through the Shapes search (Fork), which spawns a changed copy rather
// than editing in place, so a shared shape is never changed under another body. The plugin
// still reads shape entities every tick, so any change to one reaches its fixtures next tick:
// material or filter in place, geometry or IsSensor by rebuild.
//
// A shape entity lives exactly as long as some body names it: the plugin deletes it after
// the first reconcile in which no body does, including a shape spawned with no body. Spawn a
// shape in the same tick as the first body that uses it, and never hold a slot that no body
// holds — keep a Shape value and spawn from it instead.
type ShapeCommon struct {
	IsSensor    bool    `json:"is_sensor"`
	Friction    float64 `json:"friction"`
	Restitution float64 `json:"restitution"`
	Density     float64 `json:"density"`
}

// Name returns the ECS component name.
func (ShapeCommon) Name() string { return "shape_common_2d" }

// DefaultShapeCommon returns Box2D's defaults: solid, friction 0.6, restitution 0, density 1.
func DefaultShapeCommon() ShapeCommon {
	return ShapeCommon{Friction: 0.6, Density: 1}
}

// Validate checks the material fields for NaN/Inf and negatives. Box2D asserts a non-negative
// friction, restitution and density on every create and setter path, so a negative one would
// be a tick panic instead of an error at the line that built it.
func (s ShapeCommon) Validate() error {
	if !isFinite(s.Friction) || s.Friction < 0 {
		return fmt.Errorf("friction: must be finite and non-negative, got %v", s.Friction)
	}
	if !isFinite(s.Restitution) || s.Restitution < 0 {
		return fmt.Errorf("restitution: must be finite and non-negative, got %v", s.Restitution)
	}
	if !isFinite(s.Density) || s.Density < 0 {
		return fmt.Errorf("density: must be finite and non-negative, got %v", s.Density)
	}
	return nil
}
