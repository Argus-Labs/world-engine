// Oracle tests for the float64 port of Box2D v3.2.0 src/dynamic_tree.c.
//
// Every case in the upstream suite test/test_dynamic_tree.c is ported here,
// keeping the upstream sub-test names as Go sub-test names. All expectations
// come from the C source of truth (vendored src/dynamic_tree.c) or from the
// upstream test assertions; none were read back from this Go port.
//
// Upstream drift notes (vendored C wins):
//
//   - b2DynamicTree_Create takes a capacity hint in the upstream test
//     (`b2DynamicTree_Create( 16 )`). The vendored C also takes the hint but
//     this port fixed the initial node capacity at 16 and dropped the
//     parameter, so NewDynamicTree() is called without arguments. The hint is
//     only a preallocation, so the assertions are unaffected.
//   - b2DynamicTree_GetByteCount has no counterpart in this port: byte counts
//     are meaningless after the float32 -> float64 change (same reasoning as
//     the arena allocator). TreeRebuildAndValidate keeps the height assertion
//     and drops the byte-count one.
//   - The upstream TreeCreateDestroy case also asserts on tree.nodeCount,
//     which is unexported here; that half lives in
//     oracle_misc_internal_test.go (TestOracleTreeCreateDestroyNodePool).

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// oracleTreeRayHit is the context of the upstream RayCastCallbackFcn: it
// records the last proxy id reported and terminates the cast by returning 0.
type oracleTreeRayHit struct {
	proxyID int
}

// TestOracleTreeCreateDestroy ports TreeCreateDestroy from
// test/test_dynamic_tree.c. The proxy count is 1 after one CreateProxy and 0
// after Destroy (b2DynamicTree_Destroy zeroes the whole struct,
// src/dynamic_tree.c:123).
func TestOracleTreeCreateDestroy(t *testing.T) {
	t.Parallel()

	a := box2d.AABB{
		LowerBound: box2d.Vec2{X: -1.0, Y: -1.0},
		UpperBound: box2d.Vec2{X: 2.0, Y: 2.0},
	}

	tree := box2d.NewDynamicTree()
	tree.CreateProxy(a, 1, 0)

	assert.Equal(t, 1, tree.GetProxyCount())

	tree.Destroy()

	assert.Equal(t, 0, tree.GetProxyCount())
}

