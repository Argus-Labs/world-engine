// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/id_pool.c, src/id_pool.h.

package box2d

import (
	"strconv"
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIDPool_LIFOResue(t *testing.T) {
	t.Parallel()

	pool := createIDPool()

	a := allocID(&pool)
	b := allocID(&pool)
	c := allocID(&pool)
	require.Zero(t, a)
	require.Equal(t, 1, b)
	require.Equal(t, 2, c)

	freeID(&pool, a)
	freeID(&pool, b)

	d := allocID(&pool)
	e := allocID(&pool)
	f := allocID(&pool)

	tassert.Equal(t, b, d)
	tassert.Equal(t, a, e)
	tassert.Equal(t, 3, f)
}

func TestIDPool_CountAndCapacity(t *testing.T) {
	t.Parallel()

	pool := createIDPool()

	for i := 0; i < 5; i++ {
		_ = allocID(&pool)
	}

	tassert.Equal(t, 5, getIDCount(&pool))
	tassert.Equal(t, 5, getIDCapacity(&pool))

	freeID(&pool, 1)
	freeID(&pool, 3)

	tassert.Equal(t, 3, getIDCount(&pool))
	tassert.Equal(t, 5, getIDCapacity(&pool))

	_ = allocID(&pool)
	_ = allocID(&pool)

	tassert.Equal(t, 5, getIDCount(&pool))
	tassert.Equal(t, 5, getIDCapacity(&pool))
}

func TestIDPool_GetIDBytes(t *testing.T) {
	t.Parallel()

	pool := createIDPool()

	intBytes := strconv.IntSize / 8
	tassert.Equal(t, 32*intBytes, getIDBytes(&pool))

	// Allocate 64 live ids, then free them all so the free list must grow
	// beyond its initial capacity of 32.
	ids := make([]int, 0, 64)
	for i := 0; i < 64; i++ {
		ids = append(ids, allocID(&pool))
	}
	for _, id := range ids {
		freeID(&pool, id)
	}

	tassert.GreaterOrEqual(t, getIDBytes(&pool), 64*intBytes)
}

func TestIDPool_Destroy(t *testing.T) {
	t.Parallel()

	pool := createIDPool()
	for i := 0; i < 10; i++ {
		_ = allocID(&pool)
	}

	destroyIDPool(&pool)

	tassert.Zero(t, getIDCount(&pool))
	tassert.Zero(t, getIDCapacity(&pool))
	tassert.Nil(t, pool.freeArray)
}

func TestIDPool_Validate(t *testing.T) {
	t.Parallel()

	pool := createIDPool()
	id := allocID(&pool)

	validateFreeID(&pool, id)
	validateUsedID(&pool, id)
}
