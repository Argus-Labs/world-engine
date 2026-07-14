package query

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/cbridge"
)

// AABBOverlapRequest finds fixtures whose shapes overlap the axis-aligned box [Min, Max] in world space
// (inclusive bounds on the query box). Min.X may be greater than Max.X; components are swapped per axis.
type AABBOverlapRequest struct {
	Min    component.Vec2 `json:"min"`
	Max    component.Vec2 `json:"max"`
	Filter *Filter        `json:"filter,omitempty"`
}

// AABBOverlapHit is one ECS shape that overlaps the query AABB after narrow-phase test.
type AABBOverlapHit struct {
	Entity     cardinal.EntityID `json:"entity"`
	ShapeIndex int               `json:"shape_index"`
}

// AABBOverlapResult lists distinct (Entity, ShapeIndex) pairs that overlap the query box.
type AABBOverlapResult struct {
	Hits []AABBOverlapHit `json:"hits"`
}

// OverlapAABB returns distinct (Entity, ShapeIndex) pairs overlapping the query AABB.
// A zero-area AABB (Min == Max on any axis) always returns no hits.
func OverlapAABB(req AABBOverlapRequest) AABBOverlapResult {
	if req.Min.X == req.Max.X || req.Min.Y == req.Max.Y {
		return AABBOverlapResult{}
	}
	cat := ^uint64(0)
	mask := ^uint64(0)
	includeSensors := false
	if req.Filter != nil {
		cat = req.Filter.CategoryBits
		mask = req.Filter.MaskBits
		includeSensors = req.Filter.IncludeSensors
	}

	minX, maxX := req.Min.X, req.Max.X
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	minY, maxY := req.Min.Y, req.Max.Y
	if minY > maxY {
		minY, maxY = maxY, minY
	}

	hits := cbridge.OverlapAABB(minX, minY, maxX, maxY, cat, mask, includeSensors)

	out := AABBOverlapResult{
		Hits: make([]AABBOverlapHit, len(hits)),
	}
	for i, h := range hits {
		out.Hits[i] = AABBOverlapHit{
			Entity:     cardinal.EntityID(h.EntityID),
			ShapeIndex: h.ShapeIndex,
		}
	}
	return out
}
