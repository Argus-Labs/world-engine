// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/bitset.c, src/bitset.h, src/ctz.h.

package box2d

import "math/bits"

// bitSet provides fast operations on large arrays of bits (upstream b2BitSet).
type bitSet struct {
	bits          []uint64
	blockCapacity uint32
	blockCount    uint32
}

func createBitSet(bitCapacity uint32) bitSet {
	blockCapacity := (bitCapacity + 64 - 1) / 64
	return bitSet{
		bits:          make([]uint64, blockCapacity),
		blockCapacity: blockCapacity,
		blockCount:    0,
	}
}

func destroyBitSet(bitSet *bitSet) {
	bitSet.blockCapacity = 0
	bitSet.blockCount = 0
	bitSet.bits = nil
}

func setBitCountAndClear(bitSet *bitSet, bitCount uint32) {
	blockCount := (bitCount + 64 - 1) / 64
	if bitSet.blockCapacity < blockCount {
		destroyBitSet(bitSet)
		newBitCapacity := bitCount + (bitCount >> 1)
		*bitSet = createBitSet(newBitCapacity)
	}

	bitSet.blockCount = blockCount
	for i := range blockCount {
		bitSet.bits[i] = 0
	}
}

func growBitSet(bitSet *bitSet, blockCount uint32) {
	assert(blockCount > bitSet.blockCount)
	if blockCount > bitSet.blockCapacity {
		bitSet.blockCapacity = blockCount + blockCount/2
		newBits := make([]uint64, bitSet.blockCapacity)
		copy(newBits, bitSet.bits)
		bitSet.bits = newBits
	}

	bitSet.blockCount = blockCount
}

func inPlaceUnion(setA, setB *bitSet) {
	assert(setA.blockCount == setB.blockCount)
	blockCount := setA.blockCount
	for i := range blockCount {
		setA.bits[i] |= setB.bits[i]
	}
}

func setBit(bitSet *bitSet, bitIndex uint32) {
	blockIndex := bitIndex / 64
	assert(blockIndex < bitSet.blockCount)
	bitSet.bits[blockIndex] |= uint64(1) << (bitIndex % 64)
}

func setBitGrow(bitSet *bitSet, bitIndex uint32) {
	blockIndex := bitIndex / 64
	if blockIndex >= bitSet.blockCount {
		growBitSet(bitSet, blockIndex+1)
	}
	bitSet.bits[blockIndex] |= uint64(1) << (bitIndex % 64)
}

func clearBit(bitSet *bitSet, bitIndex uint32) {
	blockIndex := bitIndex / 64
	if blockIndex >= bitSet.blockCount {
		return
	}
	bitSet.bits[blockIndex] &^= uint64(1) << (bitIndex % 64)
}

func getBit(bitSet *bitSet, bitIndex uint32) bool {
	blockIndex := bitIndex / 64
	if blockIndex >= bitSet.blockCount {
		return false
	}
	return (bitSet.bits[blockIndex] & (uint64(1) << (bitIndex % 64))) != 0
}

func getBitSetBytes(bitSet *bitSet) int {
	return int(bitSet.blockCapacity) * 8
}

func countSetBits(bitSet *bitSet) int {
	popCount := 0
	blockCount := bitSet.blockCount
	for i := range blockCount {
		popCount += popCount64(bitSet.bits[i])
	}
	return popCount
}

func ctz32(block uint32) uint32 {
	return uint32(bits.TrailingZeros32(block))
}

func clz32(value uint32) uint32 {
	return uint32(bits.LeadingZeros32(value))
}

func ctz64(block uint64) uint32 {
	return uint32(bits.TrailingZeros64(block))
}

func popCount64(block uint64) int {
	return bits.OnesCount64(block)
}

func isPowerOf2(x int) bool {
	return (x & (x - 1)) == 0
}

func boundingPowerOf2(x int) int {
	if x <= 1 {
		return 1
	}

	return bits.Len32(uint32(x - 1))
}

func roundUpPowerOf2(x int) int {
	if x <= 1 {
		return 1
	}

	return int(uint32(1) << uint(bits.Len32(uint32(x-1))))
}
