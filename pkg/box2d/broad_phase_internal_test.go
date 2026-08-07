// Tests for the float64 port of Box2D v3.2.0 src/broad_phase.c. The broad-phase
// is internal upstream, so these tests live in the package.

package box2d

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func unitBox(x, y float64) AABB {
	return AABB{
		LowerBound: Vec2{X: x - 0.5, Y: y - 0.5},
		UpperBound: Vec2{X: x + 0.5, Y: y + 0.5},
	}
}

func TestProxyKey_PackUnpackRoundTrip(t *testing.T) {
	t.Parallel()

	ids := []int{0, 1, 2, 7, 63, 1024, 1 << 20, (1 << 29) - 1}
	types := []BodyType{StaticBody, KinematicBody, DynamicBody}

	for _, id := range ids {
		for _, bodyType := range types {
			key := makeProxyKey(id, bodyType)
			tassert.Equalf(t, id, proxyKeyID(key), "id %d type %d", id, bodyType)
			tassert.Equalf(t, bodyType, proxyKeyType(key), "id %d type %d", id, bodyType)
		}
	}

	// The type occupies the low two bits and the id the remaining bits.
	tassert.Equal(t, 0, makeProxyKey(0, StaticBody))
	tassert.Equal(t, 2, makeProxyKey(0, DynamicBody))
	tassert.Equal(t, 5, makeProxyKey(1, KinematicBody))
}

func TestBroadPhase_BufferMoveDeduplicates(t *testing.T) {
	t.Parallel()

	var bp broadPhase
	createBroadPhase(&bp)
	defer destroyBroadPhase(&bp)

	bp.bufferMove(5)
	bp.bufferMove(9)
	bp.bufferMove(5)
	bp.bufferMove(1)
	bp.bufferMove(9)

	// Insertion order is preserved and duplicates are dropped.
	tassert.Equal(t, []int{5, 9, 1}, bp.moveArray)
	tassert.Equal(t, len(bp.moveArray), getSetCount(&bp.moveSet))
	tassert.True(t, containsKey(&bp.moveSet, uint64(5+1)))
	tassert.False(t, containsKey(&bp.moveSet, uint64(7+1)))
}

func TestBroadPhase_UnBufferMove(t *testing.T) {
	t.Parallel()

	var bp broadPhase
	createBroadPhase(&bp)
	defer destroyBroadPhase(&bp)

	bp.bufferMove(1)
	bp.bufferMove(2)
	bp.bufferMove(3)

	// Removing an unbuffered proxy is a no-op.
	bp.unBufferMove(99)
	tassert.Equal(t, []int{1, 2, 3}, bp.moveArray)

	// Upstream uses b2IntArray_RemoveSwap, so the last entry fills the hole.
	bp.unBufferMove(1)
	tassert.Equal(t, []int{3, 2}, bp.moveArray)
	tassert.Equal(t, len(bp.moveArray), getSetCount(&bp.moveSet))
	tassert.False(t, containsKey(&bp.moveSet, uint64(1+1)))

	// Removing the tail keeps the remaining order.
	bp.unBufferMove(2)
	tassert.Equal(t, []int{3}, bp.moveArray)

	bp.unBufferMove(3)
	tassert.Empty(t, bp.moveArray)
	tassert.Equal(t, 0, getSetCount(&bp.moveSet))

	// A proxy can be buffered again after removal.
	bp.bufferMove(3)
	tassert.Equal(t, []int{3}, bp.moveArray)
}

func TestBroadPhase_CreateProxyPerBodyType(t *testing.T) {
	t.Parallel()

	var bp broadPhase
	createBroadPhase(&bp)
	defer destroyBroadPhase(&bp)

	staticKey := bp.createProxy(StaticBody, unitBox(0.0, 0.0), DefaultCategoryBits, 11, false)
	kinematicKey := bp.createProxy(KinematicBody, unitBox(1.0, 0.0), DefaultCategoryBits, 22, false)
	dynamicKey := bp.createProxy(DynamicBody, unitBox(2.0, 0.0), 0x4, 33, false)

	tassert.Equal(t, StaticBody, proxyKeyType(staticKey))
	tassert.Equal(t, KinematicBody, proxyKeyType(kinematicKey))
	tassert.Equal(t, DynamicBody, proxyKeyType(dynamicKey))

	// Each proxy lands in the tree for its body type.
	tassert.Equal(t, 1, bp.trees[StaticBody].GetProxyCount())
	tassert.Equal(t, 1, bp.trees[KinematicBody].GetProxyCount())
	tassert.Equal(t, 1, bp.trees[DynamicBody].GetProxyCount())

	// Static proxies are not buffered unless pair creation is forced.
	tassert.Equal(t, []int{kinematicKey, dynamicKey}, bp.moveArray)

	forcedKey := bp.createProxy(StaticBody, unitBox(3.0, 0.0), DefaultCategoryBits, 44, true)
	tassert.Equal(t, []int{kinematicKey, dynamicKey, forcedKey}, bp.moveArray)

	// The shape index is recoverable through the proxy key.
	tassert.Equal(t, 11, bp.getShapeIndex(staticKey))
	tassert.Equal(t, 22, bp.getShapeIndex(kinematicKey))
	tassert.Equal(t, 33, bp.getShapeIndex(dynamicKey))

	// Category bits are stored per proxy.
	tassert.Equal(t, uint64(0x4), bp.trees[DynamicBody].GetCategoryBits(proxyKeyID(dynamicKey)))

	require.NoError(t, bp.validate())
}

