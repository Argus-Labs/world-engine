// Package immutable holds value types that keep their storage private, so nothing outside this
// package can reach into one. A value is built once, read freely, and replaced as a whole when
// something new is needed. That encapsulation is what lets a component hold one without handing the
// world's backing memory to every caller of Get.
//
// The name describes the intended discipline rather than a guarantee the compiler enforces. Slice
// derivations edit in place by review decision, so read Slice's own doc before using one.
package immutable

import (
	"cmp"
	"iter"
	"reflect"
	"slices"
	"strings"

	"github.com/goccy/go-json"

	"github.com/argus-labs/world-engine/pkg/assert"
)

// TODO(immutable): derivations edit in place and do not copy, by review decision — one allocation
// per edit was judged too expensive for a game loop. The cost is that a derivation reaches the
// column's array before any Set, so derive-then-Set is a requirement rather than a convention. See
// the note above the Deriving section for what that means at a call site.

// Slice is a sequence with a private backing array, for component fields whose length is genuinely
// unbounded. It is the only variable-length collection a component may hold.
//
// A component column stores values and Get hands back a copy, but a raw []T inside that copy still
// shares its backing array with the column. Slice keeps that array unexported, so no caller can
// index into it, reslice it, or pass it to another package, and Get stays zero-copy.
//
// What Slice does not do is copy on a derivation. Most derivations edit the backing array in place
// and return a Slice over that same array, so the column has already changed by the time the
// derivation returns. Always Set the component afterwards:
//
//	inv := ref.Get()
//	inv.Items = inv.Items.With(0, item) // the column's array has changed here
//	ref.Set(inv)                        // this republishes what is already true
//
// Deriving and dropping the result does not leave the world untouched — it leaves the world holding
// an edit no snapshot recorded. Copy the elements out first (Values, or immutable.Collect) when the
// original has to survive. Each derivation's doc
// says whether it writes through.
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
//
// NOTE: Requires component set if used.
func (s Slice[T]) Chunk(n int) iter.Seq[Slice[T]] {
	assert.That(n >= 1, "immutable: Chunk(%d) size must be at least one", n)
	// Called here rather than inside the closure so its own size check runs now, not on the first
	// range — that is the check that survives a release build, where assert.That compiles away.
	parts := slices.Chunk(s.items, n)
	return func(yield func(Slice[T]) bool) {
		for part := range parts {
			// Each part is a window onto the receiver's array rather than a copy, so deriving from
			// one writes into the receiver.
			if !yield(Slice[T]{items: part}) {
				return
			}
		}
	}
}

// -------------------------------------------------------------------------------------------------
// Deriving
// -------------------------------------------------------------------------------------------------
//
// These do not copy. Where the result fits in the receiver's array they edit it in place and return
// a Slice over that same array, so the receiver — and every other Slice sharing it, including the
// one the component column still holds — sees the edit immediately.
//
// Two consequences worth stating plainly. Deriving and discarding the result still changes the
// world. And the ones that shrink (Without, Filter, Delete, CompactFunc) leave the receiver at its
// old length over a shifted, zero-filled tail, so the receiver is not merely reordered but wrong.
// Set the component after every derivation, and copy the elements out first (immutable.Collect) when
// the original has to survive.

// Append returns a Slice with items added at the end. It allocates a new array, so the receiver is
// unchanged.
//
// NOTE: Requires component set if used.
func (s Slice[T]) Append(items ...T) Slice[T] {
	return Slice[T]{items: slices.Concat(s.items, items)}
}

// With returns a Slice with the element at index i replaced by v. It writes through: the receiver
// holds v at i as well. It panics when i is out of range, like a slice.
//
// NOTE: Requires component set if used.
func (s Slice[T]) With(i int, v T) Slice[T] {
	assert.That(i >= 0 && i < len(s.items), "immutable: With(%d) out of range, length %d", i, len(s.items))
	s.items[i] = v
	return Slice[T]{items: s.items}
}

// Without returns a Slice with the element at index i removed. It writes through: the receiver keeps
// its old length over the shifted, zero-filled tail. It panics when i is out of range, like a slice.
//
// NOTE: Requires component set if used.
func (s Slice[T]) Without(i int) Slice[T] {
	assert.That(i >= 0 && i < len(s.items), "immutable: Without(%d) out of range, length %d", i, len(s.items))
	return Slice[T]{items: slices.Delete(s.items, i, i+1)}
}

// Filter returns a Slice holding the elements for which keep is true, in order, and keep runs once
// per element. It writes through: the receiver keeps its old length over the shifted, zero-filled
// tail.
//
// NOTE: Requires component set if used.
func (s Slice[T]) Filter(keep func(T) bool) Slice[T] {
	return Slice[T]{items: slices.DeleteFunc(s.items, func(v T) bool { return !keep(v) })}
}

// Insert returns a Slice with items inserted at index i, which may equal Len. It writes through
// whenever the receiver has the spare capacity to hold the result and allocates otherwise, so treat
// the receiver as changed either way. It panics when i is out of range, like slices.Insert.
//
// NOTE: Requires component set if used.
func (s Slice[T]) Insert(i int, items ...T) Slice[T] {
	assert.That(i >= 0 && i <= len(s.items), "immutable: Insert(%d) out of range, length %d", i, len(s.items))
	return Slice[T]{items: slices.Insert(s.items, i, items...)}
}

