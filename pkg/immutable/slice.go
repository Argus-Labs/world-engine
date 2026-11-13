// Package immutable holds value types that keep their storage private, so nothing outside this
// package can reach into one. A value is built once, read freely, and replaced as a whole when
// something new is needed. That encapsulation is what lets a component hold one without handing the
// world's backing memory to every caller of Get.
//
// Nothing here writes to an array once a value holds it: every derivation returns a value over a new
// array, so the receiver and every copy of it stay exactly as they were.
package immutable

import (
	"cmp"
	"iter"
	"reflect"
	"slices"
	"strings"

	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/argus-labs/world-engine/pkg/assert"
)

// Slice is a sequence with a private backing array, for component fields whose length is genuinely
// unbounded. It is the only variable-length collection a component may hold.
//
// A component column stores values and Get hands back a copy, but a raw []T inside that copy still
// shares its backing array with the column. Slice keeps that array unexported, so no caller can
// index into it, reslice it, or pass it to another package, and Get stays zero-copy.
//
// Derivations never write to the receiver's array. Each returns a Slice over a new array (Sub and
// Chunk return windows onto the old one, which is safe because nothing writes to it), so the column,
// the caller's copy and any snapshot keep what they held. A derived Slice reaches the world only
// through Set:
//
//	inv := ref.Get()
//	inv.Items = inv.Items.With(0, item) // the column is unchanged
//	ref.Set(inv)                        // this stores the new Slice
//
// Each derivation costs one copy. For several edits at once, use Edit or NewSliceBuilder, which copy
// once and hand the array over with Done.
//
// Every empty Slice is the zero value: producers, derivations, Done and both decoders return nil
// items for an empty result, so empty Slices are [reflect.DeepEqual] however they were built.
//
// The element type must be value-safe too: scalars, strings, fixed arrays, another Slice, [time.Time],
// or structs of those. Slice does not check this — Slice[*T] compiles, and hands the pointer straight
// back out of At — but registering a component that holds such a Slice fails.
type Slice[T any] struct {
	items []T
}

// -------------------------------------------------------------------------------------------------
// Construction
// -------------------------------------------------------------------------------------------------

