package performance

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
)

// -------------------------------------------------------------------------------------------------
// TestPercentile_Examples — table-driven tests for the unexported percentile helper
// -------------------------------------------------------------------------------------------------

func TestPercentile_Examples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		sorted []float64
		p      float64
		want   float64
	}{
		{"empty", nil, 0.5, 0},
		{"single element p=0", []float64{42}, 0, 42},
		{"single element p=1", []float64{42}, 1, 42},
		{"single element p=0.5", []float64{42}, 0.5, 42},
		{"two elements p=0", []float64{1, 2}, 0, 1},
		{"two elements p=1", []float64{1, 2}, 1, 2},
		{"two elements p=0.5", []float64{1, 2}, 0.5, 1.5},
		{"p negative", []float64{5, 10, 15}, -0.1, 5},
		{"p>1", []float64{5, 10, 15}, 1.5, 15},
		{"three elements p=0.5 exact index", []float64{10, 20, 30}, 0.5, 20},
		{"four elements p=0.95 interpolation", []float64{1, 2, 3, 4}, 0.95, 3.85},
		{"five elements p=0.25", []float64{10, 20, 30, 40, 50}, 0.25, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := percentile(tt.sorted, tt.p)
			assert.InDelta(t, tt.want, got, 1e-9)
		})
	}
}

// -------------------------------------------------------------------------------------------------
// TestSummarizeWindow_Examples — table-driven tests for SummarizeWindow
// -------------------------------------------------------------------------------------------------

func TestSummarizeWindow_Examples(t *testing.T) {
	t.Parallel()

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		snapshot      []TickSample
		windowSeconds int
		now           time.Time
		wantStats     TickStats
		wantFresh     Freshness
	}{
		{
			name:          "empty snapshot",
			snapshot:      nil,
			windowSeconds: 10,
			now:           base,
			wantStats:     TickStats{},
			wantFresh:     Freshness{},
		},
		{
			name: "zero window",
			snapshot: []TickSample{
				{At: base, TickHeight: 1, DurationMs: 5},
			},
			windowSeconds: 0,
			now:           base,
			wantStats:     TickStats{},
			wantFresh:     Freshness{},
		},
		{
			name: "negative window",
			snapshot: []TickSample{
				{At: base, TickHeight: 1, DurationMs: 5},
			},
			windowSeconds: -1,
			now:           base,
			wantStats:     TickStats{},
			wantFresh:     Freshness{},
		},
		{
			name: "single sample inside window",
			snapshot: []TickSample{
				{At: base, TickHeight: 7, DurationMs: 12.5},
			},
			windowSeconds: 10,
			now:           base.Add(1 * time.Second),
			wantStats: TickStats{
				Count:       1,
				AvgMs:       12.5,
				P95Ms:       12.5,
				MaxMs:       12.5,
				OverrunRate: 0,
			},
			wantFresh: Freshness{
				LastTickHeight: 7,
				LastTickAt:     base,
				AgeMs:          1000,
			},
		},
		{
			name: "all samples outside window",
			snapshot: []TickSample{
				{At: base, TickHeight: 1, DurationMs: 5},
				{At: base.Add(1 * time.Second), TickHeight: 2, DurationMs: 6},
			},
			windowSeconds: 5,
			now:           base.Add(60 * time.Second),
			wantStats:     TickStats{},
			wantFresh: Freshness{
				LastTickHeight: 2,
				LastTickAt:     base.Add(1 * time.Second),
				AgeMs:          59000,
			},
		},
		{
			name: "mixed inside and outside",
			snapshot: []TickSample{
				{At: base, TickHeight: 1, DurationMs: 100},
				{At: base.Add(5 * time.Second), TickHeight: 2, DurationMs: 10},
				{At: base.Add(8 * time.Second), TickHeight: 3, DurationMs: 20},
				{At: base.Add(9 * time.Second), TickHeight: 4, DurationMs: 30},
			},
			windowSeconds: 5,
			now:           base.Add(10 * time.Second),
			wantStats: TickStats{
				Count:       3,
				AvgMs:       20,
				P95Ms:       29,
				MaxMs:       30,
				OverrunRate: 0,
			},
			wantFresh: Freshness{
				LastTickHeight: 4,
				LastTickAt:     base.Add(9 * time.Second),
				AgeMs:          1000,
			},
		},
		{
			name: "all overruns",
			snapshot: []TickSample{
				{At: base, TickHeight: 1, DurationMs: 50, Overrun: true},
				{At: base.Add(1 * time.Second), TickHeight: 2, DurationMs: 60, Overrun: true},
			},
			windowSeconds: 10,
			now:           base.Add(2 * time.Second),
			wantStats: TickStats{
				Count:        2,
				AvgMs:        55,
				P95Ms:        59.5,
				MaxMs:        60,
				OverrunCount: 2,
				OverrunRate:  1.0,
			},
			wantFresh: Freshness{
				LastTickHeight: 2,
				LastTickAt:     base.Add(1 * time.Second),
				AgeMs:          1000,
			},
		},
		{
			name: "no overruns",
			snapshot: []TickSample{
				{At: base, TickHeight: 1, DurationMs: 10},
				{At: base.Add(1 * time.Second), TickHeight: 2, DurationMs: 20},
			},
			windowSeconds: 10,
			now:           base.Add(2 * time.Second),
			wantStats: TickStats{
				Count:       2,
				AvgMs:       15,
				P95Ms:       19.5,
				MaxMs:       20,
				OverrunRate: 0,
			},
			wantFresh: Freshness{
				LastTickHeight: 2,
				LastTickAt:     base.Add(1 * time.Second),
				AgeMs:          1000,
			},
		},
		{
			name: "exact boundary timestamp",
			snapshot: []TickSample{
				{At: base, TickHeight: 1, DurationMs: 5},
				{At: base.Add(5 * time.Second), TickHeight: 2, DurationMs: 15},
			},
			windowSeconds: 5,
			now:           base.Add(5 * time.Second),
			wantStats: TickStats{
				Count:       2,
				AvgMs:       10,
				P95Ms:       14.5,
				MaxMs:       15,
				OverrunRate: 0,
			},
			wantFresh: Freshness{
				LastTickHeight: 2,
				LastTickAt:     base.Add(5 * time.Second),
				AgeMs:          0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotStats, gotFresh := SummarizeWindow(tt.snapshot, tt.windowSeconds, tt.now)
			assert.Equal(t, tt.wantStats.Count, gotStats.Count, "Count")
			assert.InDelta(t, tt.wantStats.AvgMs, gotStats.AvgMs, 1e-9, "AvgMs")
			assert.InDelta(t, tt.wantStats.P95Ms, gotStats.P95Ms, 1e-9, "P95Ms")
			assert.InDelta(t, tt.wantStats.MaxMs, gotStats.MaxMs, 1e-9, "MaxMs")
			assert.Equal(t, tt.wantStats.OverrunCount, gotStats.OverrunCount, "OverrunCount")
			assert.InDelta(t, tt.wantStats.OverrunRate, gotStats.OverrunRate, 1e-9, "OverrunRate")
			assert.Equal(t, tt.wantFresh.LastTickHeight, gotFresh.LastTickHeight, "LastTickHeight")
			assert.Equal(t, tt.wantFresh.LastTickAt, gotFresh.LastTickAt, "LastTickAt")
			assert.Equal(t, tt.wantFresh.AgeMs, gotFresh.AgeMs, "AgeMs")
		})
	}
}