func TestBroadPhase_MoveEnlargeAndDestroy(t *testing.T) {
	t.Parallel()

	var bp broadPhase
	createBroadPhase(&bp)
	defer destroyBroadPhase(&bp)

	dynamicKey := bp.createProxy(DynamicBody, unitBox(0.0, 0.0), DefaultCategoryBits, 1, false)
	otherKey := bp.createProxy(DynamicBody, unitBox(0.25, 0.0), DefaultCategoryBits, 2, false)
	farKey := bp.createProxy(DynamicBody, unitBox(50.0, 50.0), DefaultCategoryBits, 3, false)

	tassert.True(t, bp.testOverlap(dynamicKey, otherKey))
	tassert.False(t, bp.testOverlap(dynamicKey, farKey))

	// Moving updates the right tree and rebuffers the proxy.
	bp.moveArray = bp.moveArray[:0]
	clearHashSet(&bp.moveSet)

	moved := unitBox(49.75, 50.0)
	bp.moveProxy(dynamicKey, moved)
	tassert.Equal(t, moved, bp.trees[DynamicBody].GetAABB(proxyKeyID(dynamicKey)))
	tassert.Equal(t, []int{dynamicKey}, bp.moveArray)
	tassert.True(t, bp.testOverlap(dynamicKey, farKey))

	// Enlarging marks ancestors and rebuffers the proxy once.
	grown := AABB{LowerBound: Vec2{X: 40.0, Y: 40.0}, UpperBound: Vec2{X: 60.0, Y: 60.0}}
	bp.enlargeProxy(dynamicKey, grown)
	tassert.Equal(t, grown, bp.trees[DynamicBody].GetAABB(proxyKeyID(dynamicKey)))
	tassert.Equal(t, []int{dynamicKey}, bp.moveArray)
	require.Error(t, bp.validateNoEnlarged())

	// Rebuilding clears the enlarged flags on the dynamic and kinematic trees.
	bp.rebuildTrees()
	require.NoError(t, bp.validate())
	require.NoError(t, bp.validateNoEnlarged())
	tassert.Equal(t, 3, bp.trees[DynamicBody].GetProxyCount())

	// Destroying removes the proxy from the tree and from the move buffer.
	bp.destroyProxy(dynamicKey)
	tassert.Equal(t, 2, bp.trees[DynamicBody].GetProxyCount())
	tassert.Empty(t, bp.moveArray)
	tassert.Equal(t, 0, getSetCount(&bp.moveSet))
	require.NoError(t, bp.validate())

	bp.destroyProxy(otherKey)
	bp.destroyProxy(farKey)
	tassert.Equal(t, 0, bp.trees[DynamicBody].GetProxyCount())
}

func TestBroadPhase_CreateAndDestroyLifecycle(t *testing.T) {
	t.Parallel()

	var bp broadPhase
	createBroadPhase(&bp)

	tassert.Equal(t, 16, getSetCapacity(&bp.moveSet))
	tassert.Equal(t, 32, getSetCapacity(&bp.pairSet))
	for i := range BodyTypeCount {
		tassert.Equalf(t, NullIndex, bp.trees[i].root, "tree %d root", i)
		tassert.Equalf(t, 16, bp.trees[i].nodeCapacity, "tree %d capacity", i)
	}

	bp.createProxy(DynamicBody, unitBox(0.0, 0.0), DefaultCategoryBits, 1, false)

	destroyBroadPhase(&bp)
	tassert.Empty(t, bp.moveArray)
	tassert.Equal(t, 0, getSetCapacity(&bp.moveSet))
	tassert.Equal(t, 0, getSetCapacity(&bp.pairSet))
	tassert.Equal(t, 0, bp.trees[DynamicBody].nodeCapacity)
}

func TestBroadPhase_PairSetTracksShapePairs(t *testing.T) {
	t.Parallel()

	var bp broadPhase
	createBroadPhase(&bp)
	defer destroyBroadPhase(&bp)

	// The pair set is keyed by the canonical shape pair key, so the argument
	// order does not matter.
	key := shapePairKey(3, 9)
	tassert.Equal(t, key, shapePairKey(9, 3))

	tassert.False(t, addKey(&bp.pairSet, key))
	tassert.True(t, containsKey(&bp.pairSet, shapePairKey(9, 3)))
	tassert.True(t, addKey(&bp.pairSet, key))
	tassert.Equal(t, 1, getSetCount(&bp.pairSet))

	tassert.True(t, removeKey(&bp.pairSet, key))
	tassert.Equal(t, 0, getSetCount(&bp.pairSet))
}
