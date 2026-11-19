package immutable_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/stretchr/testify/require"
)

type withSlices struct {
	Name   string
	Items  immutable.Slice[int]
	hidden immutable.Slice[int]
	Groups [2]struct{ Tags immutable.Slice[string] }
	Nested immutable.Slice[immutable.Slice[int]]
	Lists  immutable.Map[int, immutable.Slice[int]]
	Table  immutable.Map[int, point]
}

type tree struct {
	Value int
	Kids  immutable.Slice[tree]
}

type mapTree struct {
	Kids immutable.Map[int, mapTree]
}

func newWithSlices() withSlices {
	v := withSlices{
		Name:   "a",
		Items:  immutable.SliceOf(1, 2),
		hidden: immutable.SliceOf(3),
		Nested: immutable.SliceOf(immutable.SliceOf(4)),
	}
	v.Lists.Set(1, immutable.SliceOf(5))
	v.Table.Set(1, point{1, 2})
	v.Groups[1].Tags = immutable.SliceOf("x")

	return v
}

// editAll edits every Slice and Map v holds in place, as a system would after Get.
func editAll(v *withSlices) {
	v.Items.Set(0, -1)
	v.hidden.Set(0, -1)
	v.Groups[1].Tags.Set(0, "edited")
	inner := v.Nested.At(0)
	inner.Set(0, -1)

	list, _ := v.Lists.Get(1)
	list.Set(0, -1)
	v.Lists.Set(2, immutable.SliceOf(6))
	v.Table.Set(1, point{-1, -1})
	v.Table.Delete(1)
}

func TestCopy_SharesNoStorage(t *testing.T) {
	t.Parallel()

	orig := newWithSlices()
	cp := orig
	editAll(&cp)
	require.Equal(t, newWithSlices(), orig, "edits to the copy reach no Slice of the original")

	// The same holds the other way: the original edited after the copy leaves the copy alone.
	orig, cp = newWithSlices(), withSlices{}
	cp = orig
	editAll(&orig)
	require.Equal(t, newWithSlices(), cp)
}

func TestCopy_Tree(t *testing.T) {
	t.Parallel()

	leaf := tree{Value: 2, Kids: immutable.SliceOf(tree{Value: 3})}
	orig := tree{Value: 1, Kids: immutable.SliceOf(leaf)}
	cp := orig

	child := cp.Kids.At(0)
	child.Kids.Set(0, tree{Value: -1})
	require.Equal(t, 3, orig.Kids.At(0).Kids.At(0).Value, "every level of the tree is copied")
}

func TestCopy_MapTree(t *testing.T) {
	t.Parallel()

	var leaf, orig mapTree
	leaf.Kids.Set(2, mapTree{})
	orig.Kids.Set(1, leaf)
	cp := orig

	child, _ := cp.Kids.Get(1)
	child.Kids.Set(3, mapTree{})
	child.Kids.Delete(2)
	got, _ := orig.Kids.Get(1)
	require.True(t, got.Kids.Has(2), "every level of the tree is copied")
	require.False(t, got.Kids.Has(3))
}

type embedsSlice struct {
	immutable.Slice[int]

	Table immutable.Map[int, int]
	List  immutable.Slice[int]
}

func TestCopy_StructEmbeddingASlice(t *testing.T) {
	t.Parallel()

	var orig embedsSlice
	orig.Append(1)
	orig.Table.Set(1, 1)
	orig.List.Append(1)
	cp := orig
	cp.Set(0, 9)
	cp.Table.Set(1, 9)
	cp.List.Set(0, 9)
	require.Equal(t, 1, orig.At(0), "the embedded Slice is copied")
	v, _ := orig.Table.Get(1)
	require.Equal(t, 1, v, "the Map beside it is copied")
	require.Equal(t, 1, orig.List.At(0), "the Slice beside it is copied")
}

// A value held in an interface, as Cardinal holds commands and events, is unchanged by edits to a copy
// taken out of it.
func TestCopy_OutOfAnInterface(t *testing.T) {
	t.Parallel()

	var held any = newWithSlices()

	cp, ok := held.(withSlices)
	require.True(t, ok)
	editAll(&cp)
	require.Equal(t, newWithSlices(), held)
}
