package immutable_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	gojson "github.com/goccy/go-json"

	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

type point struct {
	X, Y int
}

// collect copies a Slice's elements out to a plain []T for comparison.
func collect[T any](s immutable.Slice[T]) []T {
	return slices.Collect(s.Values())
}

func TestSlice_MessagePack(t *testing.T) {
	t.Parallel()
	type holder struct {
		Items immutable.Slice[immutable.Slice[uint64]] `msgpack:"items,omitempty"`
	}
	want := holder{Items: immutable.SliceOf(immutable.SliceOf(uint64(9007199254740993), ^uint64(0)))}
	data, err := msgpack.Marshal(want)
	require.NoError(t, err)
	var got holder
	require.NoError(t, msgpack.Unmarshal(data, &got))
	require.Equal(t, want, got)
	var wire struct {
		Items [][]uint64 `msgpack:"items"`
	}
	require.NoError(t, msgpack.Unmarshal(data, &wire))
	require.Equal(t, [][]uint64{{9007199254740993, ^uint64(0)}}, wire.Items)
	data, err = msgpack.Marshal(holder{})
	require.NoError(t, err)
	require.Equal(t, []byte{0x80}, data, "empty Slice is omitted")

	type octet uint8
	for _, value := range []any{immutable.SliceOf(byte(7), byte(255)), immutable.SliceOf(octet(7), octet(255))} {
		data, err = msgpack.Marshal(value)
		require.NoError(t, err)
		require.Equal(t, byte(0x92), data[0], "byte-valued Slices are arrays, not binary")
		var back immutable.Slice[byte]
		require.NoError(t, msgpack.Unmarshal(data, &back))
		require.Equal(t, immutable.SliceOf(byte(7), byte(255)), back)
		var named immutable.Slice[octet]
		require.NoError(t, msgpack.Unmarshal(data, &named))
		require.Equal(t, immutable.SliceOf(octet(7), octet(255)), named)
	}
}

func TestSlice_MessagePackEmptyAndFailedDecode(t *testing.T) {
	t.Parallel()
	for _, empty := range []immutable.Slice[int]{{}, immutable.SliceOf(1).Without(0)} {
		data, err := msgpack.Marshal(empty)
		require.NoError(t, err)
		require.Equal(t, []byte{0x90}, data)
	}
	for _, data := range [][]byte{{0x90}, {0xc0}} {
		value := immutable.SliceOf(8, 9)
		require.NoError(t, msgpack.Unmarshal(data, &value))
		require.Equal(t, immutable.Slice[int]{}, value)
	}
	value := immutable.SliceOf(8, 9)
	alias := value
	require.Error(t, msgpack.Unmarshal([]byte{0x92, 0x01}, &value))
	require.Equal(t, []int{8, 9}, collect(value))
	require.Equal(t, []int{8, 9}, collect(alias))
}

func TestSlice_ZeroValueIsEmpty(t *testing.T) {
	t.Parallel()

	var s immutable.Slice[int]
	require.Equal(t, 0, s.Len())
	require.Empty(t, collect(s))
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
	require.Equal(t, []int{1, 2, 3}, collect(s))
}

func TestSlice_ValuesCopyIsIndependent(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3)
	out := collect(s)
	out[0] = 99

	require.Equal(t, 1, s.At(0), "changing a copy taken via Values must not reach the Slice")
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

	require.Equal(t, []int{1, 2, 3}, collect(base), "receiver must be unchanged")
	require.Equal(t, []int{1, 2, 3, 4}, collect(a))
	require.Equal(t, []int{1, 2, 3, 5, 6}, collect(b), "two appends from one base must not clobber")
	require.Equal(t, collect(base), collect(base.Append()), "empty append is a no-op")
}

func TestSlice_WithReplacesOneElement(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3)
	w := s.With(1, 42)

	require.Equal(t, []int{1, 42, 3}, collect(w))
	require.Equal(t, []int{1, 2, 3}, collect(s), "the receiver is unchanged")
	require.Panics(t, func() { immutable.SliceOf(1, 2, 3).With(3, 0) })
}

