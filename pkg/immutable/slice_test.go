package immutable_test

import (
	"encoding/json"
	"reflect"
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
		{"Clone(zero)", zero.Clone()},
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
		switch op := r.IntN(8); {
		case op == 0:
			v := r.IntN(100)
			s.Append(v)
			model = append(model, v)
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

// A Slice behaves like a Go slice inside one system: an assignment shares the array. Clone is the way
// to a copy that shares nothing, and the copy Cardinal makes on Get and Set.
func TestSlice_AssignmentSharesAndCloneDoesNot(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3)
	shared := s
	shared.Set(0, 9)
	require.Equal(t, 9, s.At(0), "an assignment shares the array, as with a Go slice")

	clone := s.Clone()
	clone.Set(0, 1)
	clone.Append(4)
	require.Equal(t, []int{9, 2, 3}, collect(s), "edits to a clone do not reach the original")
	require.Equal(t, []int{1, 2, 3, 4}, collect(clone))
}

func TestSlice_CloneCopiesNestedSlices(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(immutable.SliceOf(1, 2), immutable.SliceOf(3))
	clone := s.Clone()
	inner := clone.At(0)
	inner.Set(0, 9)
	require.Equal(t, 1, s.At(0).At(0), "an inner Slice of a clone has its own array")
	require.Equal(t, 9, clone.At(0).At(0), "the clone's inner Slice shares with the element At returned")
}

func TestSlice_Allocations(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not meaningful under -race")
	}
	s := immutable.SliceOf(1, 2, 3, 4, 5, 6, 7, 8)
	var sink int

	reads := testing.AllocsPerRun(100, func() {
		sink += s.Len() + s.At(3) + immutable.Index(s, 4)
		for _, v := range s.All() {
			sink += v
		}
	})
	require.Zero(t, reads, "reads never allocate")
	require.Zero(t, testing.AllocsPerRun(100, func() { s.Set(0, 9) }), "Set")
	require.InDelta(t, 1, testing.AllocsPerRun(100, func() { sink += s.Clone().Len() }), 0, "Clone of a flat Slice")

	// Append grows the array only when it is full, so a loop of 1024 appends allocates about log2(1024)
	// times, not 1024.
	loop := testing.AllocsPerRun(100, func() {
		var b immutable.Slice[int]
		for i := range 1024 {
			b.Append(i)
		}
		sink += b.Len()
	})
	require.LessOrEqual(t, loop, 20.0, "1024 appends")
	_ = sink
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
