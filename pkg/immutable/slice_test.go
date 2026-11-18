package immutable_test

import (
	"encoding/json"
	"math"
	"reflect"
	"runtime"
	"slices"
	"testing"

	gojson "github.com/goccy/go-json"

	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/testutils"
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
	for _, empty := range []immutable.Slice[int]{{}, emptied(immutable.SliceOf(1))} {
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

// emptied returns s after deleting every element.
func emptied[T any](s immutable.Slice[T]) immutable.Slice[T] {
	s.Delete(0, s.Len())
	return s
}

func TestSlice_ZeroValueIsEmpty(t *testing.T) {
	t.Parallel()

	var s immutable.Slice[int]
	require.Equal(t, 0, s.Len())
	require.Empty(t, collect(s))
	for range s.All() {
		t.Fatal("zero Slice must yield nothing")
	}
	s.Append(1)
	require.Equal(t, []int{1}, collect(s), "a zero Slice can be appended to")
}

func TestSliceOf_CopiesInput(t *testing.T) {
	t.Parallel()

	in := []int{1, 2, 3}
	s := immutable.SliceOf(in...)
	in[0] = 99

	require.Equal(t, 1, s.At(0), "changing the input after construction must not reach the Slice")
	require.Equal(t, []int{1, 2, 3}, collect(s))
}

func TestSlice_AtReturnsCopy(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(point{X: 1, Y: 2})
	p := s.At(0)
	p.X = 99

	require.Equal(t, 99, p.X)
	require.Equal(t, point{X: 1, Y: 2}, s.At(0), "editing the element At returned leaves the Slice")
	require.Panics(t, func() { s.At(1) })
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
}

func TestSlice_Search(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(4, 5, 6, 5)
	require.Equal(t, 1, immutable.Index(s, 5))
	require.Equal(t, -1, immutable.Index(s, 7))
	require.True(t, immutable.Contains(s, 6))
	require.False(t, immutable.Contains(s, 7))
	require.Equal(t, 2, s.IndexFunc(func(v int) bool { return v > 5 }))
	require.Equal(t, -1, s.IndexFunc(func(v int) bool { return v > 6 }))
	require.True(t, s.ContainsFunc(func(v int) bool { return v%2 == 0 }))
	require.False(t, s.ContainsFunc(func(v int) bool { return v > 6 }))
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

func TestSlice_Edits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		edit func(s *immutable.Slice[int])
		want []int
	}{
		{"Set", func(s *immutable.Slice[int]) { s.Set(1, 9) }, []int{3, 9, 2}},
		{"Append", func(s *immutable.Slice[int]) { s.Append(4, 5) }, []int{3, 1, 2, 4, 5}},
		{"Append nothing", func(s *immutable.Slice[int]) { s.Append() }, []int{3, 1, 2}},
		{"Insert front", func(s *immutable.Slice[int]) { s.Insert(0, 7, 8) }, []int{7, 8, 3, 1, 2}},
		{"Insert at end", func(s *immutable.Slice[int]) { s.Insert(3, 7) }, []int{3, 1, 2, 7}},
		{"Delete", func(s *immutable.Slice[int]) { s.Delete(0, 2) }, []int{2}},
		{"Delete nothing", func(s *immutable.Slice[int]) { s.Delete(1, 1) }, []int{3, 1, 2}},
		{"DeleteFunc", func(s *immutable.Slice[int]) { s.DeleteFunc(func(v int) bool { return v != 1 }) }, []int{1}},
		{"Reset", func(s *immutable.Slice[int]) { s.Reset() }, nil},
		{"Sort", func(s *immutable.Slice[int]) { immutable.Sort(s) }, []int{1, 2, 3}},
		{"SortStableFunc", func(s *immutable.Slice[int]) {
			s.SortStableFunc(func(a, b int) int { return b - a })
		}, []int{3, 2, 1}},
		{"Reverse", func(s *immutable.Slice[int]) { s.Reverse() }, []int{2, 1, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := immutable.SliceOf(3, 1, 2)
			tt.edit(&s)
			require.Equal(t, tt.want, collect(s))
		})
	}
}

func TestSlice_SortStableFuncKeepsEqualElementsInOrder(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(point{1, 0}, point{0, 1}, point{1, 2}, point{0, 3})
	s.SortStableFunc(func(a, b point) int { return a.X - b.X })
	require.Equal(t, []point{{0, 1}, {0, 3}, {1, 0}, {1, 2}}, collect(s))
}

func TestSlice_OutOfRangePanics(t *testing.T) {
	t.Parallel()

	tests := map[string]func(s *immutable.Slice[int]){
		"Set past end":    func(s *immutable.Slice[int]) { s.Set(3, 0) },
		"Set negative":    func(s *immutable.Slice[int]) { s.Set(-1, 0) },
		"Insert past end": func(s *immutable.Slice[int]) { s.Insert(4, 0) },
		"Delete past end": func(s *immutable.Slice[int]) { s.Delete(2, 4) },
		"Delete reversed": func(s *immutable.Slice[int]) { s.Delete(2, 1) },
	}
	for name, edit := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := immutable.SliceOf(1, 2, 3)
			require.Panics(t, func() { edit(&s) })
			require.Equal(t, []int{1, 2, 3}, collect(s), "a failed edit changes nothing")
		})
	}
}