func TestSlice_WithoutDropsOneElement(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3)
	require.Equal(t, []int{2, 3}, collect(s.Without(0)))
	require.Equal(t, []int{1, 3}, collect(s.Without(1)))
	require.Equal(t, []int{1, 2}, collect(s.Without(2)))
	require.Equal(t, 0, immutable.SliceOf(1).Without(0).Len(), "dropping the last leaves it empty")
	require.Panics(t, func() { s.Without(3) })
	require.Panics(t, func() { s.Without(-1) })
	require.Equal(t, []int{1, 2, 3}, collect(s), "the receiver is unchanged")
}

func TestSlice_FilterKeepsMatches(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3, 4)
	even := func(v int) bool { return v%2 == 0 }

	require.Equal(t, []int{2, 4}, collect(s.Filter(even)))
	require.Equal(t, 0, s.Filter(func(int) bool { return false }).Len(), "dropping all leaves it empty")
	require.Equal(t, []int{1, 2, 3, 4}, collect(s.Filter(func(int) bool { return true })),
		"keeping all keeps order")
	require.Equal(t, []int{1, 2, 3, 4}, collect(s), "the receiver is unchanged")

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

// Every producer and derivation must return the zero value for an empty result, not an
// empty-but-allocated Slice. That is the rule DecodeMsgpack and UnmarshalJSON both enforce, so a
// component built fresh compares equal to one restored from a snapshot under [reflect.DeepEqual].
// This table pins every path at once so a regression in any one of them fails here.
func TestSlice_ProducerEmptyIsZeroValue(t *testing.T) {
	t.Parallel()

	zero := immutable.Slice[int]{}
	empty := immutable.SliceOf[int]()
	id := func(int) int { return 0 }
	one := immutable.SliceOf(1)
	emptied := one.Edit()
	emptied.Delete(0, 1)

	producers := []struct {
		name string
		got  immutable.Slice[int]
	}{
		{"Repeat(zero,0)", zero.Repeat(0)},
		{"Repeat(empty,0)", empty.Repeat(0)},
		{"Repeat(non-empty,0)", immutable.SliceOf(1, 2, 3).Repeat(0)},
		{"Repeat(zero,3)", zero.Repeat(3)},
		{"Transform(zero,id)", immutable.Transform(zero, id)},
		{"Transform(empty,id)", immutable.Transform(empty, id)},
		{"Concat()", immutable.Concat[int]()},
		{"Concat(zero,empty)", immutable.Concat(zero, empty)},
		{"Collect(zero.Values)", immutable.Collect(zero.Values())},
		{"Append(zero)", zero.Append()},
		{"Append(empty)", empty.Append()},
		{"SliceOf(empty literal)", immutable.SliceOf([]int{}...)},
		{"Without", one.Without(0)},
		{"Filter", one.Filter(func(int) bool { return false })},
		{"Delete", one.Delete(0, 1)},
		{"Replace", one.Replace(0, 1)},
		{"Insert(zero)", zero.Insert(0)},
		{"Sub", one.Sub(1, 1)},
		{"Reversed(zero)", zero.Reversed()},
		{"Sorted(zero)", immutable.Sorted(zero)},
		{"Compact(zero)", immutable.Compact(zero)},
		{"CompactFunc(zero)", zero.CompactFunc(func(a, b int) bool { return a == b })},
		{"Edit().Done()", zero.Edit().Done()},
		{"Done after deleting all", emptied.Done()},
		{"NewSliceBuilder(4).Done()", immutable.NewSliceBuilder[int](4).Done()},
	}

	for _, p := range producers {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, 0, p.got.Len(), "empty producer result has no elements")
			require.True(t, reflect.DeepEqual(zero, p.got),
				"%s: empty producer result must be DeepEqual to the zero value", p.name)
		})
	}
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
	require.Equal(t, []int{0, 1, 3}, collect(s.Insert(0, 0)))
	require.Equal(t, []int{1, 2, 3}, collect(s.Insert(1, 2)))
	require.Equal(t, []int{1, 3, 4, 5}, collect(s.Insert(2, 4, 5)), "at Len appends")
	require.Equal(t, []int{1, 3}, collect(s.Insert(1)), "no items is a no-op")
	require.Panics(t, func() { s.Insert(3, 9) })
	require.Panics(t, func() { s.Insert(-1, 9) })
	require.Equal(t, []int{1, 3}, collect(s), "the receiver is unchanged")
}

