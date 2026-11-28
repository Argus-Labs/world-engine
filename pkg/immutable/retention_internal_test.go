package immutable

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"
	"unsafe"
	"weak"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPayload returns a value in its own allocation and a weak pointer to it.
func newPayload(i int) (string, weak.Pointer[byte]) {
	payload := strings.Repeat(string(rune('a'+i%26)), 4<<10)
	return payload, weak.Make(unsafe.StringData(payload))
}

// reachable collects garbage and counts the values that are still alive.
//
//go:noinline
func reachable(refs []weak.Pointer[byte]) int {
	runtime.GC()
	runtime.GC()

	n := 0

	for _, ref := range refs {
		if ref.Value() != nil {
			n++
		}
	}

	return n
}

// mapFromDuplicates builds a Map from distinct unique keys, each with a short value, followed by
// repeats values under the key "~", which sorts after them. It returns weak pointers to the values
// yielded under "~"; the last one wins. It doesn't inline, so no value lingers in the caller's frame.
//
//go:noinline
func mapFromDuplicates(distinct, repeats int) (Map[string, string], []weak.Pointer[byte]) {
	refs := make([]weak.Pointer[byte], 0, repeats)
	m := MapFrom(func(yield func(string, string) bool) {
		for i := range distinct {
			if !yield(fmt.Sprintf("k%03d", i), "short") {
				return
			}
		}

		for i := range repeats {
			payload, ref := newPayload(i)
			refs = append(refs, ref)

			if !yield("~", payload) {
				return
			}
		}
	})

	return m, refs
}

// MapFrom keeps only the last value under a repeated key, so the Map it returns must not keep the
// values it dropped alive.
func TestMapFrom_ReleasesDroppedValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		distinct, repeats int
	}{
		// Almost every entry is dropped.
		{name: "one key", distinct: 0, repeats: 32},
		// Few are dropped, and they sort last, where compacting in place leaves them behind. MapFrom's
		// array then holds at most twice the entries it keeps, so shrink keeps it rather than copy,
		// and only clearing the dropped entries frees them.
		{name: "few repeats", distinct: 300, repeats: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.distinct > 0 {
				// MapFrom appends every pair to one array, so this array's capacity is the one it gets.
				var entries []entry[string, string]
				for range tt.distinct + tt.repeats {
					entries = append(entries, entry[string, string]{})
				}

				require.LessOrEqual(t, cap(entries), 2*(tt.distinct+1), "shrink would copy, so clear goes untested")
			}

			m, refs := mapFromDuplicates(tt.distinct, tt.repeats)
			require.Equal(t, tt.distinct+1, m.Len())

			got, ok := m.Get("~")
			require.True(t, ok)
			require.Same(t, refs[len(refs)-1].Value(), unsafe.StringData(got), "the last value wins")

			dropped, kept := refs[:len(refs)-1], refs[len(refs)-1:]
			assert.Zero(t, reachable(dropped), "dropped values stay reachable from the Map")
			assert.Equal(t, 1, reachable(kept), "the kept value is lost")
			runtime.KeepAlive(m)
		})
	}
}

// payloadSlice returns a Slice of n payloads, built by SliceOf, Collect or Append, and a weak
// pointer to each.
//
//go:noinline
func payloadSlice(n int, build string) (Slice[string], []weak.Pointer[byte]) {
	items := make([]string, n)
	refs := make([]weak.Pointer[byte], n)

	for i := range items {
		items[i], refs[i] = newPayload(i)
	}

	var s Slice[string]

	switch build {
	case "SliceOf":
		s = SliceOf(items...)
	case "Collect":
		s = Collect(slices.Values(items))
	case "Append":
		s.Append(items...)
	default:
		panic("unknown build " + build)
	}

	return s, refs
}

// deleteFrom builds a Slice of n payloads and deletes [i, j) from it, or, with pops, deletes the
// last element until one is left. It returns the result and weak pointers to every payload.
//
//go:noinline
func deleteFrom(n, i, j int, build string, pops, viaFunc bool) (Slice[string], []weak.Pointer[byte]) {
	s, refs := payloadSlice(n, build)

	switch {
	case pops:
		for s.Len() > 1 {
			s.Delete(s.Len()-1, s.Len())
		}
	case viaFunc:
		k := 0

		s.DeleteFunc(func(string) bool {
			k++
			return k-1 >= i && k-1 < j
		})
	default:
		s.Delete(i, j)
	}

	return s, refs
}

