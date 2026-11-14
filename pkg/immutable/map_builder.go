package immutable

import "slices"

// MapBuilder applies a batch of edits to private copies of a Map's arrays and hands them over as a
// Map with Done. Its Get and Has see the pending edits, so a batch can accumulate:
//
//	b := c.Table.Edit()
//	for _, hit := range hits {
//		v, _ := b.Get(hit.Attacker)
//		b.Set(hit.Attacker, v+hit.Damage)
//	}
//	c.Table = b.Done()
//
// Edit copies nothing. Each array is copied once, on its first write: the values on the first Set,
// the keys as well on the first new key or Delete. A batch that only changes existing entries
// copies one array, and every copy is sized for the write that made it.
//
// Each new key or Delete shifts the arrays, so k of them in an n-entry Map cost O(k·n), as in any
// map stored as sorted arrays. To build a large Map from scratch, use MapFrom, which sorts once.
//
// After Done, and on a copy of the builder, every method panics, for the reasons SliceBuilder gives.
type MapBuilder[K Key, V any] struct {
	self     *MapBuilder[K, V] // the builder's own address; differs in a copy
	keys     []K
	vals     []V
	ownsKeys bool // keys is this builder's own array, not the source Map's
	ownsVals bool // likewise for vals
	done     bool
}

// Edit returns a builder that starts from the Map's entries. The Map itself is unchanged.
func (m Map[K, V]) Edit() *MapBuilder[K, V] {
	b := &MapBuilder[K, V]{keys: m.keys, vals: m.vals}
	b.self = b
	return b
}

// Len returns the number of entries.
func (b *MapBuilder[K, V]) Len() int {
	b.check()
	return len(b.keys)
}

// Get returns a copy of the value under k, and whether k is present, pending edits included.
func (b *MapBuilder[K, V]) Get(k K) (V, bool) {
	b.check()
	return lookup(b.keys, b.vals, k)
}

// Has reports whether k is present, pending edits included.
func (b *MapBuilder[K, V]) Has(k K) bool {
	b.check()
	_, ok := slices.BinarySearch(b.keys, k)
	return ok
}

// Set stores v under k, replacing any value already there.
func (b *MapBuilder[K, V]) Set(k K, v V) {
	b.check()
	i, ok := slices.BinarySearch(b.keys, k)
	if ok {
		b.vals = own(b.vals, &b.ownsVals)
		b.vals[i] = v
		return
	}
	b.keys = spliceIn(b.keys, &b.ownsKeys, i, k)
	b.vals = spliceIn(b.vals, &b.ownsVals, i, v)
}

// Delete removes k. It does nothing when k is absent.
func (b *MapBuilder[K, V]) Delete(k K) {
	b.check()
	i, ok := slices.BinarySearch(b.keys, k)
	if !ok {
		return
	}
	b.keys = spliceOut(b.keys, &b.ownsKeys, i, i+1)
	b.vals = spliceOut(b.vals, &b.ownsVals, i, i+1)
}

// Done returns the built Map and empties the builder, which panics if used again.
func (b *MapBuilder[K, V]) Done() Map[K, V] {
	b.check()
	keys, vals := b.keys, b.vals
	b.keys, b.vals, b.done = nil, nil, true
	return wrapMap(keys, vals)
}

func (b *MapBuilder[K, V]) check() {
	switch {
	case b.self == nil: // a zero MapBuilder, first use
		b.self = b
	case b.self != b:
		panic("immutable: MapBuilder copied by value; use the pointer Edit returned")
	}
	if b.done {
		panic("immutable: MapBuilder used after Done")
	}
}
