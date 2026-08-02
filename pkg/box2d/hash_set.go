// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/table.c, src/table.h.

package box2d

// setItem is a single slot in an open-addressing hash set (upstream b2SetItem).
type setItem struct {
	key uint64
}

// hashSet is an open-addressing hash set using the Murmur hash and linear probing (upstream b2HashSet).
type hashSet struct {
	items    []setItem
	capacity uint32
	count    uint32
}

func createHashSet(capacity int) hashSet {
	set := hashSet{}

	if capacity > 16 {
		set.capacity = uint32(roundUpPowerOf2(capacity))
	} else {
		set.capacity = 16
	}

	set.count = 0
	set.items = make([]setItem, set.capacity)

	return set
}

func destroyHashSet(set *hashSet) {
	set.items = nil
	set.count = 0
	set.capacity = 0
}

func clearHashSet(set *hashSet) {
	set.count = 0
	for i := range set.capacity {
		set.items[i].key = 0
	}
}

// shapePairKey builds a canonical 64-bit key from two 32-bit identifiers (upstream B2_SHAPE_PAIR_KEY).
func shapePairKey(k1, k2 uint32) uint64 {
	if k1 < k2 {
		return (uint64(k1) << 32) | uint64(k2)
	}
	return (uint64(k2) << 32) | uint64(k1)
}

func keyHash(key uint64) uint64 {
	// Murmur hash
	h := key
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	h *= 0xc4ceb9fe1a85ec53
	h ^= h >> 33
	return h
}

func findSlot(set *hashSet, key uint64, hash uint64) uint32 {
	capacity := set.capacity
	index := uint32(hash) & (capacity - 1)
	items := set.items
	for items[index].key != 0 && items[index].key != key {
		index = (index + 1) & (capacity - 1)
	}

	return index
}

func addKeyHaveCapacity(set *hashSet, key uint64, hash uint64) {
	index := findSlot(set, key, hash)
	items := set.items

	assert(items[index].key == 0)
	items[index].key = key
	set.count++
}

func growTable(set *hashSet) {
	oldCount := set.count
	oldCapacity := set.capacity
	oldItems := set.items

	set.count = 0
	set.capacity = 2 * oldCapacity
	set.items = make([]setItem, set.capacity)

	for i := range oldCapacity {
		item := &oldItems[i]
		if item.key == 0 {
			continue
		}

		hash := keyHash(item.key)
		addKeyHaveCapacity(set, item.key, hash)
	}

	assert(set.count == oldCount)
}

func containsKey(set *hashSet, key uint64) bool {
	assert(key != 0)
	hash := keyHash(key)
	index := findSlot(set, key, hash)
	return set.items[index].key == key
}

func getHashSetBytes(set *hashSet) int {
	return int(set.capacity) * 8
}

func addKey(set *hashSet, key uint64) bool {
	assert(key != 0)

	hash := keyHash(key)
	assert(hash != 0)

	index := findSlot(set, key, hash)
	if set.items[index].key != 0 {
		assert(set.items[index].key == key)
		return true
	}

	if 2*set.count >= set.capacity {
		growTable(set)
	}

	addKeyHaveCapacity(set, key, hash)
	return false
}

// removeKey deletes a key using backward-shift deletion.
// See: https://en.wikipedia.org/wiki/Open_addressing.
func removeKey(set *hashSet, key uint64) bool {
	hash := keyHash(key)
	i := findSlot(set, key, hash)
	items := set.items
	if items[i].key == 0 {
		return false
	}

	items[i].key = 0

	assert(set.count > 0)
	set.count--

	j := i
	capacity := set.capacity
	for {
		j = (j + 1) & (capacity - 1)
		if items[j].key == 0 {
			break
		}

		hashJ := keyHash(items[j].key)
		k := uint32(hashJ) & (capacity - 1)

		if i <= j {
			if i < k && k <= j {
				continue
			}
		} else {
			if i < k || k <= j {
				continue
			}
		}

		items[i] = items[j]
		items[j].key = 0

		i = j
	}

	return true
}

func getSetCount(set *hashSet) int {
	return int(set.count)
}

func getSetCapacity(set *hashSet) int {
	return int(set.capacity)
}