// A Delete that takes a shorter view of the tail or of the last leaf must not keep the deleted
// values alive in the part of the array the view no longer covers.
func TestSlice_DeleteReleasesDeletedValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		n, i, j  int
		build    string
		pops     bool
		viaFunc  bool
		wantKept int
	}{
		{name: "front of the tail", n: 32, i: 0, j: 31, build: "SliceOf"},
		{name: "end of the tail", n: 32, i: 1, j: 32, build: "SliceOf"},
		{name: "end of the last leaf", n: 65, i: 33, j: 65, build: "SliceOf"},
		{name: "repeated pops, SliceOf", n: 128, i: 1, j: 128, build: "SliceOf", pops: true},
		{name: "repeated pops, Append", n: 128, i: 1, j: 128, build: "Append", pops: true},
		// Controls: these rebuild or empty the Slice, which never kept deleted values.
		{name: "DeleteFunc", n: 128, i: 1, j: 128, build: "SliceOf", viaFunc: true},
		{name: "everything", n: 32, i: 0, j: 32, build: "SliceOf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, refs := deleteFrom(tt.n, tt.i, tt.j, tt.build, tt.pops, tt.viaFunc)
			require.Equal(t, tt.n-(tt.j-tt.i), s.Len())

			for k, v := range s.All() {
				orig := k
				if k >= tt.i {
					orig += tt.j - tt.i
				}

				require.Same(t, refs[orig].Value(), unsafe.StringData(v), "element %d", k)
			}

			deleted := refs[tt.i:tt.j]
			kept := slices.Concat(refs[:tt.i], refs[tt.j:])

			assert.Zero(t, reachable(deleted), "deleted values stay reachable from the Slice")
			assert.Equal(t, len(kept), reachable(kept), "a kept value is lost")
			runtime.KeepAlive(s)
		})
	}
}

// Delete must copy the survivors rather than clear the deleted part in place: a copy made before
// the Delete shares the array and still holds every value.
func TestSlice_DeleteKeepsCopies(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ n, i, j int }{{32, 0, 31}, {32, 1, 32}, {65, 33, 65}} {
		t.Run(fmt.Sprintf("%d[%d:%d]", tt.n, tt.i, tt.j), func(t *testing.T) {
			t.Parallel()

			s, refs := payloadSlice(tt.n, "SliceOf")
			copied := s
			s.Delete(tt.i, tt.j)

			require.Equal(t, tt.n, copied.Len())

			for k, v := range copied.All() {
				require.Same(t, refs[k].Value(), unsafe.StringData(v), "the copy's element %d changed", k)
			}

			assert.Equal(t, tt.n, reachable(refs), "a value the copy holds is lost")
			runtime.KeepAlive(copied)
			runtime.KeepAlive(s)
		})
	}
}

// rewriteSliceOutside builds a Slice of n payloads and sets every element outside [lo, hi) to a
// short string. It returns the result and weak pointers to every payload.
//
//go:noinline
func rewriteSliceOutside(n, lo, hi int, build string) (Slice[string], []weak.Pointer[byte]) {
	s, refs := payloadSlice(n, build)

	for i := range n {
		if i < lo || i >= hi {
			s.Set(i, "short")
		}
	}

	return s, refs
}

// After a Set of every element outside one chunk, the Slice holds only that chunk's payloads, so
// only they may stay alive. A Slice built in bulk must not keep the replaced values reachable
// through the untouched chunk, which shared their array.
func TestSlice_SetReleasesReplacedValues(t *testing.T) {
	t.Parallel()

	const n = 100 // three leaves and a tail of four

	for _, build := range []string{"SliceOf", "Collect", "Append"} {
		for _, chunk := range [][2]int{{0, 32}, {32, 64}, {96, n}} {
			t.Run(fmt.Sprintf("%s/keep [%d:%d]", build, chunk[0], chunk[1]), func(t *testing.T) {
				t.Parallel()

				s, refs := rewriteSliceOutside(n, chunk[0], chunk[1], build)
				require.Same(t, refs[chunk[0]].Value(), unsafe.StringData(s.At(chunk[0])))

				assert.Equal(t, chunk[1]-chunk[0], reachable(refs), "replaced values stay reachable from the Slice")
				runtime.KeepAlive(s)
			})
		}
	}
}

