// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/dynamic_tree.c,
// include/box2d/collision.h (Dynamic Tree group).
//
// Deviations from upstream:
//
//   - All floats are float64.
//   - Node and tree indices are Go int rather than int32_t. The narrow width
//     upstream is a memory-layout optimization; no index ever approaches 2^31,
//     so determinism is unaffected. Height and flags keep their uint16 width
//     because b2MaxUInt16 arithmetic and the flag bit masks depend on it.
//   - b2TreeNode packs (child1, child2) and userData into one union, and
//     (parent, next) into another. Go cannot overlap fields without unsafe, so
//     children/userData become separate fields (upstream never reads userData
//     of an internal node nor children of a leaf outside compiled-out
//     assertions) while parent/next remain a single reused slot, matching
//     upstream exactly.
//   - b2DynamicTree_Validate and friends assert upstream; here they return an
//     error so tests can check tree invariants without enabling panics.
//   - b2DynamicTree_GetByteCount is not ported: it reports C sizeof() values
//     that have no meaning without unsafe.
//   - The B2_TREE_HEURISTIC == 1 branch (b2PartitionSAH and the bin/plane
//     helpers) is compiled out upstream and is not ported. The leafBoxes and
//     binIndices fields exist for parity and stay nil.

package box2d

import (
	"errors"
	"fmt"
	"math"
)

// treeStackSize is B2_TREE_STACK_SIZE.
const treeStackSize = 1024

// Tree node flags (upstream enum b2TreeNodeFlags).
const (
	allocatedNode uint16 = 0x0001
	enlargedNode  uint16 = 0x0002
	leafNode      uint16 = 0x0004
)

// TreeNode is a node in the dynamic tree (upstream b2TreeNode).
type TreeNode struct {
	// AABB is the node bounding box.
	AABB AABB

	// CategoryBits holds the category bits for collision filtering.
	CategoryBits uint64

	// UserData is the user data of a leaf node. Upstream shares storage with
	// Child1/Child2; it is meaningless for internal nodes.
	UserData uint64

	// Child1 is the first child of an internal node (NullIndex for a leaf).
	Child1 int

	// Child2 is the second child of an internal node (NullIndex for a leaf).
	Child2 int

	// Parent is the parent index of an allocated node and the free-list next
	// index of a free node. Upstream reuses this single slot for both
	// meanings (union parent/next).
	Parent int

	// Height is the node height. Leaves have height zero.
	Height uint16

	// Flags is a bit mask of allocatedNode, enlargedNode and leafNode.
	Flags uint16
}

// DynamicTree is a dynamic AABB tree broad-phase, inspired by Nathanael
// Presson's btDbvt (upstream b2DynamicTree).
//
// A dynamic tree arranges data in a binary tree to accelerate queries such as
// AABB queries and ray casts. Leaf nodes are proxies with an AABB. These are
// used to hold a user collision object. Nodes are pooled and relocatable, so
// node indices are used rather than pointers.
type DynamicTree struct {
	// nodes are the tree nodes.
	nodes []TreeNode

	// root is the root index.
	root int

	// nodeCount is the number of allocated nodes.
	nodeCount int

	// nodeCapacity is the allocated node space.
	nodeCapacity int

	// freeList is the head of the node free list.
	freeList int

	// proxyCount is the number of proxies created.
	proxyCount int

	// leafIndices are the leaf indices used during a rebuild.
	leafIndices []int

	// leafBoxes are the leaf bounding boxes used by the SAH rebuild
	// heuristic. Unused: upstream compiles that heuristic out.
	leafBoxes []AABB

	// leafCenters are the leaf bounding box centers used during a rebuild.
	leafCenters []Vec2

	// binIndices are the bins used by the SAH rebuild heuristic. Unused:
	// upstream compiles that heuristic out.
	binIndices []int

	// rebuildCapacity is the allocated space for rebuilding.
	rebuildCapacity int

	// rayCastScratch and shapeCastScratch hold the per-leaf sub-input that
	// RayCast and ShapeCast hand to their callbacks. Upstream keeps this copy
	// on the C stack (`b2RayCastInput subInput = *input;`), but in Go a
	// pointer passed to a func value is assumed to escape, so a stack local
	// would be heap allocated on every query. Keeping the storage on the tree
	// makes the query path allocation free.
	//
	// This is scratch, not state: RayCast/ShapeCast overwrite it on entry and
	// restore the previous value before returning, so a callback that
	// re-enters the same tree still works. A tree, like the World that owns
	// it, must not be RayCast/ShapeCast from two goroutines at once, nor
	// while World.Step is running. The step path itself never uses these
	// fields — its tree traversals go through Query with caller-owned
	// contexts, which is why the internal worker pool (worker_pool.go) may
	// run Query concurrently on a frozen tree. These fields never take part
	// in any simulation arithmetic and so cannot affect determinism.
	rayCastScratch   RayCastInput
	shapeCastScratch ShapeCastInput
}

// TreeStats holds performance results returned by dynamic tree queries
// (upstream b2TreeStats).
type TreeStats struct {
	// NodeVisits is the number of internal nodes visited during the query.
	NodeVisits int

	// LeafVisits is the number of leaf nodes visited during the query.
	LeafVisits int
}

// TreeQueryCallbackFcn receives proxies found in an AABB query. It returns
// true if the query should continue (upstream b2TreeQueryCallbackFcn).
type TreeQueryCallbackFcn func(proxyID int, userData uint64, context any) bool

// TreeRayCastCallbackFcn receives clipped ray cast input for a proxy and
// returns the new ray fraction (upstream b2TreeRayCastCallbackFcn).
//
//   - return a value of 0 to terminate the ray cast
//   - return a value less than input.MaxFraction to clip the ray
//   - return a value of input.MaxFraction to continue without clipping
type TreeRayCastCallbackFcn func(input *RayCastInput, proxyID int, userData uint64, context any) float64

// TreeShapeCastCallbackFcn receives clipped shape cast input for a proxy and
// returns the new fraction (upstream b2TreeShapeCastCallbackFcn). The return
// value semantics match TreeRayCastCallbackFcn.
type TreeShapeCastCallbackFcn func(input *ShapeCastInput, proxyID int, userData uint64, context any) float64

// defaultTreeNode returns the node template used when allocating a node
// (upstream b2_defaultTreeNode).
func defaultTreeNode() TreeNode {
	return TreeNode{
		AABB:         AABB{LowerBound: Vec2{X: 0.0, Y: 0.0}, UpperBound: Vec2{X: 0.0, Y: 0.0}},
		CategoryBits: DefaultCategoryBits,
		UserData:     0,
		Child1:       NullIndex,
		Child2:       NullIndex,
		Parent:       NullIndex,
		Height:       0,
		Flags:        allocatedNode,
	}
}

// isLeaf reports whether the node is a leaf (upstream b2IsLeaf).
func isLeaf(node *TreeNode) bool {
	return node.Flags&leafNode != 0
}

// isAllocated reports whether the node is allocated (upstream b2IsAllocated).
func isAllocated(node *TreeNode) bool {
	return node.Flags&allocatedNode != 0
}

// maxUInt16 returns the larger of two uint16 values (upstream b2MaxUInt16).
func maxUInt16(a, b uint16) uint16 {
	if a > b {
		return a
	}
	return b
}

