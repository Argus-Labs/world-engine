package performance

import (
	"fmt"
	"sync"
	"time"
)

// TickSpan represents a single system span within a tick.
type TickSpan struct {
	TickHeight    uint64
	Phase         string
	SystemName    string
	StartOffsetNs int64
	DurationNs    int64
}

// TickSpans groups spans that occurred within a single tick.
type TickSpans struct {
	TickHeight uint64
	TickStart  time.Time
	Spans      []TickSpan
}

// SpanRing stores a bounded history of per-tick spans.
// It is safe for a single writer with concurrent readers.
type SpanRing struct {
	mu              sync.RWMutex
	buf             []TickSpans
	mask            uint64
	head            uint64
	maxSpansPerTick int
}

// NewSpanRing creates a span ring with power-of-two capacity.
// If capacity is not a power of two, it is rounded up.
func NewSpanRing(capacity int, maxSpansPerTick int) (*SpanRing, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("capacity must be > 0, got %d", capacity)
	}
	if maxSpansPerTick <= 0 {
		return nil, fmt.Errorf("maxSpansPerTick must be > 0, got %d", maxSpansPerTick)
	}
	capacity = roundUpPowerOfTwo(capacity)
	r := &SpanRing{
		buf:             make([]TickSpans, capacity),
		mask:            uint64(capacity - 1), //nolint:gosec // capacity validated > 0 and power-of-two
		maxSpansPerTick: maxSpansPerTick,
	}
	for i := range r.buf {
		r.buf[i].Spans = make([]TickSpan, 0, maxSpansPerTick)
	}
	return r, nil
}

// StartTick initializes storage for a new tick, overwriting the oldest tick entry.
// Call exactly once per tick (single-writer).
func (r *SpanRing) StartTick(tickHeight uint64, tickStart time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if tickHeight != r.head {
		return fmt.Errorf("expected tick %d, got %d", r.head, tickHeight)
	}
	idx := r.head & r.mask
	r.buf[idx] = TickSpans{
		TickHeight: tickHeight,
		TickStart:  tickStart,
		Spans:      r.buf[idx].Spans[:0],
	}
	r.head++
	return nil
}

// AddSpan appends a span to the latest tick.
func (r *SpanRing) AddSpan(span TickSpan) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.head == 0 {
		return
	}
	idx := (r.head - 1) & r.mask
	if len(r.buf[idx].Spans) >= r.maxSpansPerTick {
		return
	}
	r.buf[idx].Spans = append(r.buf[idx].Spans, span)
}

// Snapshot returns a copy of all valid tick spans in chronological order.
func (r *SpanRing) Snapshot() []TickSpans {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.head == 0 || len(r.buf) == 0 {
		return nil
	}

	start := uint64(0)
	if r.head > uint64(len(r.buf)) {
		start = r.head - uint64(len(r.buf))
	}

	out := make([]TickSpans, 0, int(r.head-start)) //nolint:gosec // difference bounded by len(buf)
	for t := start; t < r.head; t++ {
		entry := r.buf[t&r.mask]
		entry.Spans = append([]TickSpan(nil), entry.Spans...)
		out = append(out, entry)
	}
	return out
}