func TestSlice_Sub(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3, 4)
	require.Equal(t, []int{2, 3}, collect(s.Sub(1, 3)))
	require.Equal(t, []int{1, 2, 3, 4}, collect(s.Sub(0, 4)))
	require.Equal(t, 0, s.Sub(2, 2).Len(), "empty range")
	require.Equal(t, []int{1, 2, 3, 4}, collect(s), "the receiver is unchanged")
	require.Panics(t, func() { s.Sub(0, 5) })
	require.Panics(t, func() { s.Sub(3, 2) })
	require.Panics(t, func() { s.Sub(-1, 2) })

	// A Filter result carries spare capacity; Sub must still refuse to read past Len into it.
	spare := immutable.SliceOf(1, 2, 3, 4).Filter(func(v int) bool { return v != 4 })
	require.Equal(t, 3, spare.Len())
	require.Panics(t, func() { spare.Sub(0, 4) })
}

func TestSlice_ReversedAndSorted(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(3, 1, 2)
	require.Equal(t, []int{2, 1, 3}, collect(s.Reversed()))
	require.Equal(t, []int{1, 2, 3}, collect(immutable.Sorted(s)))
	require.Equal(t, []int{3, 2, 1}, collect(s.SortedFunc(func(a, b int) int { return b - a })))
	require.Equal(t, 0, immutable.Slice[int]{}.Reversed().Len())
	require.Equal(t, 0, immutable.Sorted(immutable.Slice[int]{}).Len())
	require.Equal(t, []int{3, 1, 2}, collect(s), "the receiver is unchanged")

	// Stable: equal keys keep their original order.
	byLen := func(a, b string) int { return len(a) - len(b) }
	got := collect(immutable.SliceOf("bb", "a", "cc", "b").SortedFunc(byLen))
	require.Equal(t, []string{"a", "b", "bb", "cc"}, got)
}

func TestSlice_TransformConcatCollectCompact(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3)
	require.Equal(t, []string{"1", "2", "3"}, collect(immutable.Transform(s, strconv.Itoa)))
	require.Equal(t, 0, immutable.Transform(immutable.Slice[int]{}, strconv.Itoa).Len())

	require.Equal(
		t,
		[]int{1, 2, 3, 4, 5},
		collect(immutable.Concat(s, immutable.Slice[int]{}, immutable.SliceOf(4, 5))),
	)
	require.Equal(t, 0, immutable.Concat[int]().Len())

	require.Equal(t, []int{1, 2, 3}, collect(immutable.Collect(s.Values())))
	require.Equal(t, 0, immutable.Collect(immutable.Slice[int]{}.Values()).Len())

	dup := immutable.SliceOf(1, 1, 2, 2, 2, 1)
	require.Equal(t, []int{1, 2, 1}, collect(immutable.Compact(dup)))
	require.Equal(t, []int{1, 1, 2, 2, 2, 1}, collect(dup), "the receiver is unchanged")
	require.Equal(t, 0, immutable.Compact(immutable.Slice[int]{}).Len())
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
	require.Equal(t, []int{1, 4}, collect(s.Delete(1, 3)))
	require.Equal(t, []int{1, 2, 3, 4}, collect(s.Delete(2, 2)), "empty range is a no-op")
	require.Equal(t, 0, s.Delete(0, 4).Len(), "deleting all leaves it empty")
	require.Panics(t, func() { s.Delete(3, 5) })
	require.Panics(t, func() { s.Delete(3, 2) })

	require.Equal(t, []int{1, 9, 9, 4}, collect(s.Replace(1, 3, 9, 9)))
	require.Equal(t, []int{1, 4}, collect(s.Replace(1, 3)), "replace with nothing deletes")
	require.Equal(t, 0, s.Replace(0, 4).Len(), "replacing all with nothing leaves it empty")
	require.Panics(t, func() { s.Replace(3, 2, 9) })

	require.Equal(t, []int{1, 2, 1, 2}, collect(immutable.SliceOf(1, 2).Repeat(2)))
	require.Equal(t, 0, s.Repeat(0).Len())
	require.Panics(t, func() { s.Repeat(-1) })

	var chunks [][]int
	for part := range s.Chunk(3) {
		chunks = append(chunks, collect(part))
	}
	require.Equal(t, [][]int{{1, 2, 3}, {4}}, chunks)
	require.Panics(t, func() { s.Chunk(0) })

	sameParity := func(a, b int) bool { return a%2 == b%2 }
	c := immutable.SliceOf(1, 1, 2, 2, 3)
	require.Equal(t, []int{1, 2, 3}, collect(c.CompactFunc(sameParity)))
	require.Equal(t, []int{1, 1, 2, 2, 3}, collect(c), "the receiver is unchanged")
	require.Equal(t, []int{1, 2, 3, 4}, collect(s), "the receiver is unchanged")
}

