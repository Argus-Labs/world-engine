package performance

import (
	"fmt"
	"math/bits"
	"sync"
)

// TickRing is a bounded ring buffer of TickSamples indexed by a monotonically
// increasing tick counter. It is safe for a single writer with concurrent readers.
type TickRing struct {
	mu   sync.RWMutex
	buf  []TickSample
	mask uint64 // cap-1, cap is power of two
	head uint64 // absolute tick counter / write cursor
}

// NewTickRing creates a ring buffer with power-of-two capacity.
// If capacity is not a power of two, it is rounded up.
func NewTickRing(capacity int) (*TickRing, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("capacity must be > 0, got %d", capacity)
	}
	capacity = roundUpPowerOfTwo(capacity)
	return &TickRing{
		buf:  make([]TickSample, capacity),
		mask: uint64(capacity - 1), //nolint:gosec // capacity validated > 0 and power-of-two
	}, nil
}

// Advance writes the tick sample into the next slot (overwriting oldest automatically).
// Call exactly once per tick (single-writer).
func (r *TickRing) Advance(v TickSample) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.buf[r.head&r.mask] = v
	r.head++
}

// SnapshotInto copies all valid entries into dst in chronological order.
// It reuses dst capacity when possible.
func (r *TickRing) SnapshotInto(dst []TickSample) []TickSample {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.head == 0 || len(r.buf) == 0 {
		return nil
	}

	start := uint64(0)
	if r.head > uint64(len(r.buf)) {
		start = r.head - uint64(len(r.buf))
	}

	size := int(r.head - start) //nolint:gosec // difference bounded by len(buf)
	if cap(dst) < size {
		dst = make([]TickSample, 0, size)
	} else {
		dst = dst[:0]
	}

	for t := start; t < r.head; t++ {
		dst = append(dst, r.buf[t&r.mask])
	}
	return dst
}

func roundUpPowerOfTwo(n int) int {
	if n <= 1 {
		return 1
	}
	return 1 << bits.Len(uint(n-1)) //nolint:gosec // n >= 2 at this point
}
