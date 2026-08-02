// Tests for the float64 port of Box2D v3.2.0 src/dynamic_tree.c.

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// xorshift32 is a tiny deterministic PRNG. The package forbids math/rand so
// that test data is reproducible across runs, platforms and architectures.
type xorshift32 struct {
	state uint32
}

func newXorshift32(seed uint32) *xorshift32 {
	if seed == 0 {
		seed = 0x9e3779b9
	}
	return &xorshift32{state: seed}
}

func (r *xorshift32) next() uint32 {
	x := r.state
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	r.state = x
	return x
}

// unit returns a value in [0, 1).
func (r *xorshift32) unit() float64 {
	return float64(r.next()>>8) / float64(uint32(1)<<24)
}

// float returns a value in [lo, hi).
func (r *xorshift32) float(lo, hi float64) float64 {
	return lo + float64((hi-lo)*r.unit())
}

// intn returns a value in [0, n).
func (r *xorshift32) intn(n int) int {
	return int(r.next() % uint32(n))
}

// randomAABB builds an axis-aligned box with a random center and extents.
func randomAABB(r *xorshift32) box2d.AABB {
	cx := r.float(-100.0, 100.0)
	cy := r.float(-100.0, 100.0)
	ex := r.float(0.1, 4.0)
	ey := r.float(0.1, 4.0)
	return box2d.AABB{
		LowerBound: box2d.Vec2{X: cx - ex, Y: cy - ey},
		UpperBound: box2d.Vec2{X: cx + ex, Y: cy + ey},
	}
}

// hugeAABB covers every proxy created by these tests.
func hugeAABB() box2d.AABB {
	return box2d.AABB{
		LowerBound: box2d.Vec2{X: -1.0e6, Y: -1.0e6},
		UpperBound: box2d.Vec2{X: 1.0e6, Y: 1.0e6},
	}
}

// collectQuery returns the proxy ids visited by a query, in enumeration order.
func collectQuery(tree *box2d.DynamicTree, aabb box2d.AABB) []int {
	var found []int
	tree.Query(aabb, box2d.DefaultMaskBits, func(proxyID int, _ uint64, _ any) bool {
		found = append(found, proxyID)
		return true
	}, nil)
	return found
}

// treeProxy is a proxy tracked by the test harness.
type treeProxy struct {
	id   int
	aabb box2d.AABB
}

func TestDynamicTree_ValidateAfterRandomOperations(t *testing.T) {
	t.Parallel()

	rnd := newXorshift32(0x1234abcd)
	tree := box2d.NewDynamicTree()

	live := make([]treeProxy, 0, 128)
	nextUserData := uint64(1)

	const batchCount = 40
	const batchSize = 25

	for batch := range batchCount {
		for range batchSize {
			op := rnd.intn(4)
			switch {
			case op == 0 || len(live) == 0:
				aabb := randomAABB(rnd)
				id := tree.CreateProxy(aabb, box2d.DefaultCategoryBits, nextUserData)
				nextUserData++
				live = append(live, treeProxy{id: id, aabb: aabb})
			case op == 1:
				i := rnd.intn(len(live))
				aabb := randomAABB(rnd)
				tree.MoveProxy(live[i].id, aabb)
				live[i].aabb = aabb
			case op == 2:
				i := rnd.intn(len(live))
				old := live[i].aabb
				grown := box2d.AABB{
					LowerBound: box2d.Vec2{X: old.LowerBound.X - 1.0, Y: old.LowerBound.Y - 1.0},
					UpperBound: box2d.Vec2{X: old.UpperBound.X + 1.0, Y: old.UpperBound.Y + 1.0},
				}
				tree.EnlargeProxy(live[i].id, grown)
				live[i].aabb = grown
			default:
				i := rnd.intn(len(live))
				tree.DestroyProxy(live[i].id)
				live[i] = live[len(live)-1]
				live = live[:len(live)-1]
			}
		}

		require.NoErrorf(t, tree.Validate(), "batch %d", batch)
		require.Equalf(t, len(live), tree.GetProxyCount(), "batch %d proxy count", batch)

		// The tree enumerates exactly the live proxies.
		leaves := collectQuery(&tree, hugeAABB())
		require.Lenf(t, leaves, len(live), "batch %d leaf count", batch)

		expected := make(map[int]struct{}, len(live))
		for _, p := range live {
			expected[p.id] = struct{}{}
		}
		for _, id := range leaves {
			_, ok := expected[id]
			require.Truef(t, ok, "batch %d: query returned unknown proxy %d", batch, id)
		}

		// Height bounds: a binary tree over n leaves has height in
		// [ceil(log2(n)), n-1].
		n := len(live)
		height := tree.GetHeight()
		if n <= 1 {
			require.Equalf(t, 0, height, "batch %d height", batch)
			continue
		}

		minHeight := int(math.Ceil(math.Log2(float64(n))))
		require.GreaterOrEqualf(t, height, minHeight, "batch %d height lower bound", batch)
		require.LessOrEqualf(t, height, n-1, "batch %d height upper bound", batch)
	}
}