// SliceOf returns a Slice holding a copy of items. Later changes to the caller's slice do not
// reach the returned value.
func SliceOf[T any](items ...T) Slice[T] {
	return wrap(slices.Clone(items))
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
// like [slices.MinFunc].
func (s Slice[T]) MinFunc(compare func(a, b T) int) T {
	return slices.MinFunc(s.items, compare)
}

// MaxFunc returns the maximal element according to compare. It panics when the Slice is empty,
// like [slices.MaxFunc].
func (s Slice[T]) MaxFunc(compare func(a, b T) int) T {
	return slices.MaxFunc(s.items, compare)
}

// IsSortedFunc reports whether the elements are in ascending order according to compare.
func (s Slice[T]) IsSortedFunc(compare func(a, b T) int) bool {
	return slices.IsSortedFunc(s.items, compare)
}

// Chunk returns an iterator over consecutive sub-Slices of up to n elements. Each is a window onto
// the receiver's array, not a copy. It panics when n is less than one, like [slices.Chunk].
func (s Slice[T]) Chunk(n int) iter.Seq[Slice[T]] {
	assert.That(n >= 1, "immutable: Chunk(%d) size must be at least one", n)
	// Called here rather than inside the closure so its own size check runs now, not on the first
	// range — that is the check that survives a release build, where assert.That compiles away.
	parts := slices.Chunk(s.items, n)
	return func(yield func(Slice[T]) bool) {
		for part := range parts {
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
// Each returns a Slice over a new array and leaves the receiver unchanged. Range checks that must hold
// in a release build, where assert.That compiles away, are left to the runtime's own slice bounds
// checks.

// Append returns a Slice with items added at the end.
//
// NOTE: Requires component set if used.
func (s Slice[T]) Append(items ...T) Slice[T] {
	return wrap(slices.Concat(s.items, items))
}

// With returns a Slice with the element at index i replaced by v. It panics when i is out of range,
// like a slice.
//
// NOTE: Requires component set if used.
func (s Slice[T]) With(i int, v T) Slice[T] {
	assert.That(i >= 0 && i < len(s.items), "immutable: With(%d) out of range, length %d", i, len(s.items))
	out := slices.Clone(s.items)
	out[i] = v
	return Slice[T]{items: out}
}

// Without returns a Slice with the element at index i removed. It panics when i is out of range, like
// a slice.
//
// NOTE: Requires component set if used.
func (s Slice[T]) Without(i int) Slice[T] {
	assert.That(i >= 0 && i < len(s.items), "immutable: Without(%d) out of range, length %d", i, len(s.items))
	return s.Replace(i, i+1)
}

// Filter returns a Slice holding the elements for which keep is true, in order, and keep runs once
// per element.
//
// NOTE: Requires component set if used.
func (s Slice[T]) Filter(keep func(T) bool) Slice[T] {
	// Sized for the worst case so the result costs one allocation; keep runs once per element, so
	// the kept count cannot be measured first.
	out := make([]T, 0, len(s.items))
	for _, v := range s.items {
		if keep(v) {
			out = append(out, v)
		}
	}
	return wrap(out)
}

// Insert returns a Slice with items inserted at index i, which may equal Len. It panics when i is out
// of range, like [slices.Insert].
//
// NOTE: Requires component set if used.
func (s Slice[T]) Insert(i int, items ...T) Slice[T] {
	assert.That(i >= 0 && i <= len(s.items), "immutable: Insert(%d) out of range, length %d", i, len(s.items))
	return s.Replace(i, i, items...)
}

// Sub returns a Slice holding the elements in [lo, hi). The result is a window onto the receiver's
// array rather than a copy, so it costs nothing. It panics when the range is out of bounds, like
// s[lo:hi].
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
	return wrap(s.items[lo:hi:len(s.items)])
}

// Reversed returns a Slice with the elements in reverse order. To read in reverse without a copy, use
// Backward.
//
// NOTE: Requires component set if used.
func (s Slice[T]) Reversed() Slice[T] {
	out := slices.Clone(s.items)
	slices.Reverse(out)
	return wrap(out)
}

// SortedFunc returns a Slice sorted by compare, which reports a<b as negative, a==b as zero and a>b
// as positive. The sort is stable, so equal elements keep their order. To find one extreme without a
// copy, use MinFunc or MaxFunc.
//
// NOTE: Requires component set if used.
func (s Slice[T]) SortedFunc(compare func(a, b T) int) Slice[T] {
	out := slices.Clone(s.items)
	slices.SortStableFunc(out, compare)
	return wrap(out)
}

// Delete returns a Slice with the elements in [i, j) removed. It panics when the range is out of
// bounds, like [slices.Delete].
//
// NOTE: Requires component set if used.
func (s Slice[T]) Delete(i, j int) Slice[T] {
	assert.That(0 <= i && i <= j && j <= len(s.items),
		"immutable: Delete(%d, %d) out of range, length %d", i, j, len(s.items))
	return s.Replace(i, j)
}

// Replace returns a Slice with the elements in [i, j) replaced by items. It panics when the range is
// out of bounds, like [slices.Replace].
//
// NOTE: Requires component set if used.
func (s Slice[T]) Replace(i, j int, items ...T) Slice[T] {
	assert.That(0 <= i && i <= j && j <= len(s.items),
		"immutable: Replace(%d, %d) out of range, length %d", i, j, len(s.items))
	// The range check that survives a release build: s.items[:i] and s.items[j:] alone would accept
	// i > j and duplicate the elements between them.
	_ = s.items[i:j]
	out := make([]T, 0, len(s.items)-(j-i)+len(items))
	out = append(out, s.items[:i]...)
	out = append(out, items...)
	out = append(out, s.items[j:]...)
	return wrap(out)
}

// CompactFunc returns a Slice with runs of consecutive elements that eq reports equal collapsed to
// one, like [slices.CompactFunc].
//
// NOTE: Requires component set if used.
func (s Slice[T]) CompactFunc(eq func(a, b T) bool) Slice[T] {
	return wrap(slices.CompactFunc(slices.Clone(s.items), eq))
}

// Repeat returns a Slice holding the elements count times over. It panics when count is negative or
// the result would overflow, like [slices.Repeat].
//
// NOTE: Requires component set if used.
func (s Slice[T]) Repeat(count int) Slice[T] {
	assert.That(count >= 0, "immutable: Repeat(%d) must not be negative", count)
	if count == 0 || len(s.items) == 0 {
		return Slice[T]{}
	}
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

// Compare compares a and b element by element, like [slices.Compare].
func Compare[T cmp.Ordered](a, b Slice[T]) int {
	return slices.Compare(a.items, b.items)
}

// CompareFunc is Compare with a comparison function, whose Slices may hold different element
// types, like [slices.CompareFunc].
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

// Sorted returns a Slice with the elements of s in ascending order. The sort is stable.
//
// NOTE: Requires component set if used.
func Sorted[T cmp.Ordered](s Slice[T]) Slice[T] {
	return s.SortedFunc(cmp.Compare[T])
}

// Compact returns a Slice with runs of consecutive equal elements collapsed to one, like
// [slices.Compact].
//
// NOTE: Requires component set if used.
func Compact[T comparable](s Slice[T]) Slice[T] {
	return wrap(slices.Compact(slices.Clone(s.items)))
}

// Min returns the smallest element of s. It panics when s is empty, like [slices.Min].
func Min[T cmp.Ordered](s Slice[T]) T {
	return slices.Min(s.items)
}

// Max returns the largest element of s. It panics when s is empty, like [slices.Max].
func Max[T cmp.Ordered](s Slice[T]) T {
	return slices.Max(s.items)
}

// IsSorted reports whether the elements of s are in ascending order.
func IsSorted[T cmp.Ordered](s Slice[T]) bool {
	return slices.IsSorted(s.items)
}

// BinarySearch searches a sorted s for target and returns the position where it is, or would be
// inserted, and whether it was found, like [slices.BinarySearch].
func BinarySearch[T cmp.Ordered](s Slice[T], target T) (int, bool) {
	return slices.BinarySearch(s.items, target)
}

// BinarySearchFunc is BinarySearch with a comparison function, whose target may be of another type,
// like [slices.BinarySearchFunc].
func BinarySearchFunc[E, T any](s Slice[E], target T, compare func(E, T) int) (int, bool) {
	return slices.BinarySearchFunc(s.items, target, compare)
}

// -------------------------------------------------------------------------------------------------
// Building from other values
// -------------------------------------------------------------------------------------------------

// Transform returns a new Slice holding f applied to each element of s, in order.
func Transform[T, U any](s Slice[T], f func(T) U) Slice[U] {
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
	return wrap(slices.Collect(seq))
}

// wrap returns a Slice over items, or the zero value when items is empty, so every empty Slice is
// the same value however it was produced.
func wrap[T any](items []T) Slice[T] {
	if len(items) == 0 {
		return Slice[T]{}
	}
	return Slice[T]{items: items}
}

// -------------------------------------------------------------------------------------------------
// Serialization and Reflection
// -------------------------------------------------------------------------------------------------
// A Slice is a list, so it encodes as an array, in JSON and MessagePack alike — the same as a []T
// would, except that MessagePack writes a []byte as binary and a Slice[byte] still as an array, so
// the two cannot read each other's bytes. Without the JSON methods the encoder sees a struct with
// one unexported field and writes `{}`, and the decoder refuses an array outright, so a component
// holding a Slice could neither be logged nor read back from a hand-written payload. Both
// encoding/json and goccy/go-json dispatch on these method signatures, so one implementation serves
// whichever the caller uses. (goccy is imported here only because the repo's lint policy prefers it;
// the methods are library-neutral.)

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
// fresh one under [reflect.DeepEqual], the same rule DecodeMsgpack follows.
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

// EncodeMsgpack encodes a Slice as an array, including empty and byte-valued Slices.
func (s Slice[T]) EncodeMsgpack(enc *msgpack.Encoder) error {
	if err := enc.EncodeArrayLen(len(s.items)); err != nil {
		return eris.Wrap(err, "failed to encode immutable slice length")
	}
	for _, item := range s.items {
		if err := enc.Encode(item); err != nil {
			return eris.Wrap(err, "failed to encode immutable slice element")
		}
	}
	return nil
}

// DecodeMsgpack replaces the Slice only after every element has decoded successfully.
// Empty arrays and nil normalize to the zero value, as with JSON.
func (s *Slice[T]) DecodeMsgpack(dec *msgpack.Decoder) error {
	n, err := dec.DecodeArrayLen()
	if err != nil {
		return eris.Wrap(err, "failed to decode immutable slice length")
	}
	var items []T
	for range n {
		var item T
		if err := dec.Decode(&item); err != nil {
			return eris.Wrap(err, "failed to decode immutable slice element")
		}
		items = append(items, item)
	}
	s.items = items
	return nil
}

// IsZero lets MessagePack's omitempty distinguish populated Slices from empty ones.
func (s Slice[T]) IsZero() bool { return len(s.items) == 0 }

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