// Every way of making an empty Slice must leave the zero value, not an empty-but-allocated Slice.
// That is the rule DecodeMsgpack and UnmarshalJSON both enforce, so a component built fresh compares
// equal to one restored from a snapshot under [reflect.DeepEqual].
func TestSlice_EmptyIsZeroValue(t *testing.T) {
	t.Parallel()

	zero := immutable.Slice[int]{}
	edited := func(edit func(s *immutable.Slice[int])) immutable.Slice[int] {
		s := immutable.SliceOf(1, 2)
		edit(&s)
		return s
	}

	tests := []struct {
		name string
		got  immutable.Slice[int]
	}{
		{"SliceOf()", immutable.SliceOf[int]()},
		{"SliceOf(empty literal)", immutable.SliceOf([]int{}...)},
		{"Collect(empty)", immutable.Collect(zero.Values())},
		{"Delete all", edited(func(s *immutable.Slice[int]) { s.Delete(0, 2) })},
		{"DeleteFunc all", edited(func(s *immutable.Slice[int]) { s.DeleteFunc(func(int) bool { return true }) })},
		{"Reset", edited(func(s *immutable.Slice[int]) { s.Reset() })},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, 0, tt.got.Len())
			require.True(t, reflect.DeepEqual(zero, tt.got), "%s must be DeepEqual to the zero value", tt.name)
		})
	}
}

// TestSlice_ModelMatchesPlainSlice runs random edits against a plain []int doing the same thing.
func TestSlice_ModelMatchesPlainSlice(t *testing.T) {
	t.Parallel()
	r := testutils.NewRand(t)

	var s immutable.Slice[int]
	var model []int
	for step := range 2000 {
		n := len(model)
		switch op := r.IntN(9); {
		case op == 0:
			v := r.IntN(100)
			s.Append(v)
			model = append(model, v)
		case op == 8:
			add := r.IntN(10)
			s.UpdateFunc(func(i, v int) int { return v + i%3 + add })
			for i := range model {
				model[i] += i%3 + add
			}
		case op == 1 && n > 0:
			i, v := r.IntN(n), r.IntN(100)
			s.Set(i, v)
			model[i] = v
		case op == 2:
			i, v := r.IntN(n+1), r.IntN(100)
			s.Insert(i, v)
			model = slices.Insert(model, i, v)
		case op == 3 && n > 0:
			i := r.IntN(n)
			j := i + r.IntN(n-i+1)
			s.Delete(i, j)
			model = slices.Delete(model, i, j)
		case op == 4:
			s.DeleteFunc(func(v int) bool { return v%3 == 0 })
			model = slices.DeleteFunc(model, func(v int) bool { return v%3 == 0 })
		case op == 5:
			immutable.Sort(&s)
			slices.Sort(model)
		case op == 6 && r.IntN(20) == 0:
			s.Reset()
			model = nil
		default:
			s.Reverse()
			slices.Reverse(model)
		}
		require.Equal(t, len(model), s.Len(), "step %d", step)
		require.True(t, slices.Equal(model, collect(s)), "step %d: want %v, got %v", step, model, collect(s))
	}
}

// An edit never reaches another copy, whether the copy was made by assignment or holds the edited
// Slice as an element.
func TestSlice_CopiesShareNothing(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3)
	cp := s
	cp.Set(0, 9)
	cp.Append(4)
	require.Equal(t, []int{1, 2, 3}, collect(s), "an edit through an assigned copy")
	require.Equal(t, []int{9, 2, 3, 4}, collect(cp))

	nested := immutable.SliceOf(immutable.SliceOf(1, 2), immutable.SliceOf(3))
	inner := nested.At(0)
	inner.Set(0, 9)
	require.Equal(t, 1, nested.At(0).At(0), "an edit to an element At returned")

	// Two copies that both Append keep their own elements, as Go slices sharing an array do not.
	a := immutable.SliceOf(1)
	b := a
	a.Append(2)
	b.Append(3)
	require.Equal(t, []int{1, 2}, collect(a))
	require.Equal(t, []int{1, 3}, collect(b))
}

