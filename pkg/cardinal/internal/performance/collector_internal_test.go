package performance

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// TestNewCollector_Errors
// ---------------------------------------------------------------------------

func TestNewCollector_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		tickRate  float64
		wantError bool
	}{
		{"zero", 0, true},
		{"negative integer", -1, true},
		{"negative fraction", -0.5, true},
		{"positive fraction", 0.5, false},
		{"twenty", 20, false},
		{"sixty", 60, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, err := NewCollector(tt.tickRate)
			if tt.wantError {
				require.Error(t, err)
				assert.Nil(t, c)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, c)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestCollector_FullLifecycle
// ---------------------------------------------------------------------------

func TestCollector_FullLifecycle(t *testing.T) {
	t.Parallel()

	c, err := NewCollector(20)
	require.NoError(t, err)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	for tick := 0; tick < 100; tick++ {
		now := base.Add(time.Duration(tick) * 50 * time.Millisecond)
		err := c.StartTick(uint64(tick), now)
		require.NoError(t, err)

		for s := 0; s < 3; s++ {
			c.RecordSpan(TickSpan{
				TickHeight: uint64(tick),
				Phase:      "update",
				SystemName: fmt.Sprintf("system_%d", s),
				DurationNs: int64((s + 1) * 1000),
			})
		}

		c.RecordTick(TickSample{
			At:         now,
			TickHeight: uint64(tick),
			DurationMs: 2.5,
		})
	}

	lastTickTime := base.Add(99 * 50 * time.Millisecond)

	// Overview should report ticks with consistent average.
	stats, _ := c.Overview(10, lastTickTime)
	assert.Positive(t, stats.Count)
	assert.InDelta(t, 2.5, stats.AvgMs, 0.0001)

	// Schedule should return entries each with exactly 3 spans.
	result := c.Schedule(10, 0, lastTickTime)
	for _, entry := range result.Ticks {
		assert.Len(t, entry.Spans, 3, "tick %d should have 3 spans", entry.TickHeight)
	}

	// No spans should have been dropped.
	assert.Equal(t, uint64(0), c.DroppedSpans())
}

// ---------------------------------------------------------------------------
// TestCollector_InProgressExclusion
// ---------------------------------------------------------------------------

func TestCollector_InProgressExclusion(t *testing.T) {
	t.Parallel()

	c, err := NewCollector(20)
	require.NoError(t, err)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Record 5 complete ticks.
	for tick := 0; tick < 5; tick++ {
		now := base.Add(time.Duration(tick) * 50 * time.Millisecond)
		require.NoError(t, c.StartTick(uint64(tick), now))
		c.RecordSpan(TickSpan{
			TickHeight: uint64(tick),
			Phase:      "update",
			SystemName: "sys",
			DurationNs: 1000,
		})
		c.RecordTick(TickSample{
			At:         now,
			TickHeight: uint64(tick),
			DurationMs: 1.0,
		})
	}

	// Start tick 5 but do NOT complete it.
	now := base.Add(5 * 50 * time.Millisecond)
	require.NoError(t, c.StartTick(5, now))
	c.RecordSpan(TickSpan{
		TickHeight: 5,
		Phase:      "update",
		SystemName: "sys",
		DurationNs: 1000,
	})

	queryTime := base.Add(6 * 50 * time.Millisecond)

	// Schedule should exclude the in-progress tick 5.
	schedule := c.Schedule(300, 0, queryTime).Ticks
	for _, entry := range schedule {
		assert.NotEqual(t, uint64(5), entry.TickHeight, "in-progress tick 5 should be excluded")
	}
	assert.Len(t, schedule, 5)

	// Now complete tick 5.
	c.RecordTick(TickSample{
		At:         now,
		TickHeight: 5,
		DurationMs: 1.0,
	})

	// Schedule should now include tick 5.
	schedule = c.Schedule(300, 0, queryTime).Ticks
	assert.Len(t, schedule, 6)

	found := false
	for _, entry := range schedule {
		if entry.TickHeight == 5 {
			found = true
			break
		}
	}
	assert.True(t, found, "tick 5 should be present after completion")
}

// ---------------------------------------------------------------------------
// TestCollector_TimeWindowing
// ---------------------------------------------------------------------------

func TestCollector_TimeWindowing(t *testing.T) {
	t.Parallel()

	c, err := NewCollector(20)
	require.NoError(t, err)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Record 60 ticks, 1 per second.
	for tick := 0; tick < 60; tick++ {
		now := base.Add(time.Duration(tick) * time.Second)
		require.NoError(t, c.StartTick(uint64(tick), now))
		c.RecordSpan(TickSpan{
			TickHeight: uint64(tick),
			Phase:      "update",
			SystemName: "sys",
			DurationNs: 1000,
		})
		c.RecordTick(TickSample{
			At:         now,
			TickHeight: uint64(tick),
			DurationMs: 1.0,
		})
	}

	lastTickTime := base.Add(59 * time.Second)

	// 10-second window: ticks at seconds 50..59 → ~10-11 ticks.
	stats10, _ := c.Overview(10, lastTickTime)
	assert.GreaterOrEqual(t, stats10.Count, 10)
	assert.LessOrEqual(t, stats10.Count, 11)

	// All schedule entries within the 10s window should have a TickStart in range.
	windowStart10 := lastTickTime.Add(-10 * time.Second)
	schedule10 := c.Schedule(10, 0, lastTickTime).Ticks
	for _, entry := range schedule10 {
		assert.False(t, entry.TickStart.Before(windowStart10),
			"tick %d at %v is before window start %v", entry.TickHeight, entry.TickStart, windowStart10)
	}

	// 5-second window: fewer results than 10-second window.
	stats5, _ := c.Overview(5, lastTickTime)
	assert.GreaterOrEqual(t, stats5.Count, 5)
	assert.LessOrEqual(t, stats5.Count, 6)
	assert.Less(t, stats5.Count, stats10.Count)
}

// ---------------------------------------------------------------------------
// TestCollector_ScheduleAllDataOlderThanWindow
// ---------------------------------------------------------------------------

func TestCollector_ScheduleAllDataOlderThanWindow(t *testing.T) {
	t.Parallel()

	c, err := NewCollector(20)
	require.NoError(t, err)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	for tick := 0; tick < 10; tick++ {
		now := base.Add(time.Duration(tick) * time.Second)
		require.NoError(t, c.StartTick(uint64(tick), now))
		c.RecordSpan(TickSpan{
			TickHeight: uint64(tick),
			Phase:      "update",
			SystemName: "sys",
			DurationNs: 1000,
		})
		c.RecordTick(TickSample{
			At:         now,
			TickHeight: uint64(tick),
			DurationMs: 1.0,
		})
	}

	// Query with "now" far in the future so every tick is older than the 5s window.
	farFuture := base.Add(time.Hour)
	result := c.Schedule(5, 0, farFuture)
	assert.Empty(t, result.Ticks, "schedule should be empty when all data is older than the window")
	assert.False(t, result.Truncated)
}

// ---------------------------------------------------------------------------
// TestCollector_Reset
// ---------------------------------------------------------------------------

func TestCollector_Reset(t *testing.T) {
	t.Parallel()

	c, err := NewCollector(20)
	require.NoError(t, err)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Record 10 complete ticks (0-9).
	for tick := 0; tick < 10; tick++ {
		now := base.Add(time.Duration(tick) * 50 * time.Millisecond)
		require.NoError(t, c.StartTick(uint64(tick), now))
		c.RecordSpan(TickSpan{
			TickHeight: uint64(tick),
			Phase:      "update",
			SystemName: "sys",
			DurationNs: 1000,
		})
		c.RecordTick(TickSample{
			At:         now,
			TickHeight: uint64(tick),
			DurationMs: 1.0,
		})
	}

	now := base.Add(10 * 50 * time.Millisecond)
	c.Reset(20)

	// After reset, overview should be empty.
	stats, _ := c.Overview(300, now)
	assert.Equal(t, 0, stats.Count)

	// After reset, schedule should be empty — no ghost entries from zeroed slots.
	schedule := c.Schedule(300, 0, now).Ticks
	assert.Empty(t, schedule, "schedule should be empty immediately after reset")

	// StartTick at 20 should work (reset set nextHead=20).
	require.NoError(t, c.StartTick(20, now))

	c.RecordSpan(TickSpan{
		TickHeight: 20,
		Phase:      "update",
		SystemName: "sys",
		DurationNs: 1000,
	})
	c.RecordTick(TickSample{
		At:         now,
		TickHeight: 20,
		DurationMs: 1.0,
	})

	// Should now have exactly 1 tick.
	stats, _ = c.Overview(300, now)
	assert.Equal(t, 1, stats.Count)

	schedule = c.Schedule(300, 0, now).Ticks
	require.Len(t, schedule, 1)
	assert.Equal(t, uint64(20), schedule[0].TickHeight)
}

// ---------------------------------------------------------------------------
// TestCollector_ScheduleTruncation
// ---------------------------------------------------------------------------

func TestCollector_ScheduleTruncation(t *testing.T) {
	t.Parallel()

	c, err := NewCollector(20)
	require.NoError(t, err)

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Record 50 ticks, 50ms apart.
	for tick := 0; tick < 50; tick++ {
		now := base.Add(time.Duration(tick) * 50 * time.Millisecond)
		require.NoError(t, c.StartTick(uint64(tick), now))
		c.RecordSpan(TickSpan{
			TickHeight: uint64(tick),
			Phase:      "update",
			SystemName: "sys",
			DurationNs: 1000,
		})
		c.RecordTick(TickSample{
			At:         now,
			TickHeight: uint64(tick),
			DurationMs: 1.0,
		})
	}

	queryTime := base.Add(49 * 50 * time.Millisecond)

	// maxTicks=0: no truncation.
	all := c.Schedule(300, 0, queryTime)
	assert.Len(t, all.Ticks, 50)
	assert.False(t, all.Truncated)

	// maxTicks=10: keep most recent 10 ticks.
	capped := c.Schedule(300, 10, queryTime)
	assert.Len(t, capped.Ticks, 10)
	assert.True(t, capped.Truncated)
	assert.Equal(t, uint64(40), capped.Ticks[0].TickHeight)
	assert.Equal(t, uint64(49), capped.Ticks[9].TickHeight)

	// maxTicks >= available: no truncation.
	full := c.Schedule(300, 100, queryTime)
	assert.Len(t, full.Ticks, 50)
	assert.False(t, full.Truncated)
}