// NewDynamicTree constructs a tree and initializes the node pool (upstream
// b2DynamicTree_Create).
func NewDynamicTree() DynamicTree {
	var tree DynamicTree

	tree.root = NullIndex

	tree.nodeCapacity = 16
	tree.nodeCount = 0
	tree.nodes = make([]TreeNode, tree.nodeCapacity)

	// Build a linked list for the free list.
	for i := range tree.nodeCapacity - 1 {
		tree.nodes[i].Parent = i + 1
	}

	tree.nodes[tree.nodeCapacity-1].Parent = NullIndex
	tree.freeList = 0

	tree.proxyCount = 0

	tree.leafIndices = nil
	tree.leafBoxes = nil
	tree.leafCenters = nil
	tree.binIndices = nil
	tree.rebuildCapacity = 0

	return tree
}

// Destroy releases the node pool (upstream b2DynamicTree_Destroy).
func (tree *DynamicTree) Destroy() {
	*tree = DynamicTree{}
}

// allocateNode allocates a node from the pool, growing the pool if necessary
// (upstream b2AllocateNode).
func (tree *DynamicTree) allocateNode() int {
	// Expand the node pool as needed.
	if tree.freeList == NullIndex {
		assert(tree.nodeCount == tree.nodeCapacity)

		// The free list is empty. Rebuild a bigger pool.
		oldCapacity := tree.nodeCapacity
		tree.nodeCapacity += oldCapacity >> 1
		newNodes := make([]TreeNode, tree.nodeCapacity)
		copy(newNodes[:tree.nodeCount], tree.nodes[:tree.nodeCount])
		tree.nodes = newNodes

		// Build a linked list for the free list. The parent slot becomes the
		// "next" slot.
		for i := tree.nodeCount; i < tree.nodeCapacity-1; i++ {
			tree.nodes[i].Parent = i + 1
		}

		tree.nodes[tree.nodeCapacity-1].Parent = NullIndex
		tree.freeList = tree.nodeCount
	}

	// Peel a node off the free list.
	nodeIndex := tree.freeList
	node := &tree.nodes[nodeIndex]
	tree.freeList = node.Parent
	*node = defaultTreeNode()
	tree.nodeCount++
	return nodeIndex
}

// freeNode returns a node to the pool (upstream b2FreeNode).
func (tree *DynamicTree) freeNode(nodeID int) {
	assert(0 <= nodeID && nodeID < tree.nodeCapacity)
	assert(0 < tree.nodeCount)
	tree.nodes[nodeID].Parent = tree.freeList
	tree.nodes[nodeID].Flags = 0
	tree.freeList = nodeID
	tree.nodeCount--
}

// Greedy algorithm for sibling selection using the SAH
// We have three nodes A-(B,C) and want to add a leaf D, there are three choices.
// 1: make a new parent for A and D : E-(A-(B,C), D)
// 2: associate D with B
//   a: B is a leaf : A-(E-(B,D), C)
//   b: B is an internal node: A-(B{D},C)
// 3: associate D with C
//   a: C is a leaf : A-(B, E-(C,D))
//   b: C is an internal node: A-(B, C{D})
// All of these have a clear cost except when B or C is an internal node. Hence we need to be greedy.

// The cost for cases 1, 2a, and 3a can be computed using the sibling cost formula.
// cost of sibling H = area(union(H, D)) + increased area of ancestors

// findBestSibling finds the lowest cost sibling for boxD (upstream
// b2FindBestSibling).
//
// Suppose B (or C) is an internal node, then the lowest cost would be one of
// two cases:
// case1: D becomes a sibling of B
// case2: D becomes a descendant of B along with a new internal node of area(D).
func (tree *DynamicTree) findBestSibling(boxD AABB) int {
	centerD := AABBCenter(boxD)
	areaD := perimeter(boxD)

	nodes := tree.nodes
	rootIndex := tree.root

	rootBox := nodes[rootIndex].AABB

	// Area of current node
	areaBase := perimeter(rootBox)

	// Area of inflated node
	directCost := perimeter(AABBUnion(rootBox, boxD))
	inheritedCost := 0.0

	bestSibling := rootIndex
	bestCost := directCost

	// Descend the tree from root, following a single greedy path.
	index := rootIndex
	for nodes[index].Height > 0 {
		child1 := nodes[index].Child1
		child2 := nodes[index].Child2

		// Cost of creating a new parent for this node and the new leaf
		cost := directCost + inheritedCost

		// Sometimes there are multiple identical costs within tolerance.
		// This breaks the ties using the centroid distance.
		if cost < bestCost {
			bestSibling = index
			bestCost = cost
		}

		// Inheritance cost seen by children
		inheritedCost += directCost - areaBase

		leaf1 := nodes[child1].Height == 0
		leaf2 := nodes[child2].Height == 0

		// Cost of descending into child 1
		lowerCost1 := math.MaxFloat64
		box1 := nodes[child1].AABB
		directCost1 := perimeter(AABBUnion(box1, boxD))
		area1 := 0.0
		if leaf1 {
			// Child 1 is a leaf
			// Cost of creating new node and increasing area of node P
			cost1 := directCost1 + inheritedCost

			// Need this here due to the loop condition above
			if cost1 < bestCost {
				bestSibling = child1
				bestCost = cost1
			}
		} else {
			// Child 1 is an internal node
			area1 = perimeter(box1)

			// Lower bound cost of inserting under child 1. The minimum accounts for two possibilities:
			// 1. Child1 could be the sibling with cost1 = inheritedCost + directCost1
			// 2. A descendant of child1 could be the sibling with the lower bound cost of
			//       cost1 = inheritedCost + (directCost1 - area1) + areaD
			// This minimum here leads to the minimum of these two costs.
			lowerCost1 = inheritedCost + directCost1 + minFloat(areaD-area1, 0.0)
		}

		// Cost of descending into child 2
		lowerCost2 := math.MaxFloat64
		box2 := nodes[child2].AABB
		directCost2 := perimeter(AABBUnion(box2, boxD))
		area2 := 0.0
		if leaf2 {
			cost2 := directCost2 + inheritedCost

			if cost2 < bestCost {
				bestSibling = child2
				bestCost = cost2
			}
		} else {
			area2 = perimeter(box2)
			lowerCost2 = inheritedCost + directCost2 + minFloat(areaD-area2, 0.0)
		}

		if leaf1 && leaf2 {
			break
		}

		// Can the cost possibly be decreased?
		if bestCost <= lowerCost1 && bestCost <= lowerCost2 {
			break
		}

		if lowerCost1 == lowerCost2 && !leaf1 {
			assert(lowerCost1 < math.MaxFloat64)
			assert(lowerCost2 < math.MaxFloat64)

			// No clear choice based on lower bound surface area. This can happen when both
			// children fully contain D. Fall back to node distance.
			d1 := Sub(AABBCenter(box1), centerD)
			d2 := Sub(AABBCenter(box2), centerD)
			lowerCost1 = LengthSquared(d1)
			lowerCost2 = LengthSquared(d2)
		}

		// Descend
		if lowerCost1 < lowerCost2 && !leaf1 {
			index = child1
			areaBase = area1
			directCost = directCost1
		} else {
			index = child2
			areaBase = area2
			directCost = directCost2
		}

		assert(nodes[index].Height > 0)
	}

	return bestSibling
}

// Rotation choices (upstream enum b2RotateType).
type rotateType int

const (
	rotateNone rotateType = iota
	rotateBF
	rotateBG
	rotateCD
	rotateCE
)

