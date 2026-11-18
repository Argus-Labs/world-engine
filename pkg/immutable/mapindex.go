package immutable

import "slices"

// A Map of more than one chunk indexes its chunks with a prolly tree: the chunks are grouped into
// nodes, the nodes into nodes above them, and so on up to a single root. Each group's boundaries are
// content-defined in the same way as the chunks', from the first key under each child, so the whole
// tree is a function of the keys alone, which keeps reflect.DeepEqual comparing Maps by contents.
//
// Each node stores its children's first keys next to the children, in the same allocation, so a
// lookup reads one contiguous array of keys per level instead of following a pointer per comparison.
// An edit re-cuts the nodes on the path to the edited chunk, plus any following nodes until a cut
// lands where an old node ended, which is O(log n) nodes. A Slice of chunks would be simpler, but its
// shape follows positions, so a chunk splitting or merging shifts every later chunk and rebuilds it.
//
// Nodes average about 16 children. Wider nodes make the tree shorter but cost more to copy on every
// edit, and measured slower for Maps of a few thousand entries.

const (
	idxMin    = 8
	idxSpread = 8 // a power of two
	idxBits   = 3 // log2(idxSpread)
	idxMax    = 32

	// maxDepth bounds the index's height. Every node but a level's last has at least idxMin
	// children, and every chunk but the last at least mapChunkMin entries, so 16 levels hold about
	// 2^49 entries, more than memory can.
	maxDepth = 16
)

// mnode is an index node. A node on the lowest level holds chunks; every other node holds nodes.
type mnode[K Key, V any] struct {
	keys   []K // the first key under each child
	kids   []*mnode[K, V]
	chunks [][]entry[K, V]
}

// mchild is a child of an index node: a chunk or a node, with its first key.
type mchild[K Key, V any] struct {
	k     K
	chunk []entry[K, V]
	kid   *mnode[K, V]
}

// mframe is a position in the index: a node and the index of one of its children.
type mframe[K Key, V any] struct {
	node *mnode[K, V]
	i    int
}

// mpath holds the frames from the lowest level up: f[0] is the lowest-level node and the chunk in
// it, f[h-1] the root. Level L's items, chunks for L = 0, are the children of the nodes in f[L].
type mpath[K Key, V any] struct {
	f [maxDepth]mframe[K, V]
	h int
}

// endsNode reports whether an index node on level L (1 for the lowest) of size children, the last
// with first key k, ends there. Each level reads its own bits of the hash, clear of the low bits
// endsChunk reads.
func endsNode[K Key](size int, k K, level int) bool {
	shift := uint(4 + idxBits*(level-1))
	return size == idxMax || size >= idxMin && (keyHash(k)>>shift)&(idxSpread-1) == 0
}

// newNode returns a node holding children, which are all chunks or all nodes.
func newNode[K Key, V any](children []mchild[K, V]) *mnode[K, V] {
	node := allocNode[K, V](len(children), children[0].kid == nil)
	for i, c := range children {
		node.keys[i] = c.k
		if node.kids == nil {
			node.chunks[i] = c.chunk
		} else {
			node.kids[i] = c.kid
		}
	}
	return node
}

// with returns a copy of node with child i replaced by c, whose first key must be unchanged.
func (node *mnode[K, V]) with(i int, c mchild[K, V]) *mnode[K, V] {
	b := allocNode[K, V](len(node.keys), node.kids == nil)
	copy(b.keys, node.keys)
	if node.kids == nil {
		copy(b.chunks, node.chunks)
		b.chunks[i] = c.chunk
	} else {
		copy(b.kids, node.kids)
		b.kids[i] = c.kid
	}
	return b
}

// allocNode returns a node of n children, chunks when lowest is set and nodes otherwise. The node,
// its keys and its children are one allocation, from the smallest of three sizes that fits, so a
// small node is not charged for idxMax children on every edit.
func allocNode[K Key, V any](n int, lowest bool) *mnode[K, V] {
	switch {
	case lowest && n <= idxMax/4:
		b := new(struct {
			node   mnode[K, V]
			keys   [idxMax / 4]K
			chunks [idxMax / 4][]entry[K, V]
		})
		b.node.keys, b.node.chunks = b.keys[:n], b.chunks[:n]
		return &b.node
	case lowest && n <= idxMax/2:
		b := new(struct {
			node   mnode[K, V]
			keys   [idxMax / 2]K
			chunks [idxMax / 2][]entry[K, V]
		})
		b.node.keys, b.node.chunks = b.keys[:n], b.chunks[:n]
		return &b.node
	case lowest:
		b := new(struct {
			node   mnode[K, V]
			keys   [idxMax]K
			chunks [idxMax][]entry[K, V]
		})
		b.node.keys, b.node.chunks = b.keys[:n], b.chunks[:n]
		return &b.node
	case n <= idxMax/4:
		b := new(struct {
			node mnode[K, V]
			keys [idxMax / 4]K
			kids [idxMax / 4]*mnode[K, V]
		})
		b.node.keys, b.node.kids = b.keys[:n], b.kids[:n]
		return &b.node
	case n <= idxMax/2:
		b := new(struct {
			node mnode[K, V]
			keys [idxMax / 2]K
			kids [idxMax / 2]*mnode[K, V]
		})
		b.node.keys, b.node.kids = b.keys[:n], b.kids[:n]
		return &b.node
	default:
		b := new(struct {
			node mnode[K, V]
			keys [idxMax]K
			kids [idxMax]*mnode[K, V]
		})
		b.node.keys, b.node.kids = b.keys[:n], b.kids[:n]
		return &b.node
	}
}

