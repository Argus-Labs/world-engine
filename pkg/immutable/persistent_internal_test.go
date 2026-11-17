package immutable

import (
	"maps"
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
		switch op := r.IntN(10); {
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
		case r.IntN(50) == 0:
			s.Reset()
			model = nil
		}
		require.Equal(t, len(model), s.Len(), "step %d", step)
		require.True(t, reflect.DeepEqual(fromItems(slices.Clone(model)), s), "step %d: layout differs", step)
		if len(model) > 0 {
			i := r.IntN(len(model))
			require.Equal(t, model[i], s.At(i), "step %d", step)
		}
		if r.IntN(20) == 0 {
			copies = append(copies, saved{s, slices.Clone(model)})[max(0, len(copies)-4):]
		}
		for _, c := range copies {
			require.Equal(t, c.items, slices.Collect(c.s.Values()), "step %d: a copy changed", step)
		}
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
	keySpace, peak := 20000, 0
	for step := range 2500 {
		switch op := r.IntN(10); {
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
		case r.IntN(100) == 0:
			m.Clear()
			clear(model)
		}
		want := MapFrom(maps.All(model))
		require.Equal(t, len(model), m.Len(), "step %d", step)
		require.True(t, reflect.DeepEqual(want, m), "step %d: layout differs", step)
		peak = max(peak, m.chunkCount())
		if r.IntN(20) == 0 {
			copies = append(copies, saved{m, maps.Clone(model)})[max(0, len(copies)-4):]
		}
		for _, c := range copies {
			require.Equal(t, c.model, maps.Collect(c.m.All()), "step %d: a copy changed", step)
		}
	}
	require.Greater(t, peak, 100, "the test reached a Map of many chunks")
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
	require.InDelta(t, n/(mapChunkMin+mapChunkSpread), count, n/(mapChunkMin+mapChunkSpread)/10,
		"chunks average mapChunkMin+mapChunkSpread entries")
	require.True(t, reflect.DeepEqual(MapFrom(m.All()), m))
}
