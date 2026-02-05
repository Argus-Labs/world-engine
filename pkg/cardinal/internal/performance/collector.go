package performance

import (
	"math"
	"sync/atomic"
	"time"

	"github.com/rotisserie/eris"
)

const (
	// Time windows: buffer sizes are computed from these so retention is constant across tick rates.
	// TODO: Make these configurable, if enabled on preview server reduce span window to conserve memory (56mb).
	defaultTickWindowSeconds = 300 // overview: tick samples retained for this many seconds
	defaultSpanWindowSeconds = 120 // schedule: span history retained for this many seconds
	defaultMaxSpansPerTick   = 256 // max spans (system runs) per tick; excess dropped for schedule view
	noTickInProgress         = math.MaxUint64
)

// Collector owns the tick and span ring buffers and provides methods
// for recording and querying performance data.
type Collector struct {
	ticks                *TickRing
	spans                *SpanRing
	inProgressTickHeight atomic.Uint64
}

// NewCollector creates a Collector with the given tick rate.
// Buffer sizes are derived from default time windows so retention is the same at any tick rate.
func NewCollector(tickRate float64) (*Collector, error) {
	if tickRate <= 0 {
		return nil, eris.Errorf("tick rate must be > 0, got %f", tickRate)
	}
	tickCap := int(tickRate * defaultTickWindowSeconds)
	spanCap := int(tickRate * defaultSpanWindowSeconds)
	ticks, err := NewTickRing(tickCap)
	if err != nil {
		return nil, err
	}
	spans, err := NewSpanRing(spanCap, defaultMaxSpansPerTick)
	if err != nil {
		return nil, err
	}
	c := &Collector{
		ticks: ticks,
		spans: spans,
	}
	c.inProgressTickHeight.Store(noTickInProgress)
	return c, nil
}

// Reset clears all buffered performance data and re-synchronizes the span ring
// to expect nextHead as the next tick height. Call after a world reset or restore.
func (c *Collector) Reset(nextHead uint64) {
	c.ticks.Reset()
	c.spans.Reset(nextHead)
	c.inProgressTickHeight.Store(noTickInProgress)
}

// RecordTick records a tick duration sample and marks that tick as complete
// so Schedule() will include it.
func (c *Collector) RecordTick(sample TickSample) {
	c.ticks.Advance(sample)
	c.inProgressTickHeight.Store(noTickInProgress)
}

// StartTick initializes span storage for a new tick. That tick is considered
// in-progress until RecordTick is called, and Schedule() excludes in-progress
// ticks so the frontend never sees zero/partial spans.
func (c *Collector) StartTick(tickHeight uint64, tickStart time.Time) error {
	c.inProgressTickHeight.Store(tickHeight)
	return c.spans.StartTick(tickHeight, tickStart)
}

// RecordSpan records a per-system span within the current tick.
func (c *Collector) RecordSpan(span TickSpan) {
	c.spans.AddSpan(span)
}

// DroppedSpans returns the total number of spans that were dropped because
// a tick exceeded the per-tick span limit.
func (c *Collector) DroppedSpans() uint64 {
	return c.spans.DroppedSpans()
}

// Overview computes aggregated tick stats for the given time window.
// Safe for concurrent use from multiple goroutines (e.g. gRPC handlers).
func (c *Collector) Overview(windowSeconds int, now time.Time) (TickStats, Freshness) {
	dst := c.ticks.SnapshotInto(nil)
	stats, freshness := SummarizeWindow(dst, windowSeconds, now)
	return stats, freshness
}

// ScheduleResult holds the output of Schedule including truncation metadata.
type ScheduleResult struct {
	Ticks     []TickSpans
	Truncated bool
}

// Schedule returns span data for completed ticks within the given time window,
// truncated to at most maxTicks entries (keeping the most recent).
// The in-progress tick (current tick being executed) is excluded so callers
// never see zero or partial spans.
func (c *Collector) Schedule(windowSeconds int, maxTicks int, now time.Time) ScheduleResult {
	snapshot := c.spans.Snapshot()
	if len(snapshot) == 0 {
		return ScheduleResult{}
	}
	inProgress := c.inProgressTickHeight.Load()
	windowStart := now.Add(-time.Duration(windowSeconds) * time.Second)
	start := len(snapshot)
	for i, ts := range snapshot {
		if !ts.TickStart.Before(windowStart) {
			start = i
			break
		}
	}
	out := snapshot[start:]
	if inProgress != noTickInProgress {
		filtered := make([]TickSpans, 0, len(out))
		for _, ts := range out {
			if ts.TickHeight != inProgress {
				filtered = append(filtered, ts)
			}
		}
		out = filtered
	}
	truncated := false
	if maxTicks > 0 && len(out) > maxTicks {
		out = out[len(out)-maxTicks:]
		truncated = true
	}
	return ScheduleResult{Ticks: out, Truncated: truncated}
}