// TestSlice_DerivationsLeaveReceiverUnchanged pins, for every derivation at once, that the receiver
// keeps its elements. It runs each one on two bases: one with no spare capacity, and one whose array
// has room past Len, the case where an append-style implementation would write into the receiver's
// array without anyone noticing.
func TestSlice_DerivationsLeaveReceiverUnchanged(t *testing.T) {
	t.Parallel()

	notFive := func(v int) bool { return v != 5 }
	bases := map[string]func() immutable.Slice[int]{
		"exact":          func() immutable.Slice[int] { return immutable.SliceOf(1, 2, 3, 4) },
		"spare capacity": func() immutable.Slice[int] { return immutable.SliceOf(1, 2, 3, 4, 5).Filter(notFive) },
	}
	cases := []struct {
		name   string
		derive func(immutable.Slice[int]) immutable.Slice[int]
		want   []int
	}{
		{"Append", func(s immutable.Slice[int]) immutable.Slice[int] { return s.Append(9) }, []int{1, 2, 3, 4, 9}},
		{"Insert", func(s immutable.Slice[int]) immutable.Slice[int] { return s.Insert(1, 8) }, []int{1, 8, 2, 3, 4}},
		{
			"Insert at Len",
			func(s immutable.Slice[int]) immutable.Slice[int] { return s.Insert(4, 8) },
			[]int{1, 2, 3, 4, 8},
		},
		{
			"Replace",
			func(s immutable.Slice[int]) immutable.Slice[int] { return s.Replace(0, 1, 6, 6) },
			[]int{6, 6, 2, 3, 4},
		},
		{
			"Repeat",
			func(s immutable.Slice[int]) immutable.Slice[int] { return s.Repeat(2) },
			[]int{1, 2, 3, 4, 1, 2, 3, 4},
		},
		{"Sub", func(s immutable.Slice[int]) immutable.Slice[int] { return s.Sub(0, 1) }, []int{1}},
		{"With", func(s immutable.Slice[int]) immutable.Slice[int] { return s.With(0, 7) }, []int{7, 2, 3, 4}},
		{"Reversed", immutable.Slice[int].Reversed, []int{4, 3, 2, 1}},
		{"SortedFunc", func(s immutable.Slice[int]) immutable.Slice[int] {
			return s.SortedFunc(func(a, b int) int { return b - a })
		}, []int{4, 3, 2, 1}},
		{"Without", func(s immutable.Slice[int]) immutable.Slice[int] { return s.Without(0) }, []int{2, 3, 4}},
		{"Filter", func(s immutable.Slice[int]) immutable.Slice[int] {
			return s.Filter(func(v int) bool { return v == 4 })
		}, []int{4}},
		{"Delete", func(s immutable.Slice[int]) immutable.Slice[int] { return s.Delete(0, 1) }, []int{2, 3, 4}},
		{"CompactFunc", func(s immutable.Slice[int]) immutable.Slice[int] {
			return s.CompactFunc(func(int, int) bool { return true })
		}, []int{1}},
		{
			"Compact",
			func(s immutable.Slice[int]) immutable.Slice[int] { return immutable.Compact(s.With(1, 1)) },
			[]int{1, 3, 4},
		},
	}

	for baseName, base := range bases {
		for _, c := range cases {
			t.Run(baseName+"/"+c.name, func(t *testing.T) {
				t.Parallel()

				s := base()
				got := c.derive(s)
				require.Equal(t, c.want, collect(got), "result")
				require.Equal(t, []int{1, 2, 3, 4}, collect(s), "receiver after %s", c.name)

				// A later derivation from the result must not reach back into the receiver either,
				// which is what a shared window (Sub) would allow if anything wrote in place.
				if got.Len() > 0 {
					got.With(0, -1)
					require.Equal(t, []int{1, 2, 3, 4}, collect(s), "receiver after deriving from %s", c.name)
				}
			})
		}
	}
}

// -------------------------------------------------------------------------------------------------
// JSON
// -------------------------------------------------------------------------------------------------

