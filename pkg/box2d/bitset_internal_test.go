// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/bitset.c, src/bitset.h, src/ctz.h.

package box2d

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBitSet_SetClearGet(t *testing.T) {
	t.Parallel()

	bs := createBitSet(0)
	setBitCountAndClear(&bs, 100)

	setBit(&bs, 0)
	setBit(&bs, 63)
	setBit(&bs, 64)
	setBit(&bs, 99)

	tassert.True(t, getBit(&bs, 0))
	tassert.True(t, getBit(&bs, 63))
	tassert.True(t, getBit(&bs, 64))
	tassert.True(t, getBit(&bs, 99))
	tassert.False(t, getBit(&bs, 1))
	tassert.False(t, getBit(&bs, 98))
	tassert.False(t, getBit(&bs, 100))

	clearBit(&bs, 0)
	clearBit(&bs, 64)
	tassert.False(t, getBit(&bs, 0))
	tassert.False(t, getBit(&bs, 64))
	tassert.True(t, getBit(&bs, 63))
	tassert.True(t, getBit(&bs, 99))

	clearBit(&bs, 1000)
	tassert.False(t, getBit(&bs, 1000))
}

func TestBitSet_GrowAndSetBitGrow(t *testing.T) {
	t.Parallel()

	bs := createBitSet(64)
	require.Equal(t, uint32(1), bs.blockCapacity)
	require.Equal(t, uint32(0), bs.blockCount)

	setBitGrow(&bs, 0)
	setBitGrow(&bs, 63)
	setBitGrow(&bs, 64)

	tassert.True(t, getBit(&bs, 0))
	tassert.True(t, getBit(&bs, 63))
	tassert.True(t, getBit(&bs, 64))
	tassert.GreaterOrEqual(t, bs.blockCapacity, uint32(2))
}

func TestBitSet_WordBoundaries(t *testing.T) {
	t.Parallel()

	bs := createBitSet(256)
	setBitCountAndClear(&bs, 256)

	setBit(&bs, 0)
	setBit(&bs, 63)
	setBit(&bs, 64)
	setBit(&bs, 127)
	setBit(&bs, 128)
	setBit(&bs, 191)
	setBit(&bs, 192)
	setBit(&bs, 255)

	tassert.Equal(t, 8, countSetBits(&bs))
	for i := uint32(0); i < 256; i++ {
		switch i {
		case 0, 63, 64, 127, 128, 191, 192, 255:
			tassert.Truef(t, getBit(&bs, i), "bit %d should be set", i)
		default:
			tassert.Falsef(t, getBit(&bs, i), "bit %d should be clear", i)
		}
	}
}

func TestBitSet_InPlaceUnion(t *testing.T) {
	t.Parallel()

	a := createBitSet(128)
	setBitCountAndClear(&a, 128)
	setBit(&a, 0)
	setBit(&a, 64)

	b := createBitSet(128)
	setBitCountAndClear(&b, 128)
	setBit(&b, 63)
	setBit(&b, 127)

	inPlaceUnion(&a, &b)

	tassert.True(t, getBit(&a, 0))
	tassert.True(t, getBit(&a, 63))
	tassert.True(t, getBit(&a, 64))
	tassert.True(t, getBit(&a, 127))
	tassert.Equal(t, 4, countSetBits(&a))
}

func TestBitSet_GrowPreservesBits(t *testing.T) {
	t.Parallel()

	bs := createBitSet(64)
	setBitCountAndClear(&bs, 64)
	setBit(&bs, 0)
	setBit(&bs, 63)

	growBitSet(&bs, 4)

	tassert.True(t, getBit(&bs, 0))
	tassert.True(t, getBit(&bs, 63))
	tassert.Equal(t, uint32(4), bs.blockCount)
	tassert.GreaterOrEqual(t, bs.blockCapacity, uint32(4))
}

func TestBitSet_CreateBitSetBytes(t *testing.T) {
	t.Parallel()

	bs := createBitSet(128)
	setBitCountAndClear(&bs, 128)

	tassert.Equal(t, int(bs.blockCapacity)*8, getBitSetBytes(&bs))
}

func TestBitSet_Destroy(t *testing.T) {
	t.Parallel()

	bs := createBitSet(128)
	setBitCountAndClear(&bs, 128)
	setBit(&bs, 7)

	destroyBitSet(&bs)

	tassert.Zero(t, bs.blockCapacity)
	tassert.Zero(t, bs.blockCount)
	tassert.Nil(t, bs.bits)
}

func TestBitSet_CtzHelpers(t *testing.T) {
	t.Parallel()

	tassert.Equal(t, uint32(10), ctz32(1<<10))
	tassert.Equal(t, uint32(20), ctz64(uint64(1)<<20))
	tassert.Equal(t, uint32(11), clz32(1<<20))
	tassert.Equal(t, 2, popCount64(0b101))
	tassert.True(t, isPowerOf2(16))
	tassert.False(t, isPowerOf2(15))
	tassert.Equal(t, 5, boundingPowerOf2(17))
	tassert.Equal(t, 32, roundUpPowerOf2(17))
}
