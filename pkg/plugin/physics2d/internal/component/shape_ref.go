package component

import (
	"errors"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/goccy/go-json"
)

// ShapeRef is a shape on a body: which shape entity, and where it sits in body space. The
// shape itself (geometry, material, filter) lives on its own entity — see [ShapeCommon] and
// the geometry components — so any number of bodies can share one shape.
//
// Index i in PhysicsBody2D.Shapes is fixture i: contact events and query hits report that
// index. Tag names the shape so a body can be edited without knowing the index. Set it
// through PhysicsBody2D.AddShape, which refuses a tag the body already uses; empty means
// untagged.
type ShapeRef struct {
	Shape         cardinal.EntityID `json:"shape"`
	LocalOffset   Vec2              `json:"local_offset"`
	LocalRotation float64           `json:"local_rotation"`
	Tag           string            `json:"tag,omitempty"`

	// Collision filter, per body rather than per shape, so one shape serves every team or
	// layer. Plain Box2D semantics: two shapes collide when each one's category overlaps the
	// other's mask, so a zero category or mask collides with nothing. Ref sets Box2D's
	// defaults (category 1, mask all); a bare struct literal gets zeros, like a bare
	// PhysicsBody2D literal gets an inactive body.
	CategoryBits uint64 `json:"category_bits"`
	MaskBits     uint64 `json:"mask_bits"`
	GroupIndex   int32  `json:"group_index,omitempty"`
}

// Ref references a shape entity at the body origin with Box2D's default filter (category 1,
// mask all). Chain At to place it and Filter to change the filter.
func Ref(shape cardinal.EntityID) ShapeRef {
	return ShapeRef{Shape: shape, CategoryBits: 1, MaskBits: ^uint64(0)}
}

// UnmarshalJSON decodes a ref, defaulting a missing category or mask to Box2D's defaults so
// a hand-written {"shape": 7} collides normally. An explicit 0 is kept.
func (s *ShapeRef) UnmarshalJSON(data []byte) error {
	type raw struct {
		Shape         cardinal.EntityID `json:"shape"`
		LocalOffset   Vec2              `json:"local_offset"`
		LocalRotation float64           `json:"local_rotation"`
		Tag           string            `json:"tag"`
		CategoryBits  *uint64           `json:"category_bits"`
		MaskBits      *uint64           `json:"mask_bits"`
		GroupIndex    int32             `json:"group_index"`
	}
	var aux raw
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*s = Ref(aux.Shape).At(aux.LocalOffset, aux.LocalRotation)
	s.Tag, s.GroupIndex = aux.Tag, aux.GroupIndex
	if aux.CategoryBits != nil {
		s.CategoryBits = *aux.CategoryBits
	}
	if aux.MaskBits != nil {
		s.MaskBits = *aux.MaskBits
	}
	return nil
}

// At places the shape at offset and rotation (radians) in body space.
func (s ShapeRef) At(offset Vec2, rotation float64) ShapeRef {
	s.LocalOffset, s.LocalRotation = offset, rotation
	return s
}

// Filter sets the collision category and mask bits.
func (s ShapeRef) Filter(category, mask uint64) ShapeRef {
	s.CategoryBits, s.MaskBits = category, mask
	return s
}

// Group sets the Box2D group index: shapes sharing a positive index always collide, a
// negative one never.
func (s ShapeRef) Group(index int32) ShapeRef {
	s.GroupIndex = index
	return s
}

// Validate checks the local transform for NaN/Inf. Whether Shape resolves to a live shape
// entity is checked at fixture attach, not here.
func (s ShapeRef) Validate() error {
	if err := validateVec2("local_offset", s.LocalOffset); err != nil {
		return err
	}
	if !isFinite(s.LocalRotation) {
		return errors.New("local_rotation: must be finite")
	}
	return nil
}
