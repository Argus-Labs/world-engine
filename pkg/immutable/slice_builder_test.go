package immutable_test

import (
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/require"
)

func TestSliceBuilder_EditLeavesSourceUnchanged(t *testing.T) {
	t.Parallel()

	s := immutable.SliceOf(1, 2, 3)
	b := s.Edit()
	b.Set(0, 9)
	b.Append(4, 5)
	b.Insert(1, 7)
	b.Delete(4, 5)
	require.Equal(t, 5, b.Len())
	require.Equal(t, 7, b.At(1))

	got := b.Done()
	require.Equal(t, []int{9, 7, 2, 3, 5}, collect(got))
	require.Equal(t, []int{1, 2, 3}, collect(s), "the source is unchanged")
}

// Each write takes its own path to the builder's first copy, so each must leave the source alone
// when it comes first.
func TestSliceBuilder_FirstWriteCopies(t *testing.T) {
	t.Parallel()

	first := map[string]struct {
		write func(*immutable.SliceBuilder[int])
		want  []int
	}{
		"Set":    {func(b *immutable.SliceBuilder[int]) { b.Set(0, 9) }, []int{9, 2, 3}},
		"Append": {func(b *immutable.SliceBuilder[int]) { b.Append(4) }, []int{1, 2, 3, 4}},
		"Insert": {func(b *immutable.SliceBuilder[int]) { b.Insert(1, 7) }, []int{1, 7, 2, 3}},
		"Delete": {func(b *immutable.SliceBuilder[int]) { b.Delete(0, 1) }, []int{2, 3}},
	}
	for name, c := range first {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			s := immutable.SliceOf(1, 2, 3)
			b := s.Edit()
			c.write(b)
			b.Set(0, 8) // a later write lands in the builder's copy too
			want := slices.Clone(c.want)
			want[0] = 8
			require.Equal(t, want, collect(b.Done()))
			require.Equal(t, []int{1, 2, 3}, collect(s), "the source is unchanged")
		})
	}

	s := immutable.SliceOf(1, 2, 3)
	require.Panics(t, func() { s.Edit().Delete(2, 1) }, "an inverted range panics before any copy")
	require.Equal(t, []int{1, 2, 3}, collect(s))
}

func TestSliceBuilder_NewBuildsFromScratch(t *testing.T) {
	t.Parallel()

	b := immutable.NewSliceBuilder[int](2)
	for i := range 5 {
		b.Append(i)
	}
	require.Equal(t, []int{0, 1, 2, 3, 4}, collect(b.Done()), "growing past the capacity hint is fine")
	require.Panics(t, func() { immutable.NewSliceBuilder[int](-1) })
}

func TestSliceBuilder_UseAfterDonePanics(t *testing.T) {
	t.Parallel()

	uses := map[string]func(*immutable.SliceBuilder[int]){
		"Len":    func(b *immutable.SliceBuilder[int]) { b.Len() },
		"At":     func(b *immutable.SliceBuilder[int]) { b.At(0) },
		"Set":    func(b *immutable.SliceBuilder[int]) { b.Set(0, 1) },
		"Append": func(b *immutable.SliceBuilder[int]) { b.Append(1) },
		"Insert": func(b *immutable.SliceBuilder[int]) { b.Insert(0, 1) },
		"Delete": func(b *immutable.SliceBuilder[int]) { b.Delete(0, 1) },
		"Done":   func(b *immutable.SliceBuilder[int]) { b.Done() },
	}
	for name, use := range uses {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			b := immutable.SliceOf(1, 2).Edit()
			done := b.Done()
			require.PanicsWithValue(t, "immutable: SliceBuilder used after Done", func() { use(b) })
			require.Equal(t, []int{1, 2}, collect(done), "the built Slice is untouched")
		})
	}
}

func TestSliceBuilder_CopyPanics(t *testing.T) {
	t.Parallel()

	b := immutable.SliceOf(1, 2).Edit()
	c := *b
	done := b.Done()
	require.PanicsWithValue(t,
		"immutable: SliceBuilder copied by value; use the pointer Edit or NewSliceBuilder returned",
		func() { c.Set(0, 9) })
	require.Equal(t, []int{1, 2}, collect(done), "a copy cannot write into the built Slice")

	var zero immutable.SliceBuilder[int]
	zero.Append(1)
	require.Equal(t, []int{1}, collect(zero.Done()), "a zero SliceBuilder works in place")
}

func TestSliceBuilder_OutOfRangePanics(t *testing.T) {
	t.Parallel()

	b := immutable.SliceOf(1, 2).Edit()
	require.Panics(t, func() { b.At(2) })
	require.Panics(t, func() { b.Set(2, 0) })
	require.Panics(t, func() { b.Insert(3, 0) })
	require.Panics(t, func() { b.Delete(1, 3) })
	require.Equal(t, []int{1, 2}, collect(b.Done()), "failed edits change nothing")

	// A write after a recovered panic must still copy first. The source has spare capacity, which a
	// Delete range check against capacity alone would let through.
	for name, bad := range map[string]func(*immutable.SliceBuilder[int]){
		"insert": func(b *immutable.SliceBuilder[int]) { b.Insert(3, 0) },
		"delete": func(b *immutable.SliceBuilder[int]) { b.Delete(1, 4) },
	} {
		nb := immutable.NewSliceBuilder[int](4)
		nb.Append(1, 2)
		src := nb.Done()
		eb := src.Edit()
		require.Panics(t, func() { bad(eb) }, name)
		eb.Set(0, 9)
		require.Equal(t, []int{1, 2}, collect(src), "%s: the source is unchanged", name)
		require.Equal(t, []int{9, 2}, collect(eb.Done()), name)
	}
}

