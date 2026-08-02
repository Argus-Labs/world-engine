// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file
// src/solver.c, continuous-collision portion (b2ContinuousContext,
// b2ContinuousQueryCallback, b2SolveContinuous). The bullet-body task
// (b2BulletBodyTask) and the serial bullet proxy enlargement live in the
// solve epilogue in solver.go.
//
// Deviations from upstream:
//   - b2SolveContinuous takes the executing worker's taskContext to collect
//     sensor hits, exactly like upstream. When called from the parallel
//     finalizeBodiesTask dispatch (non-bullet fast bodies) it is race-free
//     because a non-bullet sweep queries ONLY the static tree, whose nodes
//     and shapes no finalize worker mutates; bullet sweeps also query the
//     kinematic and dynamic trees and therefore never run during finalize.
//   - The bullet loop STAYS SERIAL (upstream b2BulletBodyTask is a parallel
//     for with minRange 8): a bullet sweep queries the kinematic and dynamic
//     trees while other bullets mutate their own shapes and transforms — a
//     data race Go's -race golden gate cannot accept, even though the racing
//     values never feed another bullet's arithmetic upstream. Bullets are
//     rare, so the serial loop costs little. It runs in the deterministic
//     order of the bulletBodies array, which is the ascending-worker
//     concatenation of the per-worker finalize gathers — equal to the serial
//     engine's ascending body-sim index fill (see solver.go).

package box2d

// maxContinuousSensorHits is B2_MAX_CONTINUOUS_SENSOR_HITS: the per-body cap
// on sensor hits recorded during one continuous sweep.
const maxContinuousSensorHits = 8

// coreFraction is B2_CORE_FRACTION: the fraction of the fast shape's minimum
// extent used as the core-shape radius for secondary TOI and for the chain
// clipping early-out.
const coreFraction = 0.25

// continuousContext carries the state of one continuous sweep across the
// broad-phase queries (upstream struct b2ContinuousContext).
type continuousContext struct {
	world           *World
	fastBodySim     *bodySim
	fastShape       *shape
	centroid1       Vec2
	centroid2       Vec2
	sweep           Sweep
	fraction        float64
	sensorHits      [maxContinuousSensorHits]sensorHit
	sensorFractions [maxContinuousSensorHits]float64
	sensorCount     int
}

