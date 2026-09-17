package query

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
)

// AABBOverlapRequest finds fixtures whose shapes overlap the axis-aligned box [Min, Max] in world space
// (inclusive bounds on the query box). Min.X may be greater than Max.X; components are swapped per axis.
type AABBOverlapRequest struct {
	Min    component.Vec2 `json:"min"`
	Max    component.Vec2 `json:"max"`
	Filter *Filter        `json:"filter,omitempty"`

	// Ignore lists entities this query must not report. Usually one entity: the caster, or a
	// projectile's owner.
	Ignore []cardinal.EntityID `json:"ignore,omitempty"`
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

// Entities returns the distinct entities in Hits, keeping Hits' order. A body whose several
// shapes all overlap the box appears once, which is what a caller asking "who is in here"
// wants; Hits stays available for anyone who needs the per-shape detail.
//
// Hits arrives sorted by (Entity, ShapeIndex), so one pass over neighbours is enough.
func (r AABBOverlapResult) Entities() []cardinal.EntityID {
	if len(r.Hits) == 0 {
		return nil
	}
	out := make([]cardinal.EntityID, 0, len(r.Hits))
	for _, h := range r.Hits {
		if len(out) == 0 || out[len(out)-1] != h.Entity {
			out = append(out, h.Entity)
		}
	}
	return out
}
