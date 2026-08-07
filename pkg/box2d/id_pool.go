// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/id_pool.c, src/id_pool.h.

package box2d

import "strconv"

// idPool allocates and recycles integer identifiers using a LIFO free list (upstream b2IdPool).
type idPool struct {
	freeArray []int
	nextIndex int
}

func createIDPool() idPool {
	return idPool{
		freeArray: make([]int, 0, 32),
		nextIndex: 0,
	}
}

func destroyIDPool(pool *idPool) {
	pool.freeArray = nil
	pool.nextIndex = 0
}

func allocID(pool *idPool) int {
	count := len(pool.freeArray)
	if count > 0 {
		id := pool.freeArray[count-1]
		pool.freeArray = pool.freeArray[:count-1]
		return id
	}

	id := pool.nextIndex
	pool.nextIndex++
	return id
}

func freeID(pool *idPool, id int) {
	assert(pool.nextIndex > 0)
	assert(0 <= id && id < pool.nextIndex)

	if cap(pool.freeArray) == len(pool.freeArray) {
		oldCap := cap(pool.freeArray)
		newCap := oldCap
		if newCap < 2 {
			newCap = 2
		} else {
			newCap += newCap / 2
		}
		newData := make([]int, len(pool.freeArray), newCap)
		copy(newData, pool.freeArray)
		pool.freeArray = newData
	}

	pool.freeArray = append(pool.freeArray, id)
}

func validateFreeID(pool *idPool, id int) {
	// B2_ENABLE_VALIDATION is false in release builds; this is a no-op.
	_ = pool
	_ = id
}

func validateUsedID(pool *idPool, id int) {
	// B2_ENABLE_VALIDATION is false in release builds; this is a no-op.
	_ = pool
	_ = id
}

func getIDCount(pool *idPool) int {
	return pool.nextIndex - len(pool.freeArray)
}

func getIDCapacity(pool *idPool) int {
	return pool.nextIndex
}

// getIDBytes reports the free list's memory footprint. DELIBERATE DEVIATION:
// upstream counts capacity * sizeof(int) with a 32-bit int; Go's int is 64-bit
// on all supported targets, so this reports twice the C figure — the accurate
// number for this port's actual memory.
func getIDBytes(pool *idPool) int {
	return cap(pool.freeArray) * (strconv.IntSize / 8)
}
