package immutable_test

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/stretchr/testify/require"
)

type point struct {
	X, Y int
}

func TestSlice_ZeroValueIsEmpty(t *testing.T) {
	t.Parallel()

	var s immutable.Slice[int]
	require.Equal(t, 0, s.Len())
	require.Empty(t, s.Clone())
	for range s.All() {
		t.Fatal("zero Slice must yield nothing")
	}
	require.Equal(t, 1, s.Append(1).Len(), "a zero Slice can be appended to")
}

func TestSliceOf_CopiesInput(t *testing.T) {
	t.Parallel()

	in := []int{1, 2, 3}
	s := immutable.SliceOf(in...)
	in[0] = 99

	require.Equal(t, 1, s.At(0), "changing the input after construction must not reach the Slice")
	require.Equal(t, []int{1, 2, 3}, s.Clone())
}

func TestSlice_CloneIsIndependent(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3)
	out := s.Clone()
	out[0] = 99

	require.Equal(t, 1, s.At(0), "changing a Clone must not reach the Slice")
}

func TestSlice_AtReturnsCopy(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(point{X: 1, Y: 2})
	p := s.At(0)
	p.X = 99

	require.Equal(t, 99, p.X, "the copy changes")
	require.Equal(t, point{X: 1, Y: 2}, s.At(0), "the Slice does not")
	require.Panics(t, func() { s.At(1) })
}

func TestSlice_AppendDoesNotShareBacking(t *testing.T) {
	t.Parallel()

	base := immutable.SliceOf(1, 2, 3)
	a := base.Append(4)
	b := base.Append(5, 6)

	require.Equal(t, []int{1, 2, 3}, base.Clone(), "receiver must be unchanged")
	require.Equal(t, []int{1, 2, 3, 4}, a.Clone())
	require.Equal(t, []int{1, 2, 3, 5, 6}, b.Clone(), "two appends from one base must not clobber")
	require.Equal(t, base.Clone(), base.Append().Clone(), "empty append is a no-op")
}

func TestSlice_WithReplacesOneElement(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3)
	w := s.With(1, 42)

	require.Equal(t, []int{1, 2, 3}, s.Clone(), "receiver must be unchanged")
	require.Equal(t, []int{1, 42, 3}, w.Clone())
	require.Panics(t, func() { s.With(3, 0) })
}

func TestSlice_WithoutDropsOneElement(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3)
	require.Equal(t, []int{2, 3}, s.Without(0).Clone())
	require.Equal(t, []int{1, 3}, s.Without(1).Clone())
	require.Equal(t, []int{1, 2}, s.Without(2).Clone())
	require.Equal(t, []int{1, 2, 3}, s.Clone(), "receiver must be unchanged")
	require.Equal(t, immutable.SliceOf[int](), immutable.SliceOf(1).Without(0),
		"dropping the last gives the one empty value")
	require.Panics(t, func() { s.Without(3) })
}

func TestSlice_FilterKeepsMatches(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3, 4)
	even := func(v int) bool { return v%2 == 0 }

	require.Equal(t, []int{2, 4}, s.Filter(even).Clone())
	require.Equal(t, []int{1, 2, 3, 4}, s.Clone(), "receiver must be unchanged")
	require.Equal(t, immutable.SliceOf[int](), s.Filter(func(int) bool { return false }),
		"dropping all gives the one empty value")
	require.Equal(t, s.Clone(), s.Filter(func(int) bool { return true }).Clone(), "keeping all keeps order")

	calls := 0
	s.Filter(func(v int) bool { calls++; return v != 2 })
	require.Equal(t, 4, calls, "keep runs once per element")
}

func TestSlice_AllYieldsInOrderAndStopsEarly(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf("a", "b", "c")

	var got []string
	for i, v := range s.All() {
		require.Equal(t, v, s.At(i))
		got = append(got, v)
	}
	require.Equal(t, []string{"a", "b", "c"}, got)

	seen := 0
	for range s.All() {
		seen++
		break
	}
	require.Equal(t, 1, seen)
}

func TestSlice_EqualFunc(t *testing.T) {
	t.Parallel()

	eq := func(a, b int) bool { return a == b }
	s := immutable.SliceOf(1, 2, 3)

	require.True(t, s.EqualFunc(immutable.SliceOf(1, 2, 3), eq))
	require.False(t, s.EqualFunc(immutable.SliceOf(1, 2), eq), "different length")
	require.False(t, s.EqualFunc(immutable.SliceOf(1, 2, 4), eq), "different element")
	require.True(t, immutable.Slice[int]{}.EqualFunc(immutable.SliceOf[int](), eq), "empty equals empty")
}

