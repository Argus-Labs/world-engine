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
// A builder from Edit shares the Slice's array until its first write, then copies it once, sized for
// that write: a batch of Sets or Deletes stores an exact-size array, and Append and Insert grow it
// from there as append does. A builder that never writes copies nothing.
//
// A builder is used through the pointer Edit and NewSliceBuilder return. After Done it holds nothing,
// and every method panics: the array now belongs to the Slice, and a builder that could still write
// would break that Slice's promise that its array never changes. The check is a plain panic rather
// than assert.That because it has to hold in release builds. The same goes for a copy of a builder:
// it would share the array with the original and so with the Slice the original builds, so every
// method on a copy panics too, as on a copied [strings.Builder].
type SliceBuilder[T any] struct {
	self  *SliceBuilder[T] // the builder's own address; differs in a copy
	items []T
	owns  bool // items is this builder's own array, not the source Slice's
	done  bool
}

// NewSliceBuilder returns an empty builder with room for capacity elements, for a list rebuilt from
// scratch. It panics when capacity is negative, like make.
func NewSliceBuilder[T any](capacity int) *SliceBuilder[T] {
	b := &SliceBuilder[T]{items: make([]T, 0, capacity), owns: true}
	b.self = b
	return b
}

// Edit returns a builder that starts from the Slice's elements. The Slice itself is unchanged.
func (s Slice[T]) Edit() *SliceBuilder[T] {
	b := &SliceBuilder[T]{items: s.items}
	b.self = b
	return b
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
	_ = b.items[i] // panic before copying
	b.items = own(b.items, &b.owns)
	b.items[i] = v
}

// Append adds items at the end.
func (b *SliceBuilder[T]) Append(items ...T) {
	b.check()
	b.items = spliceIn(b.items, &b.owns, len(b.items), items...)
}

// Insert inserts items at index i, which may equal Len. It panics when i is out of range, like
// [slices.Insert].
func (b *SliceBuilder[T]) Insert(i int, items ...T) {
	b.check()
	b.items = spliceIn(b.items, &b.owns, i, items...)
}

// Delete removes the elements in [i, j). It panics when the range is out of bounds, like
// [slices.Delete].
func (b *SliceBuilder[T]) Delete(i, j int) {
	b.check()
	b.items = spliceOut(b.items, &b.owns, i, j)
}

// Done returns the built Slice and empties the builder, which panics if used again.
func (b *SliceBuilder[T]) Done() Slice[T] {
	b.check()
	items := b.items
	b.items, b.done = nil, true
	return wrap(items)
}

// own, spliceIn and spliceOut are the builders' writes. Until *owns is set, s is the source's array,
// so the first write copies it, in one step sized for that write; after that they edit s in place.
func own[T any](s []T, owns *bool) []T {
	if *owns {
		return s
	}
	*owns = true
	return slices.Clone(s)
}

func spliceIn[T any](s []T, owns *bool, i int, items ...T) []T {
	if *owns {
		return slices.Insert(s, i, items...)
	}
	*owns = true
	return slices.Concat(s[:i], items, s[i:])
}

func spliceOut[T any](s []T, owns *bool, i, j int) []T {
	if *owns {
		return slices.Delete(s, i, j)
	}
	_ = s[i:j] // s[:i] and s[j:] alone would accept i > j
	*owns = true
	return slices.Concat(s[:i], s[j:])
}

func (b *SliceBuilder[T]) check() {
	switch {
	case b.self == nil: // a zero SliceBuilder, first use
		b.self = b
	case b.self != b:
		panic("immutable: SliceBuilder copied by value; use the pointer Edit or NewSliceBuilder returned")
	}
	if b.done {
		panic("immutable: SliceBuilder used after Done")
	}
}