// jsonLibs is every JSON library the engine decodes a Slice with: encoding/json in cardinal,
// goccy/go-json in physics2d's hand-written payloads. Both dispatch on the same method signatures,
// so every rule below has to hold identically under each.
var jsonLibs = []struct {
	name      string
	marshal   func(any) ([]byte, error)
	unmarshal func([]byte, any) error
}{
	{"encoding_json", json.Marshal, json.Unmarshal},
	{"goccy_go_json", gojson.Marshal, gojson.Unmarshal},
}

func TestSlice_JSONEncodesAsArray(t *testing.T) {
	t.Parallel()

	for _, lib := range jsonLibs {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()

			out, err := lib.marshal(immutable.SliceOf(point{1, 2}, point{3, 4}))
			require.NoError(t, err)
			require.JSONEq(t, `[{"X":1,"Y":2},{"X":3,"Y":4}]`, string(out))
		})
	}
}

func TestSlice_JSONZeroValueIsEmptyArray(t *testing.T) {
	t.Parallel()

	for _, lib := range jsonLibs {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()

			var s immutable.Slice[int]
			out, err := lib.marshal(s)
			require.NoError(t, err)
			require.Equal(t, "[]", string(out), "a Slice is never absent, only empty")
		})
	}
}

func TestSlice_JSONRoundTrips(t *testing.T) {
	t.Parallel()

	for _, lib := range jsonLibs {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()

			in := immutable.SliceOf("a", "", "c")
			out, err := lib.marshal(in)
			require.NoError(t, err)

			var back immutable.Slice[string]
			require.NoError(t, lib.unmarshal(out, &back))
			require.Equal(t, in, back)
		})
	}
}

// A decoded empty array or null must be the zero value, not an empty allocation, so a restored
// component compares equal to a fresh one — the same rule DecodeMsgpack follows.
func TestSlice_JSONEmptyDecodesToZeroValue(t *testing.T) {
	t.Parallel()

	for _, lib := range jsonLibs {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()

			for _, text := range []string{"[]", "null"} {
				var got immutable.Slice[point]
				require.NoError(t, lib.unmarshal([]byte(text), &got), text)
				require.Equal(t, immutable.Slice[point]{}, got, text)
				require.True(t, reflect.DeepEqual(immutable.Slice[point]{}, got), text)
			}
		})
	}
}

// Decoding into a Slice that already holds elements must leave the zero value too, not the old
// items: a field decoded from `[]` or null is empty however the target started.
func TestSlice_JSONEmptyClearsExistingItems(t *testing.T) {
	t.Parallel()

	type holder struct {
		Items immutable.Slice[point] `json:"items"`
	}

	for _, lib := range jsonLibs {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()

			for _, text := range []string{`{"items":[]}`, `{"items":null}`} {
				h := holder{Items: immutable.SliceOf(point{1, 2})}
				require.NoError(t, lib.unmarshal([]byte(text), &h), text)
				require.Equal(t, immutable.Slice[point]{}, h.Items, text)
				require.True(t, reflect.DeepEqual(immutable.Slice[point]{}, h.Items), text)
			}
		})
	}
}

func TestSlice_JSONRejectsNonArray(t *testing.T) {
	t.Parallel()

	for _, lib := range jsonLibs {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()

			var got immutable.Slice[int]
			require.Error(t, lib.unmarshal([]byte(`{"items":[1]}`), &got))
			require.Error(t, lib.unmarshal([]byte(`"nope"`), &got))
		})
	}
}

// The methods are what make a Slice FIELD usable — a bare Slice is dispatched to directly, but a
// field goes through the encoder's struct walk, which is where a library can disagree.
func TestSlice_JSONAsStructField(t *testing.T) {
	t.Parallel()

	type holder struct {
		Name  string                 `json:"name"`
		Items immutable.Slice[point] `json:"items"`
	}
	in := holder{Name: "h", Items: immutable.SliceOf(point{5, 6})}
	const want = `{"name":"h","items":[{"X":5,"Y":6}]}`

	for _, lib := range jsonLibs {
		t.Run(lib.name, func(t *testing.T) {
			t.Parallel()

			out, err := lib.marshal(in)
			require.NoError(t, err)
			require.JSONEq(t, want, string(out))

			var back holder
			require.NoError(t, lib.unmarshal(out, &back))
			require.Equal(t, in, back)
		})
	}
}