func TestDynamicTree_QueryMatchesBruteForce(t *testing.T) {
	t.Parallel()

	rnd := newXorshift32(0x0badf00d)
	tree := box2d.NewDynamicTree()

	const proxyCount = 200
	boxes := make([]box2d.AABB, proxyCount)
	ids := make([]int, proxyCount)
	for i := range proxyCount {
		boxes[i] = randomAABB(rnd)
		ids[i] = tree.CreateProxy(boxes[i], box2d.DefaultCategoryBits, uint64(i))
	}
	require.NoError(t, tree.Validate())

	for q := range 50 {
		queryBox := randomAABB(rnd)

		found := collectQuery(&tree, queryBox)

		// The enumeration order is deterministic: repeating the query gives
		// the identical sequence.
		assert.Equalf(t, found, collectQuery(&tree, queryBox), "query %d order", q)

		foundSet := make(map[int]struct{}, len(found))
		for _, id := range found {
			_, dup := foundSet[id]
			require.Falsef(t, dup, "query %d returned proxy %d twice", q, id)
			foundSet[id] = struct{}{}
		}

		expectedSet := make(map[int]struct{})
		for i := range proxyCount {
			if box2d.AABBOverlaps(boxes[i], queryBox) {
				expectedSet[ids[i]] = struct{}{}
			}
		}

		require.Equalf(t, expectedSet, foundSet, "query %d result set", q)
	}
}

func TestDynamicTree_QueryMaskBitsFilter(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()
	unit := box2d.AABB{LowerBound: box2d.Vec2{X: 0.0, Y: 0.0}, UpperBound: box2d.Vec2{X: 1.0, Y: 1.0}}

	idA := tree.CreateProxy(unit, 0x1, 1)
	idB := tree.CreateProxy(unit, 0x2, 2)

	assert.Equal(t, []int{idA}, collectQuery2(&tree, unit, 0x1))
	assert.Equal(t, []int{idB}, collectQuery2(&tree, unit, 0x2))
	assert.Len(t, collectQuery2(&tree, unit, 0x3), 2)

	// QueryAll ignores category bits entirely.
	var all []int
	tree.QueryAll(unit, func(proxyID int, _ uint64, _ any) bool {
		all = append(all, proxyID)
		return true
	}, nil)
	assert.Len(t, all, 2)

	tree.SetCategoryBits(idA, 0x2)
	assert.Len(t, collectQuery2(&tree, unit, 0x2), 2)
	assert.Equal(t, uint64(0x2), tree.GetCategoryBits(idA))
	require.NoError(t, tree.Validate())
}

// collectQuery2 runs a masked query and returns the visited proxy ids.
func collectQuery2(tree *box2d.DynamicTree, aabb box2d.AABB, maskBits uint64) []int {
	var found []int
	tree.Query(aabb, maskBits, func(proxyID int, _ uint64, _ any) bool {
		found = append(found, proxyID)
		return true
	}, nil)
	return found
}

func TestDynamicTree_QueryEarlyTermination(t *testing.T) {
	t.Parallel()

	rnd := newXorshift32(0x51ce51ce)
	tree := box2d.NewDynamicTree()
	for i := range 50 {
		tree.CreateProxy(randomAABB(rnd), box2d.DefaultCategoryBits, uint64(i))
	}

	visits := 0
	stats := tree.Query(hugeAABB(), box2d.DefaultMaskBits, func(_ int, _ uint64, _ any) bool {
		visits++
		return false
	}, nil)

	assert.Equal(t, 1, visits)
	assert.Equal(t, 1, stats.LeafVisits)
}