// TestOracleTreeRayCast ports all thirteen sub-cases of TreeRayCastTest from
// test/test_dynamic_tree.c against a single AABB [-1,-1]..[1,1]. The expected
// hit/miss follows from b2RayCastInput semantics in src/dynamic_tree.c:1230
// (b2DynamicTree_RayCast): the ray is origin + t * translation for
// t in [0, maxFraction], and slab clipping against the leaf AABB decides the
// visit.
func TestOracleTreeRayCast(t *testing.T) {
	t.Parallel()

	a := box2d.AABB{
		LowerBound: box2d.Vec2{X: -1.0, Y: -1.0},
		UpperBound: box2d.Vec2{X: 1.0, Y: 1.0},
	}

	tests := []struct {
		name   string
		p1     box2d.Vec2
		p2     box2d.Vec2
		expect bool
	}{
		{"1 hits from left side", box2d.Vec2{X: -3.0, Y: 0.0}, box2d.Vec2{X: 3.0, Y: 0.0}, true},
		{"2 hits from right side", box2d.Vec2{X: 3.0, Y: 0.0}, box2d.Vec2{X: -3.0, Y: 0.0}, true},
		{"3 hits from bottom", box2d.Vec2{X: 0.0, Y: -3.0}, box2d.Vec2{X: 0.0, Y: 3.0}, true},
		{"4 hits from top", box2d.Vec2{X: 0.0, Y: 3.0}, box2d.Vec2{X: 0.0, Y: -3.0}, true},
		{"5 misses parallel to x axis", box2d.Vec2{X: -3.0, Y: 2.0}, box2d.Vec2{X: 3.0, Y: 2.0}, false},
		{"6 misses parallel to y axis", box2d.Vec2{X: 2.0, Y: -3.0}, box2d.Vec2{X: 2.0, Y: 3.0}, false},
		{"7 starts inside", box2d.Vec2{X: 0.0, Y: 0.0}, box2d.Vec2{X: 2.0, Y: 0.0}, true},
		{"8 hits the corner diagonally", box2d.Vec2{X: -2.0, Y: -2.0}, box2d.Vec2{X: 2.0, Y: 2.0}, true},
		{"9 parallel to an edge but outside", box2d.Vec2{X: -2.0, Y: 1.5}, box2d.Vec2{X: 2.0, Y: 1.5}, false},
		{"10 parallel to an edge exactly on the boundary", box2d.Vec2{X: -2.0, Y: 1.0}, box2d.Vec2{X: 2.0, Y: 1.0}, true},
		{"11 short ray stops before the box", box2d.Vec2{X: -3.0, Y: 0.0}, box2d.Vec2{X: -2.5, Y: 0.0}, false},
		{"12 zero length ray inside", box2d.Vec2{X: 0.0, Y: 0.0}, box2d.Vec2{X: 0.0, Y: 0.0}, true},
		{"13 ends exactly on the boundary", box2d.Vec2{X: -2.0, Y: 0.0}, box2d.Vec2{X: -1.0, Y: 0.0}, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Each sub-test owns its tree: the parent returns before the
			// parallel sub-tests run, so a shared tree would already be gone.
			tree := box2d.NewDynamicTree()
			defer tree.Destroy()
			proxyID := tree.CreateProxy(a, 1, 0)

			input := box2d.RayCastInput{
				Origin:      test.p1,
				Translation: box2d.Sub(test.p2, test.p1),
				MaxFraction: 1.0,
			}

			hit := oracleTreeRayHit{proxyID: -1}
			tree.RayCast(&input, 1, func(_ *box2d.RayCastInput, id int, _ uint64, _ any) float64 {
				hit.proxyID = id
				return 0.0
			}, nil)

			if test.expect {
				assert.Equal(t, proxyID, hit.proxyID)
			} else {
				assert.Equal(t, -1, hit.proxyID)
			}
		})
	}
}

// TestOracleTreeMultipleProxies ports TreeMultipleProxiesTest from
// test/test_dynamic_tree.c: the proxy count, user data and category bits round
// trip through b2DynamicTree_GetUserData (src/dynamic_tree.c:1115) and
// b2DynamicTree_GetCategoryBits (src/dynamic_tree.c:906).
func TestOracleTreeMultipleProxies(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()
	defer tree.Destroy()

	a1 := box2d.AABB{LowerBound: box2d.Vec2{X: -5.0, Y: -1.0}, UpperBound: box2d.Vec2{X: -3.0, Y: 1.0}}
	a2 := box2d.AABB{LowerBound: box2d.Vec2{X: -1.0, Y: -1.0}, UpperBound: box2d.Vec2{X: 1.0, Y: 1.0}}
	a3 := box2d.AABB{LowerBound: box2d.Vec2{X: 3.0, Y: -1.0}, UpperBound: box2d.Vec2{X: 5.0, Y: 1.0}}

	id1 := tree.CreateProxy(a1, 0x1, 42)
	id2 := tree.CreateProxy(a2, 0x2, 43)
	id3 := tree.CreateProxy(a3, 0x4, 44)

	assert.Equal(t, 3, tree.GetProxyCount())

	assert.Equal(t, uint64(42), tree.GetUserData(id1))
	assert.Equal(t, uint64(43), tree.GetUserData(id2))
	assert.Equal(t, uint64(44), tree.GetUserData(id3))

	assert.Equal(t, uint64(0x1), tree.GetCategoryBits(id1))
	assert.Equal(t, uint64(0x2), tree.GetCategoryBits(id2))
	assert.Equal(t, uint64(0x4), tree.GetCategoryBits(id3))
}

