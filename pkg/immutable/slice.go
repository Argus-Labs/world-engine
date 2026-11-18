// Package immutable holds the collections a component may hold: Slice and Map. Both keep their
// storage private, so nothing outside this package can reach into one.
//
// Both are edited in place, like a Go slice and map, but their storage never changes once a value
// holds it: an edit copies the small part it touches. Copying a Slice or Map is therefore free and
// safe, which is what lets Cardinal pass them in and out of the world without copying them.
package immutable

import (
	"cmp"
	"iter"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
	"github.com/vmihailenco/msgpack/v5"
)

// Slice is a list with private storage, for component fields whose length is genuinely unbounded.
// It is the only variable-length list a component may hold.
//
// Edit it in place, the way you would a Go slice, and store it with Set:
//
//	inv := ref.Get()
//	inv.Items.Append(item)
//	inv.Items.Set(0, other)
//	ref.Set(inv)
//
// The storage underneath never changes once a Slice holds it: an edit copies only the small part it
// touches and leaves every other Slice that shared it as it was. So copying a Slice is free and
// always safe. d := inv.Items and then editing d never changes inv.Items, a loop over a Slice sees it
// as it was when the loop started, and Cardinal hands Slices in and out of the world without copying
// them. An edit reaches the world only through Set.
//
// Reading an element is a lookup in a tree of 32-element chunks: direct for the first 64 elements,
// one more step per 32x after that. An edit copies one chunk of up to 32 elements and the path above
// it; Append copies the last chunk. Insert, Delete and DeleteFunc rebuild the Slice, which costs
// O(n) as it does for a Go slice, except that deleting from the end only drops the tail or the last
// chunk. The sorts cost O(n log n).
//
// Every empty Slice is the zero value, and equal Slices have the same storage layout, so
// [reflect.DeepEqual] compares Slices by their elements however they were built.
//
// The element type must be value-safe too: scalars, strings, fixed arrays, another Slice, a Map,
// [time.Time], or structs of those. Slice does not check this, but registering a component that
// holds a Slice of anything else fails.
type Slice[T any] struct {
	root  *vnode[T] // full chunks before the tail; nil when there are none
	tail  []T       // the last ((n-1) mod 32)+1 elements; nil when empty
	n     int
	shift uint // levels above the leaves in root, times chunkBits
}

// -------------------------------------------------------------------------------------------------
// Construction
// -------------------------------------------------------------------------------------------------

// SliceOf returns a Slice holding a copy of items. Later changes to the caller's slice do not reach
// the returned value.
func SliceOf[T any](items ...T) Slice[T] {
	return fromItems(slices.Clone(items))
}

// Collect returns a Slice holding every element of seq, in order.
func Collect[T any](seq iter.Seq[T]) Slice[T] {
	return fromItems(slices.Collect(seq))
}

// -------------------------------------------------------------------------------------------------
// Reading
// -------------------------------------------------------------------------------------------------

// Len returns the number of elements.
func (s Slice[T]) Len() int {
	return s.n
}

// At returns the element at index i. It panics when i is out of range, like a slice.
func (s Slice[T]) At(i int) T {
	s.checkIndex(i)
	if off := s.treeLen(); i >= off {
		return s.tail[i-off]
	}
	return s.leafAt(i)[i&chunkMask]
}

// All returns an iterator over index and element, in order.
func (s Slice[T]) All() iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		i := 0
		for c := range s.chunks() {
			for _, v := range c {
				if !yield(i, v) {
					return
				}
				i++
			}
		}
	}
}

// Values returns an iterator over the elements, in order.
func (s Slice[T]) Values() iter.Seq[T] {
	return func(yield func(T) bool) {
		for c := range s.chunks() {
			for _, v := range c {
				if !yield(v) {
					return
				}
			}
		}
	}
}

// Backward returns an iterator over index and element from the last element to the first.
func (s Slice[T]) Backward() iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		i := s.n
		for c := range s.backwardChunks() {
			for _, v := range slices.Backward(c) {
				i--
				if !yield(i, v) {
					return
				}
			}
		}
	}
}