// rotateNodes performs a left or right rotation if node A is imbalanced
// (upstream b2RotateNodes).
func (tree *DynamicTree) rotateNodes(iA int) {
	assert(iA != NullIndex)

	nodes := tree.nodes

	nodeA := &nodes[iA]
	if nodeA.Height < 2 {
		return
	}

	iB := nodeA.Child1
	iC := nodeA.Child2
	assert(0 <= iB && iB < tree.nodeCapacity)
	assert(0 <= iC && iC < tree.nodeCapacity)

	nodeB := &nodes[iB]
	nodeC := &nodes[iC]

	switch {
	case nodeB.Height == 0:
		// B is a leaf and C is internal
		assert(nodeC.Height > 0)

		iF := nodeC.Child1
		iG := nodeC.Child2
		nodeF := &nodes[iF]
		nodeG := &nodes[iG]
		assert(0 <= iF && iF < tree.nodeCapacity)
		assert(0 <= iG && iG < tree.nodeCapacity)

		// Base cost
		costBase := perimeter(nodeC.AABB)

		// Cost of swapping B and F
		aabbBG := AABBUnion(nodeB.AABB, nodeG.AABB)
		costBF := perimeter(aabbBG)

		// Cost of swapping B and G
		aabbBF := AABBUnion(nodeB.AABB, nodeF.AABB)
		costBG := perimeter(aabbBF)

		if costBase < costBF && costBase < costBG {
			// Rotation does not improve cost
			return
		}

		if costBF < costBG {
			// Swap B and F
			nodeA.Child1 = iF
			nodeC.Child1 = iB

			nodeB.Parent = iC
			nodeF.Parent = iA

			nodeC.AABB = aabbBG

			nodeC.Height = 1 + maxUInt16(nodeB.Height, nodeG.Height)
			nodeA.Height = 1 + maxUInt16(nodeC.Height, nodeF.Height)
			nodeC.CategoryBits = nodeB.CategoryBits | nodeG.CategoryBits
			nodeA.CategoryBits = nodeC.CategoryBits | nodeF.CategoryBits
			nodeC.Flags |= (nodeB.Flags | nodeG.Flags) & enlargedNode
			nodeA.Flags |= (nodeC.Flags | nodeF.Flags) & enlargedNode
		} else {
			// Swap B and G
			nodeA.Child1 = iG
			nodeC.Child2 = iB

			nodeB.Parent = iC
			nodeG.Parent = iA

			nodeC.AABB = aabbBF

			nodeC.Height = 1 + maxUInt16(nodeB.Height, nodeF.Height)
			nodeA.Height = 1 + maxUInt16(nodeC.Height, nodeG.Height)
			nodeC.CategoryBits = nodeB.CategoryBits | nodeF.CategoryBits
			nodeA.CategoryBits = nodeC.CategoryBits | nodeG.CategoryBits
			nodeC.Flags |= (nodeB.Flags | nodeF.Flags) & enlargedNode
			nodeA.Flags |= (nodeC.Flags | nodeG.Flags) & enlargedNode
		}

	case nodeC.Height == 0:
		// C is a leaf and B is internal
		assert(nodeB.Height > 0)

		iD := nodeB.Child1
		iE := nodeB.Child2
		nodeD := &nodes[iD]
		nodeE := &nodes[iE]
		assert(0 <= iD && iD < tree.nodeCapacity)
		assert(0 <= iE && iE < tree.nodeCapacity)

		// Base cost
		costBase := perimeter(nodeB.AABB)

		// Cost of swapping C and D
		aabbCE := AABBUnion(nodeC.AABB, nodeE.AABB)
		costCD := perimeter(aabbCE)

		// Cost of swapping C and E
		aabbCD := AABBUnion(nodeC.AABB, nodeD.AABB)
		costCE := perimeter(aabbCD)

		if costBase < costCD && costBase < costCE {
			// Rotation does not improve cost
			return
		}

		if costCD < costCE {
			// Swap C and D
			nodeA.Child2 = iD
			nodeB.Child1 = iC

			nodeC.Parent = iB
			nodeD.Parent = iA

			nodeB.AABB = aabbCE

			nodeB.Height = 1 + maxUInt16(nodeC.Height, nodeE.Height)
			nodeA.Height = 1 + maxUInt16(nodeB.Height, nodeD.Height)
			nodeB.CategoryBits = nodeC.CategoryBits | nodeE.CategoryBits
			nodeA.CategoryBits = nodeB.CategoryBits | nodeD.CategoryBits
			nodeB.Flags |= (nodeC.Flags | nodeE.Flags) & enlargedNode
			nodeA.Flags |= (nodeB.Flags | nodeD.Flags) & enlargedNode
		} else {
			// Swap C and E
			nodeA.Child2 = iE
			nodeB.Child2 = iC

			nodeC.Parent = iB
			nodeE.Parent = iA

			nodeB.AABB = aabbCD
			nodeB.Height = 1 + maxUInt16(nodeC.Height, nodeD.Height)
			nodeA.Height = 1 + maxUInt16(nodeB.Height, nodeE.Height)
			nodeB.CategoryBits = nodeC.CategoryBits | nodeD.CategoryBits
			nodeA.CategoryBits = nodeB.CategoryBits | nodeE.CategoryBits
			nodeB.Flags |= (nodeC.Flags | nodeD.Flags) & enlargedNode
			nodeA.Flags |= (nodeB.Flags | nodeE.Flags) & enlargedNode
		}

	default:
		iD := nodeB.Child1
		iE := nodeB.Child2
		iF := nodeC.Child1
		iG := nodeC.Child2

		assert(0 <= iD && iD < tree.nodeCapacity)
		assert(0 <= iE && iE < tree.nodeCapacity)
		assert(0 <= iF && iF < tree.nodeCapacity)
		assert(0 <= iG && iG < tree.nodeCapacity)

		nodeD := &nodes[iD]
		nodeE := &nodes[iE]
		nodeF := &nodes[iF]
		nodeG := &nodes[iG]

		// Base cost
		areaB := perimeter(nodeB.AABB)
		areaC := perimeter(nodeC.AABB)
		costBase := areaB + areaC
		bestRotation := rotateNone
		bestCost := costBase

		// Cost of swapping B and F
		aabbBG := AABBUnion(nodeB.AABB, nodeG.AABB)
		costBF := areaB + perimeter(aabbBG)
		if costBF < bestCost {
			bestRotation = rotateBF
			bestCost = costBF
		}

		// Cost of swapping B and G
		aabbBF := AABBUnion(nodeB.AABB, nodeF.AABB)
		costBG := areaB + perimeter(aabbBF)
		if costBG < bestCost {
			bestRotation = rotateBG
			bestCost = costBG
		}

		// Cost of swapping C and D
		aabbCE := AABBUnion(nodeC.AABB, nodeE.AABB)
		costCD := areaC + perimeter(aabbCE)
		if costCD < bestCost {
			bestRotation = rotateCD
			bestCost = costCD
		}

		// Cost of swapping C and E
		aabbCD := AABBUnion(nodeC.AABB, nodeD.AABB)
		costCE := areaC + perimeter(aabbCD)
		if costCE < bestCost {
			bestRotation = rotateCE
			// bestCost = costCE
		}

		switch bestRotation {
		case rotateNone:

		case rotateBF:
			nodeA.Child1 = iF
			nodeC.Child1 = iB

			nodeB.Parent = iC
			nodeF.Parent = iA

			nodeC.AABB = aabbBG
			nodeC.Height = 1 + maxUInt16(nodeB.Height, nodeG.Height)
			nodeA.Height = 1 + maxUInt16(nodeC.Height, nodeF.Height)
			nodeC.CategoryBits = nodeB.CategoryBits | nodeG.CategoryBits
			nodeA.CategoryBits = nodeC.CategoryBits | nodeF.CategoryBits
			nodeC.Flags |= (nodeB.Flags | nodeG.Flags) & enlargedNode
			nodeA.Flags |= (nodeC.Flags | nodeF.Flags) & enlargedNode

		case rotateBG:
			nodeA.Child1 = iG
			nodeC.Child2 = iB

			nodeB.Parent = iC
			nodeG.Parent = iA

			nodeC.AABB = aabbBF
			nodeC.Height = 1 + maxUInt16(nodeB.Height, nodeF.Height)
			nodeA.Height = 1 + maxUInt16(nodeC.Height, nodeG.Height)
			nodeC.CategoryBits = nodeB.CategoryBits | nodeF.CategoryBits
			nodeA.CategoryBits = nodeC.CategoryBits | nodeG.CategoryBits
			nodeC.Flags |= (nodeB.Flags | nodeF.Flags) & enlargedNode
			nodeA.Flags |= (nodeC.Flags | nodeG.Flags) & enlargedNode

		case rotateCD:
			nodeA.Child2 = iD
			nodeB.Child1 = iC

			nodeC.Parent = iB
			nodeD.Parent = iA

			nodeB.AABB = aabbCE
			nodeB.Height = 1 + maxUInt16(nodeC.Height, nodeE.Height)
			nodeA.Height = 1 + maxUInt16(nodeB.Height, nodeD.Height)
			nodeB.CategoryBits = nodeC.CategoryBits | nodeE.CategoryBits
			nodeA.CategoryBits = nodeB.CategoryBits | nodeD.CategoryBits
			nodeB.Flags |= (nodeC.Flags | nodeE.Flags) & enlargedNode
			nodeA.Flags |= (nodeB.Flags | nodeD.Flags) & enlargedNode

		case rotateCE:
			nodeA.Child2 = iE
			nodeB.Child2 = iC

			nodeC.Parent = iB
			nodeE.Parent = iA

			nodeB.AABB = aabbCD
			nodeB.Height = 1 + maxUInt16(nodeC.Height, nodeD.Height)
			nodeA.Height = 1 + maxUInt16(nodeB.Height, nodeE.Height)
			nodeB.CategoryBits = nodeC.CategoryBits | nodeD.CategoryBits
			nodeA.CategoryBits = nodeB.CategoryBits | nodeE.CategoryBits
			nodeB.Flags |= (nodeC.Flags | nodeD.Flags) & enlargedNode
			nodeA.Flags |= (nodeB.Flags | nodeE.Flags) & enlargedNode

		default:
			assert(false)
		}
	}
}

