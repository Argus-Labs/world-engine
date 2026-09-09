// Package immutable holds value types whose contents cannot change in place. A value is built once,
// read freely, and replaced as a whole when something new is needed. That is what lets a component
// hold one without breaking the rule that world state only ever changes through Set.
package immutable

import (
	"cmp"
	"iter"
	"reflect"
	"slices"
	"strings"

	"github.com/argus-labs/world-engine/pkg/assert"
)

// TODO(immutable): every derivation copies the whole list, which is one allocation per edit. Review
// asked for the copy to go from the operations that keep the length — With, Reversed, SortedFunc —
// on the grounds that games cannot afford it. It stays, for two reasons.
//
// It is the guarantee the type exists for. Get hands back a struct copy whose Slice still points at
// the column's array, so an in-place derivation writes into the live world before any Set, and the
// next snapshot serializes it. A read-only use such as s.SortedFunc(cmp).At(0) would reorder the
// column, and no linter catches it because the result is used. In-place shrinking is worse than
// reordering: the column would keep its old length over a shifted, zero-filled tail.
//
// And it buys little. The operations that keep the length are called once per edit, not in a loop,
// so the allocation they save is one per tick, against a read path that is already zero-copy. The
// case worth optimizing is many edits to one list in one tick, and none of these fix that.
//
// Ways to do that properly, once profiling says it matters:
//   - Mutate(func([]T) []T): copy in, edit with the slices package, copy out. Two allocations for
//     any number of edits, and nothing the callback keeps can reach the result.
//   - Update(func(i int, v T) T): one copy, the callback sees one element at a time. Slot edits only.
//   - Ownership in spare capacity: a clone allocates one spare slot and Set reslices it away, so the
//     first derivation after Get copies and later ones go in place. Needs Set and the snapshot
//     decoder to freeze the stored value, which means a generated hook per component.
//   - A fixed array plus a count, for any list whose length has a bound worth defending. Arrays are
//     values, so the copy is the struct copy Get and Set already do, and edits are free.

// Slice is an immutable sequence for component fields whose length is genuinely unbounded. It is
// the only variable-length collection a component may hold.
//
// A component column stores values and Get hands back a copy, but a raw []T inside that copy still
// shares its backing array with the column: writing through it changes the live world without a Set,
// and a snapshot only ever holds what went through Set. Slice narrows that hole: the backing array is
// unexported and nothing mutates in place — so sharing it between copies is safe, and Get stays
// zero-copy. To change one, derive a new one and Set the component that holds it:
//
//	inv := ref.Get()
//	inv.Items = inv.Items.With(0, item)
//	ref.Set(inv)
//
// Two empty Slices are not always reflect.DeepEqual. A derivation that empties one leaves its backing
// array allocated, while the zero value and SliceOf have none, and DeepEqual looks at the field rather
// than the elements. Compare with Equal or EqualFunc, which compare elements and agree with
// slices.Equal that nil and empty are the same list. The generated decoder leaves an empty repeated
// field as the zero value, so a component restored from a snapshot does match one built fresh.
//
// The element type must be value-safe too: scalars, strings, fixed arrays, or structs of those.
// Slice does not check this — Slice[*T] compiles, and hands the pointer straight back out of At —
// but the wire generator refuses such an element, and refuses a Slice directly inside a Slice, which
// protobuf cannot carry (wrap the inner one in a named struct).
type Slice[T any] struct {
	items []T
}

// -------------------------------------------------------------------------------------------------
// Construction
// -------------------------------------------------------------------------------------------------

// SliceOf returns a Slice holding a copy of items. Later changes to the caller's slice do not
// reach the returned value.
func SliceOf[T any](items ...T) Slice[T] {
	return Slice[T]{items: slices.Clone(items)}
}

// -------------------------------------------------------------------------------------------------
// Reading
// -------------------------------------------------------------------------------------------------

// Len returns the number of elements.
func (s Slice[T]) Len() int {
	return len(s.items)
}

// At returns a copy of the element at index i. It panics when i is out of range, like a slice.
func (s Slice[T]) At(i int) T {
	return s.items[i]
}

// All returns an iterator over index and element, in order. Elements are yielded by value.
func (s Slice[T]) All() iter.Seq2[int, T] {
	return slices.All(s.items)
}