// IndexFunc returns the index of the first element for which f is true, or -1.
func (s Slice[T]) IndexFunc(f func(T) bool) int {
	for i, v := range s.All() {
		if f(v) {
			return i
		}
	}
	return -1
}

// ContainsFunc reports whether f is true for any element.
func (s Slice[T]) ContainsFunc(f func(T) bool) bool {
	return s.IndexFunc(f) >= 0
}

// EqualFunc reports whether both Slices have the same length and eq holds for every pair of elements
// at the same index.
func (s Slice[T]) EqualFunc(other Slice[T], eq func(a, b T) bool) bool {
	if s.n != other.n {
		return false
	}
	for i, v := range s.All() {
		if !eq(v, other.At(i)) {
			return false
		}
	}
	return true
}

// Equal reports whether a and b hold the same elements in the same order. Like [slices.Equal] it is
// a function, because a method cannot narrow T to comparable.
func Equal[T comparable](a, b Slice[T]) bool {
	return a.EqualFunc(b, func(x, y T) bool { return x == y })
}

// Index returns the index of the first element of s equal to v, or -1.
func Index[T comparable](s Slice[T], v T) int {
	return s.IndexFunc(func(x T) bool { return x == v })
}

// Contains reports whether v is an element of s.
func Contains[T comparable](s Slice[T], v T) bool {
	return Index(s, v) >= 0
}

// -------------------------------------------------------------------------------------------------
// Editing
// -------------------------------------------------------------------------------------------------

// Set replaces the element at index i with v. It panics when i is out of range, like a slice.
func (s *Slice[T]) Set(i int, v T) {
	s.checkIndex(i)
	if off := s.treeLen(); i >= off {
		tail := slices.Clone(s.tail)
		tail[i-off] = v
		s.tail = tail
		return
	}
	s.root = setInTree(s.root, s.shift, i, v)
}

// Append adds items at the end. It copies the last chunk of up to 32 elements, so appending in a loop
// is linear.
func (s *Slice[T]) Append(items ...T) {
	for len(items) > 0 {
		if len(s.tail) == chunkSize {
			s.pushLeaf(leafNode(s.tail))
			s.tail = nil
		}
		take := min(chunkSize-len(s.tail), len(items))
		tail := make([]T, len(s.tail)+take)
		copy(tail, s.tail)
		copy(tail[len(s.tail):], items[:take])
		s.tail = tail
		s.n += take
		items = items[take:]
	}
}

// Insert inserts items at index i, which may equal Len. It panics when i is out of range, like
// [slices.Insert].
func (s *Slice[T]) Insert(i int, items ...T) {
	if i < 0 || i > s.n {
		panic(indexError(i, s.n+1))
	}
	if i == s.n {
		s.Append(items...)
		return
	}
	if len(items) == 0 {
		return
	}
	all := s.toItems(len(items))
	*s = fromItems(slices.Insert(all, i, items...))
}

// Delete removes the elements in [i, j). It panics when the range is out of bounds, like
// [slices.Delete].
func (s *Slice[T]) Delete(i, j int) {
	if i < 0 || j > s.n || i > j {
		panic(rangeError(i, j, s.n))
	}
	if i == j {
		return
	}
	if s.root == nil && i == 0 {
		// Every element is in the tail, which stays canonical as a later view of the same array.
		s.tail = s.tail[j:]
		s.n -= j
		if s.n == 0 {
			s.tail = nil
		}
		return
	}
	if off := s.treeLen(); j == s.n && i > off {
		// Only the end of the tail goes, and some of it stays: a shorter view of the same tail.
		s.tail = s.tail[: i-off : i-off]
		s.n = i
		return
	}
	if off := s.treeLen(); j == s.n && s.root != nil && i > off-chunkSize {
		// The tail goes and the cut lands in the last leaf, which becomes the tail.
		s.tail = s.popLeaf()[: i-(off-chunkSize) : i-(off-chunkSize)]
		s.n = i
		return
	}
	kept := make([]T, 0, s.n-(j-i))
	k := 0
	for c := range s.chunks() {
		for _, v := range c {
			if k < i || k >= j {
				kept = append(kept, v)
			}
			k++
		}
	}
	*s = fromItems(kept)
}

