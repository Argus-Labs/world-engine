package performance

import (
	"sync"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
)

const subscriberChanBuf = 4

// TickSpan represents a single system execution span within a tick.
type TickSpan struct {
	TickHeight uint64
	SystemHook uint8
	SystemName string
	StartTime  time.Time
	EndTime    time.Time
}

// TickTimeline groups spans that occurred within a single tick.
type TickTimeline struct {
	TickHeight uint64
	TickStart  time.Time
	Spans      []TickSpan
}

// Batch is a batch of completed tick timelines pushed to subscribers.
// Treat as read-only: subscribers must not mutate Ticks or its elements.
type Batch struct {
	Ticks []TickTimeline
}

// Collector accumulates per-tick span data and broadcasts it in batches to
// streaming subscribers. It is designed for a single writer (the tick loop)
// with concurrent reader subscriptions (gRPC stream handlers).
type Collector struct {
	mu           sync.Mutex
	currentSpans []TickSpan
	pending      []TickTimeline
	subscribers  []chan Batch
	batchSize    int
}

// NewCollector creates a Collector that flushes every batchSize ticks.
func NewCollector(batchSize int) *Collector {
	if batchSize <= 0 {
		batchSize = 1
	}
	return &Collector{
		pending:   make([]TickTimeline, 0, batchSize),
		batchSize: batchSize,
	}
}

// StartTick initializes span collection for a new tick.
// Call exactly once per tick before any RecordSpan calls.
func (c *Collector) StartTick() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentSpans = c.currentSpans[:0]
}

// RecordSpan appends a span to the current tick.
func (c *Collector) RecordSpan(span TickSpan) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentSpans = append(c.currentSpans, span)
}

// RecordTick finalizes the current tick, appending a TickTimeline to the
// pending batch. When the batch reaches batchSize, it is flushed to all
// subscribers via non-blocking channel sends performed outside the lock.
func (c *Collector) RecordTick(tickHeight uint64, tickStart time.Time) {
	c.mu.Lock()

	spans := make([]TickSpan, len(c.currentSpans))
	copy(spans, c.currentSpans)

	c.pending = append(c.pending, TickTimeline{
		TickHeight: tickHeight,
		TickStart:  tickStart,
		Spans:      spans,
	})

	var batch Batch
	var subs []chan Batch

	assert.That(len(c.pending) <= c.batchSize,
		"performance.Collector: pending ticks (%d) exceeded batchSize (%d)", len(c.pending), c.batchSize)
	if len(c.pending) == c.batchSize {
		// Detach from c.pending so future appends in RecordTick don't overwrite sent data.
		ticks := make([]TickTimeline, len(c.pending))
		copy(ticks, c.pending)
		batch = Batch{Ticks: ticks}
		c.pending = make([]TickTimeline, 0, c.batchSize)

		subs = make([]chan Batch, len(c.subscribers))
		copy(subs, c.subscribers)
	}

	c.mu.Unlock()

	for _, sub := range subs {
		// Non-blocking send; recover guards against closed channels
		// (which can happen when Unsubscribe races with an in-flight flush).
		func() {
			defer func() { _ = recover() }()
			select {
			case sub <- batch:
			default:
			}
		}()
	}
}

// Reset clears all buffered data. Use after a world reset or snapshot restore.
func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentSpans = c.currentSpans[:0]
	c.pending = c.pending[:0]
}

// Subscribe returns a channel that receives Batch values whenever the
// collector flushes. The caller must eventually call Unsubscribe to avoid
// leaking the channel.
func (c *Collector) Subscribe() <-chan Batch {
	ch := make(chan Batch, subscriberChanBuf)
	c.mu.Lock()
	c.subscribers = append(c.subscribers, ch)
	c.mu.Unlock()
	return ch
}

// Unsubscribe removes the given channel from the subscriber list.
// The channel is intentionally not closed: closing would make receives
// return zero values instead of blocking, breaking select/default patterns.
// Callers should select on ctx.Done() to detect stream termination.
func (c *Collector) Unsubscribe(ch <-chan Batch) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, sub := range c.subscribers {
		if sub == ch {
			c.subscribers = append(c.subscribers[:i], c.subscribers[i+1:]...)
			return
		}
	}
}
