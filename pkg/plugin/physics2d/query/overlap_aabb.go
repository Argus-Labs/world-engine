package query

import (
	"cmp"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
)

// AABBOverlapRequest finds fixtures whose shapes overlap the axis-aligned box [Min, Max] in world space
// (inclusive bounds on the query box). Min.X may be greater than Max.X; components are swapped per axis.
type AABBOverlapRequest struct {
	Min    component.Vec2 `json:"min"`
	Max    component.Vec2 `json:"max"`
	Filter *Filter        `json:"filter,omitempty"`
	// Ignore skips these entities. It is scanned once per candidate shape, so it is sized for
	// "not me, not my vehicle"; exclude a whole layer or team with Filter instead.
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

// Entities returns the distinct entities in Hits, in the order they first appear. A body whose
// several shapes all overlap the box appears once, which is what a caller asking "who is in
// here" wants; Hits stays available for anyone who needs the per-shape detail.
//
// A result from OverlapAABB arrives grouped by entity, so comparing neighbours collapses it in
// one pass. Hits is an ordinary exported field though, so a result built or reordered by hand
// need not be grouped; that case costs a scan per distinct entity rather than returning the
// duplicates the grouped pass would miss.
func (r AABBOverlapResult) Entities() []cardinal.EntityID {
	if len(r.Hits) == 0 {
		return nil
	}
	grouped := slices.IsSortedFunc(r.Hits, func(a, b AABBOverlapHit) int {
		return cmp.Compare(a.Entity, b.Entity)
	})
	out := make([]cardinal.EntityID, 0, len(r.Hits))
	for _, h := range r.Hits {
		if len(out) > 0 && out[len(out)-1] == h.Entity {
			continue
		}
		if !grouped && slices.Contains(out, h.Entity) {
			continue
		}
		out = append(out, h.Entity)
	}
	return out
}