// payloadMap returns a Map of keys 0 to n-1, each holding a payload, and a weak pointer to each
// payload. It is built by MapFrom, by MapFrom and then UpdateFunc, or by MapFrom over the even keys
// and then a Set of each odd key, whose inserts re-cut chunks.
//
//go:noinline
func payloadMap(n int, build string) (Map[int, string], []weak.Pointer[byte]) {
	refs := make([]weak.Pointer[byte], n)
	payloads := make([]string, n)

	for i := range payloads {
		payloads[i], refs[i] = newPayload(i)
	}

	var m Map[int, string]

	switch build {
	case "MapFrom":
		m = MapFrom(slices.All(payloads))
	case "UpdateFunc":
		m = MapFrom(func(yield func(int, string) bool) {
			for i := range n {
				if !yield(i, "short") {
					return
				}
			}
		})
		m.UpdateFunc(func(k int, _ string) string { return payloads[k] })
	case "Set":
		m = MapFrom(func(yield func(int, string) bool) {
			for i := 0; i < n; i += 2 {
				if !yield(i, payloads[i]) {
					return
				}
			}
		})

		for i := 1; i < n; i += 2 {
			m.Set(i, payloads[i])
		}
	default:
		panic("unknown build " + build)
	}

	return m, refs
}

// rewriteMapOutside builds a Map with payloadMap and sets every key outside [lo, hi] to a short
// string.
//
//go:noinline
func rewriteMapOutside(n, lo, hi int, build string) (Map[int, string], []weak.Pointer[byte]) {
	m, refs := payloadMap(n, build)

	for k := range n {
		if k < lo || k > hi {
			m.Set(k, "short")
		}
	}

	return m, refs
}

// chunkBounds returns the first and last key of each of m's chunks.
func chunkBounds[K Key, V any](m Map[K, V]) [][2]K {
	var bounds [][2]K
	for c := range m.allChunks() {
		bounds = append(bounds, [2]K{c[0].k, c[len(c)-1].k})
	}

	return bounds
}

// After a Set of every key outside one chunk, the Map holds only that chunk's payloads, so only they
// may stay alive. Chunk boundaries follow a seed drawn at startup, so the test keeps each chunk in
// turn: whichever chunks share an array, keeping one of them catches it.
func TestMap_SetReleasesReplacedValues(t *testing.T) {
	t.Parallel()

	const n = 600 // well past mapChunkMax, so several chunks whatever the seed

	for _, build := range []string{"MapFrom", "UpdateFunc", "Set"} {
		t.Run(build, func(t *testing.T) {
			t.Parallel()

			m, _ := payloadMap(n, build)
			bounds := chunkBounds(m)
			require.Greater(t, len(bounds), 1)

			for _, b := range bounds {
				edited, refs := rewriteMapOutside(n, b[0], b[1], build)
				require.Equal(t, bounds, chunkBounds(edited), "a Set of an existing key moved a chunk boundary")

				got, _ := edited.Get(b[0])
				require.Same(t, refs[b[0]].Value(), unsafe.StringData(got))

				want, live := b[1]-b[0]+1, reachable(refs)
				if !assert.Equal(t, want, live, "keeping keys %d to %d: replaced values stay alive", b[0], b[1]) {
					return
				}

				runtime.KeepAlive(edited)
			}
		})
	}
}

// editPayloadKey builds a Map of short values with a payload under key 0, then deletes or replaces
// key 0.
//
//go:noinline
func editPayloadKey(n int, del bool) (Map[int, string], weak.Pointer[byte]) {
	payload, ref := newPayload(0)
	m := MapFrom(func(yield func(int, string) bool) {
		for i := range n {
			v := "short"
			if i == 0 {
				v = payload
			}

			if !yield(i, v) {
				return
			}
		}
	})

	if del {
		m.Delete(0)
	} else {
		m.Set(0, "replaced")
	}

	return m, ref
}

func TestMap_EditReleasesRemovedValue(t *testing.T) {
	t.Parallel()

	for _, del := range []bool{false, true} {
		t.Run(fmt.Sprintf("delete=%v", del), func(t *testing.T) {
			t.Parallel()

			m, ref := editPayloadKey(1024, del)
			require.Equal(t, del, !m.Has(0))

			assert.Zero(t, reachable([]weak.Pointer[byte]{ref}), "the removed value stays reachable from the Map")
			runtime.KeepAlive(m)
		})
	}
}