// TestOracleTreeQuery ports TreeQueryTest from test/test_dynamic_tree.c. Both
// the mask-filtered Query (src/dynamic_tree.c:1127) and the unfiltered QueryAll
// (src/dynamic_tree.c:1179) must visit at least the middle proxy and report at
// least one leaf visit.
func TestOracleTreeQuery(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()
	defer tree.Destroy()

	a1 := box2d.AABB{LowerBound: box2d.Vec2{X: -5.0, Y: -1.0}, UpperBound: box2d.Vec2{X: -3.0, Y: 1.0}}
	a2 := box2d.AABB{LowerBound: box2d.Vec2{X: -1.0, Y: -1.0}, UpperBound: box2d.Vec2{X: 1.0, Y: 1.0}}
	a3 := box2d.AABB{LowerBound: box2d.Vec2{X: 3.0, Y: -1.0}, UpperBound: box2d.Vec2{X: 5.0, Y: 1.0}}

	tree.CreateProxy(a1, 0xFF, 0)
	id2 := tree.CreateProxy(a2, 0xFF, 0)
	tree.CreateProxy(a3, 0xFF, 0)

	queryA := box2d.AABB{LowerBound: box2d.Vec2{X: -2.0, Y: -2.0}, UpperBound: box2d.Vec2{X: 2.0, Y: 2.0}}

	found := map[int]bool{}
	stats := tree.Query(queryA, 0xFFFFFFFF, func(proxyID int, _ uint64, _ any) bool {
		found[proxyID] = true
		return true
	}, nil)

	assert.True(t, found[id2], "the middle proxy must be visited")
	assert.GreaterOrEqual(t, stats.LeafVisits, 1)

	var list []int
	allStats := tree.QueryAll(queryA, func(proxyID int, _ uint64, _ any) bool {
		list = append(list, proxyID)
		return true
	}, nil)

	assert.GreaterOrEqual(t, len(list), 1)
	assert.GreaterOrEqual(t, allStats.LeafVisits, 1)
	assert.Contains(t, list, id2)
}

// TestOracleTreeMoveAndEnlarge ports TreeMoveAndEnlargeTest from
// test/test_dynamic_tree.c.
//
// b2DynamicTree_MoveProxy (src/dynamic_tree.c:824) replaces the stored AABB
// exactly, so the read-back is bit-identical. b2DynamicTree_EnlargeProxy
// (src/dynamic_tree.c:840) only grows the stored AABB, so the read-back must
// contain the requested bounds.
func TestOracleTreeMoveAndEnlarge(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()
	defer tree.Destroy()

	a := box2d.AABB{LowerBound: box2d.Vec2{X: 0.0, Y: 0.0}, UpperBound: box2d.Vec2{X: 1.0, Y: 1.0}}
	id := tree.CreateProxy(a, 0x1, 100)

	moved := box2d.AABB{LowerBound: box2d.Vec2{X: 10.0, Y: 10.0}, UpperBound: box2d.Vec2{X: 11.0, Y: 11.0}}
	tree.MoveProxy(id, moved)

	got := tree.GetAABB(id)
	assert.InDelta(t, moved.LowerBound.X, got.LowerBound.X, 0.0)
	assert.InDelta(t, moved.LowerBound.Y, got.LowerBound.Y, 0.0)
	assert.InDelta(t, moved.UpperBound.X, got.UpperBound.X, 0.0)
	assert.InDelta(t, moved.UpperBound.Y, got.UpperBound.Y, 0.0)

	enlarge := box2d.AABB{LowerBound: box2d.Vec2{X: 9.5, Y: 9.5}, UpperBound: box2d.Vec2{X: 11.5, Y: 11.5}}
	tree.EnlargeProxy(id, enlarge)

	got2 := tree.GetAABB(id)
	assert.LessOrEqual(t, got2.LowerBound.X, enlarge.LowerBound.X+1e-6)
	assert.GreaterOrEqual(t, got2.UpperBound.X, enlarge.UpperBound.X-1e-6)

	// b2DynamicTree_EnlargeProxy flags only the ANCESTORS of the enlarged leaf
	// (src/dynamic_tree.c:855-878 walks `nodes[proxyId].parent` upward and
	// never touches the leaf itself). A single-proxy tree has a leaf root with
	// no parent, so no node is flagged and the no-enlarged invariant holds.
	require.NoError(t, tree.ValidateNoEnlarged())
	require.NoError(t, tree.Validate())
}

