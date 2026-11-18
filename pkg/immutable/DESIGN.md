# immutable: design

How `Slice` and `Map` are stored and why. This is for maintainers and reviewers; for using them, see
[README.md](README.md).

## The requirement: free copies and cheap edits

Cardinal hands components across the world boundary on every `Get` and `Set`, and game code copies
values with plain assignment (`d := c.Items`). Both must be free, and an edit to one copy must never
show in another.

- **Free copies mean shared storage.** A copy that costs nothing can only copy a pointer, so every
  copy points at the same memory.
- **Shared storage must never be written.** If an edit wrote into it, every other copy would see the
  change. That is the bug in Go's own slices (`d := a; d[0] = 99` changes `a`), and the bug the
  second version of this package had.
- **So an edit builds new storage.** It copies the piece it changes and points the edited value at
  the copy. Everything else stays shared, and other copies keep the old pieces.

With one flat array, the piece is the whole array, so every edit copies everything: O(n). Cutting the
array into chunks under a list of pointers helps, but the list is shared too and must be copied as
well. Cutting that list the same way, again and again until the top fits in one node, gives a tree.
An edit then copies one node per level, the path from the root to the change, and shares everything
else. This is **path copying** (Driscoll, Sarnak, Sleator and Tarjan, 1989; CLRS problem 13-1).

A Map holding keys 1 3 5 7 9, copied and then edited:

```
d := a                     a ──► chunk @100 [1 3 5 7 9]
                           d ──┘                          (same chunk, nothing copied)

d.Set(2, "two")            a ──► chunk @100 [1 3 5 7 9]     untouched
                           d ──► chunk @200 [1 2 3 5 7 9]   new
```

A large value works the same way, one level at a time:

```
a ──► root @100 [@200 @300 @400 @500]
d ──► root @700 [@200 @600 @400 @500]     only the root and one leaf are new
                        │
                  leaf @600 (the edited copy of leaf @300, which a still uses)
```

Old nodes are freed by the garbage collector once no value points at them.

## Slice: a 32-way trie over the index

```
              root (branch: up to 32 children)
          /          |           \
      leaf 0      leaf 1   ...   leaf k        tail
   [e0 … e31]  [e32 … e63]       [ … ]      [last 1–32]
```

A Slice is a trie whose key is the index: each level takes 5 bits of `i` as the child to follow.
Leaves hold exactly 32 elements, branches up to 32 children, and the last 1 to 32 elements sit in a
separate tail so appends rarely touch the tree. This is Bagwell's array mapped trie (2000) with path
copying, the structure behind Clojure's and Scala's vectors. Code: [vector.go](vector.go).

- **Read** `At(i)`: in the tail, a direct index. Otherwise one shift and mask per level, with no
  comparisons. 1 to 4 levels below a million elements.
- **Set**: copy the leaf with the new value, then each branch above it.
- **Append**: copy the tail with the new element. A full tail becomes a leaf, pushed into the tree by
  copying the path, and a new tail starts.
- **Delete from the end**: shorten the tail, or move the last leaf into the tail.
- **Insert or delete elsewhere, sorts**: rebuild. Removing element `i` shifts every later element down
  one position, so every leaf after `i` holds different elements. This is the trie's one weakness:
  the position is the key.

**Shape.** It depends only on the length: the tail holds the last ((n−1) mod 32)+1 elements and the
tree is the shortest one holding the rest, filled from the left. Equal Slices have identical trees,
so `reflect.DeepEqual`, and testify's `Equal`, compare them by contents.

**Why a trie.** Indices are dense, 0 to n−1, so every path is full and the tree needs no balancing.
The other persistent sequences (balanced binary trees as in .NET's `ImmutableList`, ropes, finger
trees, RRB trees) all have shapes that depend on the order of edits, which breaks `DeepEqual`, and
binary ones are about 20 levels deep at a million elements.

**Why 32.** Depth is ⌈log₃₂ n⌉ levels, and each level is a dependent memory load that can miss the
cache. Each edit copies one 32-slot node per level. Halving to 16 adds a level for most sizes (5 at a
million against 4). Doubling to 64 removes at most one level, and only for some sizes (between about
1 and 16 million, for example), while doubling every copy. A power of two also makes each level's
index a shift and a mask. Clojure, Scala, Immutable.js and Kotlin use 32 too.

**Reads cost O(log₃₂ n), not O(1).** This is the one operation slower in theory than a flat array,
and the price of O(log₃₂ n) edits. A Slice of up to 32 elements is a single flat array.

## Map: a B+ tree cut by key hashes

```
              index root  [1, 400, 900]           ← each node holds its children's first keys
            /             |              \
   index [1, 120, 260]  index [400, …]  index [900, …]
     /     |     \
 chunk   chunk   chunk  …                         ← sorted entries, 16–128 each
[1…119] [120…259] [260…399]
```

Entries sit in sorted chunks, the leaves of a B+ tree. Index nodes above them hold each child's first
key next to the child pointer. A Map of a single chunk keeps it in the `one` field, with no index.
Code: [map.go](map.go) for the chunks, [mapindex.go](mapindex.go) for the index.

