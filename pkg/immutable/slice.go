// Package immutable holds value types whose contents cannot change in place. A value is built once,
// read freely, and replaced as a whole when something new is needed. That is what lets a component
// hold one without breaking the rule that world state only ever changes through Set.
package immutable

import (
	"iter"

	"github.com/goccy/go-json"
)

// Slice is an immutable sequence for component fields whose length is genuinely unbounded. It is
// the only variable-length collection a component may hold.
//
// A component column stores values, and Get hands back a copy, but a raw []T inside that copy still
// shares its backing array with the column. Writing through it changes the live world without a Set,
// which breaks the rule that snapshots only ever see state that went through Set. Slice closes that
// hole by construction: the backing array is unexported, every constructor copies its input, every
// reader returns a copy, and nothing mutates in place. Sharing the backing array between copies is
// therefore safe, so Get stays zero-copy.
//
// To change a Slice, derive a new one and Set the component that holds it:
//
//	inv := ref.Get()
//	inv.Items = inv.Items.With(0, item)
//	ref.Set(inv)
//
// The element type must itself be value-safe: scalars, strings, fixed arrays, or structs of those.
// The wire generator enforces that rule.
//
// A Slice directly inside a Slice has no protobuf form, because a repeated field cannot hold another
// repeated field. For jagged rows, put the inner Slice in a named struct:
//
//	type Row struct{ Cells Slice[int32] }
//	type Board struct{ Rows Slice[Row] }
type Slice[T any] struct {
	items []T
}

// SliceOf returns a Slice holding a copy of items. Later changes to the caller's slice do not
// reach the returned value.
func SliceOf[T any](items ...T) Slice[T] {
	if len(items) == 0 {
		return Slice[T]{}
	}
	out := make([]T, len(items))
	copy(out, items)
	return Slice[T]{items: out}
}

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
	return func(yield func(int, T) bool) {
		for i, v := range s.items {
			if !yield(i, v) {
				return
			}
		}
	}
}

// Clone returns a fresh []T holding a copy of the elements. Use it when an API needs a plain slice;
// changes to the result never reach the Slice.
func (s Slice[T]) Clone() []T {
	out := make([]T, len(s.items))
	copy(out, s.items)
	return out
}

// Append returns a new Slice with items added at the end. The receiver is unchanged.
func (s Slice[T]) Append(items ...T) Slice[T] {
	if len(items) == 0 {
		return s
	}
	out := make([]T, len(s.items)+len(items))
	copy(out, s.items)
	copy(out[len(s.items):], items)
	return Slice[T]{items: out}
}

// With returns a new Slice with the element at index i replaced by v. The receiver is unchanged.
// It panics when i is out of range, like a slice.
func (s Slice[T]) With(i int, v T) Slice[T] {
	out := s.Clone()
	out[i] = v
	return Slice[T]{items: out}
}

// EqualFunc reports whether both slices have the same length and eq holds for every pair of
// elements at the same index.
func (s Slice[T]) EqualFunc(other Slice[T], eq func(a, b T) bool) bool {
	if len(s.items) != len(other.items) {
		return false
	}
	for i := range s.items {
		if !eq(s.items[i], other.items[i]) {
			return false
		}
	}
	return true
}

// MarshalJSON encodes the elements as a JSON array. An empty Slice encodes as [] rather than null.
func (s Slice[T]) MarshalJSON() ([]byte, error) {
	if len(s.items) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(s.items)
}

// UnmarshalJSON decodes a JSON array into the Slice. JSON null decodes to an empty Slice.
func (s *Slice[T]) UnmarshalJSON(data []byte) error {
	var items []T
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	if len(items) == 0 {
		// One representation for empty. A decoded [] would otherwise hold an empty non-nil backing
		// slice while SliceOf() holds nil, and reflect.DeepEqual (so require.Equal on a component)
		// would call two empty Slices different.
		items = nil
	}
	s.items = items
	return nil
}
