package performance

import (
	"sync"
	"sync/atomic"
	"time"
)

// TickSpan represents a single system execution span within a tick.
type TickSpan struct {
	SystemHook uint8
	SystemName string
	StartTime  time.Time
	EndTime    time.Time
}

// TickTimeline groups spans that occurred within a single tick. Treat values
// received from a Collector as read-only.
type TickTimeline struct {
	TickHeight           uint64
	TickStart            time.Time
	SystemPhaseStartedAt time.Time
	SystemPhaseElapsed   time.Duration
	Generation           uint64
	Spans                []TickSpan
}

// TickCapture fixes the profiling decision and reset generation for one tick.
type TickCapture struct {
	Generation  uint64
	SystemSpans bool
}

// Collector measures completed ticks and publishes them without blocking the
// tick loop. Streaming handlers own batching and other transport concerns.
type Collector struct {
	mu           sync.Mutex
	currentSpans []TickSpan
	subscribers  []subscription
	bufferSize   int
	generation   uint64

	// subscriberCount provides a lock-free fast path when metrics are unwatched.
	subscriberCount atomic.Int64
}

type subscription struct {
	ch                  chan TickTimeline
	requestsSystemSpans bool
	active              bool
}

// NewCollector creates a Collector with a bounded queue for each subscriber.
func NewCollector(bufferSize int) *Collector {
	if bufferSize <= 0 {
		bufferSize = 1
	}
	return &Collector{bufferSize: bufferSize}
}

// StartTick fixes the profiling decision and reset generation for the tick
// that is about to run. RecordTick must receive the returned value.
func (c *Collector) StartTick() TickCapture {
	if c.subscriberCount.Load() == 0 {
		return TickCapture{}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	capture := TickCapture{Generation: c.generation}
	for i := range c.subscribers {
		c.subscribers[i].active = true
		capture.SystemSpans = capture.SystemSpans || c.subscribers[i].requestsSystemSpans
	}
	c.currentSpans = c.currentSpans[:0]
	return capture
}

// RecordSpan appends a span to the current tick.
func (c *Collector) RecordSpan(span TickSpan) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentSpans = append(c.currentSpans, span)
}

// RecordTick measures a completed tick and offers it to active subscribers.
func (c *Collector) RecordTick(
	capture TickCapture,
	tickHeight uint64,
	tickStart time.Time,
	systemPhaseStartedAt time.Time,
) {
	if c.subscriberCount.Load() == 0 {
		return
	}

	systemPhaseElapsed := time.Since(systemPhaseStartedAt)

	c.mu.Lock()
	if len(c.subscribers) == 0 || capture.Generation != c.generation {
		c.mu.Unlock()
		return
	}

	var spans []TickSpan
	if capture.SystemSpans {
		spans = make([]TickSpan, len(c.currentSpans))
		copy(spans, c.currentSpans)
	}

	tick := TickTimeline{
		TickHeight:           tickHeight,
		TickStart:            tickStart,
		SystemPhaseStartedAt: systemPhaseStartedAt,
		SystemPhaseElapsed:   systemPhaseElapsed,
		Generation:           capture.Generation,
		Spans:                spans,
	}

	for _, sub := range c.subscribers {
		if !sub.active {
			continue
		}
		offerLatest(sub.ch, tick)
	}
	c.mu.Unlock()
}

// Reset clears queued measurements and starts a new tick generation.
func (c *Collector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentSpans = c.currentSpans[:0]
	c.generation++
	for i := range c.subscribers {
		c.subscribers[i].active = false
		drain(c.subscribers[i].ch)
	}
}

// SubscribeTimings returns completed tick timings. The caller must eventually
// call Unsubscribe.
func (c *Collector) SubscribeTimings() <-chan TickTimeline {
	return c.subscribe(false)
}

// SubscribeProfiles returns completed ticks with per-system spans and keeps
// span capture enabled until the caller unsubscribes.
func (c *Collector) SubscribeProfiles() <-chan TickTimeline {
	return c.subscribe(true)
}

func (c *Collector) subscribe(requestsSystemSpans bool) <-chan TickTimeline {
	ch := make(chan TickTimeline, c.bufferSize)
	c.mu.Lock()
	c.subscribers = append(c.subscribers, subscription{
		ch:                  ch,
		requestsSystemSpans: requestsSystemSpans,
	})
	c.subscriberCount.Add(1)
	c.mu.Unlock()
	return ch
}

// Unsubscribe removes the given channel from the subscriber list.
// The channel is intentionally not closed: closing would make receives
// return zero values instead of blocking, breaking select/default patterns.
// Callers should select on ctx.Done() to detect stream termination.
func (c *Collector) Unsubscribe(ch <-chan TickTimeline) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, sub := range c.subscribers {
		if sub.ch == ch {
			c.subscribers = append(c.subscribers[:i], c.subscribers[i+1:]...)
			c.subscriberCount.Add(-1)
			return
		}
	}
}

// Keep live metrics current without ever blocking the tick loop.
func offerLatest(ch chan TickTimeline, tick TickTimeline) {
	select {
	case ch <- tick:
		return
	default:
	}

	select {
	case <-ch:
	default:
	}

	select {
	case ch <- tick:
	default:
	}
}

func drain(ch chan TickTimeline) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}