// Sub returns a Slice holding the elements in [lo, hi). It is the one derivation that writes
// nothing, but the result is a window onto the receiver's array, so deriving from that window writes
// into the receiver. It panics when the range is out of bounds, like s[lo:hi].
//
// The bounds are checked against Len explicitly: a derived Slice can have spare capacity, and a
// plain slice expression would read past the end into it.
//
// NOTE: Requires component set if used.
func (s Slice[T]) Sub(lo, hi int) Slice[T] {
	assert.That(0 <= lo && lo <= hi && hi <= len(s.items),
		"immutable: Sub(%d, %d) out of range, length %d", lo, hi, len(s.items))
	// Three-index, so the runtime bounds the range by LENGTH rather than capacity. A derived Slice can
	// carry spare capacity, and a plain s.items[lo:hi] would happily read into it. This check is the one
	// that has to survive a release build, where assert.That compiles away.
	return Slice[T]{items: s.items[lo:hi:len(s.items)]}
}

// Reversed returns a Slice with the elements in reverse order. It writes through: the receiver is
// reversed too, so calling it once a tick in a query flips the stored order every tick. To read in
// reverse without deriving anything, use Backward.
//
// NOTE: Requires component set if used.
func (s Slice[T]) Reversed() Slice[T] {
	slices.Reverse(s.items)
	return Slice[T]{items: s.items}
}

// SortedFunc returns a Slice sorted by compare, which reports a<b as negative, a==b as zero and a>b
// as positive. The sort is stable, so equal elements keep their order. It writes through: the
// receiver is sorted too, so even a read-only use such as s.SortedFunc(cmp).At(0) reorders the
// column. To find one extreme without deriving anything, use MinFunc or MaxFunc.
//
// NOTE: Requires component set if used.
func (s Slice[T]) SortedFunc(compare func(a, b T) int) Slice[T] {
	slices.SortStableFunc(s.items, compare)
	return Slice[T]{items: s.items}
}

// Delete returns a Slice with the elements in [i, j) removed. It writes through: the receiver keeps
// its old length over the shifted, zero-filled tail. It panics when the range is out of bounds, like
// slices.Delete.
//
// NOTE: Requires component set if used.
func (s Slice[T]) Delete(i, j int) Slice[T] {
	assert.That(0 <= i && i <= j && j <= len(s.items),
		"immutable: Delete(%d, %d) out of range, length %d", i, j, len(s.items))
	return Slice[T]{items: slices.Delete(s.items, i, j)}
}

// Replace returns a Slice with the elements in [i, j) replaced by items. It writes through whenever
// the receiver has the spare capacity to hold the result and allocates otherwise, so treat the
// receiver as changed either way. It panics when the range is out of bounds, like slices.Replace.
//
// NOTE: Requires component set if used.
func (s Slice[T]) Replace(i, j int, items ...T) Slice[T] {
	assert.That(0 <= i && i <= j && j <= len(s.items),
		"immutable: Replace(%d, %d) out of range, length %d", i, j, len(s.items))
	return Slice[T]{items: slices.Replace(s.items, i, j, items...)}
}

// CompactFunc returns a Slice with runs of consecutive elements that eq reports equal collapsed to
// one, like slices.CompactFunc. It writes through: the receiver keeps its old length over the
// shifted, zero-filled tail.
//
// NOTE: Requires component set if used.
func (s Slice[T]) CompactFunc(eq func(a, b T) bool) Slice[T] {
	return Slice[T]{items: slices.CompactFunc(s.items, eq)}
}

// Repeat returns a Slice holding the elements count times over. It allocates a new array, so the
// receiver is unchanged. It panics when count is negative or the result would overflow, like
// slices.Repeat.
//
// NOTE: Requires component set if used.
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

// Sorted returns a Slice with the elements of s in ascending order. The sort is stable. Like
// SortedFunc it writes through, so s ends up sorted too.
//
// NOTE: Requires component set if used.
func Sorted[T cmp.Ordered](s Slice[T]) Slice[T] {
	return s.SortedFunc(cmp.Compare[T])
}

// Compact returns a Slice with runs of consecutive equal elements collapsed to one, like
// slices.Compact. Like CompactFunc it writes through, so s keeps its old length over the shifted,
// zero-filled tail.
//
// NOTE: Requires component set if used.
func Compact[T comparable](s Slice[T]) Slice[T] {
	return Slice[T]{items: slices.Compact(s.items)}
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
// JSON and Reflection
// -------------------------------------------------------------------------------------------------
// A Slice is a list, so on the wire it is a JSON array — the same text a []T would produce. Without
// these two methods the encoder sees a struct with one unexported field and writes `{}`, and the
// decoder refuses an array outright, so a component holding a Slice could neither be logged nor
// read back from a hand-written payload. Both encoding/json and goccy/go-json dispatch on these
// method signatures, so one implementation serves whichever the caller uses. (goccy is imported
// here only because the repo's lint policy prefers it; the methods are library-neutral.)

// MarshalJSON encodes the Slice as a JSON array of its items. The zero value encodes as `[]`, not
// `null`: a Slice is never "absent", only empty.
func (s Slice[T]) MarshalJSON() ([]byte, error) {
	if len(s.items) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(s.items)
}

// UnmarshalJSON replaces the Slice with the array in data. An empty array and `null` both yield the
// zero value — nil items, not an empty allocation — so a decoded empty Slice compares equal to a
// fresh one under reflect.DeepEqual, the same rule the generated FromProto follows.
func (s *Slice[T]) UnmarshalJSON(data []byte) error {
	var items []T
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	if len(items) == 0 {
		items = nil
	}
	s.items = items
	return nil
}

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