// TestOracleTreeRebuildAndValidate ports TreeRebuildAndValidateTest from
// test/test_dynamic_tree.c. The b2DynamicTree_GetByteCount assertion is
// dropped (see the file header); the sorted-count and height assertions stay.
func TestOracleTreeRebuildAndValidate(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()
	defer tree.Destroy()

	for i := range 12 {
		x := float64(float64(i) * 2.0)
		a := box2d.AABB{
			LowerBound: box2d.Vec2{X: x - 0.5, Y: -0.5},
			UpperBound: box2d.Vec2{X: x + 0.5, Y: 0.5},
		}
		tree.CreateProxy(a, 0xFF, uint64(i))
	}

	sorted := tree.Rebuild(true)

	assert.GreaterOrEqual(t, sorted, 0)
	assert.Positive(t, tree.GetHeight())
	assert.Equal(t, 12, tree.GetProxyCount())
	require.NoError(t, tree.Validate())
}

// TestOracleTreeRowHeight ports TreeRowHeightTest from
// test/test_dynamic_tree.c: 200 unit boxes in a row must build a tree whose
// height stays below 2 * log2(n).
func TestOracleTreeRowHeight(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()
	defer tree.Destroy()

	const columnCount = 200
	for i := range columnCount {
		x := float64(1.0 * float64(i))
		a := box2d.AABB{
			LowerBound: box2d.Vec2{X: x, Y: 0.0},
			UpperBound: box2d.Vec2{X: x + 1.0, Y: 1.0},
		}
		tree.CreateProxy(a, 1, uint64(i))
	}

	minHeight := math.Log2(float64(columnCount))

	assert.Less(t, float64(tree.GetHeight()), 2.0*minHeight)
	require.NoError(t, tree.Validate())
}

// TestOracleTreeGridHeight ports TreeGridHeightTest from
// test/test_dynamic_tree.c: a 20 x 20 grid of unit boxes must build a tree
// whose height stays below 2 * log2(rows * columns).
func TestOracleTreeGridHeight(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()
	defer tree.Destroy()

	const columnCount = 20
	const rowCount = 20
	for i := range columnCount {
		x := float64(1.0 * float64(i))
		for j := range rowCount {
			y := float64(1.0 * float64(j))
			a := box2d.AABB{
				LowerBound: box2d.Vec2{X: x, Y: y},
				UpperBound: box2d.Vec2{X: x + 1.0, Y: y + 1.0},
			}
			tree.CreateProxy(a, 1, uint64(i))
		}
	}

	minHeight := math.Log2(float64(rowCount * columnCount))

	assert.Less(t, float64(tree.GetHeight()), 2.0*minHeight)
	require.NoError(t, tree.Validate())
}

// TestOracleTreeGridMovement ports TreeGridMovementTest from
// test/test_dynamic_tree.c. The incremental insert keeps the height under
// 2 * log2(n); translating every proxy degrades it but must stay under
// 3 * log2(n); a full rebuild restores the 2 * log2(n) bound.
func TestOracleTreeGridMovement(t *testing.T) {
	t.Parallel()

	const gridCount = 20

	tree := box2d.NewDynamicTree()
	defer tree.Destroy()

	proxyIDs := make([]int, 0, gridCount*gridCount)
	for i := range gridCount {
		x := float64(1.0 * float64(i))
		for j := range gridCount {
			y := float64(1.0 * float64(j))
			a := box2d.AABB{
				LowerBound: box2d.Vec2{X: x, Y: y},
				UpperBound: box2d.Vec2{X: x + 1.0, Y: y + 1.0},
			}
			proxyIDs = append(proxyIDs, tree.CreateProxy(a, 1, uint64(i)))
		}
	}

	require.Len(t, proxyIDs, gridCount*gridCount)

	minHeight := math.Log2(float64(gridCount * gridCount))

	height1 := tree.GetHeight()
	assert.Less(t, float64(height1), 2.0*minHeight)

	offset := box2d.Vec2{X: 10.0, Y: 20.0}
	for _, proxyID := range proxyIDs {
		a := tree.GetAABB(proxyID)
		a.LowerBound = box2d.Add(a.LowerBound, offset)
		a.UpperBound = box2d.Add(a.UpperBound, offset)
		tree.MoveProxy(proxyID, a)
	}

	height2 := tree.GetHeight()
	assert.Less(t, float64(height2), 3.0*minHeight)

	tree.Rebuild(true)

	height3 := tree.GetHeight()
	assert.Less(t, float64(height3), 2.0*minHeight)
	require.NoError(t, tree.Validate())
}

