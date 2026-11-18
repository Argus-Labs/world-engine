package immutable

import (
	"cmp"
	"fmt"
	"maps"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/require"
)

// The layout of a Slice or Map must depend only on its contents, so that reflect.DeepEqual, and with
// it testify's Equal, compares them by contents. These tests edit large values at random and check
// after every edit that the result is DeepEqual to the same contents built in one go, and that earlier
// copies are unchanged.

func TestSlice_LayoutDependsOnlyOnContents(t *testing.T) {
	t.Parallel()
	r := testutils.NewRand(t)

	var s Slice[int]
	var model []int
	type saved struct {
		s     Slice[int]
		items []int
	}
	var copies []saved
	for step := range 4000 {
		switch op := r.IntN(11); {
		case op < 4:
			n := r.IntN(80)
			for range n {
				v := r.Int()
				s.Append(v)
				model = append(model, v)
			}
		case op < 6 && len(model) > 0:
			i, v := r.IntN(len(model)), r.Int()
			s.Set(i, v)
			model[i] = v
		case op == 6:
			i := r.IntN(len(model) + 1)
			s.Insert(i, step, step)
			model = slices.Insert(model, i, step, step)
		case op == 7 && len(model) > 0:
			i := r.IntN(len(model))
			j := i + r.IntN(min(40, len(model)-i)+1)
			s.Delete(i, j)
			model = slices.Delete(model, i, j)
		case op == 8:
			mod := 2 + r.IntN(30)
			s.DeleteFunc(func(v int) bool { return v%mod == 0 })
			model = slices.DeleteFunc(model, func(v int) bool { return v%mod == 0 })
		case op == 9:
			s.UpdateFunc(func(i, v int) int { return v ^ i })
			for i := range model {
				model[i] ^= i
			}
		case r.IntN(50) == 0:
			s.Reset()
			model = nil
		}
		require.Equal(t, len(model), s.Len(), "step %d", step)
		require.True(t, reflect.DeepEqual(fromItems(slices.Clone(model)), s), "step %d: layout differs", step)
		if len(model) > 0 {
			i := r.IntN(len(model))
			require.Equal(t, model[i], s.At(i), "step %d", step)
			j := i + r.IntN(min(100, len(model)-i)+1)
			var got []int
			for x, v := range s.Range(i, j) {
				require.Equal(t, i+len(got), x, "step %d: Range index", step)
				got = append(got, v)
			}
			require.Equal(t, model[i:j], append([]int{}, got...), "step %d: Range(%d, %d)", step, i, j)
		}
		if r.IntN(50) == 0 {
			var back []int
			for _, v := range s.Backward() {
				back = append(back, v)
			}
			slices.Reverse(back)
			require.Equal(t, model, back, "step %d: Backward", step)
		}
		if r.IntN(20) == 0 {
			copies = append(copies, saved{s, slices.Clone(model)})[max(0, len(copies)-4):]
		}
		for _, c := range copies {
			require.Equal(t, c.items, slices.Collect(c.s.Values()), "step %d: a copy changed", step)
			// The copy shares some storage with s and not the rest, so this checks the shortcut
			// against comparing every element.
			require.Equal(t, slices.Equal(c.items, model), Equal(c.s, s), "step %d: Equal with a copy", step)
		}
	}
}

// TestSlice_PopKeepsLayout deletes from the end one element at a time, and in runs that start inside
// the last leaf, through every tail and leaf boundary.
func TestSlice_PopKeepsLayout(t *testing.T) {
	t.Parallel()
	r := testutils.NewRand(t)

	items := make([]int, 3*chunkSize*chunkSize+5)
	for i := range items {
		items[i] = i
	}
	s := fromItems(slices.Clone(items))
	for len(items) > 0 {
		before, want := s, slices.Clone(items)
		cut := len(items) - 1
		if r.IntN(4) == 0 {
			cut = max(0, cut-r.IntN(2*chunkSize))
		}
		s.Delete(cut, len(items))
		items = items[:cut]
		require.True(t, reflect.DeepEqual(fromItems(slices.Clone(items)), s), "length %d: layout differs", cut)
		require.Equal(t, want, slices.Collect(before.Values()), "length %d: the copy changed", cut)
	}
}

