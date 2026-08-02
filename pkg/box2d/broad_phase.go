// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/broad_phase.c, src/broad_phase.h.
//
// Deviations from upstream:
//
//   - The port is single-threaded: b2FindPairsTask becomes a serial loop in
//     moveArray order inside updateBroadPhasePairs, and the b2AtomicInt
//     movePairIndex plus the atomic pair-buffer bump allocator (with heap
//     fallback) collapse to a plain slice append. The per-move-result pair
//     list prepend/consume order is preserved exactly.
//   - The arena-allocated moveResults/movePairs buffers (fields on
//     b2BroadPhase) become local slices in updateBroadPhasePairs.
//   - b2UpdateTreesTask runs inline before contact creation, matching the
//     behavior of the upstream default (serial) task system.

package box2d

// Store the proxy type in the lower 2 bits of the proxy key. This leaves 30
// bits for the id.

// proxyKeyType extracts the body type from a proxy key (upstream
// B2_PROXY_TYPE).
func proxyKeyType(key int) BodyType {
	return BodyType(key & 3)
}

// proxyKeyID extracts the tree proxy id from a proxy key (upstream
// B2_PROXY_ID).
func proxyKeyID(key int) int {
	return key >> 2
}

// makeProxyKey packs a tree proxy id and a body type into a proxy key
// (upstream B2_PROXY_KEY).
func makeProxyKey(id int, bodyType BodyType) int {
	return (id << 2) | int(bodyType)
}

// broadPhase is used for computing pairs and performing volume queries and ray
// casts (upstream b2BroadPhase).
//
// This broad-phase does not persist pairs. Instead, it reports potentially new
// pairs. It is up to the client to consume the new pairs and to track
// subsequent overlap.
type broadPhase struct {
	trees [BodyTypeCount]DynamicTree

	// The move set and array are used to track shapes that have moved
	// significantly and need a pair query for new contacts. The array has a
	// deterministic order.
	moveSet   hashSet
	moveArray []int

	// Deviation from upstream: moveResults, movePairs, movePairCapacity and
	// movePairIndex are local to updateBroadPhasePairs in this
	// single-threaded port (see the file header).

	// pairSet tracks shape pairs that have a contact.
	pairSet hashSet
}

// createBroadPhase initializes a broad-phase (upstream b2CreateBroadPhase).
func createBroadPhase(bp *broadPhase) {
	bp.moveSet = createHashSet(16)
	bp.moveArray = make([]int, 0, 16)
	bp.pairSet = createHashSet(32)

	for i := range BodyTypeCount {
		bp.trees[i] = NewDynamicTree()
	}
}

// destroyBroadPhase releases a broad-phase (upstream b2DestroyBroadPhase).
func destroyBroadPhase(bp *broadPhase) {
	for i := range BodyTypeCount {
		bp.trees[i].Destroy()
	}

	destroyHashSet(&bp.moveSet)
	bp.moveArray = nil
	destroyHashSet(&bp.pairSet)

	*bp = broadPhase{}
}

// bufferMove buffers a proxy for the next pair query. This is what triggers new
// contact pairs to be created (upstream b2BufferMove).
//
// Warning: this must be called in deterministic order.
func (bp *broadPhase) bufferMove(queryProxy int) {
	// Adding 1 because 0 is the sentinel
	alreadyAdded := addKey(&bp.moveSet, uint64(queryProxy+1))
	if !alreadyAdded {
		bp.moveArray = append(bp.moveArray, queryProxy)
	}
}

// unBufferMove removes a proxy from the move buffer (upstream b2UnBufferMove).
func (bp *broadPhase) unBufferMove(proxyKey int) {
	found := removeKey(&bp.moveSet, uint64(proxyKey+1))

	if found {
		// Purge from move buffer. Linear search.
		count := len(bp.moveArray)
		for i := range count {
			if bp.moveArray[i] == proxyKey {
				// b2IntArray_RemoveSwap: the last element replaces the removed
				// one, so the move array order is not preserved here.
				last := len(bp.moveArray) - 1
				if i != last {
					bp.moveArray[i] = bp.moveArray[last]
				}
				bp.moveArray = bp.moveArray[:last]
				break
			}
		}
	}
}

