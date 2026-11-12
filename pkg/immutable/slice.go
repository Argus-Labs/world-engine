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
	"unsafe"

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
// O(n) as it does for a Go slice. Two deletes skip the rebuild: a run ending at the last element that
// starts in the tail or the last chunk, and a run from the front of a Slice of up to 32 elements. The
// sorts cost O(n log n).
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

// Range returns an iterator over index and element for the indices in [i, j), in order. It is
// ranging over s[i:j], except that each index is the element's index in s. It goes straight to i, so
// reading k elements costs O(log₃₂ n + k), where calling At for each costs O(k log₃₂ n). The loop
// panics when the range is out of bounds, like slicing.
func (s Slice[T]) Range(i, j int) iter.Seq2[int, T] {
	// The work, bounds check included, lives in rangeChunks so that Range inlines and the iterator
	// does not escape: Range allocates nothing, like the other reads.
	return func(yield func(int, T) bool) { s.rangeChunks(i, j, yield) }
}

func (s Slice[T]) rangeChunks(i, j int, yield func(int, T) bool) {
	if i < 0 || j > s.n || i > j {
		panic(rangeError(i, j, s.n))
	}

	off := s.treeLen()

	for x := i; x < j; {
		var c []T
		if x >= off {
			c = s.tail[x-off : j-off]
		} else {
			leafStart := x &^ chunkMask
			c = s.leafAt(x)[x-leafStart : min(chunkSize, j-leafStart)]
		}

		for _, v := range c {
			if !yield(x, v) {
				return
			}

			x++
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
// at the same index. Storage that both share counts as equal without calling eq, as in
// [reflect.DeepEqual], so comparing a value with an unchanged copy is O(1) and with an edited copy
// costs about one chunk. A NaN held in shared storage therefore compares equal.
func (s Slice[T]) EqualFunc(other Slice[T], eq func(a, b T) bool) bool {
	if s.n != other.n {
		return false
	}
	// Equal lengths give the same shape, so the two trees line up node for node.
	if s.root != nil && !vnodesEqual(s.root, other.root, s.shift, eq) {
		return false
	}

	return itemsEqual(s.tail, other.tail, eq)
}

func vnodesEqual[T any](a, b *vnode[T], shift uint, eq func(a, b T) bool) bool {
	if a == b {
		return true
	}

	if shift == 0 {
		return itemsEqual(a.items, b.items, eq)
	}

	for i, kid := range a.kids {
		if !vnodesEqual(kid, b.kids[i], shift-chunkBits, eq) {
			return false
		}
	}

	return true
}

// itemsEqual compares two chunks of the same length. The same first address means shared storage,
// except for a zero-size element type, where Go may give separate allocations the same address.
func itemsEqual[T any](a, b []T, eq func(a, b T) bool) bool {
	if len(a) == 0 || unsafe.Sizeof(a[0]) != 0 && &a[0] == &b[0] {
		return true
	}

	for i := range a {
		if !eq(a[i], b[i]) {
			return false
		}
	}

	return true
}

// Equal reports whether a and b hold the same elements in the same order. Like [slices.Equal] it is
// a function, because a method cannot narrow T to comparable. Shared storage counts as equal, as in
// EqualFunc.
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
		*s = fromItems(shrink(kept))
	}
}

// shrink returns kept, or a copy of it when it uses less than half its array. The result's chunks
// are windows onto the array, so without the copy a few survivors of a large DeleteFunc would keep
// the whole array alive.
func shrink[T any](kept []T) []T {
	if cap(kept) > 2*len(kept) {
		return slices.Clone(kept)
	}

	return kept
}

// UpdateFunc replaces each element v at index i with f(i, v), in order. It rebuilds the Slice once,
// O(n) with about one allocation per 32 elements, where calling Set for every element copies a
// chunk and its path each time. f sees the Slice as it was before the call, and a panic in f leaves
// the Slice unchanged.
func (s *Slice[T]) UpdateFunc(f func(i int, v T) T) {
	if s.n == 0 {
		return
	}

	items := make([]T, 0, s.n)
	for c := range s.chunks() {
		for _, v := range c {
			items = append(items, f(len(items), v))
		}
	}

	*s = fromItems(items)
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
	sortStable(items, compare)
	*s = fromItems(items)
}

// smallSort is the length up to which Go's stable sort is a plain insertion sort.
const smallSort = 20

// sortStable sorts items by compare, keeping equal elements in their original order, in
// O(n log n). Go's [slices.SortStableFunc] sorts in place without a buffer, which costs O(n log² n)
// moves. This sorts the positions instead, with [slices.SortFunc], and breaks ties by position: no two
// positions then compare equal, so there is exactly one correct order, and any correct sort finds it
// on every machine and Go release. The elements then move into place once, each moved one time.
func sortStable[T any](items []T, compare func(a, b T) int) {
	if len(items) <= smallSort {
		// Go's stable sort is an insertion sort at this size, faster than sorting positions.
		slices.SortStableFunc(items, compare)
		return
	}

	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}

	slices.SortFunc(order, func(a, b int) int {
		if c := compare(items[a], items[b]); c != 0 {
			return c
		}

		return cmp.Compare(a, b)
	})
	// items[j] must become the old items[order[j]]. Follow each cycle of the permutation once,
	// marking the slots it fills.
	for i := range order {
		if order[i] < 0 {
			continue
		}

		first, j := items[i], i
		for {
			k := order[j]

			order[j] = -1
			if k == i {
				items[j] = first
				break
			}

			items[j] = items[k]
			j = k
		}
	}
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
	// Encode(item) boxes each element into an interface, an allocation per element. EncodeValue on
	// one reused variable reaches the same encoders, so the bytes are the same, and allocates once.
	// It encodes a copy rather than the element in place, so an encoder with a pointer receiver
	// cannot write into storage other Slices share.
	var item T

	rv := reflect.ValueOf(&item).Elem()

	for c := range s.chunks() {
		for i := range c {
			item = c[i]

			if err := enc.EncodeValue(rv); err != nil {
				return eris.Wrap(err, "failed to encode immutable slice element")
			}
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
	// Each element decodes in place in the array, so decoding allocates per array growth, not per
	// element. The length comes off the wire, so it only sizes the first allocation up to a cap; a
	// forged length cannot force a large one.
	items := make([]T, 0, min(max(n, 0), decodePrealloc))
	for range n {
		var zero T

		items = append(items, zero)
		// DecodeValue, not Decode: Decode handles basic types such as *float32 and *uint8 itself and
		// skips decoders registered with msgpack.Register, such as cardinal's, which accepts a 64-bit
		// float into a float32 and rejects out-of-range integers.
		if err := dec.DecodeValue(reflect.ValueOf(&items[len(items)-1]).Elem()); err != nil {
			return eris.Wrap(err, "failed to decode immutable slice element")
		}
	}

	*s = fromItems(items)

	return nil
}

// decodePrealloc caps how many elements or entries a decoder allocates room for before any arrive.
const decodePrealloc = 1024

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