func TestMap_LayoutDependsOnlyOnContents(t *testing.T) {
	t.Parallel()
	r := testutils.NewRand(t)

	var m Map[int, int]
	model := map[int]int{}
	type saved struct {
		m     Map[int, int]
		model map[int]int
	}
	var copies []saved
	keySpace, peak := 20000, 0 // up to about 600 chunks, three levels of index
	for step := range 2500 {
		switch op := r.IntN(11); {
		case op < 5:
			for range r.IntN(60) {
				k, v := r.IntN(keySpace), r.Int()
				m.Set(k, v)
				model[k] = v
			}
		case op < 8:
			for range r.IntN(20) {
				k := r.IntN(keySpace)
				m.Delete(k)
				delete(model, k)
			}
		case op == 8:
			mod := 50 + r.IntN(200)
			del := func(k, _ int) bool { return k%mod == 0 }
			m.DeleteFunc(del)
			maps.DeleteFunc(model, del)
		case op == 9:
			m.UpdateFunc(func(k, v int) int { return v ^ k })
			for k, v := range model {
				model[k] = v ^ k
			}
		case r.IntN(100) == 0:
			m.Clear()
			clear(model)
		}
		want := MapFrom(maps.All(model))
		require.Equal(t, len(model), m.Len(), "step %d", step)
		require.True(t, reflect.DeepEqual(want, m), "step %d: layout differs", step)
		if m.root != nil {
			peak = max(peak, checkIndex(t, m))
		}
		requireKeyOrderReads(t, r, m, model, fmt.Sprintf("step %d", step))
		if r.IntN(20) == 0 {
			copies = append(copies, saved{m, maps.Clone(model)})[max(0, len(copies)-4):]
		}
		for _, c := range copies {
			require.Equal(t, c.model, maps.Collect(c.m.All()), "step %d: a copy changed", step)
			require.Equal(t, maps.Equal(c.model, model), MapEqual(c.m, m), "step %d: MapEqual with a copy", step)
		}
	}
	require.GreaterOrEqual(t, peak, 3, "the test reached a Map with an index of three levels")

	// Empty the Map one key at a time, so the index loses levels and its root shrinks to one child.
	keys := slices.Collect(maps.Keys(model))
	slices.Sort(keys)
	r.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	for i, k := range keys {
		m.Delete(k)
		delete(model, k)
		if i%25 == 0 || len(model) < 600 {
			require.True(t, reflect.DeepEqual(MapFrom(maps.All(model)), m), "%d keys left: layout differs", len(model))
		}
	}
	require.Zero(t, m.Len())
}

// TestMap_ChunksRespectLimits checks the chunking itself on a Map large enough that some chunks end
// at the length cap rather than at a key's hash.
func TestMap_ChunksRespectLimits(t *testing.T) {
	t.Parallel()

	var m Map[uint32, struct{}]
	const n = 1_000_000
	for k := range uint32(n) {
		m.Set(k*7, struct{}{})
	}
	capped, count := 0, 0
	for c := range m.allChunks() {
		count++
		last := c[len(c)-1].k
		if last == (n-1)*7 {
			require.LessOrEqual(t, len(c), mapChunkMax, "the last chunk")
			continue
		}
		require.True(t, endsChunk(len(c), last), "chunk %d ends where the rule says", count)
		for i := range len(c) - 1 {
			require.False(t, endsChunk(i+1, c[i].k), "chunk %d ends no earlier", count)
		}
		if len(c) == mapChunkMax {
			capped++
		}
	}
	require.Positive(t, capped, "some chunks reached the cap")
	require.GreaterOrEqual(t, checkIndex(t, m), 4, "the index has several levels")
	require.InDelta(t, n/(mapChunkMin+mapChunkSpread), count, n/(mapChunkMin+mapChunkSpread)/10,
		"chunks average mapChunkMin+mapChunkSpread entries")
	require.True(t, reflect.DeepEqual(MapFrom(m.All()), m))
}

// checkIndex checks that every node of m's index holds the first key of each child, that every level
// is cut where endsNode says, and that the root has more than one child. It returns the height.
func checkIndex[K Key, V any](t *testing.T, m Map[K, V]) int {
	t.Helper()
	level := []*mnode[K, V]{m.root}
	require.Greater(t, len(m.root.keys), 1, "the root has more than one child")
	h := 1
	for node := m.root; node.kids != nil; node = node.kids[0] {
		h++
	}
	for l := h; level != nil; l-- {
		var below []*mnode[K, V]
		for n, node := range level {
			require.LessOrEqual(t, len(node.keys), idxMax)
			require.Equal(t, level[0].kids == nil, node.kids == nil, "every leaf of the index is on one level")
			for i, k := range node.keys {
				if node.kids == nil {
					require.Equal(t, node.chunks[i][0].k, k)
				} else {
					require.Equal(t, node.kids[i].keys[0], k)
				}
				if n < len(level)-1 || i < len(node.keys)-1 {
					require.Equal(t, i == len(node.keys)-1, endsNode(i+1, k, l),
						"level %d node %d ends where the rule says", l, n)
				}
			}
			below = append(below, node.kids...)
		}
		level = below
	}
	return h
}