func TestEqual(t *testing.T) {
	t.Parallel()

	require.True(t, immutable.Equal(immutable.SliceOf(1, 2, 3), immutable.SliceOf(1, 2, 3)))
	require.False(t, immutable.Equal(immutable.SliceOf(1, 2, 3), immutable.SliceOf(1, 2)), "different length")
	require.False(t, immutable.Equal(immutable.SliceOf(1, 2, 3), immutable.SliceOf(1, 2, 4)), "different element")
	require.True(t, immutable.Equal(immutable.Slice[int]{}, immutable.SliceOf[int]()), "empty equals empty")
}

// Every empty Slice must be the same value however it was produced: reflect.DeepEqual, and so
// require.Equal on a whole component, must not tell them apart. Only the zero value is naturally
// nil — the stdlib helpers behind these derivations all hand back an empty but non-nil slice, so
// each one relies on wrap to canonicalise it.
func TestSlice_EmptyIsOneValue(t *testing.T) {
	t.Parallel()

	empty := immutable.SliceOf[int]()
	full := immutable.SliceOf(1, 2, 3)

	require.Equal(t, immutable.Slice[int]{}, empty, "zero value")
	require.Equal(t, empty, immutable.SliceOf([]int{}...), "built from an empty non-nil slice")
	require.Equal(t, empty, empty.Append(), "empty append")
	require.Equal(t, empty, full.Filter(func(int) bool { return false }), "filtered to nothing")
	require.Equal(t, empty, full.Delete(0, 3), "deleted everything")
	require.Equal(t, empty, full.Without(0).Without(0).Without(0), "removed one at a time")
	require.Equal(t, empty, full.Sub(1, 1), "empty range")
	require.Equal(t, empty, immutable.Compact(empty), "compacted")
	require.Equal(t, empty, immutable.Collect(empty.Values()), "collected from an empty iterator")
}

// Slice copies the engine type's name and layout but lives in this package, so SliceElem must
// refuse it.
//
//nolint:unused // the field is never read on purpose; only the layout matters
type Slice[T any] struct{ items []T }

func TestSliceElem(t *testing.T) {
	t.Parallel()

	elem, ok := immutable.SliceElem(reflect.TypeFor[immutable.Slice[point]]())
	require.True(t, ok)
	require.Equal(t, reflect.TypeFor[point](), elem)

	nested, ok := immutable.SliceElem(reflect.TypeFor[immutable.Slice[immutable.Slice[int]]]())
	require.True(t, ok)
	require.Equal(t, reflect.TypeFor[immutable.Slice[int]](), nested)

	_, ok = immutable.SliceElem(reflect.TypeFor[[]point]())
	require.False(t, ok, "a raw slice is not a Slice")
	_, ok = immutable.SliceElem(reflect.TypeFor[Slice[point]]())
	require.False(t, ok, "a look-alike from another package is not a Slice")
	_, ok = immutable.SliceElem(nil)
	require.False(t, ok)
}

func TestSlice_ValuesAndBackward(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3)
	require.Equal(t, []int{1, 2, 3}, slices.Collect(s.Values()))

	var idx, vals []int
	for i, v := range s.Backward() {
		idx, vals = append(idx, i), append(vals, v)
	}
	require.Equal(t, []int{2, 1, 0}, idx)
	require.Equal(t, []int{3, 2, 1}, vals)

	seen := 0
	for range s.Values() {
		seen++
		break
	}
	require.Equal(t, 1, seen, "early break")
}

func TestSlice_Search(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(10, 20, 30)
	even := func(v int) bool { return v%20 == 0 }
	require.Equal(t, 1, s.IndexFunc(even))
	require.Equal(t, -1, s.IndexFunc(func(int) bool { return false }))
	require.True(t, s.ContainsFunc(even))
	require.False(t, s.ContainsFunc(func(int) bool { return false }))
	require.Equal(t, 2, immutable.Index(s, 30))
	require.Equal(t, -1, immutable.Index(s, 99))
	require.True(t, immutable.Contains(s, 10))
	require.False(t, immutable.Contains(s, 99))
}

func TestSlice_Insert(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 3)
	require.Equal(t, []int{0, 1, 3}, s.Insert(0, 0).Clone())
	require.Equal(t, []int{1, 2, 3}, s.Insert(1, 2).Clone())
	require.Equal(t, []int{1, 3, 4, 5}, s.Insert(2, 4, 5).Clone(), "at Len appends")
	require.Equal(t, []int{1, 3}, s.Clone(), "receiver must be unchanged")
	require.Equal(t, s.Clone(), s.Insert(1).Clone(), "no items is a no-op")
	require.Panics(t, func() { s.Insert(3, 9) })
	require.Panics(t, func() { s.Insert(-1, 9) })
}