// insertLeaf inserts a leaf node into the tree (upstream b2InsertLeaf).
func (tree *DynamicTree) insertLeaf(leaf int, shouldRotate bool) {
	if tree.root == NullIndex {
		tree.root = leaf
		tree.nodes[tree.root].Parent = NullIndex
		return
	}

	// Stage 1: find the best sibling for this node
	leafAABB := tree.nodes[leaf].AABB
	sibling := tree.findBestSibling(leafAABB)

	// Stage 2: create a new parent for the leaf and sibling
	oldParent := tree.nodes[sibling].Parent
	newParent := tree.allocateNode()

	// Warning: the node slice can change after allocation
	nodes := tree.nodes
	nodes[newParent].Parent = oldParent
	nodes[newParent].UserData = math.MaxUint64
	nodes[newParent].AABB = AABBUnion(leafAABB, nodes[sibling].AABB)
	nodes[newParent].CategoryBits = nodes[leaf].CategoryBits | nodes[sibling].CategoryBits
	nodes[newParent].Height = nodes[sibling].Height + 1
	nodes[newParent].Child1 = sibling
	nodes[newParent].Child2 = leaf
	nodes[sibling].Parent = newParent
	nodes[leaf].Parent = newParent

	// Fix grandparent links
	if oldParent != NullIndex {
		// The sibling was not the root
		if nodes[oldParent].Child1 == sibling {
			nodes[oldParent].Child1 = newParent
		} else {
			assert(nodes[oldParent].Child2 == sibling)
			nodes[oldParent].Child2 = newParent
		}
	} else {
		// The sibling was the root
		tree.root = newParent
	}

	// Stage 3: walk back up the tree fixing heights and AABBs
	index := nodes[leaf].Parent
	for index != NullIndex {
		child1 := nodes[index].Child1
		child2 := nodes[index].Child2

		assert(child1 != NullIndex)
		assert(child2 != NullIndex)

		nodes[index].AABB = AABBUnion(nodes[child1].AABB, nodes[child2].AABB)
		nodes[index].CategoryBits = nodes[child1].CategoryBits | nodes[child2].CategoryBits
		nodes[index].Height = 1 + maxUInt16(nodes[child1].Height, nodes[child2].Height)
		nodes[index].Flags |= (nodes[child1].Flags | nodes[child2].Flags) & enlargedNode

		if shouldRotate {
			tree.rotateNodes(index)
		}

		index = nodes[index].Parent
	}
}

// removeLeaf removes a leaf node from the tree (upstream b2RemoveLeaf).
func (tree *DynamicTree) removeLeaf(leaf int) {
	if leaf == tree.root {
		tree.root = NullIndex
		return
	}

	nodes := tree.nodes

	parent := nodes[leaf].Parent
	grandParent := nodes[parent].Parent
	var sibling int
	if nodes[parent].Child1 == leaf {
		sibling = nodes[parent].Child2
	} else {
		sibling = nodes[parent].Child1
	}

	if grandParent != NullIndex {
		// Destroy parent and connect sibling to grandParent.
		if nodes[grandParent].Child1 == parent {
			nodes[grandParent].Child1 = sibling
		} else {
			nodes[grandParent].Child2 = sibling
		}
		nodes[sibling].Parent = grandParent
		tree.freeNode(parent)

		// Adjust ancestor bounds.
		index := grandParent
		for index != NullIndex {
			node := &nodes[index]
			child1 := &nodes[node.Child1]
			child2 := &nodes[node.Child2]

			node.AABB = AABBUnion(child1.AABB, child2.AABB)
			node.CategoryBits = child1.CategoryBits | child2.CategoryBits
			node.Height = 1 + maxUInt16(child1.Height, child2.Height)

			index = node.Parent
		}
	} else {
		tree.root = sibling
		tree.nodes[sibling].Parent = NullIndex
		tree.freeNode(parent)
	}
}

// CreateProxy creates a proxy in the tree as a leaf node and returns the node
// index (upstream b2DynamicTree_CreateProxy).
func (tree *DynamicTree) CreateProxy(aabb AABB, categoryBits uint64, userData uint64) int {
	assert(-Huge < aabb.LowerBound.X && aabb.LowerBound.X < Huge)
	assert(-Huge < aabb.LowerBound.Y && aabb.LowerBound.Y < Huge)
	assert(-Huge < aabb.UpperBound.X && aabb.UpperBound.X < Huge)
	assert(-Huge < aabb.UpperBound.Y && aabb.UpperBound.Y < Huge)

	proxyID := tree.allocateNode()
	node := &tree.nodes[proxyID]

	node.AABB = aabb
	node.UserData = userData
	node.CategoryBits = categoryBits
	node.Height = 0
	node.Flags = allocatedNode | leafNode

	shouldRotate := true
	tree.insertLeaf(proxyID, shouldRotate)

	tree.proxyCount++

	return proxyID
}

// DestroyProxy destroys a proxy (upstream b2DynamicTree_DestroyProxy).
func (tree *DynamicTree) DestroyProxy(proxyID int) {
	assert(0 <= proxyID && proxyID < tree.nodeCapacity)
	assert(isLeaf(&tree.nodes[proxyID]))

	tree.removeLeaf(proxyID)
	tree.freeNode(proxyID)

	assert(tree.proxyCount > 0)
	tree.proxyCount--
}

// GetProxyCount returns the number of proxies created (upstream
// b2DynamicTree_GetProxyCount).
func (tree *DynamicTree) GetProxyCount() int {
	return tree.proxyCount
}

