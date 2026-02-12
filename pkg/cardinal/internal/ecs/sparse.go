package ecs

import "github.com/argus-labs/world-engine/pkg/assert"

type sparseSet []int

const sparseCapacity = 128
const sparseTombstone = -1

// newSparseSet creates a new sparse set.
func newSparseSet() sparseSet {
	s := make(sparseSet, sparseCapacity)
	for i := range sparseCapacity {
		s[i] = sparseTombstone
	}
	return s
}

// get returns the value for a key and whether it exists.
func (s *sparseSet) get(key EntityID) (int, bool) {
	if int(key) >= len(*s) {
		return 0, false
	}

	value := (*s)[key]
	if value == sparseTombstone {
		return 0, false
	}

	return value, true
}

// set stores a value for a key, growing the backing slice if needed.
func (s *sparseSet) set(key EntityID, value int) {
	assert.That(value >= 0, "value must be a non-negative row index")

	if int(key) >= len(*s) { // Grow slice if needed
		// Grow by doubling or to key+1, whichever is larger.
		oldLen := len(*s)
		newLen := max(oldLen*2, int(key)+1)

		newSlice := make(sparseSet, newLen)
		copy(newSlice, *s)
		for i := oldLen; i < newLen; i++ {
			newSlice[i] = sparseTombstone
		}
		*s = newSlice
	}

	(*s)[key] = value
}

// remove sets a key's value to tombstone. Returns true if the key existed.
func (s *sparseSet) remove(key EntityID) bool {
	if int(key) >= len(*s) {
		return false
	}

	if (*s)[key] == sparseTombstone {
		return false
	}

	(*s)[key] = sparseTombstone
	return true
}

// toInt64Slice converts the sparseSet to a []int64 for protobuf serialization.
func (s *sparseSet) toInt64Slice() []int64 {
	result := make([]int64, len(*s))
	for i, value := range *s {
		result[i] = int64(value)
	}
	return result
}

// fromInt64Slice populates the sparseSet from a []int64.
func (s *sparseSet) fromInt64Slice(data []int64) {
	*s = make(sparseSet, len(data))
	for i, value := range data {
		(*s)[i] = int(value)
	}
}