// Values returns an iterator over the elements, in order. Elements are yielded by value.
func (s Slice[T]) Values() iter.Seq[T] {
	return slices.Values(s.items)
}

// Backward returns an iterator over index and element from the last element to the first.
func (s Slice[T]) Backward() iter.Seq2[int, T] {
	return slices.Backward(s.items)
}

// IndexFunc returns the index of the first element for which f is true, or -1.
func (s Slice[T]) IndexFunc(f func(T) bool) int {
	return slices.IndexFunc(s.items, f)
}

// ContainsFunc reports whether f is true for any element.
func (s Slice[T]) ContainsFunc(f func(T) bool) bool {
	return slices.ContainsFunc(s.items, f)
}

// MinFunc returns the minimal element according to compare. It panics when the Slice is empty,
// like slices.MinFunc.
func (s Slice[T]) MinFunc(compare func(a, b T) int) T {
	return slices.MinFunc(s.items, compare)
}

// MaxFunc returns the maximal element according to compare. It panics when the Slice is empty,
// like slices.MaxFunc.
func (s Slice[T]) MaxFunc(compare func(a, b T) int) T {
	return slices.MaxFunc(s.items, compare)
}

// IsSortedFunc reports whether the elements are in ascending order according to compare.
func (s Slice[T]) IsSortedFunc(compare func(a, b T) int) bool {
	return slices.IsSortedFunc(s.items, compare)
}

// Chunk returns an iterator over consecutive sub-Slices of up to n elements. It panics when n is
// less than one, like slices.Chunk.
func (s Slice[T]) Chunk(n int) iter.Seq[Slice[T]] {
	assert.That(n >= 1, "immutable: Chunk(%d) size must be at least one", n)
	// Called here rather than inside the closure so its own size check runs now, not on the first
	// range — that is the check that survives a release build, where assert.That compiles away.
	parts := slices.Chunk(s.items, n)
	return func(yield func(Slice[T]) bool) {
		for part := range parts {
			if !yield(Slice[T]{items: slices.Clone(part)}) {
				return
			}
		}
	}
}

// Clone returns a fresh []T holding a copy of the elements, for an API that needs a plain slice.
// Changes to the result never reach the Slice. It is never nil, even for an empty Slice.
func (s Slice[T]) Clone() []T {
	out := make([]T, len(s.items))
	copy(out, s.items)
	return out
}

// -------------------------------------------------------------------------------------------------
// Deriving
// -------------------------------------------------------------------------------------------------

// Append returns a new Slice with items added at the end. The receiver is unchanged.
func (s Slice[T]) Append(items ...T) Slice[T] {
	return Slice[T]{items: slices.Concat(s.items, items)}
}

// With returns a new Slice with the element at index i replaced by v. The receiver is unchanged.
// It panics when i is out of range, like a slice.
func (s Slice[T]) With(i int, v T) Slice[T] {
	assert.That(i >= 0 && i < len(s.items), "immutable: With(%d) out of range, length %d", i, len(s.items))
	out := s.Clone()
	out[i] = v
	return Slice[T]{items: out}
}

// Without returns a new Slice with the element at index i removed. The receiver is unchanged.
// It panics when i is out of range, like a slice.
func (s Slice[T]) Without(i int) Slice[T] {
	assert.That(i >= 0 && i < len(s.items), "immutable: Without(%d) out of range, length %d", i, len(s.items))
	return Slice[T]{items: slices.Delete(s.Clone(), i, i+1)}
}

// Filter returns a new Slice holding the elements for which keep is true, in order. The receiver is
// unchanged, and keep runs once per element.
func (s Slice[T]) Filter(keep func(T) bool) Slice[T] {
	return Slice[T]{items: slices.DeleteFunc(s.Clone(), func(v T) bool { return !keep(v) })}
}

// Insert returns a new Slice with items inserted at index i, which may equal Len. The receiver is
// unchanged. It panics when i is out of range, like slices.Insert.
func (s Slice[T]) Insert(i int, items ...T) Slice[T] {
	assert.That(i >= 0 && i <= len(s.items), "immutable: Insert(%d) out of range, length %d", i, len(s.items))
	// The clone is sized for the result, so slices.Insert finishes in place instead of growing.
	out := make([]T, len(s.items), len(s.items)+len(items))
	copy(out, s.items)
	return Slice[T]{items: slices.Insert(out, i, items...)}
}