func TestSlice_Allocations(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not meaningful under -race")
	}
	s := immutable.SliceOf(1, 2, 3, 4, 5, 6, 7, 8)
	big := immutable.Collect(func(yield func(int) bool) {
		for i := range 5000 {
			if !yield(i) {
				return
			}
		}
	})
	var sink int

	reads := testing.AllocsPerRun(100, func() {
		sink += s.Len() + s.At(3) + immutable.Index(s, 4) + big.At(4000)
		for _, v := range s.All() {
			sink += v
		}
		for i, v := range big.Range(2990, 3100) {
			sink += i + v
		}
	})
	require.Zero(t, reads, "reads never allocate")
	require.Zero(t, testing.AllocsPerRun(100, func() { d := s; sink += d.Len() }), "a copy")
	require.InDelta(t, 1, testing.AllocsPerRun(100, func() { d := s; d.Set(0, 9) }), 0, "Set: one chunk")
	require.InDelta(t, 3, testing.AllocsPerRun(100, func() { d := big; d.Set(1000, 9) }), 0,
		"Set in a 5000-element Slice: the chunk and the two nodes above it")
	require.InDelta(t, 1, testing.AllocsPerRun(100, func() { d := big; d.Append(9) }), 0, "Append: the last chunk")
	require.LessOrEqual(t, testing.AllocsPerRun(20, func() {
		d := big
		d.UpdateFunc(func(_, v int) int { return v + 1 })
	}), 5000.0/32+12, "UpdateFunc: one array, a node per 32 elements, the branches and their lists")

	// Each Append copies the last chunk, at most 32 elements, so a loop of appends is linear.
	loop := testing.AllocsPerRun(100, func() {
		var b immutable.Slice[int]
		for i := range 1024 {
			b.Append(i)
		}
		sink += b.Len()
	})
	require.LessOrEqual(t, loop, 1100.0, "1024 appends")
	_ = sink
}

// Slice copies the engine type's name and a field of its layout but lives in this package, so
// SliceElem must refuse it.
//
//nolint:unused // the field is never read on purpose; only the layout matters
type Slice[T any] struct{ tail []T }

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

func TestSlice_UpdateFunc(t *testing.T) {
	t.Parallel()

	for _, n := range []int{0, 1, 31, 32, 33, 64, 65, 1000, 1057} {
		items := make([]int, n)
		for i := range items {
			items[i] = i * 10
		}
		s := immutable.SliceOf(items...)
		before := s

		var seen []int
		s.UpdateFunc(func(i, v int) int {
			seen = append(seen, i)
			return v + 1
		})
		want := make([]int, n)
		for i := range want {
			want[i] = i*10 + 1
		}
		require.Equal(t, want, append([]int{}, collect(s)...), "n=%d: every element updated", n)
		require.Equal(t, items, append([]int{}, collect(before)...), "n=%d: the copy is unchanged", n)
		require.Len(t, seen, n, "n=%d: f runs once per element", n)
		require.True(t, slices.IsSorted(seen), "n=%d: f gets each index in order", n)
	}

	s := immutable.SliceOf(1, 2, 3)
	require.Panics(t, func() {
		s.UpdateFunc(func(i, v int) int {
			if i == 2 {
				panic("boom")
			}
			return v * 10
		})
	})
	require.Equal(t, []int{1, 2, 3}, collect(s), "a panic in f leaves the Slice unchanged")
}

// Range must match ranging over s[i:j] with each index offset by i, whether the range sits in the
// tail, in one leaf, or crosses leaves and levels of the tree.
func TestSlice_Range(t *testing.T) {
	t.Parallel()
	r := testutils.NewRand(t)

	type pair struct {
		I, V int
	}
	for _, n := range []int{0, 1, 31, 32, 33, 64, 65, 100, 1024, 1057, 33*32 + 5} {
		items := make([]int, n)
		for i := range items {
			items[i] = i * 7
		}
		s := immutable.SliceOf(items...)
		check := func(i, j int) {
			var got []pair
			for x, v := range s.Range(i, j) {
				got = append(got, pair{x, v})
			}
			var want []pair
			for x, v := range items[i:j] {
				want = append(want, pair{i + x, v})
			}
			require.Equal(t, want, got, "n=%d: Range(%d, %d)", n, i, j)
		}
		if n <= 100 {
			for i := 0; i <= n; i++ {
				for j := i; j <= n; j++ {
					check(i, j)
				}
			}
		} else {
			for range 300 {
				i := r.IntN(n + 1)
				check(i, i+r.IntN(n-i+1))
			}
			check(0, n)
			check(n-1, n)
		}
	}

	s := immutable.SliceOf(1, 2, 3, 4)
	var got []int
	for _, v := range s.Range(1, 4) {
		got = append(got, v)
		if v == 3 {
			break
		}
	}
	require.Equal(t, []int{2, 3}, got, "the loop stops at break")

	seq := s.Range(0, 2)
	s.Set(0, 9)
	require.Equal(t, []int{1, 2}, slices.Collect(func(yield func(int) bool) {
		for _, v := range seq {
			if !yield(v) {
				return
			}
		}
	}), "the iterator reads the Slice as it was when Range was called, and can run twice")

	for _, bad := range [][2]int{{-1, 2}, {0, 5}, {3, 2}} {
		require.Panics(t, func() {
			for range s.Range(bad[0], bad[1]) {
			}
		}, "Range(%d, %d)", bad[0], bad[1])
	}
}