// createProxy creates a proxy in the tree for the given body type and returns
// its proxy key (upstream b2BroadPhase_CreateProxy).
func (bp *broadPhase) createProxy(proxyType BodyType, aabb AABB, categoryBits uint64, shapeIndex int,
	forcePairCreation bool,
) int {
	assert(0 <= proxyType && proxyType < BodyTypeCount)
	proxyID := bp.trees[proxyType].CreateProxy(aabb, categoryBits, uint64(shapeIndex))
	key := makeProxyKey(proxyID, proxyType)
	if proxyType != StaticBody || forcePairCreation {
		bp.bufferMove(key)
	}
	return key
}

// destroyProxy destroys a proxy (upstream b2BroadPhase_DestroyProxy).
func (bp *broadPhase) destroyProxy(proxyKey int) {
	assert(len(bp.moveArray) == getSetCount(&bp.moveSet))
	bp.unBufferMove(proxyKey)

	proxyType := proxyKeyType(proxyKey)
	proxyID := proxyKeyID(proxyKey)

	assert(0 <= proxyType && proxyType <= BodyTypeCount)
	bp.trees[proxyType].DestroyProxy(proxyID)
}

// moveProxy moves a proxy and buffers it for the next pair query (upstream
// b2BroadPhase_MoveProxy).
func (bp *broadPhase) moveProxy(proxyKey int, aabb AABB) {
	proxyType := proxyKeyType(proxyKey)
	proxyID := proxyKeyID(proxyKey)

	bp.trees[proxyType].MoveProxy(proxyID, aabb)
	bp.bufferMove(proxyKey)
}

// enlargeProxy enlarges a proxy and buffers it for the next pair query
// (upstream b2BroadPhase_EnlargeProxy).
func (bp *broadPhase) enlargeProxy(proxyKey int, aabb AABB) {
	assert(proxyKey != NullIndex)
	typeIndex := proxyKeyType(proxyKey)
	proxyID := proxyKeyID(proxyKey)

	assert(typeIndex != StaticBody)

	bp.trees[typeIndex].EnlargeProxy(proxyID, aabb)
	bp.bufferMove(proxyKey)
}

// movePair is a candidate shape pair found by the pair query (upstream
// b2MovePair). Deviation from upstream: the pairs are stored in a flat slice
// and linked by index instead of pointers, so the arena/heap split (the
// `heap` flag) has no counterpart. The prepend order of the per-move-result
// list is preserved exactly.
type movePair struct {
	shapeIndexA int
	shapeIndexB int

	// next is the index of the next pair of the same move result in
	// queryPairContext.pairs, or NullIndex (upstream b2MovePair* next).
	next int
}

// queryPairContext carries the state of one moved proxy's pair query
// (upstream b2QueryPairContext). Deviation from upstream: the b2MoveResult
// pair list head lives here as an index (moveResult) together with the flat
// pair slice.
type queryPairContext struct {
	world *World

	// pairs is the flat pair storage shared by all move results (upstream
	// bp->movePairs plus heap fallback).
	pairs []movePair

	// moveResult is the head index of the current move result's pair list
	// (upstream queryContext->moveResult->pairList).
	moveResult int

	queryTreeType   BodyType
	queryProxyKey   int
	queryShapeIndex int
}

