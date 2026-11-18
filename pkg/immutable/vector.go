package immutable

import (
	"iter"
	"slices"
)

// A Slice is stored as a persistent vector, the structure behind Clojure's and Scala's vectors: the
// elements in chunks of 32, the full chunks in a tree with 32 children per node, and the last 1 to 32
// elements in a separate tail. No node or tail is ever written once a Slice holds it. An edit copies
// the tail, or the nodes on the path to the element, and the edited Slice points at the copies, so
// every other Slice sharing the old nodes is unchanged. That is what makes assigning a Slice, and
// Cardinal handing one across the world boundary, free and safe.
//
// 32 trades read depth against edit size: each level is a dependent memory load, and each edit copies
// one 32-wide node per level. 16 adds a level for most sizes; 64 removes at most one, for some sizes
// only, and doubles the copy. A power of two also makes each level's index a shift and a mask.
// DESIGN.md explains the structure and the choices behind it.
//
// The shape depends only on the length: the tail holds the last ((n-1) mod 32)+1 elements, and the
// tree is the shortest one holding the rest, filled from the left. Equal Slices therefore have equal
// trees, and reflect.DeepEqual, which testify's Equal uses, compares them by their elements.

const (
	chunkBits = 5
	chunkSize = 1 << chunkBits
	chunkMask = chunkSize - 1
)

// vnode is a tree node: a leaf holds exactly chunkSize items, a branch 1 to chunkSize kids.
type vnode[T any] struct {
	kids  []*vnode[T]
	items []T
}

// leafNode wraps items, which must hold exactly chunkSize elements and never be written again.
func leafNode[T any](items []T) *vnode[T] { return &vnode[T]{items: items} }

// branchNode returns a branch holding kids. The node and its kid array are one allocation.
func branchNode[T any](kids ...*vnode[T]) *vnode[T] {
	b := new(struct {
		node vnode[T]
		arr  [chunkSize]*vnode[T]
	})
	b.node.kids = append(b.arr[:0], kids...)
	return &b.node
}

// treeLen is the number of elements in the tree, before the tail.
func (s Slice[T]) treeLen() int { return s.n - len(s.tail) }

// leafAt returns the leaf chunk holding index i, which must be in the tree.
func (s Slice[T]) leafAt(i int) []T {
	node := s.root
	for shift := s.shift; shift > 0; shift -= chunkBits {
		node = node.kids[(i>>shift)&chunkMask]
	}
	return node.items
}

// setInTree returns a copy of node with index i set to v, copying only the path to it.
func setInTree[T any](node *vnode[T], shift uint, i int, v T) *vnode[T] {
	if shift == 0 {
		l := new(struct {
			node vnode[T]
			arr  [chunkSize]T
		})
		l.arr = [chunkSize]T(node.items)
		l.arr[i&chunkMask] = v
		l.node.items = l.arr[:]
		return &l.node
	}
	b := branchNode(node.kids...)
	k := (i >> shift) & chunkMask
	b.kids[k] = setInTree(node.kids[k], shift-chunkBits, i, v)
	return b
}

// pushLeaf adds a full leaf to the end of the tree.
func (s *Slice[T]) pushLeaf(leaf *vnode[T]) {
	at := s.treeLen()
	switch {
	case s.root == nil:
		s.root = leaf
	case at == 1<<(s.shift+chunkBits): // the tree is full: grow a level
		s.root = branchNode(s.root, pathTo(s.shift, leaf))
		s.shift += chunkBits
	default:
		s.root = pushInto(s.root, s.shift, at, leaf)
	}
}

// popLeaf removes the last leaf from the tree and returns its items, leaving the shortest tree that
// holds the rest so the layout stays canonical. The tail is left for the caller to replace.
func (s *Slice[T]) popLeaf() []T {
	last := s.treeLen() - 1
	items := s.leafAt(last)
	s.root = withoutLastLeaf(s.root, s.shift, last)
	for s.root != nil && s.shift > 0 && len(s.root.kids) == 1 {
		s.root = s.root.kids[0]
		s.shift -= chunkBits
	}
	if s.root == nil {
		s.shift = 0
	}
	return items
}

