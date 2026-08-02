package query

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
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