// TestOracleTreeRebuildPartialVersusFull exercises both arms of
// b2DynamicTree_Rebuild (src/dynamic_tree.c:1930). With fullBuild false the
// rebuild only gathers the sub-trees below enlarged nodes; with fullBuild true
// it gathers every leaf. In both cases the C post-conditions are the same: the
// proxy count is preserved, the tree validates and no node stays flagged
// enlarged (b2DynamicTree_ValidateNoEnlarged, src/dynamic_tree.c:1089).
func TestOracleTreeRebuildPartialVersusFull(t *testing.T) {
	t.Parallel()

	build := func() (box2d.DynamicTree, []int) {
		tree := box2d.NewDynamicTree()
		ids := make([]int, 0, 64)
		for i := range 64 {
			x := float64(float64(i%8) * 3.0)
			y := float64(float64(i/8) * 3.0)
			a := box2d.AABB{
				LowerBound: box2d.Vec2{X: x, Y: y},
				UpperBound: box2d.Vec2{X: x + 1.0, Y: y + 1.0},
			}
			ids = append(ids, tree.CreateProxy(a, 1, uint64(i)))
		}
		return tree, ids
	}

	t.Run("partial rebuild", func(t *testing.T) {
		t.Parallel()

		tree, ids := build()
		defer tree.Destroy()

		// Enlarging marks a path of nodes, which is exactly what the partial
		// rebuild collects.
		for _, index := range []int{5, 21, 37, 60} {
			a := tree.GetAABB(ids[index])
			a.LowerBound = box2d.Vec2{X: a.LowerBound.X - 1.0, Y: a.LowerBound.Y - 1.0}
			a.UpperBound = box2d.Vec2{X: a.UpperBound.X + 1.0, Y: a.UpperBound.Y + 1.0}
			tree.EnlargeProxy(ids[index], a)
		}
		require.Error(t, tree.ValidateNoEnlarged(), "EnlargeProxy must flag nodes")

		leafCount := tree.Rebuild(false)
		assert.GreaterOrEqual(t, leafCount, 0)
		assert.Equal(t, 64, tree.GetProxyCount())
		require.NoError(t, tree.Validate())
		require.NoError(t, tree.ValidateNoEnlarged())
	})

	t.Run("full rebuild", func(t *testing.T) {
		t.Parallel()

		tree, _ := build()
		defer tree.Destroy()

		leafCount := tree.Rebuild(true)
		assert.Equal(t, 64, leafCount, "a full rebuild gathers every leaf")
		assert.Equal(t, 64, tree.GetProxyCount())
		require.NoError(t, tree.Validate())
		require.NoError(t, tree.ValidateNoEnlarged())
	})
}

// TestOracleTreeRebuildEmptyAndSingle covers the two early-out shapes of
// b2DynamicTree_Rebuild (src/dynamic_tree.c:1930): a tree with no proxies has
// nothing to gather, and a tree with one proxy has a leaf root whose height is
// zero (b2DynamicTree_GetHeight returns 0 for a null or leaf root,
// src/dynamic_tree.c:912).
func TestOracleTreeRebuildEmptyAndSingle(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		tree := box2d.NewDynamicTree()
		defer tree.Destroy()

		assert.Equal(t, 0, tree.Rebuild(true))
		assert.Equal(t, 0, tree.GetHeight())
		assert.Equal(t, 0, tree.GetProxyCount())
		require.NoError(t, tree.Validate())
	})

	t.Run("single proxy", func(t *testing.T) {
		t.Parallel()

		tree := box2d.NewDynamicTree()
		defer tree.Destroy()

		a := box2d.AABB{LowerBound: box2d.Vec2{X: -1.0, Y: -1.0}, UpperBound: box2d.Vec2{X: 1.0, Y: 1.0}}
		tree.CreateProxy(a, 1, 7)

		tree.Rebuild(true)

		assert.Equal(t, 0, tree.GetHeight())
		assert.Equal(t, 1, tree.GetProxyCount())
		// b2DynamicTree_GetAreaRatio returns 0 for a leaf root
		// (src/dynamic_tree.c:922 early-outs when the root is a leaf).
		assert.InDelta(t, 0.0, tree.GetAreaRatio(), 0.0)
		require.NoError(t, tree.Validate())
	})
}