// appendChildren appends node's children [from, to) to dst.
func (node *mnode[K, V]) appendChildren(dst []mchild[K, V], from, to int) []mchild[K, V] {
	for i := from; i < to; i++ {
		c := mchild[K, V]{k: node.keys[i]}
		if node.kids == nil {
			c.chunk = node.chunks[i]
		} else {
			c.kid = node.kids[i]
		}
		dst = append(dst, c)
	}
	return dst
}

// locate returns the chunk that holds k, or would, filling p with the path to it.
func (m Map[K, V]) locate(k K, p *mpath[K, V]) []entry[K, V] {
	node, d := m.root, 0
	for {
		i := lastAtMost(node.keys, k)
		p.f[d] = mframe[K, V]{node, i}
		d++
		if node.kids == nil {
			break
		}
		node = node.kids[i]
	}
	p.h = d
	slices.Reverse(p.f[:d])
	return node.chunks[p.f[0].i]
}

// replaceAbove stores c, which replaces the level-L item on the path and has the same first key, by
// copying the path above it.
func (m *Map[K, V]) replaceAbove(p *mpath[K, V], level int, c mchild[K, V]) {
	for l := level; l < p.h; l++ {
		c = mchild[K, V]{kid: p.f[l].node.with(p.f[l].i, c)}
	}
	m.root = c.kid
}

// advance moves cur to the next item on level L, reporting false when there is none.
func advance[K Key, V any](cur *[maxDepth]mframe[K, V], level, h int) bool {
	top := level
	for top < h && cur[top].i == len(cur[top].node.keys)-1 {
		top++
	}
	if top == h {
		return false
	}
	cur[top].i++
	for top > level {
		next := cur[top].node.kids[cur[top].i]
		top--
		cur[top] = mframe[K, V]{next, 0}
	}
	return true
}

// rechunkTree replaces the chunk at p with the chunks edited splits into, then re-cuts the index
// above. edited is a new array that rechunkTree takes ownership of.
func (m *Map[K, V]) rechunkTree(p *mpath[K, V], edited []entry[K, V]) {
	cur := p.f
	var buf [4]mchild[K, V]
	rep := buf[:0]

	// Level 0: cut the entries into chunks, continuing into the chunks after until a cut lands where
	// an old chunk ended. Growing edited may move it, which leaves the windows already cut on the
	// old array; nothing writes to it again.
	start, scan := 0, 0
	for {
		for ; scan < len(edited); scan++ {
			if endsChunk(scan+1-start, edited[scan].k) {
				rep = append(rep, mchild[K, V]{k: edited[start].k, chunk: edited[start : scan+1 : scan+1]})
				start = scan + 1
			}
		}
		if start == len(edited) || !advance(&cur, 0, p.h) {
			break
		}
		edited = append(edited, cur[0].node.chunks[cur[0].i]...)
	}
	if start < len(edited) {
		rep = append(rep, mchild[K, V]{k: edited[start].k, chunk: edited[start:len(edited):len(edited)]})
	}

	if keepsCuts(p, &cur, 0, rep) {
		m.replaceAbove(p, 0, rep[0])
		return
	}
	m.recutIndex(p, &cur, rep)
}

// keepsCuts reports whether rep, which replaces the level-L items from p.f[L] to cur[L], is one item
// replacing one with the same first key. Every cut above is then unchanged, and only the path above
// needs copying.
func keepsCuts[K Key, V any](p *mpath[K, V], cur *[maxDepth]mframe[K, V], level int, rep []mchild[K, V]) bool {
	return len(rep) == 1 && cur[level] == p.f[level] && rep[0].k == p.f[level].node.keys[p.f[level].i]
}