func TestMap_EqualFuncWalksTheIndex(t *testing.T) {
	t.Parallel()
	r := testutils.NewRand(t)

	model := map[int]int{}
	for len(model) < 5000 {
		model[r.IntN(1_000_000)] = r.Int()
	}
	a := MapFrom(maps.All(model))
	keys := slices.Sorted(maps.Keys(model))
	var b Map[int, int]
	for _, i := range r.Perm(len(keys)) {
		b.Set(keys[i], model[keys[i]])
	}
	require.NotNil(t, a.root)
	require.True(t, MapEqual(a, b), "built differently")

	changed := b
	k := keys[r.IntN(len(keys))]
	changed.Set(k, model[k]+1)
	require.False(t, MapEqual(a, changed), "one value differs")
	require.True(t, a.EqualFunc(changed, func(x, y int) bool { return x/2 == y/2 || x+1 == y }), "eq decides")

	swapped := b
	swapped.Delete(k)
	for nk := k + 1; ; nk++ {
		if _, ok := model[nk]; !ok {
			swapped.Set(nk, model[k])
			break
		}
	}
	require.Equal(t, a.Len(), swapped.Len())
	require.False(t, MapEqual(a, swapped), "one key differs")
}

// requireKeyOrderReads checks MaxKey, and Range over a random window and over windows at the ends,
// against the model.
func requireKeyOrderReads(t *testing.T, r *rand.Rand, m Map[int, int], model map[int]int, msg string) {
	t.Helper()
	keys := slices.Sorted(maps.Keys(model))
	maxKey, ok := m.MaxKey()
	require.Equal(t, len(keys) > 0, ok, "%s: MaxKey ok", msg)
	if ok {
		require.Equal(t, keys[len(keys)-1], maxKey, "%s: MaxKey", msg)
	}
	span := 10
	if len(keys) > 0 {
		span = keys[len(keys)-1] + 10
	}
	lo := r.IntN(span) - 5
	windows := [][2]int{{lo, lo + r.IntN(span/4+2)}, {-10, 5}, {span - 300, span}, {lo, lo}, {lo + 1, lo}}
	for _, w := range windows {
		var gotKeys, wantKeys []int
		for k, v := range m.Range(w[0], w[1]) {
			require.Equal(t, model[k], v, "%s: Range value for %d", msg, k)
			gotKeys = append(gotKeys, k)
		}
		for _, k := range keys {
			if k >= w[0] && k < w[1] {
				wantKeys = append(wantKeys, k)
			}
		}
		require.Equal(t, wantKeys, gotKeys, "%s: Range(%d, %d)", msg, w[0], w[1])
	}
}

// sortStable must give exactly what Go's stable sort gives. The elements compare by key only and
// carry their original position, so many tie and any instability shows.
func TestSortStable_MatchesGoStableSort(t *testing.T) {
	t.Parallel()
	r := testutils.NewRand(t)

	type item struct{ key, pos int }
	byKey := func(a, b item) int { return cmp.Compare(a.key, b.key) }
	sizes := []int{0, 1, 2, 3, 12, 13, 20, 21, 100, 1000, 4096}
	for range 200 {
		sizes = append(sizes, r.IntN(300))
	}
	for _, n := range sizes {
		for _, keys := range []int{1, 2, max(1, n/8), max(1, n)} {
			items := make([]item, n)
			for i := range items {
				items[i] = item{r.IntN(keys), i}
			}
			want := slices.Clone(items)
			slices.SortStableFunc(want, byKey)
			got := slices.Clone(items)
			sortStable(got, byKey)
			require.Equal(t, want, got, "n=%d, %d distinct keys", n, keys)
		}
	}
	// Already sorted and reversed input, the usual worst cases for a sort.
	for _, n := range []int{100, 4096} {
		asc := make([]item, n)
		for i := range asc {
			asc[i] = item{i / 3, i}
		}
		desc := slices.Clone(asc)
		slices.Reverse(desc)
		for _, in := range [][]item{asc, desc} {
			want := slices.Clone(in)
			slices.SortStableFunc(want, byKey)
			got := slices.Clone(in)
			sortStable(got, byKey)
			require.Equal(t, want, got, "n=%d, presorted", n)
		}
	}
}
