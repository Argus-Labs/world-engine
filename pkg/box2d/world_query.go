// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file
// src/physics_world.c (world-level query section: b2World_OverlapAABB,
// b2World_OverlapShape, b2World_CastRay, b2World_CastRayClosest,
// b2World_CastShape, b2World_CastMover, b2World_CollideMover and their
// internal callback contexts).
//
// DESIGN DEVIATION (approved, see world.go): upstream resolves the world
// through the global b2_worlds array via b2GetWorldFromId. This port has no
// world registry, so every b2World_* query becomes a method on *World and the
// receiver replaces the lookup. The `void* context` user pointer becomes an
// `any` value and the internal callback context structs are passed to the
// dynamic tree callbacks as `any` (matching the tree callback signatures in
// dynamic_tree.go).
//
// NOTE: upstream declares an unused `b2MoverContext`/`b2CharacterCallbackContext`
// typedef next to WorldMoverCastContext. It is dead code in C and is not
// ported.

package box2d

// worldQueryScratch is the reusable backing store for the callback contexts
// and tree inputs used by the World query entry points.
//
// Why it exists: the tree callbacks take their context as `any` (mirroring the
// upstream `void* context`), so Go's escape analysis must assume every pointer
// handed to a callback escapes. Upstream keeps these structs on the C stack;
// a direct Go port heap allocates one per query call, which showed up as three
// allocations per circle sweep. Hanging one copy off *World makes the whole
// query path allocation free.
//
// Contract: a World must not be queried from two goroutines at once (the same
// single-threaded contract the solver already relies on, see taskContext in
// world.go). Re-entrancy from inside a user callback IS supported: every entry
// point saves the fields it uses and restores them before returning, so a
// nested query cannot corrupt the traversal that contains it.
//
// These fields never participate in simulation arithmetic, so they cannot
// affect bit-determinism.
type worldQueryScratch struct {
	query     worldQueryContext
	overlap   worldOverlapContext
	rayCast   worldRayCastContext
	moverCast worldMoverCastContext
	mover     worldMoverContext

	// rayCastInput backs World.CastRay and World.CastRayClosest,
	// shapeCastInput backs World.CastShape, moverCastInput backs
	// World.CastMover.
	rayCastInput   RayCastInput
	shapeCastInput ShapeCastInput
	moverCastInput ShapeCastInput

	// rayResult backs the RayResult that World.CastRayClosest passes to
	// rayCastClosestFcn as the user context.
	rayResult RayResult
}

// worldQueryContext is the callback context for World.OverlapAABB
// (upstream WorldQueryContext).
type worldQueryContext struct {
	world       *World
	fcn         OverlapResultFcn
	filter      QueryFilter
	userContext any
}

// treeQueryCallback is the dynamic tree callback used by World.OverlapAABB
// (upstream static TreeQueryCallback).
func treeQueryCallback(proxyID int, userData uint64, context any) bool {
	_ = proxyID

	shapeID := int(userData)

	worldContext, ok := context.(*worldQueryContext)
	assert(ok)
	if !ok {
		return false
	}
	world := worldContext.world

	s := &world.shapes[shapeID]

	if !shouldQueryCollide(s.filter, worldContext.filter) {
		return true
	}

	id := ShapeID{index1: int32(shapeID + 1), world0: world.worldID, generation: s.generation}
	result := worldContext.fcn(id, worldContext.userContext)
	return result
}

// OverlapAABB performs an overlap test for all shapes that *potentially*
// overlap the provided AABB (upstream b2World_OverlapAABB).
func (w *World) OverlapAABB(aabb AABB, filter QueryFilter, fcn OverlapResultFcn, context any) TreeStats {
	var treeStats TreeStats

	assert(!w.locked)
	if w.locked {
		return treeStats
	}

	assert(IsValidAABB(aabb))

	// See worldQueryScratch: reused storage keeps the context off the heap.
	saved := w.queryScratch.query
	worldContext := &w.queryScratch.query
	*worldContext = worldQueryContext{
		world:       w,
		fcn:         fcn,
		filter:      filter,
		userContext: context,
	}

	for i := range int(BodyTypeCount) {
		treeResult := w.broadPhase.trees[i].Query(aabb, filter.MaskBits, treeQueryCallback, worldContext)

		treeStats.NodeVisits += treeResult.NodeVisits
		treeStats.LeafVisits += treeResult.LeafVisits
	}

	w.queryScratch.query = saved

	return treeStats
}

