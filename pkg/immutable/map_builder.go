package immutable

import (
	"slices"
)

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
// Edit copies the values once. The keys stay shared with the source until the first insert or
// delete, so a batch that only changes existing entries copies one array, not two.
//
// After Done every method panics, for the reason SliceBuilder gives.
type MapBuilder[K Key, V any] struct {
	keys     []K
	vals     []V
	ownsKeys bool
	done     bool
}

// Edit returns a builder holding a copy of the Map's entries. The Map itself is unchanged.
func (m Map[K, V]) Edit() *MapBuilder[K, V] {
	return &MapBuilder[K, V]{keys: m.keys, vals: slices.Clone(m.vals)}
}

// Len returns the number of entries.
func (b *MapBuilder[K, V]) Len() int {
	b.check()
	return len(b.keys)
}

// Get returns a copy of the value under k, and whether k is present, pending edits included.
func (b *MapBuilder[K, V]) Get(k K) (V, bool) {
	b.check()
	if i, ok := slices.BinarySearch(b.keys, k); ok {
		return b.vals[i], true
	}
	var zero V
	return zero, false
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
		b.vals[i] = v
		return
	}
	b.ownKeys()
	b.keys = slices.Insert(b.keys, i, k)
	b.vals = slices.Insert(b.vals, i, v)
}

// Delete removes k. It does nothing when k is absent.
func (b *MapBuilder[K, V]) Delete(k K) {
	b.check()
	i, ok := slices.BinarySearch(b.keys, k)
	if !ok {
		return
	}
	b.ownKeys()
	b.keys = slices.Delete(b.keys, i, i+1)
	b.vals = slices.Delete(b.vals, i, i+1)
}

// Done returns the built Map and empties the builder, which panics if used again.
func (b *MapBuilder[K, V]) Done() Map[K, V] {
	b.check()
	keys, vals := b.keys, b.vals
	b.keys, b.vals, b.done = nil, nil, true
	if len(keys) == 0 {
		return Map[K, V]{}
	}
	return Map[K, V]{keys: keys, vals: vals}
}

// ownKeys copies the keys before the first write to them, since until then they are the source Map's.
func (b *MapBuilder[K, V]) ownKeys() {
	if !b.ownsKeys {
		b.keys = slices.Clone(b.keys)
		b.ownsKeys = true
	}
}

func (b *MapBuilder[K, V]) check() {
	if b.done {
		panic("immutable: MapBuilder used after Done")
	}
}