// TestOracleTreeQueryMaskBits pins down the mask test of
// b2DynamicTree_Query (src/dynamic_tree.c:1127):
//
//	if ( ( node->categoryBits & maskBits ) != 0 && b2AABB_Overlaps( ... ) )
//
// A zero mask therefore visits nothing at all, and each category bit selects
// exactly its own proxy. Internal nodes carry the union of their children's
// category bits (b2ValidateMetrics, src/dynamic_tree.c:1015), so the filter is
// applied at every level.
func TestOracleTreeQueryMaskBits(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()
	defer tree.Destroy()

	ids := make([]int, 0, 3)
	bits := []uint64{0x1, 0x2, 0x4}
	for i, categoryBits := range bits {
		x := float64(float64(i) * 4.0)
		a := box2d.AABB{
			LowerBound: box2d.Vec2{X: x, Y: 0.0},
			UpperBound: box2d.Vec2{X: x + 1.0, Y: 1.0},
		}
		ids = append(ids, tree.CreateProxy(a, categoryBits, uint64(i)))
	}

	everything := box2d.AABB{
		LowerBound: box2d.Vec2{X: -100.0, Y: -100.0},
		UpperBound: box2d.Vec2{X: 100.0, Y: 100.0},
	}

	collect := func(maskBits uint64) []int {
		var visited []int
		tree.Query(everything, maskBits, func(proxyID int, _ uint64, _ any) bool {
			visited = append(visited, proxyID)
			return true
		}, nil)
		return visited
	}

	assert.Empty(t, collect(0), "a zero mask can never overlap a category bit")
	assert.Equal(t, []int{ids[0]}, collect(0x1))
	assert.Equal(t, []int{ids[1]}, collect(0x2))
	assert.ElementsMatch(t, ids, collect(0x7))

	// b2DynamicTree_SetCategoryBits (src/dynamic_tree.c:881) rewrites the leaf
	// bits and re-unions the ancestors, so the filter follows the change.
	tree.SetCategoryBits(ids[0], 0x8)
	assert.Empty(t, collect(0x1))
	assert.Equal(t, []int{ids[0]}, collect(0x8))
	assert.Equal(t, uint64(0x8), tree.GetCategoryBits(ids[0]))
	require.NoError(t, tree.Validate())
}

// TestOracleTreeQueryEarlyOut pins down the early-out contract shared by
// b2DynamicTree_Query and b2DynamicTree_QueryAll: returning false from the
// callback stops the traversal (src/dynamic_tree.c:1127 and :1179 both
// `return result` on a false callback).
func TestOracleTreeQueryEarlyOut(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()
	defer tree.Destroy()

	for i := range 16 {
		x := float64(i)
		a := box2d.AABB{
			LowerBound: box2d.Vec2{X: x, Y: 0.0},
			UpperBound: box2d.Vec2{X: x + 1.0, Y: 1.0},
		}
		tree.CreateProxy(a, 1, uint64(i))
	}

	everything := box2d.AABB{
		LowerBound: box2d.Vec2{X: -100.0, Y: -100.0},
		UpperBound: box2d.Vec2{X: 100.0, Y: 100.0},
	}

	queryVisits := 0
	tree.Query(everything, 1, func(_ int, _ uint64, _ any) bool {
		queryVisits++
		return false
	}, nil)
	assert.Equal(t, 1, queryVisits, "Query stops on the first false callback")

	allVisits := 0
	tree.QueryAll(everything, func(_ int, _ uint64, _ any) bool {
		allVisits++
		return false
	}, nil)
	assert.Equal(t, 1, allVisits, "QueryAll stops on the first false callback")
}