// MoveProxy moves a proxy to a new AABB by removing and reinserting it
// (upstream b2DynamicTree_MoveProxy).
func (tree *DynamicTree) MoveProxy(proxyID int, aabb AABB) {
	assert(IsValidAABB(aabb))
	assert(aabb.UpperBound.X-aabb.LowerBound.X < Huge)
	assert(aabb.UpperBound.Y-aabb.LowerBound.Y < Huge)
	assert(0 <= proxyID && proxyID < tree.nodeCapacity)
	assert(isLeaf(&tree.nodes[proxyID]))

	tree.removeLeaf(proxyID)

	tree.nodes[proxyID].AABB = aabb

	shouldRotate := false
	tree.insertLeaf(proxyID, shouldRotate)
}

// EnlargeProxy enlarges a proxy and its ancestors as necessary (upstream
// b2DynamicTree_EnlargeProxy).
func (tree *DynamicTree) EnlargeProxy(proxyID int, aabb AABB) {
	nodes := tree.nodes

	assert(IsValidAABB(aabb))
	assert(aabb.UpperBound.X-aabb.LowerBound.X < Huge)
	assert(aabb.UpperBound.Y-aabb.LowerBound.Y < Huge)
	assert(0 <= proxyID && proxyID < tree.nodeCapacity)
	assert(isLeaf(&tree.nodes[proxyID]))

	// Caller must ensure this
	assert(!AABBContains(nodes[proxyID].AABB, aabb))

	nodes[proxyID].AABB = aabb

	parentIndex := nodes[proxyID].Parent
	for parentIndex != NullIndex {
		changed := enlargeAABB(&nodes[parentIndex].AABB, aabb)
		nodes[parentIndex].Flags |= enlargedNode
		parentIndex = nodes[parentIndex].Parent

		if !changed {
			break
		}
	}

	for parentIndex != NullIndex {
		if nodes[parentIndex].Flags&enlargedNode != 0 {
			// early out because this ancestor was previously ascended and marked as enlarged
			break
		}

		nodes[parentIndex].Flags |= enlargedNode
		parentIndex = nodes[parentIndex].Parent
	}
}

// SetCategoryBits modifies the category bits on a proxy. This is an expensive
// operation (upstream b2DynamicTree_SetCategoryBits).
func (tree *DynamicTree) SetCategoryBits(proxyID int, categoryBits uint64) {
	nodes := tree.nodes

	assert(nodes[proxyID].Child1 == NullIndex)
	assert(nodes[proxyID].Child2 == NullIndex)
	assert(nodes[proxyID].Flags&leafNode == leafNode)

	nodes[proxyID].CategoryBits = categoryBits

	// Fix up category bits in ancestor internal nodes
	nodeIndex := nodes[proxyID].Parent
	for nodeIndex != NullIndex {
		node := &nodes[nodeIndex]
		child1 := node.Child1
		assert(child1 != NullIndex)
		child2 := node.Child2
		assert(child2 != NullIndex)
		node.CategoryBits = nodes[child1].CategoryBits | nodes[child2].CategoryBits

		nodeIndex = node.Parent
	}
}

// GetCategoryBits returns the category bits on a proxy (upstream
// b2DynamicTree_GetCategoryBits).
func (tree *DynamicTree) GetCategoryBits(proxyID int) uint64 {
	assert(0 <= proxyID && proxyID < tree.nodeCapacity)
	return tree.nodes[proxyID].CategoryBits
}

// GetHeight returns the height of the binary tree (upstream
// b2DynamicTree_GetHeight).
func (tree *DynamicTree) GetHeight() int {
	if tree.root == NullIndex {
		return 0
	}

	return int(tree.nodes[tree.root].Height)
}

// GetAreaRatio returns the ratio of the sum of the internal node areas to the
// root area (upstream b2DynamicTree_GetAreaRatio).
func (tree *DynamicTree) GetAreaRatio() float64 {
	if tree.root == NullIndex {
		return 0.0
	}

	root := &tree.nodes[tree.root]
	rootArea := perimeter(root.AABB)

	totalArea := 0.0
	for i := range tree.nodeCapacity {
		node := &tree.nodes[i]
		if !isAllocated(node) || isLeaf(node) || i == tree.root {
			continue
		}

		totalArea += perimeter(node.AABB)
	}

	return totalArea / rootArea
}

// GetRootBounds returns the bounding box that contains the entire tree
// (upstream b2DynamicTree_GetRootBounds).
func (tree *DynamicTree) GetRootBounds() AABB {
	if tree.root != NullIndex {
		return tree.nodes[tree.root].AABB
	}

	return AABB{LowerBound: Vec2Zero, UpperBound: Vec2Zero}
}

// GetUserData returns the user data of a proxy (upstream
// b2DynamicTree_GetUserData).
func (tree *DynamicTree) GetUserData(proxyID int) uint64 {
	assert(0 <= proxyID && proxyID < tree.nodeCapacity)
	return tree.nodes[proxyID].UserData
}

// GetAABB returns the AABB of a proxy (upstream b2DynamicTree_GetAABB).
func (tree *DynamicTree) GetAABB(proxyID int) AABB {
	assert(0 <= proxyID && proxyID < tree.nodeCapacity)
	return tree.nodes[proxyID].AABB
}

// errTreeValidation reports a broken dynamic tree invariant.
var errTreeValidation = errors.New("box2d: dynamic tree validation failed")

// computeHeight computes the height of a sub-tree (upstream b2ComputeHeight).
func (tree *DynamicTree) computeHeight(nodeID int) int {
	assert(0 <= nodeID && nodeID < tree.nodeCapacity)
	node := &tree.nodes[nodeID]

	if isLeaf(node) {
		return 0
	}

	height1 := tree.computeHeight(node.Child1)
	height2 := tree.computeHeight(node.Child2)
	return 1 + maxInt(height1, height2)
}

// validateStructure checks the parent/child links of a sub-tree (upstream
// b2ValidateStructure).
func (tree *DynamicTree) validateStructure(index int) error {
	if index == NullIndex {
		return nil
	}

	if index == tree.root && tree.nodes[index].Parent != NullIndex {
		return fmt.Errorf("%w: root %d has parent %d", errTreeValidation, index, tree.nodes[index].Parent)
	}

	node := &tree.nodes[index]

	if node.Flags != 0 && node.Flags&allocatedNode == 0 {
		return fmt.Errorf("%w: node %d has flags %d but is not allocated", errTreeValidation, index, node.Flags)
	}

	if isLeaf(node) {
		if node.Height != 0 {
			return fmt.Errorf("%w: leaf %d has height %d", errTreeValidation, index, node.Height)
		}
		return nil
	}

	child1 := node.Child1
	child2 := node.Child2

	if child1 < 0 || child1 >= tree.nodeCapacity {
		return fmt.Errorf("%w: node %d child1 %d out of range", errTreeValidation, index, child1)
	}
	if child2 < 0 || child2 >= tree.nodeCapacity {
		return fmt.Errorf("%w: node %d child2 %d out of range", errTreeValidation, index, child2)
	}

	if tree.nodes[child1].Parent != index {
		return fmt.Errorf("%w: node %d child1 %d has parent %d", errTreeValidation, index, child1,
			tree.nodes[child1].Parent)
	}
	if tree.nodes[child2].Parent != index {
		return fmt.Errorf("%w: node %d child2 %d has parent %d", errTreeValidation, index, child2,
			tree.nodes[child2].Parent)
	}

	if (tree.nodes[child1].Flags|tree.nodes[child2].Flags)&enlargedNode != 0 && node.Flags&enlargedNode == 0 {
		return fmt.Errorf("%w: node %d is not marked enlarged but a child is", errTreeValidation, index)
	}

	if err := tree.validateStructure(child1); err != nil {
		return err
	}

	return tree.validateStructure(child2)
}

