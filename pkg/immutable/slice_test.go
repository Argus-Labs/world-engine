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
)

type point struct {
	X, Y int
}

// collect copies a Slice's elements out to a plain []T for comparison. Slice has no Clone method —
// derivations write through, so a test that needs a stable snapshot copies via Values, same as any
// other caller would.
func collect[T any](s immutable.Slice[T]) []T {
	return slices.Collect(s.Values())
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
	require.Equal(t, []int{1, 42, 3}, collect(s), "With writes through to the receiver")
	require.Panics(t, func() { immutable.SliceOf(1, 2, 3).With(3, 0) })
}

func TestSlice_WithoutDropsOneElement(t *testing.T) {
	t.Parallel()

	// Each case starts from a fresh receiver: Without writes through, so reusing one would feed the
	// next case a shrunken list.
	fresh := func() immutable.Slice[int] { return immutable.SliceOf(1, 2, 3) }
	require.Equal(t, []int{2, 3}, collect(fresh().Without(0)))
	require.Equal(t, []int{1, 3}, collect(fresh().Without(1)))
	require.Equal(t, []int{1, 2}, collect(fresh().Without(2)))
	require.Equal(t, 0, immutable.SliceOf(1).Without(0).Len(), "dropping the last leaves it empty")
	require.Panics(t, func() { fresh().Without(3) })

	s := fresh()
	s.Without(0)
	require.Equal(t, []int{2, 3, 0}, collect(s),
		"Without writes through: the receiver keeps its length over a zero-filled tail")
}

func TestSlice_FilterKeepsMatches(t *testing.T) {
	t.Parallel()

	// Each case starts from a fresh receiver: Filter writes through, so reusing one would feed the
	// next case a shrunken list.
	fresh := func() immutable.Slice[int] { return immutable.SliceOf(1, 2, 3, 4) }
	even := func(v int) bool { return v%2 == 0 }

	require.Equal(t, []int{2, 4}, collect(fresh().Filter(even)))
	require.Equal(t, 0, fresh().Filter(func(int) bool { return false }).Len(), "dropping all leaves it empty")
	require.Equal(t, []int{1, 2, 3, 4}, collect(fresh().Filter(func(int) bool { return true })),
		"keeping all keeps order")

	s := fresh()
	s.Filter(even)
	require.Equal(t, []int{2, 4, 0, 0}, collect(s),
		"Filter writes through: the receiver keeps its length over a zero-filled tail")

	calls := 0
	fresh().Filter(func(v int) bool { calls++; return v != 2 })
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

// Empty Slices are not interchangeable under reflect.DeepEqual, and that is the deliberate trade:
// a derivation that empties one leaves its backing array allocated, while the zero value has none.
// Equal compares elements and agrees with slices.Equal that nil and empty are the same list, so that
// is what comparisons go through.
//
// The case this used to protect — a component restored from a snapshot versus one built fresh — is
// handled on the decode side instead: the generated FromProto leaves an empty repeated field as the
// zero value rather than building an allocated empty.
func TestSlice_EmptyComparison(t *testing.T) {
	t.Parallel()

	zero := immutable.Slice[int]{}
	built := immutable.SliceOf[int]()
	derived := immutable.SliceOf(1, 2, 3).Filter(func(int) bool { return false })

	require.Equal(t, 0, derived.Len())
	require.True(t, immutable.Equal(zero, built), "Equal compares elements")
	require.True(t, immutable.Equal(zero, derived), "Equal compares elements")
	require.True(t, zero.EqualFunc(derived, func(a, b int) bool { return a == b }))

	require.NotEqual(t, zero, derived, "use Equal, not require.Equal, on a possibly-empty Slice")
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

	// Insert writes through when the receiver has the spare capacity to hold the result and
	// allocates otherwise, so every case starts from a fresh receiver rather than reusing one.
	fresh := func() immutable.Slice[int] { return immutable.SliceOf(1, 3) }
	require.Equal(t, []int{0, 1, 3}, collect(fresh().Insert(0, 0)))
	require.Equal(t, []int{1, 2, 3}, collect(fresh().Insert(1, 2)))
	require.Equal(t, []int{1, 3, 4, 5}, collect(fresh().Insert(2, 4, 5)), "at Len appends")
	require.Equal(t, []int{1, 3}, collect(fresh().Insert(1)), "no items is a no-op")
	require.Panics(t, func() { fresh().Insert(3, 9) })
	require.Panics(t, func() { fresh().Insert(-1, 9) })
}

func TestSlice_Sub(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3, 4)
	require.Equal(t, []int{2, 3}, collect(s.Sub(1, 3)))
	require.Equal(t, []int{1, 2, 3, 4}, collect(s.Sub(0, 4)))
	require.Equal(t, 0, s.Sub(2, 2).Len(), "empty range")
	require.Equal(t, []int{1, 2, 3, 4}, collect(s), "Sub is the one derivation that writes nothing")
	require.Panics(t, func() { s.Sub(0, 5) })
	require.Panics(t, func() { s.Sub(3, 2) })
	require.Panics(t, func() { s.Sub(-1, 2) })

	// A Filter result carries spare capacity; Sub must still refuse to read past Len into it. It
	// derives from its own slice because Filter writes through.
	spare := immutable.SliceOf(1, 2, 3, 4).Filter(func(v int) bool { return v != 4 })
	require.Equal(t, 3, spare.Len())
	require.Panics(t, func() { spare.Sub(0, 4) })
}