// withoutLastLeaf returns a copy of node without the leaf holding index last, the tree's last
// element, or nil when nothing is left under node.
func withoutLastLeaf[T any](node *vnode[T], shift uint, last int) *vnode[T] {
	if shift == 0 {
		return nil
	}
	k := (last >> shift) & chunkMask
	kid := withoutLastLeaf(node.kids[k], shift-chunkBits, last)
	if kid == nil && k == 0 {
		return nil
	}
	b := branchNode(node.kids[:k]...)
	if kid != nil {
		b.kids = append(b.kids, kid)
	}
	return b
}

// pathTo wraps leaf in single-kid branches up to the given level.
func pathTo[T any](shift uint, leaf *vnode[T]) *vnode[T] {
	if shift == 0 {
		return leaf
	}
	return branchNode(pathTo(shift-chunkBits, leaf))
}

// pushInto returns a copy of the branch node with leaf added at index at, the first index past the
// tree's elements.
func pushInto[T any](node *vnode[T], shift uint, at int, leaf *vnode[T]) *vnode[T] {
	k := (at >> shift) & chunkMask
	b := branchNode(node.kids...)
	if k == len(node.kids) {
		b.kids = append(b.kids, pathTo(shift-chunkBits, leaf))
		return b
	}
	b.kids[k] = pushInto(node.kids[k], shift-chunkBits, at, leaf)
	return b
}

// fromItems returns the Slice holding items, taking ownership of the array: neither the caller nor
// anything else may write to it again. Leaves and the tail are windows onto it, so building costs
// one node per 32 elements and no copying.
func fromItems[T any](items []T) Slice[T] {
	n := len(items)
	if n == 0 {
		return Slice[T]{}
	}
	tailLen := (n-1)&chunkMask + 1
	s := Slice[T]{tail: items[n-tailLen : n : n], n: n}
	treeLen := n - tailLen
	if treeLen == 0 {
		return s
	}
	level := make([]*vnode[T], 0, treeLen/chunkSize)
	for i := 0; i < treeLen; i += chunkSize {
		level = append(level, leafNode(items[i:i+chunkSize:i+chunkSize]))
	}
	for len(level) > 1 {
		up := make([]*vnode[T], 0, (len(level)+chunkMask)/chunkSize)
		for i := 0; i < len(level); i += chunkSize {
			up = append(up, branchNode(level[i:min(i+chunkSize, len(level))]...))
		}
		level = up
		s.shift += chunkBits
	}
	s.root = level[0]
	return s
}

// chunks yields the elements in order, one chunk at a time. The chunks must not be written.
func (s Slice[T]) chunks() iter.Seq[[]T] {
	return func(yield func([]T) bool) {
		if s.root != nil && !forwardLeaves(s.root, s.shift, yield) {
			return
		}
		if len(s.tail) > 0 {
			yield(s.tail)
		}
	}
}

func forwardLeaves[T any](node *vnode[T], shift uint, yield func([]T) bool) bool {
	if shift == 0 {
		return yield(node.items)
	}
	for _, kid := range node.kids {
		if !forwardLeaves(kid, shift-chunkBits, yield) {
			return false
		}
	}
	return true
}

// backwardChunks yields the chunks from last to first.
func (s Slice[T]) backwardChunks() iter.Seq[[]T] {
	return func(yield func([]T) bool) {
		if len(s.tail) > 0 && !yield(s.tail) {
			return
		}
		if s.root != nil {
			backwardLeaves(s.root, s.shift, yield)
		}
	}
}

func backwardLeaves[T any](node *vnode[T], shift uint, yield func([]T) bool) bool {
	if shift == 0 {
		return yield(node.items)
	}
	for _, kid := range slices.Backward(node.kids) {
		if !backwardLeaves(kid, shift-chunkBits, yield) {
			return false
		}
	}
	return true
}

// toItems returns a new array holding every element, with room for extra more.
func (s Slice[T]) toItems(extra int) []T {
	items := make([]T, 0, s.n+extra)
	for c := range s.chunks() {
		items = append(items, c...)
	}
	return items
}