// Decoding writes each element into its place in the array, so it allocates per array growth, and a
// forged length that promises more elements than arrive fails without a large allocation.
func TestSlice_DecodeAllocations(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not meaningful under -race")
	}
	items := make([]point, 256)
	for i := range items {
		items[i] = point{i, -i}
	}
	data, err := msgpack.Marshal(immutable.SliceOf(items...))
	require.NoError(t, err)
	allocs := testing.AllocsPerRun(20, func() {
		var s immutable.Slice[point]
		require.NoError(t, msgpack.Unmarshal(data, &s))
	})
	require.Less(t, allocs, 40.0, "decoding 256 elements")

	forged := []byte{0xdd, 0x7f, 0xff, 0xff, 0xff, 0x01} // an array of 2^31-1 elements, holding one
	var s immutable.Slice[int]
	allocated := allocatedBytes(func() { err = msgpack.Unmarshal(forged, &s) })
	require.Error(t, err)
	require.Less(t, allocated, uint64(1<<20), "a forged length must not size the allocation")
}

// allocatedBytes returns how many bytes f allocates. Callers must not run in parallel, or other
// tests' allocations count too.
func allocatedBytes(f func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// Shared storage counts as equal without calling eq, so comparing with a copy costs only what the
// edits since the copy touched.
func TestSlice_EqualSkipsSharedStorage(t *testing.T) {
	t.Parallel()

	items := make([]int, 5000)
	for i := range items {
		items[i] = i
	}
	big := immutable.SliceOf(items...)
	calls := 0
	eq := func(a, b int) bool {
		calls++
		return a == b
	}
	equal := func(a, b immutable.Slice[int]) (bool, int) {
		calls = 0
		return a.EqualFunc(b, eq), calls
	}

	cp := big
	ok, n := equal(big, cp)
	require.True(t, ok)
	require.Zero(t, n, "an unchanged copy shares everything")

	cp.Set(1000, 1000) // same value, new chunk
	ok, n = equal(big, cp)
	require.True(t, ok)
	require.Equal(t, 32, n, "only the edited chunk is compared")

	cp.Set(4999, -1) // in the tail
	ok, _ = equal(big, cp)
	require.False(t, ok, "a change in the tail is found")
	cp.Set(4999, 4999)
	ok, n = equal(big, cp)
	require.True(t, ok)
	require.Equal(t, 32+5000%32, n, "the edited chunk and the tail")

	ok, n = equal(big, immutable.SliceOf(items...))
	require.True(t, ok)
	require.Equal(t, 5000, n, "Slices built separately share nothing, so every element is compared")

	edited := big
	edited.Set(10, -1)
	require.False(t, big.EqualFunc(edited, eq))
	require.False(t, immutable.Equal(big, edited))

	// As with reflect.DeepEqual, a NaN in shared storage compares equal; built separately, it does not.
	nan := immutable.SliceOf(math.NaN())
	nanCopy := nan
	require.True(t, immutable.Equal(nan, nanCopy))
	require.False(t, immutable.Equal(nan, immutable.SliceOf(math.NaN())))
}

// Go may give separately allocated zero-size elements the same address, so that must not count as
// shared storage: eq still runs for Slices built separately.
func TestSlice_EqualFuncCallsEqForZeroSizeElements(t *testing.T) {
	t.Parallel()

	for _, n := range []int{1, 5, 100} {
		a := immutable.SliceOf(make([]struct{}, n)...)
		b := immutable.SliceOf(make([]struct{}, n)...)
		calls := 0
		require.True(t, a.EqualFunc(b, func(struct{}, struct{}) bool {
			calls++
			return true
		}))
		require.Equal(t, n, calls, "n=%d: every element compared", n)
		require.False(t, a.EqualFunc(b, func(struct{}, struct{}) bool { return false }), "n=%d", n)
	}
}