func TestDynamicTree_RayCastMatchesBruteForce(t *testing.T) {
	t.Parallel()

	rnd := newXorshift32(0xfeed5eed)
	tree := box2d.NewDynamicTree()

	const proxyCount = 150
	boxes := make(map[int]box2d.AABB, proxyCount)
	for i := range proxyCount {
		aabb := randomAABB(rnd)
		id := tree.CreateProxy(aabb, box2d.DefaultCategoryBits, uint64(i))
		boxes[id] = aabb
	}
	require.NoError(t, tree.Validate())

	for c := range 30 {
		origin := box2d.Vec2{X: rnd.float(-120.0, 120.0), Y: rnd.float(-120.0, 120.0)}
		target := box2d.Vec2{X: rnd.float(-120.0, 120.0), Y: rnd.float(-120.0, 120.0)}
		input := box2d.RayCastInput{
			Origin:      origin,
			Translation: box2d.Sub(target, origin),
			MaxFraction: 1.0,
		}

		hits := make(map[int]struct{})
		tree.RayCast(&input, box2d.DefaultMaskBits,
			func(sub *box2d.RayCastInput, proxyID int, _ uint64, _ any) float64 {
				p2 := box2d.MulAdd(sub.Origin, sub.MaxFraction, sub.Translation)
				out := box2d.AABBRayCast(boxes[proxyID], sub.Origin, p2)
				if out.Hit {
					hits[proxyID] = struct{}{}
				}
				// Continue without clipping so every candidate is reported.
				return sub.MaxFraction
			}, nil)

		p2 := box2d.MulAdd(input.Origin, input.MaxFraction, input.Translation)
		expected := make(map[int]struct{})
		for id, aabb := range boxes {
			if box2d.AABBRayCast(aabb, input.Origin, p2).Hit {
				expected[id] = struct{}{}
			}
		}

		require.Equalf(t, expected, hits, "ray %d", c)
	}
}

func TestDynamicTree_RayCastEarlyTermination(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()
	for i := range 20 {
		x := float64(i)
		aabb := box2d.AABB{
			LowerBound: box2d.Vec2{X: x, Y: -0.5},
			UpperBound: box2d.Vec2{X: x + 0.5, Y: 0.5},
		}
		tree.CreateProxy(aabb, box2d.DefaultCategoryBits, uint64(i))
	}

	input := box2d.RayCastInput{
		Origin:      box2d.Vec2{X: -1.0, Y: 0.0},
		Translation: box2d.Vec2{X: 40.0, Y: 0.0},
		MaxFraction: 1.0,
	}

	// Returning zero terminates the cast immediately.
	terminated := tree.RayCast(&input, box2d.DefaultMaskBits,
		func(_ *box2d.RayCastInput, _ int, _ uint64, _ any) float64 {
			return 0.0
		}, nil)
	assert.Equal(t, 1, terminated.LeafVisits)

	// Returning the incoming max fraction visits every candidate leaf.
	full := tree.RayCast(&input, box2d.DefaultMaskBits,
		func(sub *box2d.RayCastInput, _ int, _ uint64, _ any) float64 {
			return sub.MaxFraction
		}, nil)
	assert.Equal(t, 20, full.LeafVisits)

	// Clipping the ray at the first hit prunes the rest.
	clipped := tree.RayCast(&input, box2d.DefaultMaskBits,
		func(_ *box2d.RayCastInput, _ int, _ uint64, _ any) float64 {
			return 0.05
		}, nil)
	assert.Less(t, clipped.LeafVisits, full.LeafVisits)
}

func TestDynamicTree_ShapeCastFindsOverlaps(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()
	ids := make([]int, 0, 10)
	for i := range 10 {
		x := 2.0 * float64(i)
		aabb := box2d.AABB{
			LowerBound: box2d.Vec2{X: x, Y: -0.5},
			UpperBound: box2d.Vec2{X: x + 1.0, Y: 0.5},
		}
		ids = append(ids, tree.CreateProxy(aabb, box2d.DefaultCategoryBits, uint64(i)))
	}

	var proxy box2d.ShapeProxy
	proxy.Points[0] = box2d.Vec2{X: -3.0, Y: 0.0}
	proxy.Count = 1
	proxy.Radius = 0.25

	input := box2d.ShapeCastInput{
		Proxy:       proxy,
		Translation: box2d.Vec2{X: 30.0, Y: 0.0},
		MaxFraction: 1.0,
	}

	visited := make(map[int]struct{})
	stats := tree.ShapeCast(&input, box2d.DefaultMaskBits,
		func(sub *box2d.ShapeCastInput, proxyID int, _ uint64, _ any) float64 {
			visited[proxyID] = struct{}{}
			return sub.MaxFraction
		}, nil)

	assert.Equal(t, len(ids), stats.LeafVisits)
	assert.Len(t, visited, len(ids))

	// Category filtering excludes everything.
	none := tree.ShapeCast(&input, 0x2, func(_ *box2d.ShapeCastInput, _ int, _ uint64, _ any) float64 {
		return 1.0
	}, nil)
	assert.Equal(t, 0, none.LeafVisits)

	// A zero return terminates the cast.
	terminated := tree.ShapeCast(&input, box2d.DefaultMaskBits,
		func(_ *box2d.ShapeCastInput, _ int, _ uint64, _ any) float64 {
			return 0.0
		}, nil)
	assert.Equal(t, 1, terminated.LeafVisits)
}

