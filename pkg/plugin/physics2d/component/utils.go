package component

// Shared helpers for Validate methods on spatial, rigidbody, and collider types.

import (
	"fmt"
	"math"
)

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

func validateVec2(field string, v Vec2) error {
	if !isFinite(v.X) || !isFinite(v.Y) {
		return fmt.Errorf("%s: must be finite (got %v, %v)", field, v.X, v.Y)
	}
	return nil
}
