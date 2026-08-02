// Tests for the float64 port of Box2D v3.2.0 src/dynamic_tree.c that need
// access to the unexported node pool.

package box2d

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// treeRand is a tiny deterministic PRNG. The package forbids math/rand so that
// test data is reproducible across runs, platforms and architectures.
type treeRand struct {
	state uint32
}

func (r *treeRand) next() uint32 {
	x := r.state
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	r.state = x
	return x
}

func (r *treeRand) float(lo, hi float64) float64 {
	unit := float64(r.next()>>8) / float64(uint32(1)<<24)
	return lo + float64((hi-lo)*unit)
}

func (r *treeRand) intn(n int) int {
	return int(r.next() % uint32(n))
}

func (r *treeRand) aabb() AABB {
	cx := r.float(-50.0, 50.0)
	cy := r.float(-50.0, 50.0)
	ex := r.float(0.2, 3.0)
	ey := r.float(0.2, 3.0)
	return AABB{
		LowerBound: Vec2{X: cx - ex, Y: cy - ey},
		UpperBound: Vec2{X: cx + ex, Y: cy + ey},
	}
}

// treeOp is one recorded operation of a deterministic op sequence.
type treeOp struct {
	kind int // 0 create, 1 move, 2 enlarge, 3 destroy
	slot int
	aabb AABB
}

// buildOpSequence records a random but reproducible operation sequence.
func buildOpSequence(seed uint32, count int) []treeOp {
	rnd := &treeRand{state: seed}
	ops := make([]treeOp, 0, count)
	live := 0

	for range count {
		kind := rnd.intn(4)
		if live == 0 {
			kind = 0
		}

		op := treeOp{kind: kind}
		switch kind {
		case 0:
			op.aabb = rnd.aabb()
			live++
		case 1:
			op.slot = rnd.intn(live)
			op.aabb = rnd.aabb()
		case 2:
			op.slot = rnd.intn(live)
		default:
			op.slot = rnd.intn(live)
			live--
		}

		ops = append(ops, op)
	}

	return ops
}

// replayOps applies an operation sequence to a fresh tree.
func replayOps(ops []treeOp) DynamicTree {
	tree := NewDynamicTree()
	live := make([]int, 0, len(ops))
	userData := uint64(1)

	for _, op := range ops {
		switch op.kind {
		case 0:
			live = append(live, tree.CreateProxy(op.aabb, DefaultCategoryBits, userData))
			userData++
		case 1:
			tree.MoveProxy(live[op.slot], op.aabb)
		case 2:
			old := tree.GetAABB(live[op.slot])
			grown := AABB{
				LowerBound: Vec2{X: old.LowerBound.X - 1.5, Y: old.LowerBound.Y - 1.5},
				UpperBound: Vec2{X: old.UpperBound.X + 1.5, Y: old.UpperBound.Y + 1.5},
			}
			tree.EnlargeProxy(live[op.slot], grown)
		default:
			tree.DestroyProxy(live[op.slot])
			live[op.slot] = live[len(live)-1]
			live = live[:len(live)-1]
		}
	}

	return tree
}

func TestDynamicTree_DeterministicNodeLayout(t *testing.T) {
	t.Parallel()

	ops := buildOpSequence(0x5eed1234, 600)

	treeA := replayOps(ops)
	treeB := replayOps(ops)

	require.NoError(t, treeA.Validate())
	require.NoError(t, treeB.Validate())

	require.Equal(t, treeA.root, treeB.root)
	require.Equal(t, treeA.nodeCount, treeB.nodeCount)
	require.Equal(t, treeA.nodeCapacity, treeB.nodeCapacity)
	require.Equal(t, treeA.freeList, treeB.freeList)
	require.Equal(t, treeA.proxyCount, treeB.proxyCount)
	require.Len(t, treeB.nodes, len(treeA.nodes))

	for i := range treeA.nodes {
		a := treeA.nodes[i]
		b := treeB.nodes[i]
		tassert.Equalf(t, a.AABB, b.AABB, "node %d aabb", i)
		tassert.Equalf(t, a.CategoryBits, b.CategoryBits, "node %d category bits", i)
		tassert.Equalf(t, a.UserData, b.UserData, "node %d user data", i)
		tassert.Equalf(t, a.Child1, b.Child1, "node %d child1", i)
		tassert.Equalf(t, a.Child2, b.Child2, "node %d child2", i)
		tassert.Equalf(t, a.Parent, b.Parent, "node %d parent", i)
		tassert.Equalf(t, a.Height, b.Height, "node %d height", i)
		tassert.Equalf(t, a.Flags, b.Flags, "node %d flags", i)
	}

	// Identical enumeration order for a fixed set of query boxes.
	queryRand := &treeRand{state: 0x0f0f0f0f}
	for q := range 25 {
		box := queryRand.aabb()

		var seenA, seenB []int
		treeA.Query(box, DefaultMaskBits, func(proxyID int, _ uint64, _ any) bool {
			seenA = append(seenA, proxyID)
			return true
		}, nil)
		treeB.Query(box, DefaultMaskBits, func(proxyID int, _ uint64, _ any) bool {
			seenB = append(seenB, proxyID)
			return true
		}, nil)

		tassert.Equalf(t, seenA, seenB, "query %d enumeration order", q)
	}
}

