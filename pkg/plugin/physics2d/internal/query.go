package internal

import (
	"cmp"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/query"
)

// queryFilterBits resolves a request filter into (category, mask, includeSensors). A nil filter
// means all category/mask pairs match and sensors are skipped.
func queryFilterBits(f *query.Filter) (uint64, uint64, bool) {
	cat := ^uint64(0)
	mask := ^uint64(0)
	includeSensors := false
	if f != nil {
		cat = f.CategoryBits
		mask = f.MaskBits
		includeSensors = f.IncludeSensors
	}
	return cat, mask, includeSensors
}

// castHit accumulates the closest cast hit inside the CastRay/CastShape callbacks.
// Mirrors the CGO bridge's RaycastCtx/CircleSweepCtx closest-hit logic exactly: sensors
// are skipped by returning -1 when includeSensors is false, every other hit is recorded
// and its fraction returned so Box2D clips the remaining traversal.
//
// One instance lives on the Runtime (Runtime.castScratch) rather than on the
// stack: Box2D takes the callback context as `any`, so a stack local would
// escape and cost an allocation per query. Callers save the field, overwrite
// it, copy the result out and restore it, so a nested query is still correct.
// A Runtime, like the box2d.World it owns, is single-threaded.
type castHit struct {
	rt             *Runtime
	includeSensors bool

	hit        bool
	entityID   cardinal.EntityID
	shapeIndex int
	point      component.Vec2
	normal     component.Vec2
	fraction   float64
}

// castCallback implements box2d.CastResultFcn with the bridge's closest-hit semantics.
func castCallback(shapeID box2d.ShapeID, point, normal box2d.Vec2, fraction float64, ctx any) float64 {
	c, ok := ctx.(*castHit)
	if !ok {
		return 0
	}
	if !c.includeSensors && c.rt.World.IsShapeSensor(shapeID) {
		return -1 // skip sensors
	}
	entityID, shapeIndex := c.rt.shapeIdentity(shapeID)
	c.hit = true
	c.entityID = entityID
	c.shapeIndex = shapeIndex
	c.point = component.Vec2{X: point.X, Y: point.Y}
	c.normal = component.Vec2{X: normal.X, Y: normal.Y}
	c.fraction = fraction
	return fraction // keep searching for closer hits
}

// overlapCollector gathers the shapes reported by World.OverlapShape. It is the
// struct form of what used to be a capturing closure, so the callback can be a
// package-level function and the context a pointer into the Runtime
// (Runtime.overlapScratch) instead of a per-call heap allocation.
type overlapCollector struct {
	rt             *Runtime
	includeSensors bool
	hits           []query.AABBOverlapHit
}

// overlapCallback implements box2d.OverlapResultFcn for Runtime.OverlapAABB.
func overlapCallback(shapeID box2d.ShapeID, ctx any) bool {
	c, ok := ctx.(*overlapCollector)
	if !ok {
		return false
	}
	if !c.includeSensors && c.rt.World.IsShapeSensor(shapeID) {
		return true // skip sensor, continue
	}
	entityID, shapeIndex := c.rt.shapeIdentity(shapeID)
	c.hits = append(c.hits, query.AABBOverlapHit{Entity: entityID, ShapeIndex: shapeIndex})
	return true // continue
}

// Raycast returns the closest fixture hit along [req.Origin, req.End], or Hit=false.
// A zero-length segment (Origin == End) always returns no hit.
func (rt *Runtime) Raycast(req query.RaycastRequest) query.RaycastResult {
	if req.Origin.X == req.End.X && req.Origin.Y == req.End.Y {
		return query.RaycastResult{}
	}
	cat, mask, includeSensors := queryFilterBits(req.Filter)

	saved := rt.castScratch
	rt.castScratch = castHit{rt: rt, includeSensors: includeSensors}

	origin := box2d.Vec2{X: req.Origin.X, Y: req.Origin.Y}
	translation := box2d.Vec2{X: req.End.X - req.Origin.X, Y: req.End.Y - req.Origin.Y}
	rt.World.CastRay(origin, translation, box2d.QueryFilter{CategoryBits: cat, MaskBits: mask},
		castCallback, &rt.castScratch)

	ctx := rt.castScratch
	rt.castScratch = saved

	if !ctx.hit {
		return query.RaycastResult{}
	}
	return query.RaycastResult{
		Hit:        true,
		Entity:     ctx.entityID,
		ShapeIndex: ctx.shapeIndex,
		Point:      ctx.point,
		Normal:     ctx.normal,
		Fraction:   ctx.fraction,
	}
}

