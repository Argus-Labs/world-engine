package immutable_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/stretchr/testify/require"
)

type flat struct {
	A     int
	B     [4]string
	Table immutable.Map[int, point]
}

type withSlices struct {
	Name   string
	Items  immutable.Slice[int]
	hidden immutable.Slice[int]
	Groups [2]struct{ Tags immutable.Slice[string] }
	Nested immutable.Slice[immutable.Slice[int]]
	Lists  immutable.Map[int, immutable.Slice[int]]
}

type tree struct {
	Value int
	Kids  immutable.Slice[tree]
}

type mapTree struct {
	Kids immutable.Map[int, mapTree]
}

func TestCloneFunc_NilWithoutSlices(t *testing.T) {
	t.Parallel()

	require.Nil(t, immutable.CloneFunc[int]())
	require.Nil(t, immutable.CloneFunc[flat]())
	require.Nil(t, immutable.CloneFunc[immutable.Map[int, point]](), "a Map of plain values is never edited in place")
	require.Nil(t, immutable.CloneFunc[mapTree](), "a type that reaches itself only through Maps holds no Slice")
	require.Nil(t, immutable.CloneFunc[[0]immutable.Slice[int]]())
	require.NotNil(t, immutable.CloneFunc[withSlices]())
	require.NotNil(t, immutable.CloneFunc[tree]())
}

func newWithSlices() withSlices {
	v := withSlices{
		Name:   "a",
		Items:  immutable.SliceOf(1, 2),
		hidden: immutable.SliceOf(3),
		Nested: immutable.SliceOf(immutable.SliceOf(4)),
		Lists:  immutable.Map[int, immutable.Slice[int]]{}.With(1, immutable.SliceOf(5)),
	}
	v.Groups[1].Tags = immutable.SliceOf("x")
	return v
}

// editAll edits every Slice v holds in place, as a system would after Get.
func editAll(v *withSlices) {
	v.Items.Set(0, -1)
	v.hidden.Set(0, -1)
	v.Groups[1].Tags.Set(0, "edited")
	inner := v.Nested.At(0)
	inner.Set(0, -1)
	list, _ := v.Lists.Get(1)
	list.Set(0, -1)
}

func TestCloneFunc_CopySharesNoSlice(t *testing.T) {
	t.Parallel()

	clone := immutable.CloneFunc[withSlices]()
	orig := newWithSlices()
	cp := orig
	clone(&cp)
	editAll(&cp)
	require.Equal(t, newWithSlices(), orig, "edits to the copy reach no Slice of the original")

	// The same holds the other way: the original edited after the copy leaves the copy alone.
	orig, cp = newWithSlices(), withSlices{}
	cp = orig
	clone(&cp)
	editAll(&orig)
	require.Equal(t, newWithSlices(), cp)
}

func TestCloneFunc_Tree(t *testing.T) {
	t.Parallel()

	leaf := tree{Value: 2, Kids: immutable.SliceOf(tree{Value: 3})}
	orig := tree{Value: 1, Kids: immutable.SliceOf(leaf)}
	cp := orig
	immutable.CloneFunc[tree]()(&cp)

	child := cp.Kids.At(0)
	child.Kids.Set(0, tree{Value: -1})
	require.Equal(t, 3, orig.Kids.At(0).Kids.At(0).Value, "every level of the tree is copied")
}

func TestClone(t *testing.T) {
	t.Parallel()

	orig := newWithSlices()
	cp := immutable.Clone(orig)
	editAll(&cp)
	require.Equal(t, newWithSlices(), orig)

	require.Equal(t, 7, immutable.Clone(7))
	require.Nil(t, immutable.Clone[any](nil))
}

func TestClone_InterfaceCopiesHeldValue(t *testing.T) {
	t.Parallel()

	orig := newWithSlices()
	var held any = orig
	cp, ok := immutable.Clone(held).(withSlices)
	require.True(t, ok)
	editAll(&cp)
	require.Equal(t, newWithSlices(), orig)
}

func TestCloneFunc_Allocations(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not meaningful under -race")
	}
	type component struct {
		ID    int
		Items immutable.Slice[int]
		Log   immutable.Slice[point]
	}
	clone := immutable.CloneFunc[component]()
	v := component{Items: immutable.SliceOf(1, 2, 3), Log: immutable.SliceOf(point{1, 2})}
	// One allocation per non-empty Slice, plus one for the value itself: passing it to reflect moves
	// it to the heap.
	require.InDelta(t, 3, testing.AllocsPerRun(100, func() {
		cp := v
		clone(&cp)
	}), 0)
}