// Sub returns a new Slice holding the elements in [lo, hi). The receiver is unchanged. It panics
// when the range is out of bounds, like s[lo:hi].
//
// The bounds are checked against Len explicitly: a derived Slice can have spare capacity, and a
// plain slice expression would read past the end into it.
func (s Slice[T]) Sub(lo, hi int) Slice[T] {
	assert.That(0 <= lo && lo <= hi && hi <= len(s.items),
		"immutable: Sub(%d, %d) out of range, length %d", lo, hi, len(s.items))
	// Three-index, so the runtime bounds the range by LENGTH rather than capacity. A derived Slice can
	// carry spare capacity, and a plain s.items[lo:hi] would happily read into it. This check is the one
	// that has to survive a release build, where assert.That compiles away.
	return Slice[T]{items: slices.Clone(s.items[lo:hi:len(s.items)])}
}

// Reversed returns a new Slice with the elements in reverse order. The receiver is unchanged. To
// read in reverse without deriving anything, use Backward.
func (s Slice[T]) Reversed() Slice[T] {
	out := s.Clone()
	slices.Reverse(out)
	return Slice[T]{items: out}
}

// SortedFunc returns a new Slice sorted by compare, which reports a<b as negative, a==b as zero and
// a>b as positive. The sort is stable, so equal elements keep their order. The receiver is unchanged.
// To find one extreme without deriving anything, use MinFunc or MaxFunc.
func (s Slice[T]) SortedFunc(compare func(a, b T) int) Slice[T] {
	out := s.Clone()
	slices.SortStableFunc(out, compare)
	return Slice[T]{items: out}
}

// Delete returns a new Slice with the elements in [i, j) removed. The receiver is unchanged. It
// panics when the range is out of bounds, like slices.Delete.
func (s Slice[T]) Delete(i, j int) Slice[T] {
	assert.That(0 <= i && i <= j && j <= len(s.items),
		"immutable: Delete(%d, %d) out of range, length %d", i, j, len(s.items))
	return Slice[T]{items: slices.Delete(s.Clone(), i, j)}
}

// Replace returns a new Slice with the elements in [i, j) replaced by items. The receiver is
// unchanged. It panics when the range is out of bounds, like slices.Replace.
func (s Slice[T]) Replace(i, j int, items ...T) Slice[T] {
	assert.That(0 <= i && i <= j && j <= len(s.items),
		"immutable: Replace(%d, %d) out of range, length %d", i, j, len(s.items))
	// The clone is sized for the result, so slices.Replace finishes in place instead of growing.
	out := make([]T, len(s.items), len(s.items)+max(len(items)-(j-i), 0))
	copy(out, s.items)
	return Slice[T]{items: slices.Replace(out, i, j, items...)}
}

// CompactFunc returns a new Slice with runs of consecutive elements that eq reports equal collapsed
// to one, like slices.CompactFunc. The receiver is unchanged.
func (s Slice[T]) CompactFunc(eq func(a, b T) bool) Slice[T] {
	return Slice[T]{items: slices.CompactFunc(s.Clone(), eq)}
}

// Repeat returns a new Slice holding the elements count times over. It panics when count is
// negative or the result would overflow, like slices.Repeat.
func (s Slice[T]) Repeat(count int) Slice[T] {
	assert.That(count >= 0, "immutable: Repeat(%d) must not be negative", count)
	return Slice[T]{items: slices.Repeat(s.items, count)}
}

// -------------------------------------------------------------------------------------------------
// Comparing
// -------------------------------------------------------------------------------------------------

// EqualFunc reports whether both slices have the same length and eq holds for every pair of
// elements at the same index.
func (s Slice[T]) EqualFunc(other Slice[T], eq func(a, b T) bool) bool {
	return slices.EqualFunc(s.items, other.items, eq)
}

// Equal reports whether a and b hold the same elements in the same order. It is a function rather
// than a method because a method cannot narrow T to comparable; use EqualFunc for other element types.
func Equal[T comparable](a, b Slice[T]) bool {
	return slices.Equal(a.items, b.items)
}