// worldOverlapContext is the callback context for World.OverlapShape
// (upstream WorldOverlapContext).
type worldOverlapContext struct {
	world       *World
	fcn         OverlapResultFcn
	filter      QueryFilter
	proxy       *ShapeProxy
	userContext any
}

// treeOverlapCallback is the dynamic tree callback used by World.OverlapShape
// (upstream static TreeOverlapCallback).
func treeOverlapCallback(proxyID int, userData uint64, context any) bool {
	_ = proxyID

	shapeID := int(userData)

	worldContext, ok := context.(*worldOverlapContext)
	assert(ok)
	if !ok {
		return false
	}
	world := worldContext.world

	s := &world.shapes[shapeID]

	if !shouldQueryCollide(s.filter, worldContext.filter) {
		return true
	}

	b := &world.bodies[s.bodyID]
	transform := world.getBodyTransformQuick(b)

	var input DistanceInput
	input.ProxyA = *worldContext.proxy
	input.ProxyB = makeShapeDistanceProxy(s)
	input.TransformA = TransformIdentity
	input.TransformB = transform
	input.UseRadii = true

	var cache SimplexCache
	output := ShapeDistance(&input, &cache, nil)

	tolerance := 0.1 * LinearSlop
	if output.Distance > tolerance {
		return true
	}

	id := ShapeID{index1: int32(s.id + 1), world0: world.worldID, generation: s.generation}
	result := worldContext.fcn(id, worldContext.userContext)
	return result
}

// OverlapShape performs an overlap test for all shapes that overlap the
// provided shape proxy (upstream b2World_OverlapShape).
func (w *World) OverlapShape(proxy *ShapeProxy, filter QueryFilter, fcn OverlapResultFcn, context any) TreeStats {
	var treeStats TreeStats

	assert(!w.locked)
	if w.locked {
		return treeStats
	}

	aabb := MakeAABB(proxy.Points[:proxy.Count], proxy.Radius)

	// See worldQueryScratch: reused storage keeps the context off the heap.
	saved := w.queryScratch.overlap
	worldContext := &w.queryScratch.overlap
	*worldContext = worldOverlapContext{
		world:       w,
		fcn:         fcn,
		filter:      filter,
		proxy:       proxy,
		userContext: context,
	}

	for i := range int(BodyTypeCount) {
		treeResult := w.broadPhase.trees[i].Query(aabb, filter.MaskBits, treeOverlapCallback, worldContext)

		treeStats.NodeVisits += treeResult.NodeVisits
		treeStats.LeafVisits += treeResult.LeafVisits
	}

	w.queryScratch.overlap = saved

	return treeStats
}

// worldRayCastContext is the callback context shared by World.CastRay,
// World.CastRayClosest and World.CastShape (upstream WorldRayCastContext).
type worldRayCastContext struct {
	world       *World
	fcn         CastResultFcn
	filter      QueryFilter
	fraction    float64
	userContext any
}

// rayCastCallback is the dynamic tree callback used by World.CastRay and
// World.CastRayClosest (upstream static RayCastCallback).
func rayCastCallback(input *RayCastInput, proxyID int, userData uint64, context any) float64 {
	_ = proxyID

	shapeID := int(userData)

	worldContext, ok := context.(*worldRayCastContext)
	assert(ok)
	if !ok {
		return 0.0
	}
	world := worldContext.world

	s := &world.shapes[shapeID]

	if !shouldQueryCollide(s.filter, worldContext.filter) {
		return input.MaxFraction
	}

	b := &world.bodies[s.bodyID]
	transform := world.getBodyTransformQuick(b)
	output := rayCastShape(input, s, transform)

	if output.Hit {
		id := ShapeID{index1: int32(shapeID + 1), world0: world.worldID, generation: s.generation}
		fraction := worldContext.fcn(id, output.Point, output.Normal, output.Fraction, worldContext.userContext)

		// The user may return -1 to skip this shape.
		if fraction >= 0.0 && fraction <= 1.0 {
			worldContext.fraction = fraction
		}

		return fraction
	}

	return input.MaxFraction
}