// validateMetrics checks the heights, bounds and category bits of a sub-tree
// (upstream b2ValidateMetrics).
func (tree *DynamicTree) validateMetrics(index int) error {
	if index == NullIndex {
		return nil
	}

	node := &tree.nodes[index]

	if isLeaf(node) {
		if node.Height != 0 {
			return fmt.Errorf("%w: leaf %d has height %d", errTreeValidation, index, node.Height)
		}
		return nil
	}

	child1 := node.Child1
	child2 := node.Child2

	if child1 < 0 || child1 >= tree.nodeCapacity {
		return fmt.Errorf("%w: node %d child1 %d out of range", errTreeValidation, index, child1)
	}
	if child2 < 0 || child2 >= tree.nodeCapacity {
		return fmt.Errorf("%w: node %d child2 %d out of range", errTreeValidation, index, child2)
	}

	height1 := tree.nodes[child1].Height
	height2 := tree.nodes[child2].Height
	height := 1 + maxUInt16(height1, height2)
	if node.Height != height {
		return fmt.Errorf("%w: node %d height %d, expected %d", errTreeValidation, index, node.Height, height)
	}

	if !AABBContains(node.AABB, tree.nodes[child1].AABB) {
		return fmt.Errorf("%w: node %d does not contain child1 %d", errTreeValidation, index, child1)
	}
	if !AABBContains(node.AABB, tree.nodes[child2].AABB) {
		return fmt.Errorf("%w: node %d does not contain child2 %d", errTreeValidation, index, child2)
	}

	categoryBits := tree.nodes[child1].CategoryBits | tree.nodes[child2].CategoryBits
	if node.CategoryBits != categoryBits {
		return fmt.Errorf("%w: node %d category bits %d, expected %d", errTreeValidation, index,
			node.CategoryBits, categoryBits)
	}

	if err := tree.validateMetrics(child1); err != nil {
		return err
	}

	return tree.validateMetrics(child2)
}

// Validate checks the tree invariants (upstream b2DynamicTree_Validate).
// Upstream asserts; this port returns an error so tests can validate without
// enabling panics.
func (tree *DynamicTree) Validate() error {
	if tree.root == NullIndex {
		return nil
	}

	if err := tree.validateStructure(tree.root); err != nil {
		return err
	}

	if err := tree.validateMetrics(tree.root); err != nil {
		return err
	}

	freeCount := 0
	freeIndex := tree.freeList
	for freeIndex != NullIndex {
		if freeIndex < 0 || freeIndex >= tree.nodeCapacity {
			return fmt.Errorf("%w: free list index %d out of range", errTreeValidation, freeIndex)
		}
		freeIndex = tree.nodes[freeIndex].Parent
		freeCount++
	}

	height := tree.GetHeight()
	computedHeight := tree.computeHeight(tree.root)
	if height != computedHeight {
		return fmt.Errorf("%w: tree height %d, computed %d", errTreeValidation, height, computedHeight)
	}

	if tree.nodeCount+freeCount != tree.nodeCapacity {
		return fmt.Errorf("%w: node count %d plus free count %d does not equal capacity %d", errTreeValidation,
			tree.nodeCount, freeCount, tree.nodeCapacity)
	}

	return nil
}

// ValidateNoEnlarged checks that no allocated node is flagged enlarged
// (upstream b2DynamicTree_ValidateNoEnlarged).
func (tree *DynamicTree) ValidateNoEnlarged() error {
	capacity := tree.nodeCapacity
	for i := range capacity {
		node := &tree.nodes[i]
		if node.Flags&allocatedNode != 0 && node.Flags&enlargedNode != 0 {
			return fmt.Errorf("%w: node %d is flagged enlarged", errTreeValidation, i)
		}
	}

	return nil
}

// Query finds all proxies overlapping the supplied AABB, calling the callback
// for each (upstream b2DynamicTree_Query).
func (tree *DynamicTree) Query(aabb AABB, maskBits uint64, callback TreeQueryCallbackFcn, context any) TreeStats {
	var result TreeStats

	if tree.nodeCount == 0 {
		return result
	}

	var stack [treeStackSize]int
	stackCount := 0
	stack[stackCount] = tree.root
	stackCount++

	for stackCount > 0 {
		stackCount--
		nodeID := stack[stackCount]

		node := &tree.nodes[nodeID]
		result.NodeVisits++

		if AABBOverlaps(node.AABB, aabb) && node.CategoryBits&maskBits != 0 {
			switch {
			case isLeaf(node):
				// callback to user code with proxy id
				proceed := callback(nodeID, node.UserData, context)
				result.LeafVisits++

				if !proceed {
					return result
				}
			case stackCount < treeStackSize-1:
				stack[stackCount] = node.Child1
				stackCount++
				stack[stackCount] = node.Child2
				stackCount++
			default:
				assert(stackCount < treeStackSize-1)
			}
		}
	}

	return result
}

// QueryAll finds all proxies overlapping the supplied AABB without filtering
// (upstream b2DynamicTree_QueryAll).
func (tree *DynamicTree) QueryAll(aabb AABB, callback TreeQueryCallbackFcn, context any) TreeStats {
	var result TreeStats

	if tree.nodeCount == 0 {
		return result
	}

	var stack [treeStackSize]int
	stackCount := 0
	stack[stackCount] = tree.root
	stackCount++

	for stackCount > 0 {
		stackCount--
		nodeID := stack[stackCount]

		node := &tree.nodes[nodeID]
		result.NodeVisits++

		if AABBOverlaps(node.AABB, aabb) {
			switch {
			case isLeaf(node):
				// callback to user code with proxy id
				proceed := callback(nodeID, node.UserData, context)
				result.LeafVisits++

				if !proceed {
					return result
				}
			case stackCount < treeStackSize-1:
				stack[stackCount] = node.Child1
				stackCount++
				stack[stackCount] = node.Child2
				stackCount++
			default:
				assert(stackCount < treeStackSize-1)
			}
		}
	}

	return result
}

