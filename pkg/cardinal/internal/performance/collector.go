package performance

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
)

const subscriberChanBuf = 4

// TickSpan represents a single system execution span within a tick.
type TickSpan struct {
	SystemHook uint8
	SystemName string
	StartTime  time.Time
	EndTime    time.Time
}

// TickTimeline groups spans that occurred within a single tick.
type TickTimeline struct {
	TickHeight         uint64
	TickStart          time.Time
	SystemPhaseElapsed time.Duration
	Spans              []TickSpan
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
	subscribers  []subscription
	batchSize    int

	// subscriberCount provides a lock-free fast path when metrics are unwatched.
	subscriberCount atomic.Int64

	// systemSpanSubscriberCount enables per-system span capture when non-zero.
	systemSpanSubscriberCount atomic.Int64
}

type subscription struct {
	ch                  chan Batch
	requestsSystemSpans bool
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

// StartTick returns whether the next tick should capture per-system spans. The
// result must be carried through RecordTick so one tick cannot be partially
// profiled when a profile subscriber connects or disconnects mid-tick.
func (c *Collector) StartTick() bool {
	if c.systemSpanSubscriberCount.Load() == 0 {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentSpans = c.currentSpans[:0]
	return true
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
func (c *Collector) RecordTick(
	captureSystemSpans bool,
	tickHeight uint64,
	tickStart time.Time,
	systemPhaseElapsed time.Duration,
) {
	if c.subscriberCount.Load() == 0 {
		return
	}

	c.mu.Lock()
	if len(c.subscribers) == 0 {
		c.mu.Unlock()
		return
	}

	var spans []TickSpan
	if captureSystemSpans {
		spans = make([]TickSpan, len(c.currentSpans))
		copy(spans, c.currentSpans)
	}

	c.pending = append(c.pending, TickTimeline{
		TickHeight:         tickHeight,
		TickStart:          tickStart,
		SystemPhaseElapsed: systemPhaseElapsed,
		Spans:              spans,
	})

	var batch Batch
	var subs []subscription

	assert.That(len(c.pending) <= c.batchSize,
		"performance.Collector: pending ticks (%d) exceeded batchSize (%d)", len(c.pending), c.batchSize)
	if len(c.pending) == c.batchSize {
		// Detach from c.pending so future appends in RecordTick don't overwrite sent data.
		ticks := make([]TickTimeline, len(c.pending))
		copy(ticks, c.pending)
		batch = Batch{Ticks: ticks}
		c.pending = make([]TickTimeline, 0, c.batchSize)

		subs = make([]subscription, len(c.subscribers))
		copy(subs, c.subscribers)
	}

	c.mu.Unlock()

	for _, sub := range subs {
		// Non-blocking send; recover guards against closed channels
		// (which can happen when Unsubscribe races with an in-flight flush).
		func() {
			defer func() { _ = recover() }()
			select {
			case sub.ch <- batch:
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

// SubscribeTimings returns aggregate system timing batches. The caller must
// eventually call Unsubscribe.
func (c *Collector) SubscribeTimings() <-chan Batch {
	return c.subscribe(false)
}

// SubscribeProfiles returns timing batches with per-system spans and keeps span
// capture enabled until the caller unsubscribes.
func (c *Collector) SubscribeProfiles() <-chan Batch {
	return c.subscribe(true)
}

func (c *Collector) subscribe(requestsSystemSpans bool) <-chan Batch {
	ch := make(chan Batch, subscriberChanBuf)
	c.mu.Lock()
	c.subscribers = append(c.subscribers, subscription{
		ch:                  ch,
		requestsSystemSpans: requestsSystemSpans,
	})
	c.subscriberCount.Add(1)
	if requestsSystemSpans {
		c.systemSpanSubscriberCount.Add(1)
	}
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
		if sub.ch == ch {
			c.subscribers = append(c.subscribers[:i], c.subscribers[i+1:]...)
			c.subscriberCount.Add(-1)
			if sub.requestsSystemSpans {
				c.systemSpanSubscriberCount.Add(-1)
			}
			if len(c.subscribers) == 0 {
				c.pending = c.pending[:0]
			}
			return
		}
	}
}
