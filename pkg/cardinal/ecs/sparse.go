package ecs

import "github.com/argus-labs/world-engine/pkg/assert"

type sparseSet []int

const tombstone = -1

func newSparseSet() sparseSet {
	const capacity = 128

	s := make(sparseSet, capacity)
	for i := range capacity {
		s[i] = tombstone
	}

	return s
}

func (s *sparseSet) get(key EntityID) (int, bool) {
	if int(key) >= len(*s) {
		return 0, false
	}

	value := (*s)[key]
	if value == tombstone {
		return 0, false
	}

	return value, true
}

func (s *sparseSet) set(key EntityID, value int) {
	assert.That(value != tombstone, "value cannot be tombstone")

	if int(key) >= len(*s) { // Grow slice if needed
		// Grow by doubling or to key+1, whichever is larger.
		oldLen := len(*s)
		newLen := max(oldLen*2, int(key)+1)

		newSlice := make(sparseSet, newLen)
		copy(newSlice, *s)
		for i := oldLen; i < newLen; i++ {
			newSlice[i] = tombstone
		}
		*s = newSlice
	}

	(*s)[key] = value
}

func (s *sparseSet) remove(key EntityID) bool {
	if int(key) >= len(*s) {
		return false
	}

	if (*s)[key] == tombstone {
		return false
	}

	(*s)[key] = tombstone
	return true
}

// serialize converts the sparseSet to a slice of int64 for protobuf serialization.
func (s *sparseSet) serialize() []int64 {
	result := make([]int64, len(*s))
	for i, value := range *s {
		result[i] = int64(value)
	}
	return result
}

// deserialize populates the sparseSet from a slice of int64 from protobuf.
func (s *sparseSet) deserialize(data []int64) {
	*s = make(sparseSet, len(data))
	for i, value := range data {
		(*s)[i] = int(value)
	}
}