// RayCast casts a ray against the proxies in the tree (upstream
// b2DynamicTree_RayCast). This relies on the callback to perform an exact ray
// cast in the case where the proxy contains a shape. The callback also
// performs any collision filtering.
func (tree *DynamicTree) RayCast(input *RayCastInput, maskBits uint64, callback TreeRayCastCallbackFcn,
	context any,
) TreeStats {
	var result TreeStats

	if tree.nodeCount == 0 {
		return result
	}

	p1 := input.Origin
	d := input.Translation

	r := Normalize(d)

	// v is perpendicular to the segment.
	v := CrossSV(1.0, r)
	absV := Abs(v)

	// Separating axis for segment (Gino, p80).
	// |dot(v, p1 - c)| > dot(|v|, h)

	maxFraction := input.MaxFraction

	p2 := MulAdd(p1, maxFraction, d)

	// Build a bounding box for the segment.
	segmentAABB := AABB{LowerBound: Min(p1, p2), UpperBound: Max(p1, p2)}

	var stack [treeStackSize]int
	stackCount := 0
	stack[stackCount] = tree.root
	stackCount++

	nodes := tree.nodes

	// See DynamicTree.rayCastScratch: reused storage instead of a stack local
	// so the pointer handed to the callback does not allocate. Saved and
	// restored so a re-entrant callback cannot corrupt an outer traversal.
	savedScratch := tree.rayCastScratch
	tree.rayCastScratch = *input
	subInput := &tree.rayCastScratch

	for stackCount > 0 {
		stackCount--
		nodeID := stack[stackCount]
		if nodeID == NullIndex {
			assert(false)
			continue
		}

		node := &nodes[nodeID]
		result.NodeVisits++

		nodeAABB := node.AABB

		if node.CategoryBits&maskBits == 0 || !AABBOverlaps(nodeAABB, segmentAABB) {
			continue
		}

		// Separating axis for segment (Gino, p80).
		// |dot(v, p1 - c)| > dot(|v|, h)
		// radius extension is added to the node in this case
		c := AABBCenter(nodeAABB)
		h := AABBExtents(nodeAABB)
		term1 := absFloat(Dot(v, Sub(p1, c)))
		term2 := Dot(absV, h)
		if term2 < term1 {
			continue
		}

		switch {
		case isLeaf(node):
			subInput.MaxFraction = maxFraction

			value := callback(subInput, nodeID, node.UserData, context)
			result.LeafVisits++

			// The user may return -1 to indicate this shape should be skipped

			if value == 0.0 {
				// The client has terminated the ray cast.
				tree.rayCastScratch = savedScratch
				return result
			}

			if 0.0 < value && value <= maxFraction {
				// Update segment bounding box.
				maxFraction = value
				p2 = MulAdd(p1, maxFraction, d)
				segmentAABB.LowerBound = Min(p1, p2)
				segmentAABB.UpperBound = Max(p1, p2)
			}
		case stackCount < treeStackSize-1:
			c1 := AABBCenter(nodes[node.Child1].AABB)
			c2 := AABBCenter(nodes[node.Child2].AABB)
			if DistanceSquared(c1, p1) < DistanceSquared(c2, p1) {
				stack[stackCount] = node.Child2
				stackCount++
				stack[stackCount] = node.Child1
				stackCount++
			} else {
				stack[stackCount] = node.Child1
				stackCount++
				stack[stackCount] = node.Child2
				stackCount++
			}
		default:
			assert(stackCount < treeStackSize-1)
		}
	}

	tree.rayCastScratch = savedScratch

	return result
}

// ShapeCast casts a shape against the proxies in the tree (upstream
// b2DynamicTree_ShapeCast).
func (tree *DynamicTree) ShapeCast(input *ShapeCastInput, maskBits uint64, callback TreeShapeCastCallbackFcn,
	context any,
) TreeStats {
	var stats TreeStats

	if tree.nodeCount == 0 || input.Proxy.Count == 0 {
		return stats
	}

	originAABB := AABB{LowerBound: input.Proxy.Points[0], UpperBound: input.Proxy.Points[0]}
	for i := 1; i < input.Proxy.Count; i++ {
		originAABB.LowerBound = Min(originAABB.LowerBound, input.Proxy.Points[i])
		originAABB.UpperBound = Max(originAABB.UpperBound, input.Proxy.Points[i])
	}

	radius := Vec2{X: input.Proxy.Radius, Y: input.Proxy.Radius}

	originAABB.LowerBound = Sub(originAABB.LowerBound, radius)
	originAABB.UpperBound = Add(originAABB.UpperBound, radius)

	p1 := AABBCenter(originAABB)
	extension := AABBExtents(originAABB)

	// v is perpendicular to the segment.
	r := input.Translation
	v := CrossSV(1.0, r)
	absV := Abs(v)

	// Separating axis for segment (Gino, p80).
	// |dot(v, p1 - c)| > dot(|v|, h)

	maxFraction := input.MaxFraction

	// Build total box for the shape cast
	t := MulSV(maxFraction, input.Translation)
	totalAABB := AABB{
		LowerBound: Min(originAABB.LowerBound, Add(originAABB.LowerBound, t)),
		UpperBound: Max(originAABB.UpperBound, Add(originAABB.UpperBound, t)),
	}

	// See DynamicTree.shapeCastScratch: reused storage instead of a stack
	// local so the pointer handed to the callback does not allocate. Saved and
	// restored so a re-entrant callback cannot corrupt an outer traversal.
	savedScratch := tree.shapeCastScratch
	tree.shapeCastScratch = *input
	subInput := &tree.shapeCastScratch

	nodes := tree.nodes

	var stack [treeStackSize]int
	stackCount := 0
	stack[stackCount] = tree.root
	stackCount++

	for stackCount > 0 {
		stackCount--
		nodeID := stack[stackCount]
		if nodeID == NullIndex {
			assert(false)
			continue
		}

		node := &nodes[nodeID]
		stats.NodeVisits++

		if node.CategoryBits&maskBits == 0 || !AABBOverlaps(node.AABB, totalAABB) {
			continue
		}

		// Separating axis for segment (Gino, p80).
		// |dot(v, p1 - c)| > dot(|v|, h)
		// radius extension is added to the node in this case
		c := AABBCenter(node.AABB)
		h := Add(AABBExtents(node.AABB), extension)
		term1 := absFloat(Dot(v, Sub(p1, c)))
		term2 := Dot(absV, h)
		if term2 < term1 {
			continue
		}

		switch {
		case isLeaf(node):
			subInput.MaxFraction = maxFraction

			value := callback(subInput, nodeID, node.UserData, context)
			stats.LeafVisits++

			if value == 0.0 {
				// The client has terminated the ray cast.
				tree.shapeCastScratch = savedScratch
				return stats
			}

			if 0.0 < value && value < maxFraction {
				// Update segment bounding box.
				maxFraction = value
				t = MulSV(maxFraction, input.Translation)
				totalAABB.LowerBound = Min(originAABB.LowerBound, Add(originAABB.LowerBound, t))
				totalAABB.UpperBound = Max(originAABB.UpperBound, Add(originAABB.UpperBound, t))
			}
		case stackCount < treeStackSize-1:
			c1 := AABBCenter(nodes[node.Child1].AABB)
			c2 := AABBCenter(nodes[node.Child2].AABB)
			if DistanceSquared(c1, p1) < DistanceSquared(c2, p1) {
				stack[stackCount] = node.Child2
				stackCount++
				stack[stackCount] = node.Child1
				stackCount++
			} else {
				stack[stackCount] = node.Child1
				stackCount++
				stack[stackCount] = node.Child2
				stackCount++
			}
		default:
			assert(stackCount < treeStackSize-1)
		}
	}

	tree.shapeCastScratch = savedScratch

	return stats
}

// partitionMid partitions leaves using the median split heuristic and returns
// the left child count (upstream b2PartitionMid with B2_TREE_HEURISTIC == 0).
func partitionMid(indices []int, centers []Vec2, count int) int {
	// Handle trivial case
	if count <= 2 {
		return count / 2
	}

	//nolint:gosec // G602: count > 2 here, and every caller passes count <= len(centers) (buildTree passes leafCount over tree.leafCenters, or a sub-slice of exactly the same span), so index 0 exists.
	lowerBound := centers[0]
	//nolint:gosec // G602: same bound as the line above.
	upperBound := centers[0]

	for i := 1; i < count; i++ {
		//nolint:gosec // G602: count <= len(centers) for every caller; buildTree passes leafCount over tree.leafCenters, or centers[startIndex:] with count = endIndex-startIndex.
		lowerBound = Min(lowerBound, centers[i])
		//nolint:gosec // G602: same bound as the line above.
		upperBound = Max(upperBound, centers[i])
	}

	d := Sub(upperBound, lowerBound)
	c := Vec2{X: 0.5 * (lowerBound.X + upperBound.X), Y: 0.5 * (lowerBound.Y + upperBound.Y)}

	// Partition longest axis using the Hoare partition scheme
	// https://en.wikipedia.org/wiki/Quicksort
	// https://nicholasvadivelu.com/2021/01/11/array-partition/
	i1, i2 := 0, count
	if d.X > d.Y {
		pivot := c.X

		for i1 < i2 {
			for i1 < i2 && centers[i1].X < pivot {
				i1++
			}

			for i1 < i2 && centers[i2-1].X >= pivot {
				i2--
			}

			if i1 < i2 {
				// Swap indices
				indices[i1], indices[i2-1] = indices[i2-1], indices[i1]

				// Swap centers
				centers[i1], centers[i2-1] = centers[i2-1], centers[i1]

				i1++
				i2--
			}
		}
	} else {
		pivot := c.Y

		for i1 < i2 {
			for i1 < i2 && centers[i1].Y < pivot {
				i1++
			}

			for i1 < i2 && centers[i2-1].Y >= pivot {
				i2--
			}

			if i1 < i2 {
				// Swap indices
				indices[i1], indices[i2-1] = indices[i2-1], indices[i1]

				// Swap centers
				centers[i1], centers[i2-1] = centers[i2-1], centers[i1]

				i1++
				i2--
			}
		}
	}
	assert(i1 == i2)

	if i1 > 0 && i1 < count {
		return i1
	}

	return count / 2
}