// TestOracleTreeRayCastClipsMaxFraction pins down the clipping contract of
// b2DynamicTree_RayCast (src/dynamic_tree.c:1230): the value returned by the
// callback becomes the new maxFraction, a zero return terminates the cast and
// a returned value of 1 leaves the ray unchanged. This is exactly how
// b2World_CastRay narrows the ray, so the two behaviours must match.
func TestOracleTreeRayCastClipsMaxFraction(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()
	defer tree.Destroy()

	// Three unit boxes in a row along +x at x = 1, 3 and 5.
	near := tree.CreateProxy(box2d.AABB{
		LowerBound: box2d.Vec2{X: 1.0, Y: -0.5},
		UpperBound: box2d.Vec2{X: 2.0, Y: 0.5},
	}, 1, 0)
	mid := tree.CreateProxy(box2d.AABB{
		LowerBound: box2d.Vec2{X: 3.0, Y: -0.5},
		UpperBound: box2d.Vec2{X: 4.0, Y: 0.5},
	}, 1, 1)
	far := tree.CreateProxy(box2d.AABB{
		LowerBound: box2d.Vec2{X: 5.0, Y: -0.5},
		UpperBound: box2d.Vec2{X: 6.0, Y: 0.5},
	}, 1, 2)

	input := box2d.RayCastInput{
		Origin:      box2d.Vec2{X: 0.0, Y: 0.0},
		Translation: box2d.Vec2{X: 10.0, Y: 0.0},
		MaxFraction: 1.0,
	}

	// Returning 1 keeps the full ray: every box on the line is visited.
	var all []int
	tree.RayCast(&input, 1, func(_ *box2d.RayCastInput, proxyID int, _ uint64, _ any) float64 {
		all = append(all, proxyID)
		return 1.0
	}, nil)
	assert.ElementsMatch(t, []int{near, mid, far}, all)

	// Returning 0 from the first hit terminates the cast immediately.
	stopped := 0
	tree.RayCast(&input, 1, func(_ *box2d.RayCastInput, _ int, _ uint64, _ any) float64 {
		stopped++
		return 0.0
	}, nil)
	assert.Equal(t, 1, stopped)

	// A maxFraction of 0.25 covers x in [0, 2.5], so only the near box is on
	// the ray. This is pure slab clipping, no callback feedback involved.
	input.MaxFraction = 0.25
	var clipped []int
	tree.RayCast(&input, 1, func(_ *box2d.RayCastInput, proxyID int, _ uint64, _ any) float64 {
		clipped = append(clipped, proxyID)
		return 1.0
	}, nil)
	assert.Equal(t, []int{near}, clipped)
}

// TestOracleTreeShapeCastMaskAndEarlyOut pins down b2DynamicTree_ShapeCast
// (src/dynamic_tree.c:1350): the category mask gates the traversal exactly
// like the ray cast, and a zero return from the callback stops it.
func TestOracleTreeShapeCastMaskAndEarlyOut(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()
	defer tree.Destroy()

	hit := tree.CreateProxy(box2d.AABB{
		LowerBound: box2d.Vec2{X: 2.0, Y: -1.0},
		UpperBound: box2d.Vec2{X: 3.0, Y: 1.0},
	}, 0x1, 0)
	tree.CreateProxy(box2d.AABB{
		LowerBound: box2d.Vec2{X: 4.0, Y: -1.0},
		UpperBound: box2d.Vec2{X: 5.0, Y: 1.0},
	}, 0x2, 1)

	var input box2d.ShapeCastInput
	input.Proxy = box2d.MakeProxy([]box2d.Vec2{{X: 0.0, Y: 0.0}}, 1, 0.25)
	input.Translation = box2d.Vec2{X: 10.0, Y: 0.0}
	input.MaxFraction = 1.0

	var masked []int
	tree.ShapeCast(&input, 0x1, func(_ *box2d.ShapeCastInput, proxyID int, _ uint64, _ any) float64 {
		masked = append(masked, proxyID)
		return 1.0
	}, nil)
	assert.Equal(t, []int{hit}, masked, "only the 0x1 category proxy passes the mask")

	var none []int
	tree.ShapeCast(&input, 0x4, func(_ *box2d.ShapeCastInput, proxyID int, _ uint64, _ any) float64 {
		none = append(none, proxyID)
		return 1.0
	}, nil)
	assert.Empty(t, none, "an unused category bit matches nothing")

	stopped := 0
	tree.ShapeCast(&input, 0x3, func(_ *box2d.ShapeCastInput, _ int, _ uint64, _ any) float64 {
		stopped++
		return 0.0
	}, nil)
	assert.Equal(t, 1, stopped, "a zero return terminates the shape cast")
}

