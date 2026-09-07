package immutable_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/goccy/go-json"
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

func TestSlice_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(point{X: 1, Y: 2}, point{X: 3, Y: 4})

	data, err := json.Marshal(s)
	require.NoError(t, err)
	require.JSONEq(t, `[{"X":1,"Y":2},{"X":3,"Y":4}]`, string(data))

	var back immutable.Slice[point]
	require.NoError(t, json.Unmarshal(data, &back))
	require.Equal(t, s.Clone(), back.Clone())
}

func TestSlice_JSONEmptyAndNull(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(immutable.Slice[int]{})
	require.NoError(t, err)
	require.JSONEq(t, `[]`, string(data), "empty encodes as [] not null")

	var fromNull immutable.Slice[int]
	require.NoError(t, json.Unmarshal([]byte(`null`), &fromNull))
	require.Equal(t, 0, fromNull.Len())

	var fromEmpty immutable.Slice[int]
	require.NoError(t, json.Unmarshal([]byte(`[]`), &fromEmpty))
	require.Equal(t, 0, fromEmpty.Len())
}

// Every empty Slice must be the same value however it was made: reflect.DeepEqual, and so
// require.Equal on a whole component, must not tell a decoded [] or null apart from SliceOf().
func TestSlice_EmptyIsOneValue(t *testing.T) {
	t.Parallel()

	var fromEmpty, fromNull immutable.Slice[int]
	require.NoError(t, json.Unmarshal([]byte(`[]`), &fromEmpty))
	require.NoError(t, json.Unmarshal([]byte(`null`), &fromNull))

	empty := immutable.SliceOf[int]()
	require.Equal(t, immutable.Slice[int]{}, empty, "zero value")
	require.Equal(t, empty, fromEmpty, "decoded []")
	require.Equal(t, empty, fromNull, "decoded null")
	require.Equal(t, empty, empty.Append(), "empty append")
}

// A Slice is a struct, so omitempty does not omit it. Pinned here because migrating a []T field
// tagged omitempty to a Slice changes the encoded shape from absent to [].
func TestSlice_JSONInsideStructIgnoresOmitempty(t *testing.T) {
	t.Parallel()

	type holder struct {
		Points immutable.Slice[point] `json:"points,omitempty"`
	}

	data, err := json.Marshal(holder{})
	require.NoError(t, err)
	require.JSONEq(t, `{"points":[]}`, string(data))

	var back holder
	require.NoError(t, json.Unmarshal([]byte(`{"points":[{"X":5,"Y":6}]}`), &back))
	require.Equal(t, []point{{X: 5, Y: 6}}, back.Points.Clone())
}
