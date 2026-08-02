// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/table.c, src/table.h.

package box2d

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashSet_AddContainsRemove(t *testing.T) {
	t.Parallel()

	set := createHashSet(1)

	tassert.False(t, addKey(&set, 7))
	tassert.False(t, addKey(&set, 42))
	tassert.False(t, addKey(&set, 99))

	tassert.True(t, containsKey(&set, 7))
	tassert.True(t, containsKey(&set, 42))
	tassert.True(t, containsKey(&set, 99))
	tassert.False(t, containsKey(&set, 100))

	tassert.True(t, addKey(&set, 42))
	tassert.Equal(t, 3, getSetCount(&set))

	tassert.True(t, removeKey(&set, 42))
	tassert.False(t, containsKey(&set, 42))
	tassert.False(t, removeKey(&set, 42))

	tassert.Equal(t, 2, getSetCount(&set))
}

func TestHashSet_GrowAndCapacity(t *testing.T) {
	t.Parallel()

	set := createHashSet(1)
	require.Equal(t, 16, getSetCapacity(&set))

	for i := uint64(1); i <= 20; i++ {
		tassert.False(t, addKey(&set, i))
	}

	tassert.Greater(t, getSetCapacity(&set), 16)
	tassert.Equal(t, 20, getSetCount(&set))

	for i := uint64(1); i <= 20; i++ {
		tassert.True(t, containsKey(&set, i))
	}
}

func TestHashSet_CollisionHeavyKeys(t *testing.T) {
	t.Parallel()

	set := createHashSet(16)

	const n = 200
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			key := shapePairKey(uint32(i), uint32(j))
			tassert.Falsef(t, addKey(&set, key), "duplicate at (%d,%d)", i, j)
		}
	}

	expectedCount := n * (n - 1) / 2
	tassert.Equal(t, expectedCount, getSetCount(&set))

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			key := shapePairKey(uint32(i), uint32(j))
			tassert.Truef(t, containsKey(&set, key), "missing (%d,%d)", i, j)
		}
	}

	removed := 0
	for i := 0; i < n; i += 2 {
		for j := i + 1; j < n; j += 2 {
			key := shapePairKey(uint32(i), uint32(j))
			tassert.True(t, removeKey(&set, key))
			removed++
		}
	}

	tassert.Equal(t, expectedCount-removed, getSetCount(&set))

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			key := shapePairKey(uint32(i), uint32(j))
			if i%2 == 0 && j%2 == 1 {
				tassert.False(t, containsKey(&set, key))
			} else {
				tassert.True(t, containsKey(&set, key))
			}
		}
	}
}

func TestHashSet_Determinism(t *testing.T) {
	t.Parallel()

	setA := createHashSet(16)
	setB := createHashSet(16)

	keys := []uint64{
		shapePairKey(0, 1),
		shapePairKey(0, 2),
		shapePairKey(1, 2),
		shapePairKey(3, 7),
		shapePairKey(4, 9),
		shapePairKey(10, 11),
		shapePairKey(12, 13),
		shapePairKey(14, 15),
		shapePairKey(16, 17),
		shapePairKey(18, 19),
	}

	for _, key := range keys {
		addKey(&setA, key)
	}
	for _, key := range keys {
		addKey(&setB, key)
	}

	tassert.Equal(t, setA.capacity, setB.capacity)
	tassert.Equal(t, setA.count, setB.count)
	tassert.Equal(t, setA.items, setB.items)
}

func TestHashSet_GetHashSetBytes(t *testing.T) {
	t.Parallel()

	set := createHashSet(16)
	tassert.Equal(t, int(set.capacity)*8, getHashSetBytes(&set))
}

func TestHashSet_DestroyAndClear(t *testing.T) {
	t.Parallel()

	set := createHashSet(16)
	addKey(&set, 7)
	addKey(&set, 42)

	clearHashSet(&set)
	tassert.Zero(t, getSetCount(&set))
	tassert.False(t, containsKey(&set, 7))

	addKey(&set, 99)
	tassert.True(t, containsKey(&set, 99))
	tassert.Equal(t, 1, getSetCount(&set))

	destroyHashSet(&set)
	tassert.Zero(t, getSetCapacity(&set))
	tassert.Zero(t, getSetCount(&set))
	tassert.Nil(t, set.items)
}