func TestDynamicTree_DeterministicAfterRebuild(t *testing.T) {
	t.Parallel()

	ops := buildOpSequence(0xabcd0001, 400)

	treeA := replayOps(ops)
	treeB := replayOps(ops)

	tassert.Equal(t, treeA.Rebuild(false), treeB.Rebuild(false))
	require.NoError(t, treeA.Validate())

	require.Equal(t, treeA.root, treeB.root)
	require.Len(t, treeB.nodes, len(treeA.nodes))
	for i := range treeA.nodes {
		tassert.Equalf(t, treeA.nodes[i], treeB.nodes[i], "node %d after rebuild", i)
	}

	tassert.Equal(t, treeA.Rebuild(true), treeB.Rebuild(true))
	require.NoError(t, treeA.Validate())
	require.NoError(t, treeA.ValidateNoEnlarged())

	require.Equal(t, treeA.root, treeB.root)
	for i := range treeA.nodes {
		tassert.Equalf(t, treeA.nodes[i], treeB.nodes[i], "node %d after full rebuild", i)
	}
}

func TestDynamicTree_FreeListReusesNodes(t *testing.T) {
	t.Parallel()

	tree := NewDynamicTree()

	// A fresh pool holds 16 nodes chained through the parent/next slot.
	tassert.Equal(t, 16, tree.nodeCapacity)
	tassert.Equal(t, 0, tree.freeList)
	tassert.Equal(t, 1, tree.nodes[0].Parent)
	tassert.Equal(t, NullIndex, tree.nodes[15].Parent)

	box := AABB{LowerBound: Vec2{X: 0.0, Y: 0.0}, UpperBound: Vec2{X: 1.0, Y: 1.0}}
	first := tree.CreateProxy(box, DefaultCategoryBits, 1)
	tassert.Equal(t, 0, first)
	tassert.Equal(t, 1, tree.nodeCount)

	tree.DestroyProxy(first)
	tassert.Equal(t, 0, tree.nodeCount)
	tassert.Equal(t, first, tree.freeList)
	tassert.Equal(t, uint16(0), tree.nodes[first].Flags)

	// The freed node is handed out again (LIFO free list).
	again := tree.CreateProxy(box, DefaultCategoryBits, 2)
	tassert.Equal(t, first, again)

	// Growing the pool preserves the existing nodes.
	rnd := &treeRand{state: 0x1111}
	for i := range 100 {
		tree.CreateProxy(rnd.aabb(), DefaultCategoryBits, uint64(i)+3)
	}
	tassert.Greater(t, tree.nodeCapacity, 16)
	require.NoError(t, tree.Validate())
	tassert.Equal(t, 101, tree.proxyCount)
}

func TestDynamicTree_ValidateDetectsCorruption(t *testing.T) {
	t.Parallel()

	rnd := &treeRand{state: 0x2222}
	tree := NewDynamicTree()
	for i := range 32 {
		tree.CreateProxy(rnd.aabb(), DefaultCategoryBits, uint64(i))
	}
	require.NoError(t, tree.Validate())

	root := tree.root
	child1 := tree.nodes[root].Child1

	// Broken parent link.
	saved := tree.nodes[child1].Parent
	tree.nodes[child1].Parent = root + 1
	require.Error(t, tree.validateStructure(root))
	tree.nodes[child1].Parent = saved
	require.NoError(t, tree.Validate())

	// Broken height.
	savedHeight := tree.nodes[root].Height
	tree.nodes[root].Height = savedHeight + 1
	require.Error(t, tree.validateMetrics(root))
	tree.nodes[root].Height = savedHeight

	// Broken category bits.
	savedBits := tree.nodes[root].CategoryBits
	tree.nodes[root].CategoryBits = savedBits | 0x80
	require.Error(t, tree.validateMetrics(root))
	tree.nodes[root].CategoryBits = savedBits

	// Broken bounds.
	savedAABB := tree.nodes[root].AABB
	tree.nodes[root].AABB.UpperBound.X -= 1000.0
	require.Error(t, tree.validateMetrics(root))
	tree.nodes[root].AABB = savedAABB

	require.NoError(t, tree.Validate())

	// Stale enlarged flag.
	tree.nodes[root].Flags |= enlargedNode
	require.Error(t, tree.ValidateNoEnlarged())
}

func TestDynamicTree_PartitionMid(t *testing.T) {
	t.Parallel()

	// Trivial cases return count / 2.
	tassert.Equal(t, 0, partitionMid(nil, nil, 0))
	tassert.Equal(t, 0, partitionMid([]int{0}, []Vec2{{X: 0.0, Y: 0.0}}, 1))
	tassert.Equal(t, 1, partitionMid([]int{0, 1}, []Vec2{{X: 0.0, Y: 0.0}, {X: 1.0, Y: 0.0}}, 2))

	// A wide spread along x splits on x and keeps indices paired with centers.
	indices := []int{10, 11, 12, 13}
	centers := []Vec2{{X: 9.0, Y: 0.0}, {X: 0.0, Y: 0.1}, {X: 10.0, Y: 0.0}, {X: 1.0, Y: 0.2}}
	pivot := partitionMid(indices, centers, 4)
	tassert.Equal(t, 2, pivot)
	for i := range 4 {
		want := Vec2{X: 9.0, Y: 0.0}
		switch indices[i] {
		case 10:
			want = Vec2{X: 9.0, Y: 0.0}
		case 11:
			want = Vec2{X: 0.0, Y: 0.1}
		case 12:
			want = Vec2{X: 10.0, Y: 0.0}
		case 13:
			want = Vec2{X: 1.0, Y: 0.2}
		}
		tassert.Equalf(t, want, centers[i], "slot %d kept its center", i)
	}
	for i := range pivot {
		tassert.Lessf(t, centers[i].X, 5.0, "left slot %d", i)
	}
	for i := pivot; i < 4; i++ {
		tassert.GreaterOrEqualf(t, centers[i].X, 5.0, "right slot %d", i)
	}
}