func TestSlice_ReversedAndSorted(t *testing.T) {
	t.Parallel()

	// Both reorder in place, so every case starts from a fresh receiver.
	fresh := func() immutable.Slice[int] { return immutable.SliceOf(3, 1, 2) }
	require.Equal(t, []int{2, 1, 3}, collect(fresh().Reversed()))
	require.Equal(t, []int{1, 2, 3}, collect(immutable.Sorted(fresh())))
	require.Equal(t, []int{3, 2, 1}, collect(fresh().SortedFunc(func(a, b int) int { return b - a })))
	require.Equal(t, 0, immutable.Slice[int]{}.Reversed().Len())
	require.Equal(t, 0, immutable.Sorted(immutable.Slice[int]{}).Len())

	s := fresh()
	s.Reversed()
	require.Equal(t, []int{2, 1, 3}, collect(s), "Reversed writes through to the receiver")

	s = fresh()
	immutable.Sorted(s)
	require.Equal(t, []int{1, 2, 3}, collect(s), "Sorted writes through to the receiver")

	// Stable: equal keys keep their original order.
	byLen := func(a, b string) int { return len(a) - len(b) }
	got := collect(immutable.SliceOf("bb", "a", "cc", "b").SortedFunc(byLen))
	require.Equal(t, []string{"a", "b", "bb", "cc"}, got)
}