// TestSlice_ModelMatchesPlainSlice runs random edits, through derivations and builders alike, against
// a plain []int doing the same thing. It also keeps every value it produced and checks after each
// step that none of them changed, which is the no-hidden-writes guarantee: a value, once made, never
// changes, whatever is later derived from it or from the arrays it shares.
func TestSlice_ModelMatchesPlainSlice(t *testing.T) {
	t.Parallel()
	r := testutils.NewRand(t)

	type version struct {
		s    immutable.Slice[int]
		want []int
	}
	cur := immutable.Slice[int]{}
	var model []int
	var history []version

	for step := range 2000 {
		n := len(model)
		switch op := r.IntN(9); {
		case op == 0:
			v := r.IntN(100)
			cur, model = cur.Append(v), append(slices.Clone(model), v)
		case op == 1 && n > 0:
			i, v := r.IntN(n), r.IntN(100)
			cur = cur.With(i, v)
			model = slices.Clone(model)
			model[i] = v
		case op == 2:
			i, v := r.IntN(n+1), r.IntN(100)
			cur, model = cur.Insert(i, v), slices.Insert(slices.Clone(model), i, v)
		case op == 3 && n > 0:
			i := r.IntN(n)
			j := i + r.IntN(n-i+1)
			cur, model = cur.Delete(i, j), slices.Delete(slices.Clone(model), i, j)
		case op == 4:
			cur = cur.Filter(func(v int) bool { return v%3 != 0 })
			model = slices.DeleteFunc(slices.Clone(model), func(v int) bool { return v%3 == 0 })
		case op == 5:
			cur, model = immutable.Sorted(cur), slices.Sorted(slices.Values(model))
		case op == 6 && n > 0:
			i := r.IntN(n)
			j := i + r.IntN(n-i+1)
			cur, model = cur.Sub(i, j), slices.Clone(model[i:j])
		case op == 7:
			b := cur.Edit()
			model = slices.Clone(model)
			for range r.IntN(5) {
				v := r.IntN(100)
				b.Append(v)
				model = append(model, v)
				if b.Len() > 1 {
					b.Delete(0, 1)
					model = model[1:]
				}
			}
			cur = b.Done()
		default:
			cur, model = cur.Reversed(), slices.Clone(model)
			slices.Reverse(model)
		}

		require.Equal(t, len(model), cur.Len(), "step %d", step)
		require.True(t, slices.Equal(model, collect(cur)), "step %d: want %v, got %v", step, model, collect(cur))

		history = append(history, version{s: cur, want: model})
		for i, h := range history {
			require.True(t, slices.Equal(h.want, collect(h.s)), "version %d changed at step %d", i, step)
		}
		if len(history) > 64 {
			history = history[1:]
		}
	}
}

func TestSlice_Allocations(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not meaningful under -race")
	}
	s := immutable.SliceOf(1, 2, 3, 4, 5, 6, 7, 8)
	var sink int

	reads := testing.AllocsPerRun(100, func() {
		sink += s.Len() + s.At(3)
		for _, v := range s.All() {
			sink += v
		}
	})
	require.Zero(t, reads, "reads never allocate")

	require.InDelta(t, 1, testing.AllocsPerRun(100, func() { sink += s.With(0, 9).Len() }), 0, "With")
	require.InDelta(t, 1, testing.AllocsPerRun(100, func() { sink += s.Delete(0, 2).Len() }), 0, "Delete")
	require.InDelta(t, 0, testing.AllocsPerRun(100, func() { sink += s.Sub(1, 3).Len() }), 0, "Sub")

	// A builder sized up front never grows: 64 appends cost the builder and its array, not one
	// allocation per append.
	batch := testing.AllocsPerRun(100, func() {
		b := immutable.NewSliceBuilder[int](64)
		for i := range 64 {
			b.Append(i)
		}
		sink += b.Done().Len()
	})
	require.LessOrEqual(t, batch, 2.0, "64 appends into a sized builder")

	// Edit copies on the first write, sized for it, so these cost the builder and at most one array.
	require.InDelta(t, 1, testing.AllocsPerRun(100, func() { sink += s.Edit().Done().Len() }), 0,
		"a builder that never writes copies nothing")
	require.InDelta(t, 2, testing.AllocsPerRun(100, func() {
		b := s.Edit()
		b.Append(9)
		sink += b.Done().Len()
	}), 0, "Edit then Append copies once")
	_ = sink
}
