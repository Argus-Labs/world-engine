package performance

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultMaxSpansPerTick = 256
	subscriberChanBuf      = 4
)

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
type Batch struct {
	Ticks          []TickTimeline
	DroppedSpans   uint64 // Spans dropped during these ticks (per-batch delta, reset after flush).
	DroppedBatches uint64 // Subscriber sends that failed since last successfully delivered batch (per-batch delta).
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
	tickActive   bool
	droppedSpans uint64 // guarded by mu

	// droppedBatches is incremented outside mu during broadcast and read under
	// mu when building the next batch, so it must be atomic.
	droppedBatches atomic.Uint64
}

// NewCollector creates a Collector that flushes every batchSize ticks.
func NewCollector(batchSize int) *Collector {
	if batchSize <= 0 {
		batchSize = 1
	}
	return &Collector{
		currentSpans: make([]TickSpan, 0, defaultMaxSpansPerTick),
		pending:      make([]TickTimeline, 0, batchSize),
		batchSize:    batchSize,
	}
}

// StartTick initializes span collection for a new tick.
// Call exactly once per tick before any RecordSpan calls.
func (c *Collector) StartTick() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentSpans = c.currentSpans[:0]
	c.tickActive = true
}

// RecordSpan appends a span to the current tick. Drops the span if the
// per-tick limit is exceeded.
func (c *Collector) RecordSpan(span TickSpan) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.tickActive {
		return
	}
	if len(c.currentSpans) >= defaultMaxSpansPerTick {
		c.droppedSpans++
		return
	}
	c.currentSpans = append(c.currentSpans, span)
}

// RecordTick finalizes the current tick, appending a TickTimeline to the
// pending batch. When the batch reaches batchSize, it is flushed to all
// subscribers via non-blocking channel sends performed outside the lock.
func (c *Collector) RecordTick(tickHeight uint64, tickStart time.Time) {
	c.mu.Lock()

	if !c.tickActive {
		c.mu.Unlock()
		return
	}
	c.tickActive = false

	spans := make([]TickSpan, len(c.currentSpans))
	copy(spans, c.currentSpans)

	c.pending = append(c.pending, TickTimeline{
		TickHeight: tickHeight,
		TickStart:  tickStart,
		Spans:      spans,
	})

	var batch Batch
	var subs []chan Batch

	if len(c.pending) >= c.batchSize {
		batch = Batch{
			Ticks:          c.pending,
			DroppedSpans:   c.droppedSpans,
			DroppedBatches: c.droppedBatches.Swap(0),
		}
		c.pending = make([]TickTimeline, 0, c.batchSize)
		c.droppedSpans = 0

		subs = make([]chan Batch, len(c.subscribers))
		copy(subs, c.subscribers)
	}

	c.mu.Unlock()

	for _, sub := range subs {
		select {
		case sub <- batch:
		default:
			c.droppedBatches.Add(1)
		}
	}
}

// Reset clears all buffered data. Use after a world reset or snapshot restore.
func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentSpans = c.currentSpans[:0]
	c.pending = c.pending[:0]
	c.tickActive = false
	c.droppedSpans = 0
	c.droppedBatches.Store(0)
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
// The channel is NOT closed; callers should select on ctx.Done() to detect
// stream termination rather than relying on channel closure.
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

// DroppedSpans returns the number of spans dropped since the last flush.
func (c *Collector) DroppedSpans() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.droppedSpans
}