// -------------------------------------------------------------------------------------------------
// TestSummarizeWindow_Fuzz — model-based fuzz testing with random inputs
// -------------------------------------------------------------------------------------------------

func randTickSample(prng *rand.Rand, tickHeight uint64, at time.Time) TickSample {
	return TickSample{
		At:         at,
		TickHeight: tickHeight,
		DurationMs: prng.Float64() * 100,
		Overrun:    prng.IntN(2) == 1,
	}
}

func TestSummarizeWindow_Fuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const iterations = 4096

	for range iterations {
		n := prng.IntN(201) // 0..200
		snapshot := make([]TickSample, n)
		at := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		for i := range n {
			at = at.Add(time.Duration(prng.IntN(500)+1) * time.Millisecond)
			snapshot[i] = randTickSample(prng, uint64(i+1), at)
		}

		windowSeconds := prng.IntN(121) // 0..120
		now := at.Add(time.Duration(prng.IntN(5000)) * time.Millisecond)

		stats, freshness := SummarizeWindow(snapshot, windowSeconds, now)

		// Invariant 1: Count <= len(snapshot)
		assert.LessOrEqual(t, stats.Count, len(snapshot), "Count <= len(snapshot)")

		if stats.Count > 0 {
			// Invariant 2: MaxMs >= AvgMs (all durations non-negative)
			assert.GreaterOrEqual(t, stats.MaxMs, stats.AvgMs, "MaxMs >= AvgMs")

			// Invariant 3: P95Ms <= MaxMs
			assert.LessOrEqual(t, stats.P95Ms, stats.MaxMs+1e-9, "P95Ms <= MaxMs")

			// Invariant 4: P95Ms >= AvgMs (non-negative durations)
			assert.GreaterOrEqual(t, stats.P95Ms+1e-9, stats.AvgMs, "P95Ms >= AvgMs")

			// Invariant 5: OverrunCount <= Count
			assert.LessOrEqual(t, stats.OverrunCount, stats.Count, "OverrunCount <= Count")

			// Invariant 6: OverrunRate == OverrunCount / Count
			expectedRate := float64(stats.OverrunCount) / float64(stats.Count)
			assert.InDelta(t, expectedRate, stats.OverrunRate, 1e-9, "OverrunRate")
		}

		if len(snapshot) == 0 || windowSeconds <= 0 {
			// Source returns zero values for both when empty or non-positive window.
			assert.Equal(t, TickStats{}, stats, "zero stats")
			assert.Equal(t, Freshness{}, freshness, "zero freshness")
		} else {
			// Invariant 7 & 8: Freshness references last element regardless of window.
			last := snapshot[len(snapshot)-1]
			assert.Equal(t, last.TickHeight, freshness.LastTickHeight, "LastTickHeight")
			assert.Equal(t, last.At, freshness.LastTickAt, "LastTickAt")
			expectedAge := int64(now.Sub(last.At) / time.Millisecond)
			assert.Equal(t, expectedAge, freshness.AgeMs, "AgeMs")
		}
	}
}