func TestSlice_Sub(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3, 4)
	require.Equal(t, []int{2, 3}, s.Sub(1, 3).Clone())
	require.Equal(t, []int{1, 2, 3, 4}, s.Sub(0, 4).Clone())
	require.Equal(t, immutable.SliceOf[int](), s.Sub(2, 2), "empty range gives the one empty value")
	require.Equal(t, []int{1, 2, 3, 4}, s.Clone(), "receiver must be unchanged")
	require.Panics(t, func() { s.Sub(0, 5) })
	require.Panics(t, func() { s.Sub(3, 2) })
	require.Panics(t, func() { s.Sub(-1, 2) })

	// A Filter result carries spare capacity; Sub must still refuse to read past Len into it.
	spare := s.Filter(func(v int) bool { return v != 4 })
	require.Equal(t, 3, spare.Len())
	require.Panics(t, func() { spare.Sub(0, 4) })
}

func TestSlice_ReversedAndSorted(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(3, 1, 2)
	require.Equal(t, []int{2, 1, 3}, s.Reversed().Clone())
	require.Equal(t, []int{1, 2, 3}, immutable.Sorted(s).Clone())
	require.Equal(t, []int{3, 2, 1}, s.SortedFunc(func(a, b int) int { return b - a }).Clone())
	require.Equal(t, []int{3, 1, 2}, s.Clone(), "receiver must be unchanged")
	require.Equal(t, immutable.SliceOf[int](), immutable.Slice[int]{}.Reversed())
	require.Equal(t, immutable.SliceOf[int](), immutable.Sorted(immutable.Slice[int]{}))

	// Stable: equal keys keep their original order.
	byLen := func(a, b string) int { return len(a) - len(b) }
	got := immutable.SliceOf("bb", "a", "cc", "b").SortedFunc(byLen).Clone()
	require.Equal(t, []string{"a", "b", "bb", "cc"}, got)
}

func TestSlice_MapConcatCollectCompact(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3)
	require.Equal(t, []string{"1", "2", "3"}, immutable.Map(s, strconv.Itoa).Clone())
	require.Equal(t, immutable.SliceOf[string](), immutable.Map(immutable.Slice[int]{}, strconv.Itoa))

	require.Equal(t, []int{1, 2, 3, 4, 5}, immutable.Concat(s, immutable.Slice[int]{}, immutable.SliceOf(4, 5)).Clone())
	require.Equal(t, immutable.SliceOf[int](), immutable.Concat[int]())

	require.Equal(t, []int{1, 2, 3}, immutable.Collect(s.Values()).Clone())
	require.Equal(t, immutable.SliceOf[int](), immutable.Collect(immutable.Slice[int]{}.Values()))

	dup := immutable.SliceOf(1, 1, 2, 2, 2, 1)
	require.Equal(t, []int{1, 2, 1}, immutable.Compact(dup).Clone())
	require.Equal(t, []int{1, 1, 2, 2, 2, 1}, dup.Clone(), "receiver must be unchanged")
	require.Equal(t, immutable.SliceOf[int](), immutable.Compact(immutable.Slice[int]{}))
}

func TestSlice_MinMaxAndSortedChecks(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(3, 1, 2)
	byValue := func(a, b int) int { return a - b }
	require.Equal(t, 1, immutable.Min(s))
	require.Equal(t, 3, immutable.Max(s))
	require.Equal(t, 1, s.MinFunc(byValue))
	require.Equal(t, 3, s.MaxFunc(byValue))
	require.Panics(t, func() { immutable.Min(immutable.Slice[int]{}) })
	require.Panics(t, func() { immutable.Slice[int]{}.MaxFunc(byValue) })

	require.False(t, immutable.IsSorted(s))
	require.True(t, immutable.IsSorted(immutable.Sorted(s)))
	require.True(t, s.IsSortedFunc(func(int, int) int { return 0 }), "everything equal counts as sorted")
}

func TestSlice_BinarySearchAndCompare(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(10, 20, 30)
	i, found := immutable.BinarySearch(s, 20)
	require.True(t, found)
	require.Equal(t, 1, i)
	i, found = immutable.BinarySearch(s, 25)
	require.False(t, found)
	require.Equal(t, 2, i, "insertion point")

	i, found = immutable.BinarySearchFunc(s, "30", func(e int, target string) int {
		return strings.Compare(strconv.Itoa(e), target)
	})
	require.True(t, found)
	require.Equal(t, 2, i)

	require.Equal(t, 0, immutable.Compare(s, immutable.SliceOf(10, 20, 30)))
	require.Equal(t, -1, immutable.Compare(s, immutable.SliceOf(10, 20, 31)))
	require.Equal(t, 1, immutable.Compare(s, immutable.SliceOf(10, 20)), "longer wins when the prefix ties")
	require.Equal(t, 0, immutable.CompareFunc(s, immutable.SliceOf("10", "20", "30"),
		func(e int, o string) int { return strings.Compare(strconv.Itoa(e), o) }))
}