// CastRay casts a ray into the world to collect shapes in the path of the ray
// (upstream b2World_CastRay). The callback function controls whether you get
// the closest point, any point, or n-points.
//
// Note: the callback function may receive shapes in any order.
//
// origin is the start point of the ray, translation the translation of the ray
// from the start point to the end point, filter contains bit flags to filter
// unwanted shapes from the results, fcn is a user implemented callback
// function and context a user context passed along to the callback. It returns
// the traversal performance counters.
func (w *World) CastRay(origin, translation Vec2, filter QueryFilter, fcn CastResultFcn, context any) TreeStats {
	var treeStats TreeStats

	assert(!w.locked)
	if w.locked {
		return treeStats
	}

	assert(IsValidVec2(origin))
	assert(IsValidVec2(translation))

	// See worldQueryScratch: reused storage keeps the input and the context
	// off the heap.
	savedInput, savedContext := w.queryScratch.rayCastInput, w.queryScratch.rayCast
	input := &w.queryScratch.rayCastInput
	*input = RayCastInput{Origin: origin, Translation: translation, MaxFraction: 1.0}

	worldContext := &w.queryScratch.rayCast
	*worldContext = worldRayCastContext{
		world:       w,
		fcn:         fcn,
		filter:      filter,
		fraction:    1.0,
		userContext: context,
	}

	for i := range int(BodyTypeCount) {
		treeResult := w.broadPhase.trees[i].RayCast(input, filter.MaskBits, rayCastCallback, worldContext)
		treeStats.NodeVisits += treeResult.NodeVisits
		treeStats.LeafVisits += treeResult.LeafVisits

		if worldContext.fraction == 0.0 {
			break
		}

		input.MaxFraction = worldContext.fraction
	}

	w.queryScratch.rayCastInput, w.queryScratch.rayCast = savedInput, savedContext

	return treeStats
}

// rayCastClosestFcn finds the closest hit. This is the most common callback
// used in games (upstream static b2RayCastClosestFcn).
func rayCastClosestFcn(shapeID ShapeID, point, normal Vec2, fraction float64, context any) float64 {
	// Ignore initial overlap.
	if fraction == 0.0 {
		return -1.0
	}

	rayResult, ok := context.(*RayResult)
	assert(ok)
	if !ok {
		return -1.0
	}

	rayResult.ShapeID = shapeID
	rayResult.Point = point
	rayResult.Normal = normal
	rayResult.Fraction = fraction
	rayResult.Hit = true
	return fraction
}

// CastRayClosest casts a ray into the world to collect the closest hit
// (upstream b2World_CastRayClosest). This is a convenience function that
// ignores initial overlap. It is less general than World.CastRay and does not
// allow for custom filtering.
func (w *World) CastRayClosest(origin, translation Vec2, filter QueryFilter) RayResult {
	var result RayResult

	assert(!w.locked)
	if w.locked {
		return result
	}

	assert(IsValidVec2(origin))
	assert(IsValidVec2(translation))

	// See worldQueryScratch: reused storage keeps the input, the context and
	// the RayResult the callback writes through off the heap. The result is
	// copied out to the caller's return value before the scratch is restored.
	savedInput, savedContext, savedResult :=
		w.queryScratch.rayCastInput, w.queryScratch.rayCast, w.queryScratch.rayResult

	input := &w.queryScratch.rayCastInput
	*input = RayCastInput{Origin: origin, Translation: translation, MaxFraction: 1.0}

	rayResult := &w.queryScratch.rayResult
	*rayResult = RayResult{}

	worldContext := &w.queryScratch.rayCast
	*worldContext = worldRayCastContext{
		world:       w,
		fcn:         rayCastClosestFcn,
		filter:      filter,
		fraction:    1.0,
		userContext: rayResult,
	}

	for i := range int(BodyTypeCount) {
		treeResult := w.broadPhase.trees[i].RayCast(input, filter.MaskBits, rayCastCallback, worldContext)
		rayResult.NodeVisits += treeResult.NodeVisits
		rayResult.LeafVisits += treeResult.LeafVisits

		if worldContext.fraction == 0.0 {
			break
		}

		input.MaxFraction = worldContext.fraction
	}

	result = *rayResult

	w.queryScratch.rayCastInput, w.queryScratch.rayCast, w.queryScratch.rayResult =
		savedInput, savedContext, savedResult

	return result
}

