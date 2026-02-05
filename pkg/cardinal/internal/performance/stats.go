package performance

import (
	"math"
	"sort"
	"time"
)

// TickSample represents a single tick duration sample.
type TickSample struct {
	At         time.Time
	TickHeight uint64
	DurationMs float64
	Overrun    bool
}

// TickStats summarizes tick timing samples.
type TickStats struct {
	Count        int
	AvgMs        float64
	P95Ms        float64
	MaxMs        float64
	OverrunCount int
	OverrunRate  float64
}

// Freshness describes the latest tick's recency.
type Freshness struct {
	LastTickHeight uint64
	LastTickAt     time.Time
	AgeMs          int64
}

// SummarizeWindow computes stats for the last windowSeconds of samples.
// It returns zero stats if there are no samples in the window.
func SummarizeWindow(
	ring *TickRing,
	windowSeconds int,
	now time.Time,
	dst []TickSample,
) (TickStats, Freshness) {
	if ring == nil || windowSeconds <= 0 {
		return TickStats{}, Freshness{}
	}

	snapshot := ring.SnapshotInto(dst)
	if len(snapshot) == 0 {
		return TickStats{}, Freshness{}
	}

	freshness := Freshness{
		LastTickHeight: snapshot[len(snapshot)-1].TickHeight,
		LastTickAt:     snapshot[len(snapshot)-1].At,
		AgeMs:          int64(now.Sub(snapshot[len(snapshot)-1].At) / time.Millisecond),
	}

	windowStart := now.Add(-time.Duration(windowSeconds) * time.Second)

	durations := make([]float64, 0, len(snapshot))
	var overrunCount int
	var sum float64
	var maxMs float64

	for i := len(snapshot) - 1; i >= 0; i-- {
		sample := snapshot[i]
		if sample.At.Before(windowStart) {
			break
		}
		durations = append(durations, sample.DurationMs)
		sum += sample.DurationMs
		if sample.DurationMs > maxMs {
			maxMs = sample.DurationMs
		}
		if sample.Overrun {
			overrunCount++
		}
	}

	if len(durations) == 0 {
		return TickStats{}, freshness
	}

	sort.Float64s(durations)
	p95 := percentile(durations, 0.95)

	return TickStats{
		Count:        len(durations),
		AvgMs:        sum / float64(len(durations)),
		P95Ms:        p95,
		MaxMs:        maxMs,
		OverrunCount: overrunCount,
		OverrunRate:  float64(overrunCount) / float64(len(durations)),
	}, freshness
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	idx := p * float64(len(sorted)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi {
		return sorted[lo]
	}
	frac := idx - float64(lo)
	return sorted[lo] + (sorted[hi]-sorted[lo])*frac
}
