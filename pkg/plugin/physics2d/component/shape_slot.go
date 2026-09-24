package component

import (
	"errors"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

// ShapeSlot is one fixture slot on a PhysicsBody2D: which shape entity to use and where it
// sits in body space. The shape itself (geometry, material, filter) lives on its own entity —
// see [ShapeCommon] and the geometry components — so any number of bodies can share one shape.
//
// Slot index i is fixture i: contact events and query hits report that index, so don't
// reorder slots after creation if you care about per-shape references.
type ShapeSlot struct {
	Shape         cardinal.EntityID `json:"shape"`
	LocalOffset   Vec2              `json:"local_offset"`
	LocalRotation float64           `json:"local_rotation"`
}

// Slot references a shape entity at the body origin. Chain At to place it.
func Slot(shape cardinal.EntityID) ShapeSlot { return ShapeSlot{Shape: shape} }

// At places the shape at offset and rotation (radians) in body space.
func (s ShapeSlot) At(offset Vec2, rotation float64) ShapeSlot {
	s.LocalOffset, s.LocalRotation = offset, rotation
	return s
}

// Validate checks the local transform for NaN/Inf. Whether Shape resolves to a live shape
// entity is checked at fixture attach, not here.
func (s ShapeSlot) Validate() error {
	if err := validateVec2("local_offset", s.LocalOffset); err != nil {
		return err
	}
	if !isFinite(s.LocalRotation) {
		return errors.New("local_rotation: must be finite")
	}
	return nil
}