// continuousQueryCallback is called from DynamicTree.Query for continuous
// collision (upstream b2ContinuousQueryCallback). It returns true to continue
// the query.
func continuousQueryCallback(proxyID int, userData uint64, context any) bool {
	_ = proxyID

	shapeID := int(userData)

	cc := context.(*continuousContext) //nolint:errcheck // the tree query context is always *continuousContext; the panic mirrors the upstream void* cast
	fastShape := cc.fastShape
	fastBodySim := cc.fastBodySim

	assert(fastShape.sensorIndex == NullIndex)

	// Skip same shape
	if shapeID == fastShape.id {
		return true
	}

	w := cc.world

	s := &w.shapes[shapeID]

	// Skip same body
	if s.bodyID == fastShape.bodyID {
		return true
	}

	isSensor := s.sensorIndex != NullIndex

	// Skip sensors unless the shapes want sensor events
	if isSensor && (!s.enableSensorEvents || !fastShape.enableSensorEvents) {
		return true
	}

	// Skip filtered shapes
	canCollide := shouldShapesCollide(fastShape.filter, s.filter)
	if !canCollide {
		return true
	}

	b := &w.bodies[s.bodyID]

	bSim := w.getBodySim(b)
	assert(b.bodyType == StaticBody || fastBodySim.flags&isBullet != 0)

	// Skip bullets
	if bSim.flags&isBullet != 0 {
		return true
	}

	// Skip filtered bodies
	fastBody := &w.bodies[fastBodySim.bodyID]
	canCollide = w.shouldBodiesCollide(fastBody, b)
	if !canCollide {
		return true
	}

	// Custom user filtering
	if s.enableCustomFiltering || fastShape.enableCustomFiltering {
		customFilterFcn := w.customFilterFcn
		if customFilterFcn != nil {
			idA := ShapeID{index1: int32(s.id + 1), world0: w.worldID, generation: s.generation}
			idB := ShapeID{index1: int32(fastShape.id + 1), world0: w.worldID, generation: fastShape.generation}
			canCollide = customFilterFcn(idA, idB, w.customFilterContext)
			if !canCollide {
				return true
			}
		}
	}

	// Early out on fast parallel movement over a chain shape.
	if s.shapeType == ChainSegmentShape {
		transform := bSim.transform
		p1 := TransformPoint(transform, s.chainSegment.Segment.Point1)
		p2 := TransformPoint(transform, s.chainSegment.Segment.Point2)
		e := Sub(p2, p1)
		e, length := GetLengthAndNormalize(e)
		if length > LinearSlop {
			c1 := cc.centroid1
			separation1 := Cross(Sub(c1, p1), e)
			c2 := cc.centroid2
			separation2 := Cross(Sub(c2, p1), e)

			coreDistance := coreFraction * fastBodySim.minExtent

			if separation1 < 0.0 || (separation1-separation2 < coreDistance && separation2 > coreDistance) {
				// Minimal clipping
				return true
			}
		}
	}

	var input TOIInput
	input.ProxyA = makeShapeDistanceProxy(s)
	input.ProxyB = makeShapeDistanceProxy(fastShape)
	input.SweepA = makeSweep(bSim)
	input.SweepB = cc.sweep
	input.MaxFraction = cc.fraction

	output := TimeOfImpact(&input)
	if isSensor {
		// Only accept a sensor hit that is sooner than the current solid hit.
		if output.Fraction <= cc.fraction && cc.sensorCount < maxContinuousSensorHits {
			index := cc.sensorCount

			// The hit shape is a sensor
			hit := sensorHit{
				sensorID:  s.id,
				visitorID: fastShape.id,
			}

			cc.sensorHits[index] = hit
			cc.sensorFractions[index] = output.Fraction
			cc.sensorCount++
		}
	} else {
		hitFraction := cc.fraction
		didHit := false

		if 0.0 < output.Fraction && output.Fraction < cc.fraction {
			hitFraction = output.Fraction
			didHit = true
		} else if output.Fraction == 0.0 {
			// fallback to TOI of a small circle around the fast shape centroid
			centroid := getShapeCentroid(fastShape)
			extent := computeShapeExtent(fastShape, centroid)
			radius := coreFraction * extent.minExtent
			input.ProxyB = MakeProxy([]Vec2{centroid}, 1, radius)
			output = TimeOfImpact(&input)
			if 0.0 < output.Fraction && output.Fraction < cc.fraction {
				hitFraction = output.Fraction
				didHit = true
			}
		}

		if didHit && (s.enablePreSolveEvents || fastShape.enablePreSolveEvents) && w.preSolveFcn != nil {
			shapeIDA := ShapeID{index1: int32(s.id + 1), world0: w.worldID, generation: s.generation}
			shapeIDB := ShapeID{index1: int32(fastShape.id + 1), world0: w.worldID, generation: fastShape.generation}
			didHit = w.preSolveFcn(shapeIDA, shapeIDB, output.Point, output.Normal, w.preSolveContext)
		}

		if didHit {
			fastBodySim.flags |= hadTimeOfImpact
			cc.fraction = hitFraction
		}
	}

	// Continue query
	return true
}

