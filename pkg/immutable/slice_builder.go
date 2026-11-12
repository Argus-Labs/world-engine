package immutable

import "slices"

// SliceBuilder applies a batch of edits to one private array and hands it over as a Slice with Done.
// It exists so many edits cost one copy instead of one copy each:
//
//	b := inv.Items.Edit()
//	b.Append(item)
//	b.Delete(0, 1)
//	inv.Items = b.Done()
//
// A builder is used through the pointer Edit and NewSliceBuilder return. After Done it holds nothing,
// and every method panics: the array now belongs to the Slice, and a builder that could still write
// would break that Slice's promise that its array never changes. The check is a plain panic rather
// than assert.That because it has to hold in release builds.
type SliceBuilder[T any] struct {
	items []T
	done  bool
}

// NewSliceBuilder returns an empty builder with room for capacity elements, for a list rebuilt from
// scratch. It panics when capacity is negative, like make.
func NewSliceBuilder[T any](capacity int) *SliceBuilder[T] {
	return &SliceBuilder[T]{items: make([]T, 0, capacity)}
}

// Edit returns a builder holding a copy of the Slice's elements. The Slice itself is unchanged.
func (s Slice[T]) Edit() *SliceBuilder[T] {
	return &SliceBuilder[T]{items: slices.Clone(s.items)}
}

// Len returns the number of elements.
func (b *SliceBuilder[T]) Len() int {
	b.check()
	return len(b.items)
}

// At returns a copy of the element at index i. It panics when i is out of range, like a slice.
func (b *SliceBuilder[T]) At(i int) T {
	b.check()
	return b.items[i]
}

// Set replaces the element at index i. It panics when i is out of range, like a slice.
func (b *SliceBuilder[T]) Set(i int, v T) {
	b.check()
	b.items[i] = v
}

// Append adds items at the end.
func (b *SliceBuilder[T]) Append(items ...T) {
	b.check()
	b.items = append(b.items, items...)
}

// Insert inserts items at index i, which may equal Len. It panics when i is out of range, like
// [slices.Insert].
func (b *SliceBuilder[T]) Insert(i int, items ...T) {
	b.check()
	b.items = slices.Insert(b.items, i, items...)
}

// Delete removes the elements in [i, j). It panics when the range is out of bounds, like
// [slices.Delete].
func (b *SliceBuilder[T]) Delete(i, j int) {
	b.check()
	b.items = slices.Delete(b.items, i, j)
}

// Done returns the built Slice and empties the builder, which panics if used again.
func (b *SliceBuilder[T]) Done() Slice[T] {
	b.check()
	items := b.items
	b.items, b.done = nil, true
	return wrap(items)
}

func (b *SliceBuilder[T]) check() {
	if b.done {
		panic("immutable: SliceBuilder used after Done")
	}
}
