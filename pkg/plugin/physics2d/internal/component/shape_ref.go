package component

import (
	"errors"

	"github.com/argus-labs/world-engine/pkg/cardinal"
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
	// layer. Zero CategoryBits or MaskBits means Box2D's default (category 1, mask all);
	// GroupIndex 0 means no group. Read them through FilterBits.
	CategoryBits uint64 `json:"category_bits,omitempty"`
	MaskBits     uint64 `json:"mask_bits,omitempty"`
	GroupIndex   int32  `json:"group_index,omitempty"`
}

// Ref references a shape entity at the body origin. Chain At to place it.
func Ref(shape cardinal.EntityID) ShapeRef { return ShapeRef{Shape: shape} }

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

// FilterBits returns the collision filter with defaults applied: category 1 and mask all
// where the fields are zero.
func (s ShapeRef) FilterBits() (category, mask uint64, group int32) {
	category, mask, group = s.CategoryBits, s.MaskBits, s.GroupIndex
	if category == 0 {
		category = 1
	}
	if mask == 0 {
		mask = ^uint64(0)
	}
	return category, mask, group
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
