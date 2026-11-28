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

// newLeaf returns a leaf of zero values. The node and its array are one allocation, which the leaf
// alone holds.
func newLeaf[T any]() *vnode[T] {
	l := new(struct {
		node vnode[T]
		arr  [chunkSize]T
	})
	l.node.items = l.arr[:]

	return &l.node
}

// copyLeaf returns a new leaf holding a copy of items, which must hold exactly chunkSize elements.
func copyLeaf[T any](items []T) *vnode[T] {
	l := newLeaf[T]()
	copy(l.items, items)

	return l
}

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
		l := copyLeaf(node.items)
		l.items[i&chunkMask] = v

		return l
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
// anything else may write to it again. When the Slice fits in its tail, or T holds no pointers, the
// leaves and the tail are windows onto the array, so building costs one node per 32 elements and no
// copying. Otherwise each gets an array of its own, as copyItems builds them: windows would keep the
// whole array alive while any one of them is, and with it every value an edit has since replaced in
// the others. pointers.go explains why that matters only for pointers.
func fromItems[T any](items []T) Slice[T] {
	if !packs[T](len(items)) {
		return copyItems(items)
	}

	n := len(items)
	if n == 0 {
		return Slice[T]{}
	}

	tailLen := (n-1)&chunkMask + 1
	s := Slice[T]{tail: items[n-tailLen : n : n], n: n}

	treeLen := n - tailLen

	leaves := make([]*vnode[T], 0, treeLen/chunkSize)
	for i := 0; i < treeLen; i += chunkSize {
		leaves = append(leaves, leafNode(items[i:i+chunkSize:i+chunkSize]))
	}

	s.setLeaves(leaves)

	return s
}

// packs reports whether fromItems keeps a Slice of n elements as windows onto one array.
func packs[T any](n int) bool { return n <= chunkSize || !holdsPointers[T]() }

// copyItems returns the Slice holding a copy of items, each leaf and the tail in an array of its
// own, so it holds no reference to items.
func copyItems[T any](items []T) Slice[T] {
	c := newCopier[T](len(items))
	c.pushAll(items)

	return c.slice()
}

// A copier builds a Slice from elements pushed in order, copying them straight into leaves of their
// own. The rebuilding edits use it for the Slices fromItems would not pack, so that they copy each
// element once rather than into one array first and then out of it.
type copier[T any] struct {
	leaves []*vnode[T]
	open   *vnode[T] // the chunk being filled; nil before the first element
	fill   int       // the elements in open
}

// newCopier returns a copier with room for the leaves of a Slice of about n elements.
func newCopier[T any](n int) copier[T] {
	return copier[T]{leaves: make([]*vnode[T], 0, max(n-1, 0)/chunkSize)}
}

func (c *copier[T]) push(v T) {
	if c.open == nil || c.fill == chunkSize {
		c.next()
	}

	c.open.items[c.fill] = v
	c.fill++
}

func (c *copier[T]) pushAll(items []T) {
	for len(items) > 0 {
		if c.open == nil || c.fill == chunkSize {
			c.next()
		}

		k := copy(c.open.items[c.fill:], items)
		c.fill += k
		items = items[k:]
	}
}

// pushRange pushes the elements of s in [i, j).
func (c *copier[T]) pushRange(s Slice[T], i, j int) {
	off := 0
	for chunk := range s.chunks() {
		if off >= j {
			return
		}

		if lo, hi := max(i-off, 0), min(j-off, len(chunk)); lo < hi {
			c.pushAll(chunk[lo:hi])
		}

		off += len(chunk)
	}
}

// next starts a new chunk. A full chunk becomes a leaf only once another element follows it: the
// last chunk, full or not, is the tail.
func (c *copier[T]) next() {
	if c.open != nil {
		c.leaves = append(c.leaves, c.open)
	}

	c.open, c.fill = newLeaf[T](), 0
}

// slice returns the Slice holding every element pushed. The tail is a window onto its own leaf.
func (c *copier[T]) slice() Slice[T] {
	if c.open == nil {
		return Slice[T]{}
	}

	s := Slice[T]{tail: c.open.items[:c.fill:c.fill], n: len(c.leaves)*chunkSize + c.fill}
	s.setLeaves(c.leaves)

	return s
}

// setLeaves builds the tree over leaves, the Slice's full chunks in order, as the shortest tree
// filled from the left.
func (s *Slice[T]) setLeaves(level []*vnode[T]) {
	if len(level) == 0 {
		return
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
}

// detach returns part, a view of an array whose other elements an edit is retiring. When T holds
// pointers it returns a copy instead, so that the retired elements are not kept alive with part.
// It never clears them: other values may share the array.
func detach[T any](part []T) []T {
	if holdsPointers[T]() {
		return slices.Clone(part)
	}

	return part
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