- **Get**: binary search the keys in each index node on the way down, then within the chunk. O(log n).
- **Set, Delete**: copy the chunk with the change, re-cut it if a boundary moved, then copy the index
  nodes on the path to it. O(log n) expected.

**Why sorted.** Iteration and encoding must run in key order so every machine produces the same order
and bytes. A sorted layout gives that directly, and lets `MapEqual` walk two Maps in one pass.

**Why a B+ tree.** Wide nodes for the same memory reasons as the Slice. Entries only in the leaves
keep index nodes small and make iteration a scan of the chunks in order.

**Why not a trie like the Slice.** Map keys are sparse (player IDs, strings), so trie paths would be
long and mostly empty without path compression.

**Why not a hash trie (HAMT).** It iterates in hash order, not key order.

**Why boundaries come from hashes.** A normal B+ tree splits a node when it fills, so the same keys
inserted in a different order give a different tree, and `DeepEqual` breaks. Here a key ends a chunk
when its hash says so, and a key's hash never changes, so the same keys always give the same tree.

- A chunk ends after a key whose hash has its low 4 bits zero, once the chunk holds at least 16
  entries, or at 128 entries. Chunks average 32.
- An index node ends the same way, using other hash bits of its last child's first key, with 8 to 32
  children, about 16 on average.

This is a uniquely represented B-tree: Golovin's B-skip-list (2010) and B-treap (2009), and the
"prolly tree" of Noms and Dolt. The idea is the skip list's (Pugh, 1990), with the coin flip that
promotes a key replaced by its hash.

**Shape.** Within one process it depends only on the keys. Order and encoding depend only on the keys
everywhere.

### Why the hash is seeded

If the hash were fixed, anyone could read it in this public repository and pick keys, such as player
names, that never end a chunk. Every chunk would then fill to the 128 cap, and an edit near the front
would re-cut the whole Map: O(n) per edit, enough for one player to slow a shard down for everyone.
The same class of attack hit web servers' hash tables in 2011, and it is why Go seeds its own maps.

So the hash (`hash/maphash`) is seeded at random when the program starts. Callers see no difference:
there is no constructor, the zero value works, and order, encoding and snapshots follow the keys.
Every Map in a process shares the seed, so equal Maps still have the same layout.

**Rule for this package: anything that takes a seed draws it at random when the program starts.**
Never a constant: this repository is public, so a fixed seed is a published one.

## Worst cases and how often they happen

| Operation | Worst case | When it happens |
| --- | --- | --- |
| Slice read, set, append, delete from the end | O(log₃₂ n) | every call; the depth is fixed by the length |
| Slice insert or delete elsewhere | O(n) | every call |
| Map get | about 1.7 × log₂ n comparisons | not seen in practice; depth stays near average |
| Map set, delete | O(n) | only with keys built against the seed |

A chunk reaches the 128 cap only when the roughly 110 keys after its first 16 all miss the boundary
pattern. Over a million keys
(sequential, strided and random integers, and `"player_%d"` strings), 0.06–0.07% of chunks reached
the cap, never two in a row.

## Allocations

Reads and copies allocate nothing. An edit allocates about once. Avoiding that would mean writing in
place when no other value holds the storage, and Go cannot tell: it has no reference counts (Swift
checks `isKnownUniquelyReferenced`) and no ownership rules (Rust). Running one system at a time
prevents data races, not shared storage, since a value can still be copied into a local, another
entity or an event.

Ways around it, none built: a fixed-size array field (`[16]Item` plus a count) for small bounded
lists; inline storage for the first N elements; an edit session that writes nodes it created in place
(Clojure's transients, .NET's freezable builders, HashiCorp go-immutable-radix's transactions), which
brings back a second API with an end call.

## Rejected designs

- **Copy on every edit, plus builders** (the first version). Safe and fastest to read, but every edit
  copied the whole array, and loops needed a second API with its own misuse rules.
- **Edit in place, copy at Cardinal's boundaries** (the second version). Every `Get` and `Set` copied
  the whole collection, and `d := a` shared storage, so an edit to `d` could change `a`.
- **A persistent vector of Map chunks.** O(log n) lookups, but a chunk splitting or merging shifts
  every later chunk. The prolly index made lookups 1.6x and random inserts 5.5x faster at a million
  entries.
- **HAMT, red-black or AVL trees, B-trees split on full.** Hash order, or shapes that depend on the
  order of edits.

## References

- J. Driscoll, N. Sarnak, D. Sleator, R. Tarjan. Making data structures persistent. JCSS 38, 1989.
- T. Cormen et al. *Introduction to Algorithms*, problem 13-1 (persistent dynamic sets), chapter 18
  (B-trees).
- P. Bagwell. Fast and space efficient trie searches. EPFL, 2000.
- W. Pugh. Skip lists: a probabilistic alternative to balanced trees. CACM, 1990.
- D. Golovin. B-treaps: a uniquely represented alternative to B-trees. ICALP 2009.
- D. Golovin. The B-skip-list: a simpler uniquely represented alternative to B-trees.
  arXiv:1005.0662, 2010.
- Noms and Dolt: prolly trees.