// TestOracleTreeAreaRatioAndRootBounds pins down b2DynamicTree_GetAreaRatio
// (src/dynamic_tree.c:922) and b2DynamicTree_GetRootBounds
// (src/dynamic_tree.c:947).
//
// The area ratio is the total internal-node perimeter divided by the root
// perimeter, so it is at least 1 for any tree with internal nodes, and the
// root bounds are the union of every leaf AABB.
func TestOracleTreeAreaRatioAndRootBounds(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()
	defer tree.Destroy()

	// Four unit boxes spanning [0, 7] x [0, 7].
	corners := [][2]float64{{0, 0}, {6, 0}, {0, 6}, {6, 6}}
	for i, corner := range corners {
		a := box2d.AABB{
			LowerBound: box2d.Vec2{X: corner[0], Y: corner[1]},
			UpperBound: box2d.Vec2{X: corner[0] + 1.0, Y: corner[1] + 1.0},
		}
		tree.CreateProxy(a, 1, uint64(i))
	}

	bounds := tree.GetRootBounds()
	assert.InDelta(t, 0.0, bounds.LowerBound.X, 1e-12)
	assert.InDelta(t, 0.0, bounds.LowerBound.Y, 1e-12)
	assert.InDelta(t, 7.0, bounds.UpperBound.X, 1e-12)
	assert.InDelta(t, 7.0, bounds.UpperBound.Y, 1e-12)

	assert.GreaterOrEqual(t, tree.GetAreaRatio(), 1.0,
		"the root perimeter is one of the summed internal perimeters")

	// An empty tree has a null root, and b2DynamicTree_GetRootBounds returns
	// the zero AABB in that case.
	empty := box2d.NewDynamicTree()
	defer empty.Destroy()
	emptyBounds := empty.GetRootBounds()
	assert.InDelta(t, 0.0, emptyBounds.LowerBound.X, 0.0)
	assert.InDelta(t, 0.0, emptyBounds.UpperBound.X, 0.0)
	assert.InDelta(t, 0.0, empty.GetAreaRatio(), 0.0)
}

// TestOracleTreeDestroyProxyEmptiesTree checks the b2DynamicTree_DestroyProxy
// contract (src/dynamic_tree.c:807): removing the last leaf leaves a null root,
// so the tree reports zero proxies, zero height and validates.
func TestOracleTreeDestroyProxyEmptiesTree(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()
	defer tree.Destroy()

	ids := make([]int, 0, 5)
	for i := range 5 {
		x := float64(float64(i) * 2.0)
		a := box2d.AABB{
			LowerBound: box2d.Vec2{X: x, Y: 0.0},
			UpperBound: box2d.Vec2{X: x + 1.0, Y: 1.0},
		}
		ids = append(ids, tree.CreateProxy(a, 1, uint64(i)))
	}

	for i, id := range ids {
		tree.DestroyProxy(id)
		assert.Equal(t, len(ids)-i-1, tree.GetProxyCount())
		require.NoError(t, tree.Validate())
	}

	assert.Equal(t, 0, tree.GetHeight())

	// The pool is reusable after the tree drains.
	a := box2d.AABB{LowerBound: box2d.Vec2{X: 0.0, Y: 0.0}, UpperBound: box2d.Vec2{X: 1.0, Y: 1.0}}
	reused := tree.CreateProxy(a, 1, 99)
	assert.Equal(t, 1, tree.GetProxyCount())
	assert.Equal(t, uint64(99), tree.GetUserData(reused))
	require.NoError(t, tree.Validate())
}