// DeleteFunc removes every element for which del is true, keeping the rest in order.
func (s *Slice[T]) DeleteFunc(del func(T) bool) {
	var kept []T
	k := 0
	deleted := false
	for c := range s.chunks() {
		for _, v := range c {
			switch {
			case del(v):
				if !deleted {
					deleted = true
					kept = make([]T, 0, s.n-1)
					kept = appendFirst(kept, *s, k)
				}
			case deleted:
				kept = append(kept, v)
			}
			k++
		}
	}
	if deleted {
		*s = fromItems(kept)
	}
}

// appendFirst appends the first k elements of s to dst.
func appendFirst[T any](dst []T, s Slice[T], k int) []T {
	for c := range s.chunks() {
		if k <= len(c) {
			return append(dst, c[:k]...)
		}
		dst = append(dst, c...)
		k -= len(c)
	}
	return dst
}

// Reset removes every element, like [bytes.Buffer.Reset]. It is not Go's clear, which zeroes the
// elements and keeps the length.
func (s *Slice[T]) Reset() {
	*s = Slice[T]{}
}

// SortStableFunc sorts the elements by compare, which reports a<b as negative, a==b as zero and a>b
// as positive. Equal elements keep their order, so the result depends only on the elements and never
// on the algorithm a Go release sorts with. There is no unstable sort for that reason: a world must
// sort the same way on every machine and build.
func (s *Slice[T]) SortStableFunc(compare func(a, b T) int) {
	if s.n < 2 {
		return
	}
	items := s.toItems(0)
	slices.SortStableFunc(items, compare)
	*s = fromItems(items)
}

// Sort sorts the elements of s in ascending order. It is stable for the same reason SortStableFunc
// is, which matters for floats: -0.0 and 0.0 compare equal, so an unstable sort may order them either
// way.
func Sort[T cmp.Ordered](s *Slice[T]) {
	s.SortStableFunc(cmp.Compare[T])
}

// Reverse reverses the order of the elements.
func (s *Slice[T]) Reverse() {
	if s.n < 2 {
		return
	}
	items := s.toItems(0)
	slices.Reverse(items)
	*s = fromItems(items)
}

func (s Slice[T]) checkIndex(i int) {
	if i < 0 || i >= s.n {
		panic(indexError(i, s.n))
	}
}

func indexError(i, n int) string {
	return "immutable: index " + strconv.Itoa(i) + " out of range with length " + strconv.Itoa(n)
}

func rangeError(i, j, n int) string {
	return "immutable: range [" + strconv.Itoa(i) + ":" + strconv.Itoa(j) + "] out of range with length " +
		strconv.Itoa(n)
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
	if s.n == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(s.toItems(0))
}

// UnmarshalJSON replaces the Slice with the array in data. An empty array and `null` both yield the
// zero value — nil items, not an empty allocation — so a decoded empty Slice compares equal to a
// fresh one under [reflect.DeepEqual], the same rule DecodeMsgpack follows.
func (s *Slice[T]) UnmarshalJSON(data []byte) error {
	var items []T
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	*s = fromItems(items)
	return nil
}

// EncodeMsgpack encodes a Slice as an array, including empty and byte-valued Slices.
func (s Slice[T]) EncodeMsgpack(enc *msgpack.Encoder) error {
	if err := enc.EncodeArrayLen(s.n); err != nil {
		return eris.Wrap(err, "failed to encode immutable slice length")
	}
	for item := range s.Values() {
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
	*s = fromItems(items)
	return nil
}

// IsZero lets MessagePack's omitempty distinguish populated Slices from empty ones.
func (s Slice[T]) IsZero() bool { return s.n == 0 }

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
	tail, ok := t.FieldByName("tail")
	if !ok || tail.Type.Kind() != reflect.Slice {
		return nil, false
	}
	return tail.Type.Elem(), true
}