func TestDynamicTree_RebuildPreservesLeaves(t *testing.T) {
	t.Parallel()

	rnd := newXorshift32(0xc0ffee11)
	tree := box2d.NewDynamicTree()

	const proxyCount = 200
	boxes := make(map[int]box2d.AABB, proxyCount)
	for i := range proxyCount {
		aabb := randomAABB(rnd)
		boxes[tree.CreateProxy(aabb, box2d.DefaultCategoryBits, uint64(i))] = aabb
	}

	queries := make([]box2d.AABB, 40)
	before := make([]map[int]struct{}, len(queries))
	for i := range queries {
		queries[i] = randomAABB(rnd)
		before[i] = setOf(collectQuery(&tree, queries[i]))
	}

	rootBefore := tree.GetRootBounds()

	sorted := tree.Rebuild(true)
	assert.Equal(t, proxyCount, sorted)
	require.NoError(t, tree.Validate())
	assert.Equal(t, proxyCount, tree.GetProxyCount())

	// The leaf set is unchanged: same ids, same boxes.
	leaves := collectQuery(&tree, hugeAABB())
	require.Len(t, leaves, proxyCount)
	for _, id := range leaves {
		aabb, ok := boxes[id]
		require.Truef(t, ok, "rebuild introduced unknown proxy %d", id)
		assert.Equal(t, aabb, tree.GetAABB(id))
	}

	assert.Equal(t, rootBefore, tree.GetRootBounds())

	for i := range queries {
		assert.Equalf(t, before[i], setOf(collectQuery(&tree, queries[i])), "query %d after rebuild", i)
	}

	// A partial rebuild of a tree with no enlarged nodes is a no-op that must
	// still validate.
	require.NoError(t, tree.ValidateNoEnlarged())
	tree.Rebuild(false)
	require.NoError(t, tree.Validate())
	assert.Len(t, collectQuery(&tree, hugeAABB()), proxyCount)
}

func TestDynamicTree_RebuildAfterEnlarge(t *testing.T) {
	t.Parallel()

	rnd := newXorshift32(0x2244aabb)
	tree := box2d.NewDynamicTree()

	const proxyCount = 60
	ids := make([]int, 0, proxyCount)
	for i := range proxyCount {
		ids = append(ids, tree.CreateProxy(randomAABB(rnd), box2d.DefaultCategoryBits, uint64(i)))
	}

	for i := 0; i < proxyCount; i += 3 {
		old := tree.GetAABB(ids[i])
		grown := box2d.AABB{
			LowerBound: box2d.Vec2{X: old.LowerBound.X - 2.0, Y: old.LowerBound.Y - 2.0},
			UpperBound: box2d.Vec2{X: old.UpperBound.X + 2.0, Y: old.UpperBound.Y + 2.0},
		}
		tree.EnlargeProxy(ids[i], grown)
	}

	require.NoError(t, tree.Validate())

	sorted := tree.Rebuild(false)
	assert.Positive(t, sorted)
	require.NoError(t, tree.Validate())
	require.NoError(t, tree.ValidateNoEnlarged())
	assert.Len(t, collectQuery(&tree, hugeAABB()), proxyCount)
}

func TestDynamicTree_SingleProxyRebuild(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()
	aabb := box2d.AABB{LowerBound: box2d.Vec2{X: -1.0, Y: -1.0}, UpperBound: box2d.Vec2{X: 1.0, Y: 1.0}}
	id := tree.CreateProxy(aabb, box2d.DefaultCategoryBits, 7)

	assert.Equal(t, 1, tree.Rebuild(true))
	require.NoError(t, tree.Validate())
	assert.Equal(t, 0, tree.GetHeight())
	assert.Equal(t, aabb, tree.GetRootBounds())
	assert.Equal(t, uint64(7), tree.GetUserData(id))

	tree.DestroyProxy(id)
	assert.Equal(t, 0, tree.GetProxyCount())
	assert.Equal(t, box2d.AABB{}, tree.GetRootBounds())
	assert.Equal(t, 0, tree.Rebuild(true))
	assert.InDelta(t, 0.0, tree.GetAreaRatio(), 0.0)
}

