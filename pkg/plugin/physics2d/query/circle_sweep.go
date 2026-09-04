package query

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// CircleSweepRequest sweeps a circle with center moving along the segment from Start to End in world space.
// Radius must be positive. MaxFraction is the TOI search bound in [0,1] along that segment; 0 means 1.0.
// A nil Filter uses the same defaults as RaycastRequest (all layers, solids only).
type CircleSweepRequest struct {
	Start       component.Vec2 `json:"start"`
	End         component.Vec2 `json:"end"`
	Radius      float64        `json:"radius"`
	Filter      *Filter        `json:"filter,omitempty"`
	MaxFraction float64        `json:"max_fraction"`
}

// CircleSweepResult is the closest first contact along the sweep, if any.
type CircleSweepResult struct {
	Hit        bool              `json:"hit"`
	Entity     cardinal.EntityID `json:"entity"`
	ShapeIndex int               `json:"shape_index"`
	Point      component.Vec2    `json:"point"`
	Normal     component.Vec2    `json:"normal"`
	Fraction   float64           `json:"fraction"`
}
