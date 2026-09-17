package component

import "fmt"

// ShapeCommon is the part of a shape every geometry kind shares: sensor flag, material and
// collision filter. A shape entity carries exactly one ShapeCommon and exactly one geometry
// component ([CircleGeom], [BoxGeom], [PolygonGeom], [ChainGeom], [EdgeGeom] or [CapsuleGeom]).
// Bodies reference the entity from their [ShapeSlot]s.
//
// Games change a shape through the shape searches (Fork), which spawn a changed copy rather
// than editing in place, so a shared shape is never changed under another body. The plugin
// still reads shape entities every tick, so any change to one reaches its fixtures next tick:
// material or filter in place, geometry or IsSensor by rebuild.
//
// A shape entity lives exactly as long as some body names it: the plugin deletes it after
// the first reconcile in which no body does, including a shape spawned with no body. Spawn a
// shape in the same tick as the first body that uses it, and never hold a slot that no body
// holds — keep a ShapeDef and spawn from it instead.
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