// OverlapAABB returns distinct (Entity, ShapeIndex) pairs overlapping the query AABB,
// sorted by (Entity, ShapeIndex). Broadphase traversal order is an implementation detail and
// was never part of the wrapper contract; sorting makes results deterministic.
// A zero-area AABB (Min == Max on any axis) always returns no hits.
func (rt *Runtime) OverlapAABB(req query.AABBOverlapRequest) query.AABBOverlapResult {
	if req.Min.X == req.Max.X || req.Min.Y == req.Max.Y {
		return query.AABBOverlapResult{}
	}
	cat, mask, includeSensors := queryFilterBits(req.Filter)

	minX, maxX := req.Min.X, req.Max.X
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	minY, maxY := req.Min.Y, req.Max.Y
	if minY > maxY {
		minY, maxY = maxY, minY
	}

	// OverlapShape narrow-phases every broad-phase candidate against this proxy; plain
	// OverlapAABB reports fat-AABB hits whose geometry misses the box entirely.
	// The proxy lives on the Runtime (see overlapProxyScratch) so it does not escape.
	proxy := &rt.overlapProxyScratch
	proxy.Points[0] = box2d.Vec2{X: minX, Y: minY}
	proxy.Points[1] = box2d.Vec2{X: maxX, Y: minY}
	proxy.Points[2] = box2d.Vec2{X: maxX, Y: maxY}
	proxy.Points[3] = box2d.Vec2{X: minX, Y: maxY}
	proxy.Count = 4
	proxy.Radius = 0

	// The collector lives on the Runtime instead of being a closure so that neither
	// the func value nor its captures are heap allocated per call; it gathers into
	// a reused buffer, and the returned slice is copied out of it below.
	saved := rt.overlapScratch
	rt.overlapScratch = overlapCollector{
		rt:             rt,
		includeSensors: includeSensors,
		hits:           rt.overlapHitsScratch[:0],
	}

	rt.World.OverlapShape(proxy, box2d.QueryFilter{CategoryBits: cat, MaskBits: mask},
		overlapCallback, &rt.overlapScratch)

	gathered := rt.overlapScratch.hits
	rt.overlapScratch = saved
	rt.overlapHitsScratch = gathered // keep the grown capacity for the next call

	slices.SortFunc(gathered, func(a, b query.AABBOverlapHit) int {
		if c := cmp.Compare(a.Entity, b.Entity); c != 0 {
			return c
		}
		return cmp.Compare(a.ShapeIndex, b.ShapeIndex)
	})
	gathered = slices.Compact(gathered)

	// The result belongs to the caller, so it cannot alias the scratch: allocate it
	// once at the exact final size. make never returns nil, so a miss still marshals
	// as [] rather than null.
	hits := make([]query.AABBOverlapHit, len(gathered))
	copy(hits, gathered)
	return query.AABBOverlapResult{Hits: hits}
}

// CircleSweep sweeps a circle along Start->End and returns the earliest TOI hit.
// A zero-radius or zero-length sweep always returns no hit.
func (rt *Runtime) CircleSweep(req query.CircleSweepRequest) query.CircleSweepResult {
	if req.Radius <= 0 {
		return query.CircleSweepResult{}
	}
	if req.Start.X == req.End.X && req.Start.Y == req.End.Y {
		return query.CircleSweepResult{}
	}
	cat, mask, includeSensors := queryFilterBits(req.Filter)

	maxFrac := req.MaxFraction
	if maxFrac <= 0 {
		maxFrac = 1
	}
	if maxFrac > 1 {
		maxFrac = 1
	}

	// A circle is a single point at the origin with non-zero radius.
	var proxy box2d.ShapeProxy
	proxy.Points[0] = box2d.Vec2{X: req.Start.X, Y: req.Start.Y}
	proxy.Count = 1
	proxy.Radius = req.Radius
	translation := box2d.Vec2{
		X: (req.End.X - req.Start.X) * maxFrac,
		Y: (req.End.Y - req.Start.Y) * maxFrac,
	}

	saved := rt.castScratch
	rt.castScratch = castHit{rt: rt, includeSensors: includeSensors}

	rt.World.CastShape(&proxy, translation, box2d.QueryFilter{CategoryBits: cat, MaskBits: mask},
		castCallback, &rt.castScratch)

	ctx := rt.castScratch
	rt.castScratch = saved

	if !ctx.hit {
		return query.CircleSweepResult{}
	}
	return query.CircleSweepResult{
		Hit:        true,
		Entity:     ctx.entityID,
		ShapeIndex: ctx.shapeIndex,
		Point:      ctx.point,
		Normal:     ctx.normal,
		Fraction:   ctx.fraction,
	}
}