// rebuildItem is temporary data used to track the rebuild of a tree node
// (upstream struct b2RebuildItem).
type rebuildItem struct {
	nodeIndex  int
	childCount int

	// Leaf indices
	startIndex int
	splitIndex int
	endIndex   int
}

// buildTree builds a tree from the gathered leaves and returns the root node
// index (upstream b2BuildTree).
func (tree *DynamicTree) buildTree(leafCount int) int {
	leafIndices := tree.leafIndices

	if leafCount == 1 {
		tree.nodes[leafIndices[0]].Parent = NullIndex
		return leafIndices[0]
	}

	leafCenters := tree.leafCenters

	var stack [treeStackSize]rebuildItem
	top := 0

	stack[0].nodeIndex = tree.allocateNode()
	// The node pool may have grown.
	nodes := tree.nodes
	stack[0].childCount = -1
	stack[0].startIndex = 0
	stack[0].endIndex = leafCount
	stack[0].splitIndex = partitionMid(leafIndices, leafCenters, leafCount)

	for {
		//nolint:gosec // G602: top is the rebuild recursion depth into a fixed [treeStackSize]rebuildItem: it starts at 0, is pushed only under assert(top < treeStackSize) and popped on every completed node, and each push strictly shrinks the leaf range because partitionMid returns a split inside (0, count). This is upstream's B2_TREE_STACK_SIZE contract, not a caller-supplied index.
		item := &stack[top]

		item.childCount++

		if item.childCount == 2 {
			// This internal node has both children established

			if top == 0 {
				// all done
				break
			}

			parentItem := &stack[top-1]
			parentNode := &nodes[parentItem.nodeIndex]

			if parentItem.childCount == 0 {
				assert(parentNode.Child1 == NullIndex)
				parentNode.Child1 = item.nodeIndex
			} else {
				assert(parentItem.childCount == 1)
				assert(parentNode.Child2 == NullIndex)
				parentNode.Child2 = item.nodeIndex
			}

			node := &nodes[item.nodeIndex]

			assert(node.Parent == NullIndex)
			node.Parent = parentItem.nodeIndex

			assert(node.Child1 != NullIndex)
			assert(node.Child2 != NullIndex)
			child1 := &nodes[node.Child1]
			child2 := &nodes[node.Child2]

			node.AABB = AABBUnion(child1.AABB, child2.AABB)
			node.Height = 1 + maxUInt16(child1.Height, child2.Height)
			node.CategoryBits = child1.CategoryBits | child2.CategoryBits

			// Pop stack
			top--
		} else {
			var startIndex, endIndex int
			if item.childCount == 0 {
				startIndex = item.startIndex
				endIndex = item.splitIndex
			} else {
				assert(item.childCount == 1)
				startIndex = item.splitIndex
				endIndex = item.endIndex
			}

			count := endIndex - startIndex

			if count == 1 {
				childIndex := leafIndices[startIndex]
				node := &nodes[item.nodeIndex]

				if item.childCount == 0 {
					assert(node.Child1 == NullIndex)
					node.Child1 = childIndex
				} else {
					assert(item.childCount == 1)
					assert(node.Child2 == NullIndex)
					node.Child2 = childIndex
				}

				childNode := &nodes[childIndex]
				assert(childNode.Parent == NullIndex)
				childNode.Parent = item.nodeIndex
			} else {
				assert(count > 0)
				assert(top < treeStackSize)

				top++
				newItem := &stack[top]
				newItem.nodeIndex = tree.allocateNode()
				// The node pool may have grown.
				nodes = tree.nodes
				newItem.childCount = -1
				newItem.startIndex = startIndex
				newItem.endIndex = endIndex
				newItem.splitIndex = partitionMid(leafIndices[startIndex:], leafCenters[startIndex:], count)
				newItem.splitIndex += startIndex
			}
		}
	}

	rootNode := &nodes[stack[0].nodeIndex]
	assert(rootNode.Parent == NullIndex)
	assert(rootNode.Child1 != NullIndex)
	assert(rootNode.Child2 != NullIndex)

	child1 := &nodes[rootNode.Child1]
	child2 := &nodes[rootNode.Child2]

	rootNode.AABB = AABBUnion(child1.AABB, child2.AABB)
	rootNode.Height = 1 + maxUInt16(child1.Height, child2.Height)
	rootNode.CategoryBits = child1.CategoryBits | child2.CategoryBits

	return stack[0].nodeIndex
}

// Rebuild rebuilds the tree while retaining subtrees that haven't changed and
// returns the number of boxes sorted (upstream b2DynamicTree_Rebuild).
//
// It is not safe to access the tree during this operation because it may grow.
func (tree *DynamicTree) Rebuild(fullBuild bool) int {
	proxyCount := tree.proxyCount
	if proxyCount == 0 {
		return 0
	}

	// Ensure capacity for rebuild space
	if proxyCount > tree.rebuildCapacity {
		newCapacity := proxyCount + proxyCount/2

		tree.leafIndices = make([]int, newCapacity)
		tree.leafCenters = make([]Vec2, newCapacity)
		tree.rebuildCapacity = newCapacity
	}

	leafCount := 0
	var stack [treeStackSize]int
	stackCount := 0

	nodeIndex := tree.root
	nodes := tree.nodes
	node := &nodes[nodeIndex]

	// These are the nodes that get sorted to rebuild the tree.
	// Indices are used because the node pool may grow during the build.
	leafIndices := tree.leafIndices
	leafCenters := tree.leafCenters

	// Gather all proxy nodes that have grown and all internal nodes that haven't grown. Both are
	// considered leaves in the tree rebuild.
	// Free all internal nodes that have grown.
	for {
		if node.Height == 0 || (node.Flags&enlargedNode == 0 && !fullBuild) {
			leafIndices[leafCount] = nodeIndex
			leafCenters[leafCount] = AABBCenter(node.AABB)
			leafCount++

			// Detach
			node.Parent = NullIndex
		} else {
			doomedNodeIndex := nodeIndex

			// Handle children
			nodeIndex = node.Child1

			if stackCount < treeStackSize {
				stack[stackCount] = node.Child2
				stackCount++
			} else {
				assert(stackCount < treeStackSize)
			}

			node = &nodes[nodeIndex]

			// Remove doomed node
			tree.freeNode(doomedNodeIndex)

			continue
		}

		if stackCount == 0 {
			break
		}

		stackCount--
		nodeIndex = stack[stackCount]
		node = &nodes[nodeIndex]
	}

	assert(leafCount <= proxyCount)

	tree.root = tree.buildTree(leafCount)

	if debugAsserts {
		if err := tree.Validate(); err != nil {
			panic(err.Error())
		}
	}

	return leafCount
}