// recutIndex stores rep, the chunks that replace those from p.f[0] to cur[0], by re-cutting each
// level of the index in turn. Its buffers are large, so it is kept off the common path, where one
// chunk replaces one with the same first key.
func (m *Map[K, V]) recutIndex(p *mpath[K, V], cur *[maxDepth]mframe[K, V], rep []mchild[K, V]) {
	var repBuf [8]mchild[K, V]
	var listBuf [2*idxMax + 8]mchild[K, V]
	list := listBuf[:0]
	// Each level: replace the run of items the level below rewrote, from p.f[L] to cur[L], with rep,
	// and re-cut the nodes around them the same way endsChunk cuts entries.
	for level := range p.h {
		if level > 0 && keepsCuts(p, cur, level, rep) {
			m.replaceAbove(p, level, rep[0])
			return
		}
		a, b := p.f[level], cur[level]
		list = a.node.appendChildren(list[:0], 0, a.i)
		list = append(list, rep...)
		list = b.node.appendChildren(list, b.i+1, len(b.node.keys))
		rep = repBuf[:0]
		start, scan := 0, 0
		for {
			rep, start = appendCuts(rep, list, start, scan, level+1)
			if start == len(list) || level+1 == p.h || !advance(cur, level+1, p.h) {
				break
			}
			scan = len(list)
			next := cur[level+1].node.kids[cur[level+1].i]
			list = next.appendChildren(list, 0, len(next.keys))
			cur[level] = mframe[K, V]{next, len(next.keys) - 1}
		}
		if start < len(list) {
			rep = append(rep, mchild[K, V]{k: list[start].k, kid: newNode(list[start:])})
		}
	}
	m.setTop(rep, p.h)
}

// appendCuts cuts list into nodes on level L the way endsNode says, continuing a cut that began at
// start and has been checked up to scan. It appends the nodes to dst and returns where the uncut
// remainder begins.
func appendCuts[K Key, V any](dst, list []mchild[K, V], start, scan, level int) ([]mchild[K, V], int) {
	for ; scan < len(list); scan++ {
		if endsNode(scan+1-start, list[scan].k, level) {
			dst = append(dst, mchild[K, V]{k: list[start].k, kid: newNode(list[start : scan+1])})
			start = scan + 1
		}
	}
	return dst, start
}

// setTop makes level, the items on level L, the top of the Map: it adds levels above while there is
// more than one item, and drops levels below while the root has one child, so the tree is the
// shortest that holds the chunks.
func (m *Map[K, V]) setTop(level []mchild[K, V], l int) {
	for ; len(level) > 1; l++ {
		up, start := appendCuts(make([]mchild[K, V], 0, len(level)/idxMin+1), level, 0, 0, l+1)
		if start < len(level) {
			up = append(up, mchild[K, V]{k: level[start].k, kid: newNode(level[start:])})
		}
		level = up
	}
	top := level[0]
	for top.kid != nil && len(top.kid.keys) == 1 {
		if top.kid.kids == nil {
			top = mchild[K, V]{chunk: top.kid.chunks[0]}
		} else {
			top = mchild[K, V]{kid: top.kid.kids[0]}
		}
	}
	m.one, m.root = top.chunk, top.kid
}

// cutEntries cuts sorted entries into chunks, appending them to dst. The chunks are windows onto
// entries.
func cutEntries[K Key, V any](dst []mchild[K, V], entries []entry[K, V]) []mchild[K, V] {
	start := 0
	for i, e := range entries {
		if endsChunk(i+1-start, e.k) {
			dst = append(dst, mchild[K, V]{k: entries[start].k, chunk: entries[start : i+1 : i+1]})
			start = i + 1
		}
	}
	if start < len(entries) {
		dst = append(dst, mchild[K, V]{k: entries[start].k, chunk: entries[start:len(entries):len(entries)]})
	}
	return dst
}

// walkChunks yields the chunks under node in key order.
func walkChunks[K Key, V any](node *mnode[K, V], yield func([]entry[K, V]) bool) bool {
	if node.kids == nil {
		for _, c := range node.chunks {
			if !yield(c) {
				return false
			}
		}
		return true
	}
	for _, kid := range node.kids {
		if !walkChunks(kid, yield) {
			return false
		}
	}
	return true
}

// nodesEqual compares two index nodes chunk by chunk. Equal key sets give equal trees, so a
// difference in shape means the Maps differ.
func nodesEqual[K Key, V any](a, b *mnode[K, V], eq func(a, b V) bool) bool {
	if len(a.keys) != len(b.keys) || (a.kids == nil) != (b.kids == nil) {
		return false
	}
	if a.kids == nil {
		for i, c := range a.chunks {
			if !chunksEqual(c, b.chunks[i], eq) {
				return false
			}
		}
		return true
	}
	for i, kid := range a.kids {
		if !nodesEqual(kid, b.kids[i], eq) {
			return false
		}
	}
	return true
}

func chunksEqual[K Key, V any](a, b []entry[K, V], eq func(a, b V) bool) bool {
	if len(a) != len(b) {
		return false
	}
	for j := range a {
		if a[j].k != b[j].k || !eq(a[j].v, b[j].v) {
			return false
		}
	}
	return true
}

// lastAtMost returns the index of the last key at most k, or 0 when there is none.
func lastAtMost[K Key](keys []K, k K) int {
	lo, hi := 1, len(keys)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if keys[mid] <= k {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo - 1
}