func TestSlice_MapConcatCollectCompact(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3)
	require.Equal(t, []string{"1", "2", "3"}, collect(immutable.Map(s, strconv.Itoa)))
	require.Equal(t, 0, immutable.Map(immutable.Slice[int]{}, strconv.Itoa).Len())

	require.Equal(t, []int{1, 2, 3, 4, 5}, collect(immutable.Concat(s, immutable.Slice[int]{}, immutable.SliceOf(4, 5))))
	require.Equal(t, 0, immutable.Concat[int]().Len())

	require.Equal(t, []int{1, 2, 3}, collect(immutable.Collect(s.Values())))
	require.Equal(t, 0, immutable.Collect(immutable.Slice[int]{}.Values()).Len())

	dup := immutable.SliceOf(1, 1, 2, 2, 2, 1)
	require.Equal(t, []int{1, 2, 1}, collect(immutable.Compact(dup)))
	require.Equal(t, []int{1, 2, 1, 0, 0, 0}, collect(dup),
		"Compact writes through: the receiver keeps its length over a zero-filled tail")
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

	// Delete, Replace and CompactFunc all reach the receiver, so every case starts from a fresh one.
	fresh := func() immutable.Slice[int] { return immutable.SliceOf(1, 2, 3, 4) }
	require.Equal(t, []int{1, 4}, collect(fresh().Delete(1, 3)))
	require.Equal(t, []int{1, 2, 3, 4}, collect(fresh().Delete(2, 2)), "empty range is a no-op")
	require.Equal(t, 0, fresh().Delete(0, 4).Len(), "deleting all leaves it empty")
	require.Panics(t, func() { fresh().Delete(3, 5) })

	s := fresh()
	s.Delete(1, 3)
	require.Equal(t, []int{1, 4, 0, 0}, collect(s),
		"Delete writes through: the receiver keeps its length over a zero-filled tail")

	require.Equal(t, []int{1, 9, 9, 4}, collect(fresh().Replace(1, 3, 9, 9)))
	require.Equal(t, []int{1, 4}, collect(fresh().Replace(1, 3)), "replace with nothing deletes")
	require.Equal(t, 0, fresh().Replace(0, 4).Len(), "replacing all with nothing leaves it empty")

	require.Equal(t, []int{1, 2, 1, 2}, collect(immutable.SliceOf(1, 2).Repeat(2)))
	require.Equal(t, 0, fresh().Repeat(0).Len())
	require.Panics(t, func() { fresh().Repeat(-1) })

	var chunks [][]int
	for part := range fresh().Chunk(3) {
		chunks = append(chunks, collect(part))
	}
	require.Equal(t, [][]int{{1, 2, 3}, {4}}, chunks)
	require.Panics(t, func() { fresh().Chunk(0) })

	sameParity := func(a, b int) bool { return a%2 == b%2 }
	require.Equal(t, []int{1, 2, 3}, collect(immutable.SliceOf(1, 1, 2, 2, 3).CompactFunc(sameParity)))

	c := immutable.SliceOf(1, 1, 2, 2, 3)
	c.CompactFunc(sameParity)
	require.Equal(t, []int{1, 2, 3, 0, 0}, collect(c),
		"CompactFunc writes through: the receiver keeps its length over a zero-filled tail")
}

// TestSlice_DerivationsWriteThrough pins, for every derivation at once, whether it reaches the
// receiver's backing array. Review removed the copy from each one that can finish in place, so this
// table is the contract callers work against rather than an implementation detail: derive, then Set,
// and copy the elements out first (immutable.Collect) when the original has to survive.
//
// The base has length 4 and no spare capacity, so the ops that grow past it are forced to allocate
// and leave the receiver alone. Give one spare room and Insert and Replace write through too, which
// is why their docs promise nothing either way.
func TestSlice_DerivationsWriteThrough(t *testing.T) {
	t.Parallel()

	base := func() immutable.Slice[int] { return immutable.SliceOf(1, 2, 3, 4) }

	cases := []struct {
		name     string
		derive   func(immutable.Slice[int]) immutable.Slice[int]
		want     []int
		receiver []int // the receiver as it stands after the derivation
	}{
		// Allocate a new array, so the receiver survives.
		{"Append", func(s immutable.Slice[int]) immutable.Slice[int] { return s.Append(9) },
			[]int{1, 2, 3, 4, 9}, []int{1, 2, 3, 4}},
		{"Insert", func(s immutable.Slice[int]) immutable.Slice[int] { return s.Insert(1, 8) },
			[]int{1, 8, 2, 3, 4}, []int{1, 2, 3, 4}},
		{"Replace", func(s immutable.Slice[int]) immutable.Slice[int] { return s.Replace(0, 1, 6, 6) },
			[]int{6, 6, 2, 3, 4}, []int{1, 2, 3, 4}},
		{"Repeat", func(s immutable.Slice[int]) immutable.Slice[int] { return s.Repeat(2) },
			[]int{1, 2, 3, 4, 1, 2, 3, 4}, []int{1, 2, 3, 4}},

		// Read-only window: writes nothing, but shares the array it reads.
		{"Sub", func(s immutable.Slice[int]) immutable.Slice[int] { return s.Sub(0, 1) },
			[]int{1}, []int{1, 2, 3, 4}},

		// Reorder in place.
		{"With", func(s immutable.Slice[int]) immutable.Slice[int] { return s.With(0, 7) },
			[]int{7, 2, 3, 4}, []int{7, 2, 3, 4}},
		{"Reversed", immutable.Slice[int].Reversed,
			[]int{4, 3, 2, 1}, []int{4, 3, 2, 1}},
		{"SortedFunc", func(s immutable.Slice[int]) immutable.Slice[int] {
			return s.SortedFunc(func(a, b int) int { return b - a })
		}, []int{4, 3, 2, 1}, []int{4, 3, 2, 1}},

		// Shrink in place, leaving the receiver long over a zero-filled tail.
		{"Without", func(s immutable.Slice[int]) immutable.Slice[int] { return s.Without(0) },
			[]int{2, 3, 4}, []int{2, 3, 4, 0}},
		{"Filter", func(s immutable.Slice[int]) immutable.Slice[int] {
			return s.Filter(func(v int) bool { return v == 4 })
		}, []int{4}, []int{4, 0, 0, 0}},
		{"Delete", func(s immutable.Slice[int]) immutable.Slice[int] { return s.Delete(0, 1) },
			[]int{2, 3, 4}, []int{2, 3, 4, 0}},
		{"CompactFunc", func(s immutable.Slice[int]) immutable.Slice[int] {
			return s.CompactFunc(func(int, int) bool { return true })
		}, []int{1}, []int{1, 0, 0, 0}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			s := base()
			require.Equal(t, c.want, collect(c.derive(s)), "result")
			require.Equal(t, c.receiver, collect(s), "receiver after %s", c.name)
		})
	}
}