// pairQueryCallback is called from DynamicTree.Query when gathering pairs
// (upstream static b2PairQueryCallback).
func pairQueryCallback(proxyID int, userData uint64, context any) bool {
	shapeID := int(userData)

	queryContext, ok := context.(*queryPairContext)
	assert(ok)
	if !ok {
		return false
	}
	bp := &queryContext.world.broadPhase

	proxyKey := makeProxyKey(proxyID, queryContext.queryTreeType)
	queryProxyKey := queryContext.queryProxyKey

	// A proxy cannot form a pair with itself.
	if proxyKey == queryContext.queryProxyKey {
		return true
	}

	treeType := queryContext.queryTreeType
	queryProxyType := proxyKeyType(queryProxyKey)

	// De-duplication
	// It is important to prevent duplicate contacts from being created.
	// Ideally I can prevent duplicates early and in the worker. Most of the
	// time the moveSet contains dynamic and kinematic proxies, but sometimes
	// it has static proxies.

	// I had an optimization here to skip checking the move set if this is a
	// query into the static tree. The assumption is that the static proxies
	// are never in the move set so there is no risk of duplication. However,
	// this is not true with b2ShapeDef::invokeContactCreation or when a
	// static shape is modified. There can easily be scenarios where the
	// static proxy is in the moveSet but the dynamic proxy is not.
	// I could have some flag to indicate that there are any static bodies in
	// the moveSet.

	// Is this proxy also moving?
	if queryProxyType == DynamicBody {
		if treeType == DynamicBody && proxyKey < queryProxyKey {
			moved := containsKey(&bp.moveSet, uint64(proxyKey+1))
			if moved {
				// Both proxies are moving. Avoid duplicate pairs.
				return true
			}
		}
	} else {
		assert(treeType == DynamicBody)
		moved := containsKey(&bp.moveSet, uint64(proxyKey+1))
		if moved {
			// Both proxies are moving. Avoid duplicate pairs.
			return true
		}
	}

	pairKey := shapePairKey(uint32(shapeID), uint32(queryContext.queryShapeIndex))
	pairExists := containsKey(&bp.pairSet, pairKey)
	if pairExists {
		// contact exists
		return true
	}

	var shapeIDA, shapeIDB int
	if proxyKey < queryProxyKey {
		shapeIDA = shapeID
		shapeIDB = queryContext.queryShapeIndex
	} else {
		shapeIDA = queryContext.queryShapeIndex
		shapeIDB = shapeID
	}

	w := queryContext.world

	shapeA := &w.shapes[shapeIDA]
	shapeB := &w.shapes[shapeIDB]

	bodyIDA := shapeA.bodyID
	bodyIDB := shapeB.bodyID

	// Are the shapes on the same body?
	if bodyIDA == bodyIDB {
		return true
	}

	// Sensors are handled elsewhere
	if shapeA.sensorIndex != NullIndex || shapeB.sensorIndex != NullIndex {
		return true
	}

	if !shouldShapesCollide(shapeA.filter, shapeB.filter) {
		return true
	}

	if !canCollide(shapeA.shapeType, shapeB.shapeType) {
		// For example, no segment vs segment collision
		return true
	}

	// Does a joint override collision?
	bodyA := &w.bodies[bodyIDA]
	bodyB := &w.bodies[bodyIDB]
	if !w.shouldBodiesCollide(bodyA, bodyB) {
		return true
	}

	// Custom user filter
	if shapeA.enableCustomFiltering || shapeB.enableCustomFiltering {
		customFilterFcn := queryContext.world.customFilterFcn
		if customFilterFcn != nil {
			idA := ShapeID{index1: int32(shapeIDA + 1), world0: w.worldID, generation: shapeA.generation}
			idB := ShapeID{index1: int32(shapeIDB + 1), world0: w.worldID, generation: shapeB.generation}
			shouldCollide := customFilterFcn(idA, idB, queryContext.world.customFilterContext)
			if !shouldCollide {
				return true
			}
		}
	}

	// Deviation from upstream: the atomic pair-index bump allocation and the
	// heap fallback collapse to a slice append in this single-threaded port.
	// The new pair is prepended to the move result's list like upstream.
	queryContext.pairs = append(queryContext.pairs, movePair{
		shapeIndexA: shapeIDA,
		shapeIndexB: shapeIDB,
		next:        queryContext.moveResult,
	})
	queryContext.moveResult = len(queryContext.pairs) - 1

	// continue the query
	return true
}