func TestDynamicTree_FatAABBBehavior(t *testing.T) {
	t.Parallel()

	tree := box2d.NewDynamicTree()

	tight := box2d.AABB{LowerBound: box2d.Vec2{X: -2.0, Y: -1.0}, UpperBound: box2d.Vec2{X: 2.0, Y: 1.0}}
	fat := fatten(tight)
	id := tree.CreateProxy(fat, box2d.DefaultCategoryBits, 1)

	assert.Equal(t, fat, tree.GetAABB(id))
	assert.True(t, box2d.AABBContains(fat, tight))
	assert.Greater(t, fat.UpperBound.X, tight.UpperBound.X)

	// Moving stores the new fat box verbatim and keeps containing the tight box.
	movedTight := box2d.AABB{LowerBound: box2d.Vec2{X: 8.0, Y: 8.0}, UpperBound: box2d.Vec2{X: 12.0, Y: 10.0}}
	movedFat := fatten(movedTight)
	tree.MoveProxy(id, movedFat)
	assert.Equal(t, movedFat, tree.GetAABB(id))
	assert.True(t, box2d.AABBContains(tree.GetAABB(id), movedTight))
	require.NoError(t, tree.Validate())

	// Enlarging grows ancestors so the root keeps containing the leaf.
	other := tree.CreateProxy(fatten(box2d.AABB{
		LowerBound: box2d.Vec2{X: -20.0, Y: -20.0},
		UpperBound: box2d.Vec2{X: -18.0, Y: -18.0},
	}), box2d.DefaultCategoryBits, 2)
	require.NotEqual(t, id, other)

	grownTight := box2d.AABB{LowerBound: box2d.Vec2{X: 8.0, Y: 8.0}, UpperBound: box2d.Vec2{X: 30.0, Y: 30.0}}
	grownFat := fatten(grownTight)
	require.False(t, box2d.AABBContains(tree.GetAABB(id), grownFat))
	tree.EnlargeProxy(id, grownFat)

	assert.Equal(t, grownFat, tree.GetAABB(id))
	assert.True(t, box2d.AABBContains(grownFat, grownTight))
	assert.True(t, box2d.AABBContains(tree.GetRootBounds(), grownFat))
	require.NoError(t, tree.Validate())
	assert.Error(t, tree.ValidateNoEnlarged())
}

func TestDynamicTree_AreaRatioAndDestroy(t *testing.T) {
	t.Parallel()

	rnd := newXorshift32(0x777f00d)
	tree := box2d.NewDynamicTree()

	assert.InDelta(t, 0.0, tree.GetAreaRatio(), 0.0)
	assert.Equal(t, 0, tree.GetHeight())

	ids := make([]int, 0, 64)
	for i := range 64 {
		ids = append(ids, tree.CreateProxy(randomAABB(rnd), box2d.DefaultCategoryBits, uint64(i)))
	}
	assert.Positive(t, tree.GetAreaRatio())

	for _, id := range ids {
		tree.DestroyProxy(id)
	}
	assert.Equal(t, 0, tree.GetProxyCount())
	require.NoError(t, tree.Validate())

	// Destroy zeroes the tree, exactly like the upstream memset. The result is
	// not a usable tree; only the counters are meaningful.
	tree.Destroy()
	assert.Equal(t, 0, tree.GetProxyCount())
}

// fatten inflates an AABB the way upstream b2ComputeShapeMargin does for a
// non-static shape.
func fatten(a box2d.AABB) box2d.AABB {
	extents := box2d.AABBExtents(a)
	minExtent := math.Min(extents.X, extents.Y)
	margin := math.Min(box2d.MaxAABBMargin, box2d.AABBMarginFraction*minExtent)
	return box2d.AABB{
		LowerBound: box2d.Vec2{X: a.LowerBound.X - margin, Y: a.LowerBound.Y - margin},
		UpperBound: box2d.Vec2{X: a.UpperBound.X + margin, Y: a.UpperBound.Y + margin},
	}
}

// setOf converts a slice of proxy ids into a set.
func setOf(ids []int) map[int]struct{} {
	set := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}