// shapeCastCallback is the dynamic tree callback used by World.CastShape
// (upstream static ShapeCastCallback).
func shapeCastCallback(input *ShapeCastInput, proxyID int, userData uint64, context any) float64 {
	_ = proxyID

	shapeID := int(userData)

	worldContext, ok := context.(*worldRayCastContext)
	assert(ok)
	if !ok {
		return 0.0
	}
	world := worldContext.world

	s := &world.shapes[shapeID]

	if !shouldQueryCollide(s.filter, worldContext.filter) {
		return input.MaxFraction
	}

	b := &world.bodies[s.bodyID]
	transform := world.getBodyTransformQuick(b)

	output := shapeCastShape(input, s, transform)

	if output.Hit {
		id := ShapeID{index1: int32(shapeID + 1), world0: world.worldID, generation: s.generation}
		fraction := worldContext.fcn(id, output.Point, output.Normal, output.Fraction, worldContext.userContext)

		// The user may return -1 to skip this shape.
		if fraction >= 0.0 && fraction <= 1.0 {
			worldContext.fraction = fraction
		}

		return fraction
	}

	return input.MaxFraction
}

// CastShape casts a shape through the world (upstream b2World_CastShape).
// Similar to a ray cast except that a shape is cast instead of a point.
// See World.CastRay.
func (w *World) CastShape(proxy *ShapeProxy, translation Vec2, filter QueryFilter, fcn CastResultFcn,
	context any,
) TreeStats {
	var treeStats TreeStats

	assert(!w.locked)
	if w.locked {
		return treeStats
	}

	assert(IsValidVec2(translation))

	// See worldQueryScratch: reused storage keeps the input and the context
	// off the heap.
	savedInput, savedContext := w.queryScratch.shapeCastInput, w.queryScratch.rayCast

	input := &w.queryScratch.shapeCastInput
	*input = ShapeCastInput{}
	input.Proxy = *proxy
	input.Translation = translation
	input.MaxFraction = 1.0

	worldContext := &w.queryScratch.rayCast
	*worldContext = worldRayCastContext{
		world:       w,
		fcn:         fcn,
		filter:      filter,
		fraction:    1.0,
		userContext: context,
	}

	for i := range int(BodyTypeCount) {
		treeResult := w.broadPhase.trees[i].ShapeCast(input, filter.MaskBits, shapeCastCallback, worldContext)
		treeStats.NodeVisits += treeResult.NodeVisits
		treeStats.LeafVisits += treeResult.LeafVisits

		if worldContext.fraction == 0.0 {
			break
		}

		input.MaxFraction = worldContext.fraction
	}

	w.queryScratch.shapeCastInput, w.queryScratch.rayCast = savedInput, savedContext

	return treeStats
}

// worldMoverCastContext is the callback context for World.CastMover
// (upstream WorldMoverCastContext).
type worldMoverCastContext struct {
	world    *World
	filter   QueryFilter
	fraction float64
}

// moverCastCallback is the dynamic tree callback used by World.CastMover
// (upstream static MoverCastCallback).
func moverCastCallback(input *ShapeCastInput, proxyID int, userData uint64, context any) float64 {
	_ = proxyID

	shapeID := int(userData)

	worldContext, ok := context.(*worldMoverCastContext)
	assert(ok)
	if !ok {
		return 0.0
	}
	world := worldContext.world

	s := &world.shapes[shapeID]

	if !shouldQueryCollide(s.filter, worldContext.filter) {
		return worldContext.fraction
	}

	b := &world.bodies[s.bodyID]
	transform := world.getBodyTransformQuick(b)

	output := shapeCastShape(input, s, transform)
	if output.Fraction == 0.0 {
		// Ignore overlapping shapes.
		return worldContext.fraction
	}

	worldContext.fraction = output.Fraction
	return output.Fraction
}