// updateBroadPhasePairs queries the broad-phase trees for every moved proxy
// and creates the resulting contacts in deterministic order (upstream
// b2UpdateBroadPhasePairs).
//
// Deviations from upstream (single-threaded port):
//   - b2FindPairsTask is not split over worker threads; the same loop runs
//     serially in moveArray order. The results follow the order of the
//     moveArray, which is the determinism cornerstone.
//   - The arena-allocated moveResults/movePairs buffers become local slices.
//   - b2UpdateTreesTask runs inline before contact creation, matching the
//     upstream default (serial) task system; taskCount parity is kept.
func (w *World) updateBroadPhasePairs() {
	bp := &w.broadPhase

	moveCount := len(bp.moveArray)
	assert(moveCount == getSetCount(&bp.moveSet))

	if moveCount == 0 {
		return
	}

	// moveResults[i] is the head index of the pair list built for
	// bp.moveArray[i] (upstream b2MoveResult.pairList).
	moveResults := make([]int, moveCount)

	queryContext := queryPairContext{
		world: w,
		// This capacity can be exceeded if there are many overlapping pairs
		// (e.g. all shapes at the origin); append then grows the slice.
		pairs: make([]movePair, 0, 8*moveCount),
	}

	// Find pairs for each moved proxy in moveArray order (upstream
	// b2FindPairsTask over [0, moveCount)).
	for i := range moveCount {
		// Initialize move result for this moved proxy
		queryContext.moveResult = NullIndex

		proxyKey := bp.moveArray[i]
		if proxyKey == NullIndex {
			// proxy was destroyed after it moved
			moveResults[i] = NullIndex
			continue
		}

		proxyType := proxyKeyType(proxyKey)

		proxyID := proxyKeyID(proxyKey)
		queryContext.queryProxyKey = proxyKey

		baseTree := &bp.trees[proxyType]

		// We have to query the tree with the fat AABB so that
		// we don't fail to create a contact that may touch later.
		fatAABB := baseTree.GetAABB(proxyID)
		queryContext.queryShapeIndex = int(baseTree.GetUserData(proxyID))

		// Query trees. Only dynamic proxies collide with kinematic and
		// static proxies. Using DefaultMaskBits so that Filter.GroupIndex
		// works.
		if proxyType == DynamicBody {
			// consider using bits = groupIndex > 0 ? B2_DEFAULT_MASK_BITS : maskBits
			queryContext.queryTreeType = KinematicBody
			_ = bp.trees[KinematicBody].Query(fatAABB, DefaultMaskBits, pairQueryCallback, &queryContext)

			queryContext.queryTreeType = StaticBody
			_ = bp.trees[StaticBody].Query(fatAABB, DefaultMaskBits, pairQueryCallback, &queryContext)
		}

		// All proxies collide with dynamic proxies
		// Using DefaultMaskBits so that Filter.GroupIndex works.
		queryContext.queryTreeType = DynamicBody
		_ = bp.trees[DynamicBody].Query(fatAABB, DefaultMaskBits, pairQueryCallback, &queryContext)

		moveResults[i] = queryContext.moveResult
	}

	// Task that upstream runs in parallel with the narrow-phase
	// (b2UpdateTreesTask): rebuild the collision tree for dynamic and
	// kinematic bodies to keep their query performance good. The upstream
	// default task system runs it inline right here.
	bp.rebuildTrees()
	w.taskCount++

	// Single-threaded work
	// - Clear move flags
	// - Create contacts in deterministic order
	// This is deterministic because the results follow the order of
	// broadPhase.moveArray.
	for i := range moveCount {
		pairIndex := moveResults[i]
		for pairIndex != NullIndex {
			pair := queryContext.pairs[pairIndex]
			shapeIDA := pair.shapeIndexA
			shapeIDB := pair.shapeIndexB

			shapeA := &w.shapes[shapeIDA]
			shapeB := &w.shapes[shapeIDB]

			w.createContact(shapeA, shapeB)

			pairIndex = pair.next
		}
	}

	// Reset move buffer
	bp.moveArray = bp.moveArray[:0]
	clearHashSet(&bp.moveSet)

	w.validateSolverSets()
}

// testOverlap reports whether the fat AABBs of two proxies overlap (upstream
// b2BroadPhase_TestOverlap).
func (bp *broadPhase) testOverlap(proxyKeyA, proxyKeyB int) bool {
	typeIndexA := proxyKeyType(proxyKeyA)
	proxyIDA := proxyKeyID(proxyKeyA)
	typeIndexB := proxyKeyType(proxyKeyB)
	proxyIDB := proxyKeyID(proxyKeyB)

	aabbA := bp.trees[typeIndexA].GetAABB(proxyIDA)
	aabbB := bp.trees[typeIndexB].GetAABB(proxyIDB)
	return AABBOverlaps(aabbA, aabbB)
}

// rebuildTrees rebuilds the dynamic and kinematic trees (upstream
// b2BroadPhase_RebuildTrees).
func (bp *broadPhase) rebuildTrees() {
	bp.trees[DynamicBody].Rebuild(false)
	bp.trees[KinematicBody].Rebuild(false)
}

// getShapeIndex returns the shape index stored in a proxy (upstream
// b2BroadPhase_GetShapeIndex).
func (bp *broadPhase) getShapeIndex(proxyKey int) int {
	typeIndex := proxyKeyType(proxyKey)
	proxyID := proxyKeyID(proxyKey)

	return int(bp.trees[typeIndex].GetUserData(proxyID))
}

// validate checks the dynamic and kinematic trees (upstream
// b2ValidateBroadphase).
func (bp *broadPhase) validate() error {
	if err := bp.trees[DynamicBody].Validate(); err != nil {
		return err
	}

	return bp.trees[KinematicBody].Validate()
}

// validateNoEnlarged checks that no tree has enlarged nodes (upstream
// b2ValidateNoEnlarged).
func (bp *broadPhase) validateNoEnlarged() error {
	for j := range BodyTypeCount {
		if err := bp.trees[j].ValidateNoEnlarged(); err != nil {
			return err
		}
	}

	return nil
}
