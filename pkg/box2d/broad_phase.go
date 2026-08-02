// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/broad_phase.c, src/broad_phase.h.
//
// Deviations from upstream:
//
//   - b2FindPairsTask keeps its parallel-for shape over the move array: it
//     runs serially on worker 0 when the world has no pool, and over static
//     contiguous ascending ranges on the internal worker pool otherwise
//     (worker_pool.go). The b2AtomicInt movePairIndex plus the atomic
//     pair-buffer bump allocator (with heap fallback) collapse to a plain
//     slice append into the owning worker's private pair slice: each worker
//     has its own queryPairContext, moveResults[i] stores an index LOCAL to
//     the owner's slice, and the serial creation loop derives the owner from
//     i with the same partition the dispatch used. Because each move item's
//     pair list is built entirely by one worker in tree traversal order, the
//     per-move-result prepend/consume order — and therefore contact creation
//     order — is byte-identical to the serial loop for every worker count.
//   - The arena-allocated moveResults buffer and the per-worker pair slices
//     stay fields on broadPhase (as upstream has them on b2BroadPhase) and
//     are reused across steps: updateBroadPhasePairs resets their length at
//     entry and lets them grow geometrically, never shrinking, which is how
//     the upstream arena behaves. The buffers are pure scratch — every
//     moveResults slot is written before it is read and each pair slice is
//     only ever read back through indices produced by the appends of the
//     same call — so reuse cannot change contents or ordering.
//   - b2UpdateTreesTask runs inline after the pair find joins and before
//     contact creation, matching the upstream default (serial) task system.
//     It must not overlap the pair queries: it mutates the trees they read.

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

	// moveResults and the per-worker query contexts are the per-step
	// pair-query scratch reused across updateBroadPhasePairs calls (upstream
	// bp->moveResults and bp->movePairs, which are arena allocations).
	// Deviation from upstream: movePairCapacity and movePairIndex have no
	// counterpart because each worker appends into its own
	// queryContexts[k].pairs slice instead of bump-allocating slots from one
	// shared array atomically (see the file header). moveResults[i] holds an
	// index LOCAL to the owning worker's pair slice; the owner is derived
	// from i with the same partition the dispatch used. queryContexts is
	// sized lazily to the dispatch width (the broad-phase does not know the
	// world's worker count at creation); everything grows and never shrinks,
	// and the serial path allocates nothing new (worker 0's context is the
	// old single scratch).
	moveResults   []int
	queryContexts []queryPairContext

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
	bp.moveResults = nil
	bp.queryContexts = nil
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
// b2MovePair). Deviation from upstream: the pairs are stored in a flat
// per-worker slice and linked by index instead of pointers, so the
// arena/heap split (the `heap` flag) has no counterpart. The prepend order
// of the per-move-result list is preserved exactly.
type movePair struct {
	shapeIndexA int
	shapeIndexB int

	// next is the index of the next pair of the same move result in the
	// owning worker's queryPairContext.pairs, or NullIndex (upstream
	// b2MovePair* next). Always local to one worker's slice: a move item's
	// whole pair list is built by the worker that owns the item.
	next int
}

// queryPairContext carries the state of one moved proxy's pair query
// (upstream b2QueryPairContext). Deviation from upstream: the b2MoveResult
// pair list head lives here as an index (moveResult) together with the pair
// slice. One context per worker, persisted on broadPhase.queryContexts;
// during a dispatch a worker touches only its own context.
type queryPairContext struct {
	world *World

	// pairs is the worker-private pair storage shared by the move results
	// this worker owns (upstream bp->movePairs plus heap fallback).
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
	// heap fallback collapse to an append into the calling worker's private
	// pair slice. The new pair is prepended to the move result's list like
	// upstream.
	queryContext.pairs = append(queryContext.pairs, movePair{
		shapeIndexA: shapeIDA,
		shapeIndexB: shapeIDB,
		next:        queryContext.moveResult,
	})
	queryContext.moveResult = len(queryContext.pairs) - 1

	// continue the query
	return true
}

// broadPhaseMinRange is the pair-find dispatch grain (upstream: the minRange
// of 64 passed when enqueueing b2FindPairsTask in b2UpdateBroadPhasePairs).
// Below this many moved proxies the dispatch runs inline on worker 0.
const broadPhaseMinRange = 64

// findPairsTask queries the trees for a contiguous range of moved proxies
// (upstream b2FindPairsTask; workerIndex is upstream's threadIndex). It
// writes only the worker's own queryPairContext and the moveResults slots of
// its own range, and reads the trees, moveSet and pairSet, which are all
// frozen while the pair find runs — so disjoint ranges may run concurrently.
func (w *World) findPairsTask(startIndex, endIndex, workerIndex int) {
	bp := &w.broadPhase
	moveResults := bp.moveResults
	queryContext := &bp.queryContexts[workerIndex]

	// Find pairs for each moved proxy in moveArray order.
	for i := startIndex; i < endIndex; i++ {
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
			_ = bp.trees[KinematicBody].Query(fatAABB, DefaultMaskBits, pairQueryCallback, queryContext)

			queryContext.queryTreeType = StaticBody
			_ = bp.trees[StaticBody].Query(fatAABB, DefaultMaskBits, pairQueryCallback, queryContext)
		}

		// All proxies collide with dynamic proxies
		// Using DefaultMaskBits so that Filter.GroupIndex works.
		queryContext.queryTreeType = DynamicBody
		_ = bp.trees[DynamicBody].Query(fatAABB, DefaultMaskBits, pairQueryCallback, queryContext)

		moveResults[i] = queryContext.moveResult
	}
}