// CastMover casts a capsule mover through the world (upstream
// b2World_CastMover). This is a special shape cast that handles sliding along
// other shapes while reducing clipping.
func (w *World) CastMover(mover *Capsule, translation Vec2, filter QueryFilter) float64 {
	assert(IsValidVec2(translation))
	assert(mover.Radius > 2.0*LinearSlop)

	assert(!w.locked)
	if w.locked {
		return 1.0
	}

	// See worldQueryScratch: reused storage keeps the input and the context
	// off the heap.
	savedInput, savedContext := w.queryScratch.moverCastInput, w.queryScratch.moverCast

	input := &w.queryScratch.moverCastInput
	*input = ShapeCastInput{}
	input.Proxy.Points[0] = mover.Center1
	input.Proxy.Points[1] = mover.Center2
	input.Proxy.Count = 2
	input.Proxy.Radius = mover.Radius
	input.Translation = translation
	input.MaxFraction = 1.0
	input.CanEncroach = true

	worldContext := &w.queryScratch.moverCast
	*worldContext = worldMoverCastContext{
		world:    w,
		filter:   filter,
		fraction: 1.0,
	}

	for i := range int(BodyTypeCount) {
		w.broadPhase.trees[i].ShapeCast(input, filter.MaskBits, moverCastCallback, worldContext)

		if worldContext.fraction == 0.0 {
			break
		}

		input.MaxFraction = worldContext.fraction
	}

	fraction := worldContext.fraction

	w.queryScratch.moverCastInput, w.queryScratch.moverCast = savedInput, savedContext

	return fraction
}

// worldMoverContext is the callback context for World.CollideMover
// (upstream WorldMoverContext).
type worldMoverContext struct {
	world       *World
	fcn         PlaneResultFcn
	filter      QueryFilter
	mover       Capsule
	userContext any

	// planeResult backs the *PlaneResult handed to fcn. Upstream passes the
	// address of a C stack local, so the pointee is only valid for the
	// duration of the callback either way; reusing one slot here keeps the
	// per-hit heap allocation out of the traversal.
	planeResult PlaneResult
}

// treeCollideCallback is the dynamic tree callback used by World.CollideMover
// (upstream static TreeCollideCallback).
func treeCollideCallback(proxyID int, userData uint64, context any) bool {
	_ = proxyID

	shapeID := int(userData)

	worldContext, ok := context.(*worldMoverContext)
	assert(ok)
	if !ok {
		return false
	}
	world := worldContext.world

	s := &world.shapes[shapeID]

	if !shouldQueryCollide(s.filter, worldContext.filter) {
		return true
	}

	b := &world.bodies[s.bodyID]
	transform := world.getBodyTransformQuick(b)

	// The pointer handed to fcn is only valid for the duration of the call
	// (as upstream, where it points at a C stack local), so the shared slot
	// on the context is safe. See worldMoverContext.planeResult.
	result := &worldContext.planeResult
	*result = collideMover(&worldContext.mover, s, transform)

	// todo handle deep overlap
	if result.Hit && IsNormalized(result.Plane.Normal) {
		id := ShapeID{index1: int32(s.id + 1), world0: world.worldID, generation: s.generation}
		return worldContext.fcn(id, result, worldContext.userContext)
	}

	return true
}

// CollideMover collides a capsule mover with the world, gathering collision
// planes that can be fed to SolvePlanes (upstream b2World_CollideMover).
// Useful for kinematic character movement.
//
// It is tempting to use a shape proxy for the mover, but this makes handling
// deep overlap difficult and the generality may not be worth it.
func (w *World) CollideMover(mover *Capsule, filter QueryFilter, fcn PlaneResultFcn, context any) {
	assert(!w.locked)
	if w.locked {
		return
	}

	r := Vec2{X: mover.Radius, Y: mover.Radius}

	var aabb AABB
	aabb.LowerBound = Sub(Min(mover.Center1, mover.Center2), r)
	aabb.UpperBound = Add(Max(mover.Center1, mover.Center2), r)

	// See worldQueryScratch: reused storage keeps the context off the heap.
	saved := w.queryScratch.mover
	worldContext := &w.queryScratch.mover
	*worldContext = worldMoverContext{
		world:       w,
		fcn:         fcn,
		filter:      filter,
		mover:       *mover,
		userContext: context,
	}

	for i := range int(BodyTypeCount) {
		w.broadPhase.trees[i].Query(aabb, filter.MaskBits, treeCollideCallback, worldContext)
	}

	w.queryScratch.mover = saved
}
