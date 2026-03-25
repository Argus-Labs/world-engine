package query

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/cbridge"
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

// CircleSweep sweeps a circle along Start->End and returns the earliest TOI hit.
// A zero-radius or zero-length sweep always returns no hit.
func CircleSweep(req CircleSweepRequest) CircleSweepResult {
	if req.Radius <= 0 {
		return CircleSweepResult{}
	}
	if req.Start.X == req.End.X && req.Start.Y == req.End.Y {
		return CircleSweepResult{}
	}
	cat := ^uint64(0)
	mask := ^uint64(0)
	includeSensors := false
	if req.Filter != nil {
		cat = req.Filter.CategoryBits
		mask = req.Filter.MaskBits
		includeSensors = req.Filter.IncludeSensors
	}

	maxFrac := req.MaxFraction
	if maxFrac <= 0 {
		maxFrac = 1
	}
	if maxFrac > 1 {
		maxFrac = 1
	}

	r := cbridge.CircleSweep(
		req.Start.X, req.Start.Y,
		req.End.X, req.End.Y,
		req.Radius,
		cat, mask, includeSensors,
		maxFrac,
	)
	if !r.Hit {
		return CircleSweepResult{}
	}
	return CircleSweepResult{
		Hit:        true,
		Entity:     cardinal.EntityID(r.EntityID),
		ShapeIndex: r.ShapeIndex,
		Point:      component.Vec2{X: r.PX, Y: r.PY},
		Normal:     component.Vec2{X: r.NX, Y: r.NY},
		Fraction:   r.Fraction,
	}
}