func TestSlice_DeleteReplaceRepeatChunkCompactFunc(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3, 4)
	require.Equal(t, []int{1, 4}, s.Delete(1, 3).Clone())
	require.Equal(t, s.Clone(), s.Delete(2, 2).Clone(), "empty range is a no-op")
	require.Equal(t, immutable.SliceOf[int](), s.Delete(0, 4), "deleting all gives the one empty value")
	require.Panics(t, func() { s.Delete(3, 5) })

	require.Equal(t, []int{1, 9, 9, 4}, s.Replace(1, 3, 9, 9).Clone())
	require.Equal(t, []int{1, 4}, s.Replace(1, 3).Clone(), "replace with nothing deletes")
	require.Equal(t, immutable.SliceOf[int](), s.Replace(0, 4),
		"replacing all with nothing gives the one empty value")

	require.Equal(t, []int{1, 2, 1, 2}, immutable.SliceOf(1, 2).Repeat(2).Clone())
	require.Equal(t, immutable.SliceOf[int](), s.Repeat(0))
	require.Panics(t, func() { s.Repeat(-1) })

	var chunks [][]int
	for part := range s.Chunk(3) {
		chunks = append(chunks, part.Clone())
	}
	require.Equal(t, [][]int{{1, 2, 3}, {4}}, chunks)
	require.Panics(t, func() { s.Chunk(0) })

	sameParity := func(a, b int) bool { return a%2 == b%2 }
	require.Equal(t, []int{1, 2, 3}, immutable.SliceOf(1, 1, 2, 2, 3).CompactFunc(sameParity).Clone())
	require.Equal(t, []int{1, 2, 3, 4}, s.Clone(), "receiver must be unchanged")
}

// TestSlice_DerivationsNeverWriteSharedBacking is the guarantee the type exists for, checked across
// every derivation at once: none of them may write into a backing array another Slice can see.
//
// The base is built with Delete, which leaves spare capacity (length 2, capacity 4). That is the
// shape the hazard needs: a derivation implemented as append(s.items, ...) would write into the
// spare room instead of allocating, so two derivations from one base would land on the same memory
// and the second would silently rewrite the first. Every result is computed before any is checked,
// so cross-contamination in either direction fails here.
func TestSlice_DerivationsNeverWriteSharedBacking(t *testing.T) {
	t.Parallel()

	base := immutable.SliceOf(1, 2, 3, 4).Delete(1, 3)
	require.Equal(t, []int{1, 4}, base.Clone())

	cases := []struct {
		name   string
		derive func() immutable.Slice[int]
		want   []int
	}{
		{"Append", func() immutable.Slice[int] { return base.Append(9) }, []int{1, 4, 9}},
		{"Insert", func() immutable.Slice[int] { return base.Insert(1, 8) }, []int{1, 8, 4}},
		{"With", func() immutable.Slice[int] { return base.With(0, 7) }, []int{7, 4}},
		{"Without", func() immutable.Slice[int] { return base.Without(0) }, []int{4}},
		{"Filter", func() immutable.Slice[int] { return base.Filter(func(v int) bool { return v == 4 }) }, []int{4}},
		{"Delete", func() immutable.Slice[int] { return base.Delete(0, 1) }, []int{4}},
		{"Replace", func() immutable.Slice[int] { return base.Replace(0, 1, 6, 6) }, []int{6, 6, 4}},
		{"Sub", func() immutable.Slice[int] { return base.Sub(0, 1) }, []int{1}},
		{"Reversed", base.Reversed, []int{4, 1}},
		{"SortedFunc", func() immutable.Slice[int] {
			return base.SortedFunc(func(a, b int) int { return b - a })
		}, []int{4, 1}},
		{"CompactFunc", func() immutable.Slice[int] {
			return base.CompactFunc(func(int, int) bool { return true })
		}, []int{1}},
		{"Repeat", func() immutable.Slice[int] { return base.Repeat(2) }, []int{1, 4, 1, 4}},
	}

	got := make([]immutable.Slice[int], len(cases))
	for i, c := range cases {
		got[i] = c.derive()
	}
	for i, c := range cases {
		require.Equal(t, c.want, got[i].Clone(), "%s was rewritten by a later derivation", c.name)
	}
	require.Equal(t, []int{1, 4}, base.Clone(), "the base must survive every derivation")
}
