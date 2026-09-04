package component

import "fmt"

// ChainGeometry2D holds the polyline for chain-type collider shapes, on its own entity so any
// number of colliders can share it by id (ColliderShape.ChainGeometry).
//
// Points are immutable once referenced: the reconciler diffs the id, not the points, so to
// change terrain spawn a new geometry entity and swap the id. A dangling id fails that body's
// reconcile loudly. Cleanup is automatic: once referenced, the entity is deleted on the tick
// its last reference disappears — never cache a geometry id across a moment when nothing
// references it.
type ChainGeometry2D struct {
	Points []Vec2 `json:"points"`
}

// Name returns the ECS component name.
func (ChainGeometry2D) Name() string { return "chain_geometry_2d" }

// Validate checks every point for NaN/Inf. Point-count rules (Box2D requires at least 4 for a
// chain) are enforced by the backend at fixture creation, matching the other shape types.
func (g ChainGeometry2D) Validate() error {
	for i, v := range g.Points {
		if err := validateVec2(fmt.Sprintf("points[%d]", i), v); err != nil {
			return err
		}
	}
	return nil
}
