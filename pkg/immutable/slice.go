// Package immutable holds the collections a component may hold: Slice and Map. Both keep their
// storage private, so nothing outside this package can reach into one.
//
// Both are edited in place, like a Go slice and map. They are safe in a component because Cardinal
// copies them on the way in and out of the world (Get, Set, events and commands) with Clone and
// CloneFunc, so a system only ever edits its own copy.
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
)

// Slice is a list with a private array, for component fields whose length is genuinely unbounded.
// It is the only variable-length list a component may hold.
//
// Edit it in place, the way you would a Go slice, and store it with Set:
//
//	inv := ref.Get()       // inv.Items is a copy; the world is unchanged
//	inv.Items.Append(item)
//	inv.Items.Set(0, other)
//	ref.Set(inv)           // the world stores a copy of inv.Items
//
// Cardinal copies every Slice a component holds when Get hands the component out and when Set takes
// it in, so an edit never reaches the world without Set, and an edit after Set never reaches what was
// stored. Events and commands are copied the same way. It is the usual Go rule for a type that stores
// slices: copy them when they come in and when they go out.
//
// Inside one system a Slice behaves exactly like a Go slice. Assigning one (d := inv.Items) shares
// its array, so an edit through d can show up in inv.Items, and two copies that both Append can
// overwrite each other's new elements. Use Clone for a copy that shares nothing.
//
// Every empty Slice is the zero value: an edit that empties one, Reset, SliceOf, Collect, Clone and
// both decoders leave nil items, so empty Slices are [reflect.DeepEqual] however they were made.
//
// The element type must be value-safe too: scalars, strings, fixed arrays, another Slice, a Map,
// [time.Time], or structs of those. Slice does not check this, but registering a component that
// holds a Slice of anything else fails.
type Slice[T any] struct {
	items []T
}

// -------------------------------------------------------------------------------------------------
// Construction
// -------------------------------------------------------------------------------------------------

// SliceOf returns a Slice holding a copy of items. Later changes to the caller's slice do not reach
// the returned value.
func SliceOf[T any](items ...T) Slice[T] {
	return Slice[T]{items: emptyToNil(slices.Clone(items))}
}

// Collect returns a Slice holding every element of seq, in order.
func Collect[T any](seq iter.Seq[T]) Slice[T] {
	return Slice[T]{items: emptyToNil(slices.Collect(seq))}
}

// Clone returns a copy of s that shares no array with it, including the arrays of Slices nested in
// its elements.
func (s Slice[T]) Clone() Slice[T] {
	s.cloneDeep()
	return s
}

// -------------------------------------------------------------------------------------------------
// Reading
// -------------------------------------------------------------------------------------------------

// Len returns the number of elements.
func (s Slice[T]) Len() int {
	return len(s.items)
}

// At returns the element at index i. It panics when i is out of range, like a slice.
func (s Slice[T]) At(i int) T {
	return s.items[i]
}

// All returns an iterator over index and element, in order.
func (s Slice[T]) All() iter.Seq2[int, T] {
	return slices.All(s.items)
}

// Values returns an iterator over the elements, in order.
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

// EqualFunc reports whether both Slices have the same length and eq holds for every pair of elements
// at the same index.
func (s Slice[T]) EqualFunc(other Slice[T], eq func(a, b T) bool) bool {
	return slices.EqualFunc(s.items, other.items, eq)
}

// Equal reports whether a and b hold the same elements in the same order. Like [slices.Equal] it is
// a function, because a method cannot narrow T to comparable.
func Equal[T comparable](a, b Slice[T]) bool {
	return slices.Equal(a.items, b.items)
}

// Index returns the index of the first element of s equal to v, or -1.
func Index[T comparable](s Slice[T], v T) int {
	return slices.Index(s.items, v)
}

// Contains reports whether v is an element of s.
func Contains[T comparable](s Slice[T], v T) bool {
	return slices.Contains(s.items, v)
}

// -------------------------------------------------------------------------------------------------
// Editing
// -------------------------------------------------------------------------------------------------

// Set replaces the element at index i with v. It panics when i is out of range, like a slice.
func (s *Slice[T]) Set(i int, v T) {
	s.items[i] = v
}

// Append adds items at the end. Like append, it grows the array only when it is full, so appending
// in a loop is linear.
func (s *Slice[T]) Append(items ...T) {
	s.items = append(s.items, items...)
}

// Insert inserts items at index i, which may equal Len. It panics when i is out of range, like
// [slices.Insert].
func (s *Slice[T]) Insert(i int, items ...T) {
	s.items = slices.Insert(s.items, i, items...)
}

// Delete removes the elements in [i, j). It panics when the range is out of bounds, like
// [slices.Delete].
func (s *Slice[T]) Delete(i, j int) {
	s.items = emptyToNil(slices.Delete(s.items, i, j))
}

// DeleteFunc removes every element for which del is true, keeping the rest in order.
func (s *Slice[T]) DeleteFunc(del func(T) bool) {
	s.items = emptyToNil(slices.DeleteFunc(s.items, del))
}

// Reset removes every element, like [bytes.Buffer.Reset]. It is not Go's clear, which zeroes the
// elements and keeps the length.
func (s *Slice[T]) Reset() {
	s.items = nil
}

// SortStableFunc sorts the elements by compare, which reports a<b as negative, a==b as zero and a>b
// as positive. Equal elements keep their order, so the result depends only on the elements and never
// on the algorithm a Go release sorts with. There is no unstable sort for that reason: a world must
// sort the same way on every machine and build.
func (s *Slice[T]) SortStableFunc(compare func(a, b T) int) {
	slices.SortStableFunc(s.items, compare)
}

// Sort sorts the elements of s in ascending order. It is stable for the same reason SortStableFunc
// is, which matters for floats: -0.0 and 0.0 compare equal, so an unstable sort may order them either
// way.
func Sort[T cmp.Ordered](s *Slice[T]) {
	slices.SortStableFunc(s.items, cmp.Compare[T])
}

// Reverse reverses the order of the elements.
func (s *Slice[T]) Reverse() {
	slices.Reverse(s.items)
}

func emptyToNil[T any](items []T) []T {
	if len(items) == 0 {
		return nil
	}
	return items
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
	s.items = emptyToNil(items)
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
		// DecodeValue, not Decode: Decode handles *float32 itself and skips decoders registered with
		// msgpack.Register, such as cardinal's, which accepts a 64-bit float into a float32.
		if err := dec.DecodeValue(reflect.ValueOf(&item).Elem()); err != nil {
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