// solveContinuous performs the continuous sweep for one fast body: it queries
// the broad-phase trees along the swept AABB of each shape, finds the
// earliest time of impact, advances the body to it and refreshes the shape
// AABBs (upstream b2SolveContinuous). Sensor hits are pushed onto the
// executing worker's task context for serial processing after the join.
//
// Concurrency contract: every write is owned by this body (its sim, its
// shapes, its bodyMoveEvents slot) or by the worker (tc.sensorHits). The
// only tree read of a non-bullet sweep is the static tree, which is frozen
// during the finalize dispatch — see the file header.
func (w *World) solveContinuous(bodySimIndex int, tc *taskContext) {
	awake := &w.solverSets[awakeSet]
	fastBodySim := &awake.bodySims[bodySimIndex]
	assert(fastBodySim.flags&isFast != 0)

	sweep := makeSweep(fastBodySim)

	var xf1 Transform
	xf1.Q = sweep.Q1
	xf1.P = Sub(sweep.C1, RotateVector(sweep.Q1, sweep.LocalCenter))

	var xf2 Transform
	xf2.Q = sweep.Q2
	xf2.P = Sub(sweep.C2, RotateVector(sweep.Q2, sweep.LocalCenter))

	staticTree := &w.broadPhase.trees[StaticBody]
	kinematicTree := &w.broadPhase.trees[KinematicBody]
	dynamicTree := &w.broadPhase.trees[DynamicBody]
	fastBody := &w.bodies[fastBodySim.bodyID]

	var context continuousContext
	context.world = w
	context.sweep = sweep
	context.fastBodySim = fastBodySim
	context.fraction = 1.0

	isBulletBody := fastBodySim.flags&isBullet != 0

	shapeID := fastBody.headShapeID
	for shapeID != NullIndex {
		fastShape := &w.shapes[shapeID]
		shapeID = fastShape.nextShapeID

		context.fastShape = fastShape
		context.centroid1 = TransformPoint(xf1, fastShape.localCentroid)
		context.centroid2 = TransformPoint(xf2, fastShape.localCentroid)

		box1 := fastShape.aabb
		box2 := computeShapeAABB(fastShape, xf2)

		// Store this to avoid double computation in the case there is no
		// impact event
		fastShape.aabb = box2

		// No continuous collision for sensors (but still need the updated
		// bounds)
		if fastShape.sensorIndex != NullIndex {
			continue
		}

		sweptBox := AABBUnion(box1, box2)

		// Non-bullet fast bodies query ONLY the static tree, which is why
		// this call is race-free inside the parallel finalize dispatch (see
		// the file header). Bullets also query the kinematic and dynamic
		// trees, and run in the serial bullet stage only.
		_ = staticTree.Query(sweptBox, DefaultMaskBits, continuousQueryCallback, &context)

		if isBulletBody {
			_ = kinematicTree.Query(sweptBox, DefaultMaskBits, continuousQueryCallback, &context)
			_ = dynamicTree.Query(sweptBox, DefaultMaskBits, continuousQueryCallback, &context)
		}
	}

	speculativeDistance := SpeculativeDistance

	if context.fraction < 1.0 {
		// Handle time of impact event
		q := NLerp(sweep.Q1, sweep.Q2, context.fraction)
		c := Lerp(sweep.C1, sweep.C2, context.fraction)
		origin := Sub(c, RotateVector(q, sweep.LocalCenter))

		// Advance body
		transform := Transform{P: origin, Q: q}
		fastBodySim.transform = transform
		fastBodySim.center = c
		fastBodySim.rotation0 = q
		fastBodySim.center0 = c

		// Update body move event
		w.bodyMoveEvents[bodySimIndex].Transform = transform

		// Prepare AABBs for broad-phase.
		// Even though a body is fast, it may not move much. So the AABB may
		// not need enlargement.

		shapeID = fastBody.headShapeID
		for shapeID != NullIndex {
			s := &w.shapes[shapeID]

			// Must recompute aabb at the interpolated transform
			aabb := computeShapeAABB(s, transform)
			aabb.LowerBound.X -= speculativeDistance
			aabb.LowerBound.Y -= speculativeDistance
			aabb.UpperBound.X += speculativeDistance
			aabb.UpperBound.Y += speculativeDistance
			s.aabb = aabb

			if !AABBContains(s.fatAABB, aabb) {
				margin := s.aabbMargin
				var fatAABB AABB
				fatAABB.LowerBound.X = aabb.LowerBound.X - margin
				fatAABB.LowerBound.Y = aabb.LowerBound.Y - margin
				fatAABB.UpperBound.X = aabb.UpperBound.X + margin
				fatAABB.UpperBound.Y = aabb.UpperBound.Y + margin
				s.fatAABB = fatAABB

				s.enlargedAABB = true
				fastBodySim.flags |= enlargeBounds
			}

			shapeID = s.nextShapeID
		}
	} else {
		// No time of impact event

		// Advance body
		fastBodySim.rotation0 = fastBodySim.transform.Q
		fastBodySim.center0 = fastBodySim.center

		// Prepare AABBs for broad-phase
		shapeID = fastBody.headShapeID
		for shapeID != NullIndex {
			s := &w.shapes[shapeID]

			// shape.aabb is still valid from above

			if !AABBContains(s.fatAABB, s.aabb) {
				margin := s.aabbMargin
				var fatAABB AABB
				fatAABB.LowerBound.X = s.aabb.LowerBound.X - margin
				fatAABB.LowerBound.Y = s.aabb.LowerBound.Y - margin
				fatAABB.UpperBound.X = s.aabb.UpperBound.X + margin
				fatAABB.UpperBound.Y = s.aabb.UpperBound.Y + margin
				s.fatAABB = fatAABB

				s.enlargedAABB = true
				fastBodySim.flags |= enlargeBounds
			}

			shapeID = s.nextShapeID
		}
	}

	// Push sensor hits on the executing worker's task context for serial
	// processing (upstream: taskContext->sensorHits keyed by threadIndex).
	// The per-worker lists are concatenated into slot 0 in ascending worker
	// order before the bullet stage — see solver.go.
	for i := range context.sensorCount {
		// Skip any sensor hits that occurred after a solid hit
		if context.sensorFractions[i] < context.fraction {
			tc.sensorHits = append(tc.sensorHits, context.sensorHits[i])
		}
	}
}
