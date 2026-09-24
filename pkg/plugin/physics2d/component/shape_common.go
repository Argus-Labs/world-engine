package component

import "fmt"

// ShapeCommon is the part of a shape every geometry kind shares: sensor flag, material and
// collision filter. A shape entity carries exactly one ShapeCommon and exactly one geometry
// component ([CircleGeom], [BoxGeom], [PolygonGeom], [ChainGeom], [EdgeGeom] or [CapsuleGeom]).
// Bodies reference the entity from their [ShapeSlot]s.
//
// The plugin reads shape entities every tick, so editing one in place works: a material or
// filter change updates the fixtures of every body using the shape, a geometry or IsSensor
// change rebuilds them.
//
// Cleanup is automatic: once a body has used a shape, the plugin deletes the shape entity on
// the tick its last such use goes away. A shape spawned but never used is left alone. So don't
// hold on to a shape id across a moment when no body uses it — spawn a new one instead.
type ShapeCommon struct {
	IsSensor     bool    `json:"is_sensor"`
	Friction     float64 `json:"friction"`
	Restitution  float64 `json:"restitution"`
	Density      float64 `json:"density"`
	CategoryBits uint64  `json:"category_bits"`
	MaskBits     uint64  `json:"mask_bits"`
	GroupIndex   int32   `json:"group_index,omitempty"`
}

// Name returns the ECS component name.
func (ShapeCommon) Name() string { return "shape_common_2d" }

// DefaultShapeCommon returns Box2D's defaults: solid, friction 0.6, restitution 0, density 1,
// category 1, mask all.
func DefaultShapeCommon() ShapeCommon {
	return ShapeCommon{
		Friction:     0.6,
		Density:      1,
		CategoryBits: 1,
		MaskBits:     ^uint64(0),
	}
}

// Validate checks the material fields for NaN/Inf.
func (s ShapeCommon) Validate() error {
	if !isFinite(s.Friction) {
		return fmt.Errorf("friction: must be finite, got %v", s.Friction)
	}
	if !isFinite(s.Restitution) {
		return fmt.Errorf("restitution: must be finite, got %v", s.Restitution)
	}
	if !isFinite(s.Density) {
		return fmt.Errorf("density: must be finite, got %v", s.Density)
	}
	return nil
}
