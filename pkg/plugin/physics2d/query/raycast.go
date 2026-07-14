package query

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/cbridge"
)

// Filter restricts which fixtures a query considers. A fixture passes when all of the
// following hold:
//   - (Filter.MaskBits & fixture.CategoryBits) != 0
//   - (Filter.CategoryBits & fixture.MaskBits) != 0
//   - if IncludeSensors is false, the fixture is not a sensor
//
// Fixture group index is not part of v1 query filtering; category/mask only.
//
// A nil *Filter uses CategoryBits and MaskBits ^uint64(0) and IncludeSensors false (solids only, all layers)
// for RaycastRequest, AABBOverlapRequest, and CircleSweepRequest.
type Filter struct {
	CategoryBits   uint64 `json:"category_bits"`
	MaskBits       uint64 `json:"mask_bits"`
	IncludeSensors bool   `json:"include_sensors"`
}

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

// Raycast returns the closest fixture hit along [req.Origin, req.End], or Hit=false.
// A zero-length segment (Origin == End) always returns no hit.
func Raycast(req RaycastRequest) RaycastResult {
	if req.Origin.X == req.End.X && req.Origin.Y == req.End.Y {
		return RaycastResult{}
	}
	cat := ^uint64(0)
	mask := ^uint64(0)
	includeSensors := false
	if req.Filter != nil {
		cat = req.Filter.CategoryBits
		mask = req.Filter.MaskBits
		includeSensors = req.Filter.IncludeSensors
	}

	r := cbridge.Raycast(
		req.Origin.X, req.Origin.Y,
		req.End.X, req.End.Y,
		cat, mask, includeSensors,
	)
	if !r.Hit {
		return RaycastResult{}
	}
	return RaycastResult{
		Hit:        true,
		Entity:     cardinal.EntityID(r.EntityID),
		ShapeIndex: r.ShapeIndex,
		Point:      component.Vec2{X: r.PX, Y: r.PY},
		Normal:     component.Vec2{X: r.NX, Y: r.NY},
		Fraction:   r.Fraction,
	}
}