// updateBroadPhasePairs queries the broad-phase trees for every moved proxy
// and creates the resulting contacts in deterministic order (upstream
// b2UpdateBroadPhasePairs).
//
// Deviations from upstream:
//   - b2FindPairsTask runs serially on worker 0 or over static contiguous
//     ascending ranges on the internal worker pool; either way the results
//     follow the order of the moveArray, which is the determinism
//     cornerstone (see the file header).
//   - The moveResults buffer and the per-worker pair slices are
//     per-broadPhase scratch reused across steps (see the file header).
//   - b2UpdateTreesTask runs inline after the pair find and before contact
//     creation, matching the upstream default (serial) task system;
//     taskCount parity is kept.
func (w *World) updateBroadPhasePairs() {
	bp := &w.broadPhase

	moveCount := len(bp.moveArray)
	assert(moveCount == getSetCount(&bp.moveSet))

	if moveCount == 0 {
		return
	}

	// moveResults[i] is the head index of the pair list built for
	// bp.moveArray[i] (upstream b2MoveResult.pairList), LOCAL to the owning
	// worker's pair slice. Reset discipline: the scratch is resized to
	// moveCount and left uncleared because the tasks assign every index
	// [0, moveCount) before anything reads it.
	moveResults := growScratch(bp.moveResults, moveCount)
	bp.moveResults = moveResults

	// INVARIANT: dispatch bound == presize bound == consumer bound ==
	// pairWorkers, the exact worker count the forRange below engages — a
	// pure function of (moveCount, broadPhaseMinRange, workerCount), so it
	// is partition-independent. The serial pair-creation loop at the bottom
	// derives pair ownership from the same forRangeWorkers/workerRange
	// calls, so the two sides must agree; queryContexts slots >=
	// pairWorkers are never written this step and never read this step, so
	// stale pairs in them are harmless. (workerCount is 1 when the world
	// has no pool, so this collapses to 1 on the serial path.)
	pairWorkers := forRangeWorkers(moveCount, broadPhaseMinRange, w.workerCount)

	// Size the per-worker contexts lazily (the broad-phase does not know the
	// world's worker count at creation). Existing contexts keep their grown
	// pair slices.
	if len(bp.queryContexts) < pairWorkers {
		queryContexts := make([]queryPairContext, pairWorkers)
		copy(queryContexts, bp.queryContexts)
		bp.queryContexts = queryContexts
	}

	for k := range pairWorkers {
		start, end := workerRange(moveCount, pairWorkers, k)
		queryContext := &bp.queryContexts[k]
		queryContext.world = w
		// Reset discipline: truncated to zero length so the appends
		// reproduce exactly the indices a fresh slice would. The reserve is
		// the same 8 pairs per owned move item as before and can still be
		// exceeded if there are many overlapping pairs (e.g. all shapes at
		// the origin); append then grows the slice.
		queryContext.pairs = growScratch(queryContext.pairs, 8*(end-start))[:0]
	}

	if w.pool == nil {
		w.findPairsTask(0, moveCount, 0)
	} else {
		w.pool.forRange(moveCount, broadPhaseMinRange, func(workerIndex, startIndex, endIndex int) {
			w.findPairsTask(startIndex, endIndex, workerIndex)
		})
	}

	// Task that upstream runs in parallel with the narrow-phase
	// (b2UpdateTreesTask): rebuild the collision tree for dynamic and
	// kinematic bodies to keep their query performance good. It stays after
	// the pair-find join in this port — the rebuild mutates the trees the
	// pair queries read.
	bp.rebuildTrees()
	w.taskCount++

	// Single-threaded work
	// - Clear move flags
	// - Create contacts in deterministic order
	// This is deterministic because the results follow the order of
	// broadPhase.moveArray: the per-worker ranges are contiguous ascending,
	// so walking workers in ascending index visits move indices in exactly
	// the serial order, and each move item's pair list lives whole in its
	// owner's slice.
	for k := range pairWorkers {
		start, end := workerRange(moveCount, pairWorkers, k)
		pairs := bp.queryContexts[k].pairs
		for i := start; i < end; i++ {
			pairIndex := moveResults[i]
			for pairIndex != NullIndex {
				pair := pairs[pairIndex]
				shapeIDA := pair.shapeIndexA
				shapeIDB := pair.shapeIndexB

				shapeA := &w.shapes[shapeIDA]
				shapeB := &w.shapes[shapeIDB]

				w.createContact(shapeA, shapeB)

				pairIndex = pair.next
			}
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