// Compare compares a and b element by element, like slices.Compare.
func Compare[T cmp.Ordered](a, b Slice[T]) int {
	return slices.Compare(a.items, b.items)
}

// CompareFunc is Compare with a comparison function, whose Slices may hold different element
// types, like slices.CompareFunc.
func CompareFunc[E1, E2 any](a Slice[E1], b Slice[E2], compare func(E1, E2) int) int {
	return slices.CompareFunc(a.items, b.items, compare)
}

// -------------------------------------------------------------------------------------------------
// Comparable and ordered elements
// -------------------------------------------------------------------------------------------------

// Index returns the index of the first element of s equal to v, or -1.
func Index[T comparable](s Slice[T], v T) int {
	return slices.Index(s.items, v)
}

// Contains reports whether v is an element of s.
func Contains[T comparable](s Slice[T], v T) bool {
	return slices.Contains(s.items, v)
}

// Sorted returns a new Slice with the elements of s in ascending order. The sort is stable.
func Sorted[T cmp.Ordered](s Slice[T]) Slice[T] {
	return s.SortedFunc(cmp.Compare[T])
}

// Compact returns a new Slice with runs of consecutive equal elements collapsed to one, like
// slices.Compact.
func Compact[T comparable](s Slice[T]) Slice[T] {
	return Slice[T]{items: slices.Compact(s.Clone())}
}

// Min returns the smallest element of s. It panics when s is empty, like slices.Min.
func Min[T cmp.Ordered](s Slice[T]) T {
	return slices.Min(s.items)
}

// Max returns the largest element of s. It panics when s is empty, like slices.Max.
func Max[T cmp.Ordered](s Slice[T]) T {
	return slices.Max(s.items)
}

// IsSorted reports whether the elements of s are in ascending order.
func IsSorted[T cmp.Ordered](s Slice[T]) bool {
	return slices.IsSorted(s.items)
}

// BinarySearch searches a sorted s for target and returns the position where it is, or would be
// inserted, and whether it was found, like slices.BinarySearch.
func BinarySearch[T cmp.Ordered](s Slice[T], target T) (int, bool) {
	return slices.BinarySearch(s.items, target)
}

// BinarySearchFunc is BinarySearch with a comparison function, whose target may be of another type,
// like slices.BinarySearchFunc.
func BinarySearchFunc[E, T any](s Slice[E], target T, compare func(E, T) int) (int, bool) {
	return slices.BinarySearchFunc(s.items, target, compare)
}

// -------------------------------------------------------------------------------------------------
// Building from other values
// -------------------------------------------------------------------------------------------------

// Map returns a new Slice holding f applied to each element of s, in order.
func Map[T, U any](s Slice[T], f func(T) U) Slice[U] {
	if len(s.items) == 0 {
		return Slice[U]{}
	}
	out := make([]U, len(s.items))
	for i, v := range s.items {
		out[i] = f(v)
	}
	return Slice[U]{items: out}
}

// Concat returns a new Slice holding the elements of every argument, in order.
func Concat[T any](ss ...Slice[T]) Slice[T] {
	total := 0
	for _, s := range ss {
		total += len(s.items)
	}
	if total == 0 {
		return Slice[T]{}
	}
	out := make([]T, 0, total)
	for _, s := range ss {
		out = append(out, s.items...)
	}
	return Slice[T]{items: out}
}

// Collect returns a Slice holding every element of seq, in order.
func Collect[T any](seq iter.Seq[T]) Slice[T] {
	return Slice[T]{items: slices.Collect(seq)}
}

// -------------------------------------------------------------------------------------------------
// Reflection
// -------------------------------------------------------------------------------------------------

// SliceElem reports whether t is an instantiation of Slice and, if so, returns its element type.
// It exists so tooling that works through reflection, such as the DST random filler, can recognise
// a Slice without depending on how it is laid out.
func SliceElem(t reflect.Type) (reflect.Type, bool) {
	if t == nil || t.Kind() != reflect.Struct || !strings.HasPrefix(t.Name(), "Slice[") {
		return nil, false
	}
	if t.PkgPath() != reflect.TypeFor[Slice[struct{}]]().PkgPath() {
		return nil, false
	}
	items, ok := t.FieldByName("items")
	if !ok || items.Type.Kind() != reflect.Slice {
		return nil, false
	}
	return items.Type.Elem(), true
}