// -------------------------------------------------------------------------------------------------
// JSON
// -------------------------------------------------------------------------------------------------

func TestSlice_JSONEncodesAsArray(t *testing.T) {
	t.Parallel()

	out, err := json.Marshal(immutable.SliceOf(point{1, 2}, point{3, 4}))
	require.NoError(t, err)
	require.JSONEq(t, `[{"X":1,"Y":2},{"X":3,"Y":4}]`, string(out))
}

func TestSlice_JSONZeroValueIsEmptyArray(t *testing.T) {
	t.Parallel()

	var s immutable.Slice[int]
	out, err := json.Marshal(s)
	require.NoError(t, err)
	require.Equal(t, "[]", string(out), "a Slice is never absent, only empty")
}

func TestSlice_JSONRoundTrips(t *testing.T) {
	t.Parallel()

	in := immutable.SliceOf("a", "", "c")
	out, err := json.Marshal(in)
	require.NoError(t, err)

	var back immutable.Slice[string]
	require.NoError(t, json.Unmarshal(out, &back))
	require.Equal(t, in, back)
}

// A decoded empty array or null must be the zero value, not an empty allocation, so a restored
// component compares equal to a fresh one — the same rule the generated FromProto follows.
func TestSlice_JSONEmptyDecodesToZeroValue(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"[]", "null"} {
		var got immutable.Slice[point]
		require.NoError(t, json.Unmarshal([]byte(text), &got))
		require.Equal(t, immutable.Slice[point]{}, got, text)
		require.True(t, reflect.DeepEqual(immutable.Slice[point]{}, got), text)
	}
}

func TestSlice_JSONRejectsNonArray(t *testing.T) {
	t.Parallel()

	var got immutable.Slice[int]
	require.Error(t, json.Unmarshal([]byte(`{"items":[1]}`), &got))
	require.Error(t, json.Unmarshal([]byte(`"nope"`), &got))
}

// The methods are what make a Slice FIELD usable, and both JSON libraries the engine uses must
// see them: encoding/json in cardinal, goccy/go-json in physics2d's hand-written payloads.
func TestSlice_JSONAsStructFieldUnderBothEncoders(t *testing.T) {
	t.Parallel()

	type holder struct {
		Name  string                 `json:"name"`
		Items immutable.Slice[point] `json:"items"`
	}
	in := holder{Name: "h", Items: immutable.SliceOf(point{5, 6})}
	const want = `{"name":"h","items":[{"X":5,"Y":6}]}`

	std, err := json.Marshal(in)
	require.NoError(t, err)
	require.JSONEq(t, want, string(std))
	var backStd holder
	require.NoError(t, json.Unmarshal(std, &backStd))
	require.Equal(t, in, backStd)

	gc, err := gojson.Marshal(in)
	require.NoError(t, err)
	require.JSONEq(t, want, string(gc))
	var backGc holder
	require.NoError(t, gojson.Unmarshal(gc, &backGc))
	require.Equal(t, in, backGc)
}
