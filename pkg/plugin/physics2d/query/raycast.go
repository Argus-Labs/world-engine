// Package query declares the request, result, and filter types for the physics2d query API
// (raycast, AABB overlap, circle sweep). Execution lives on the plugin runtime; call the
// methods on physics2d.Plugin.
package query

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// RaycastRequest is a world-space segment cast from Origin toward End (inclusive segment; hit
// fraction is in [0,1] along Origin->End). The ray must have non-zero length.
type RaycastRequest struct {
	Origin component.Vec2 `json:"origin"`
	End    component.Vec2 `json:"end"`
	Filter *Filter        `json:"filter,omitempty"`
}

// RaycastResult is the closest hit along the segment, if any. When Hit is false, other fields are zero.
type RaycastResult struct {
	Hit        bool              `json:"hit"`
	Entity     cardinal.EntityID `json:"entity"`
	ShapeIndex int               `json:"shape_index"`
	Point      component.Vec2    `json:"point"`
	Normal     component.Vec2    `json:"normal"`
	Fraction   float64           `json:"fraction"`
}
