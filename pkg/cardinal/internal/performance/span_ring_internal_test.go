package performance

import (
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Constructor validation
// -------------------------------------------------------------------------------------------------

func TestNewSpanRing_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		capacity  int
		maxSpans  int
		wantError bool
	}{
		{"valid", 8, 64, false},
		{"zero capacity", 0, 64, true},
		{"negative capacity", -1, 64, true},
		{"zero maxSpans", 8, 0, true},
		{"negative maxSpans", 8, -1, true},
		{"both zero", 0, 0, true},
		{"capacity 1 maxSpans 1", 1, 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ring, err := NewSpanRing(tt.capacity, tt.maxSpans)
			if tt.wantError {
				require.Error(t, err)
				assert.Nil(t, ring)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, ring)
			}
		})
	}
}

// -------------------------------------------------------------------------------------------------
// Sequential tick enforcement
// -------------------------------------------------------------------------------------------------

func TestSpanRing_StartTick_SequentialEnforcement(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	ring, err := NewSpanRing(8, 64)
	require.NoError(t, err)

	// Tick 0 succeeds.
	require.NoError(t, ring.StartTick(0, now))

	// Tick 2 fails (expected 1).
	require.Error(t, ring.StartTick(2, now))

	// Tick 1 succeeds.
	require.NoError(t, ring.StartTick(1, now))

	// Tick 1 again fails (expected 2).
	require.Error(t, ring.StartTick(1, now))

	// After reset to 10, tick 10 succeeds.
	ring.Reset(10)
	require.NoError(t, ring.StartTick(10, now))

	// Tick 10 again fails (expected 11).
	require.Error(t, ring.StartTick(10, now))

	// Tick 11 succeeds.
	require.NoError(t, ring.StartTick(11, now))
}

// -------------------------------------------------------------------------------------------------
// Span dropping when maxSpansPerTick is exceeded
// -------------------------------------------------------------------------------------------------

func TestSpanRing_SpanDropping(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	ring, err := NewSpanRing(8, 3)
	require.NoError(t, err)

	require.NoError(t, ring.StartTick(0, now))

	// Add 5 spans; only 3 should be kept, 2 dropped.
	for i := range 5 {
		ring.AddSpan(TickSpan{TickHeight: 0, Phase: "phase", SystemName: fmt.Sprintf("sys%d", i)})
	}

	assert.Equal(t, uint64(2), ring.DroppedSpans())

	snap := ring.Snapshot()
	require.Len(t, snap, 1)
	assert.Len(t, snap[0].Spans, 3)

	// Start another tick, add 1 span. Drops remain cumulative.
	require.NoError(t, ring.StartTick(1, now))
	ring.AddSpan(TickSpan{TickHeight: 1, Phase: "phase", SystemName: "sysA"})

	assert.Equal(t, uint64(2), ring.DroppedSpans())
}

// -------------------------------------------------------------------------------------------------
// AddSpan before StartTick is a safe no-op
// -------------------------------------------------------------------------------------------------

func TestSpanRing_AddSpan_BeforeStartTick(t *testing.T) {
	t.Parallel()

	ring, err := NewSpanRing(4, 8)
	require.NoError(t, err)

	// Should not panic.
	ring.AddSpan(TickSpan{Phase: "init", SystemName: "sys0"})

	assert.Nil(t, ring.Snapshot())
	assert.Equal(t, uint64(0), ring.DroppedSpans())
}

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing
// -------------------------------------------------------------------------------------------------

func tickSpansOrNil(s []TickSpans) []TickSpans {
	if len(s) == 0 {
		return nil
	}
	return s
}

// normalizeModel returns a copy of the model with empty Spans slices set to nil,
// matching the Snapshot() deep-copy behavior (append([]TickSpan(nil), empty...) → nil).
func normalizeModel(model []TickSpans) []TickSpans {
	if len(model) == 0 {
		return nil
	}
	out := make([]TickSpans, len(model))
	for i, ts := range model {
		out[i] = ts
		if len(ts.Spans) == 0 {
			out[i].Spans = nil
		}
	}
	return out
}

func TestSpanRing_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		iterations  = 4096
		opStartTick = "startTick"
		opAddSpan   = "addSpan"
		opSnapshot  = "snapshot"
	)

	for range iterations {
		capacity := 1 + prng.IntN(32)
		maxSpans := 1 + prng.IntN(16)
		ringCap := roundUpPowerOfTwo(capacity)

		ring, err := NewSpanRing(capacity, maxSpans)
		require.NoError(t, err)

		operations := []string{opStartTick, opAddSpan, opSnapshot}
		weights := testutils.RandOpWeights(prng, operations)

		var model []TickSpans
		var expectedDrops uint64
		var nextTick uint64

		numOps := prng.IntN(257) // 0-256

		for range numOps {
			op := testutils.RandWeightedOp(prng, weights)
			switch op {
			case opStartTick:
				tickTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(
					time.Duration(nextTick) * time.Second,
				)
				require.NoError(t, ring.StartTick(nextTick, tickTime))

				model = append(model, TickSpans{
					TickHeight: nextTick,
					TickStart:  tickTime,
					Spans:      []TickSpan{},
				})

				// Trim model to ring capacity.
				if len(model) > ringCap {
					model = model[len(model)-ringCap:]
				}

				nextTick++

			case opAddSpan:
				if len(model) == 0 {
					continue
				}

				span := TickSpan{
					TickHeight:    model[len(model)-1].TickHeight,
					Phase:         fmt.Sprintf("p%d", prng.IntN(4)),
					SystemName:    fmt.Sprintf("s%d", prng.IntN(10)),
					StartOffsetNs: int64(prng.IntN(1_000_000)),
					DurationNs:    int64(prng.IntN(1_000_000)),
				}
				ring.AddSpan(span)

				last := &model[len(model)-1]
				if len(last.Spans) < maxSpans {
					last.Spans = append(last.Spans, span)
				} else {
					expectedDrops++
				}

			case opSnapshot:
				snap := ring.Snapshot()
				assert.Equal(t, normalizeModel(model), tickSpansOrNil(snap))

			default:
				panic("unreachable")
			}
		}

		assert.Equal(t, expectedDrops, ring.DroppedSpans())
	}
}

// -------------------------------------------------------------------------------------------------
// Reset clears state and re-synchronizes tick sequence
// -------------------------------------------------------------------------------------------------

func TestSpanRing_Reset(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	ring, err := NewSpanRing(4, 8)
	require.NoError(t, err)

	// Fill with ticks 0..5.
	for i := range uint64(6) {
		require.NoError(t, ring.StartTick(i, now.Add(time.Duration(i)*time.Second)))
		ring.AddSpan(TickSpan{TickHeight: i, Phase: "run", SystemName: "sys"})
	}

	assert.Equal(t, uint64(0), ring.DroppedSpans())

	// Reset to tick 100.
	ring.Reset(100)

	// Verify DroppedSpans is cleared and Snapshot returns no ghost entries.
	assert.Equal(t, uint64(0), ring.DroppedSpans())
	assert.Nil(t, ring.Snapshot(), "snapshot should be empty right after reset")

	// Tick 100 succeeds.
	require.NoError(t, ring.StartTick(100, now.Add(100*time.Second)))

	// Tick 99 fails (expected 101).
	require.Error(t, ring.StartTick(99, now))

	// Tick 101 succeeds.
	require.NoError(t, ring.StartTick(101, now.Add(101*time.Second)))

	// Add a span and verify snapshot contains only the two real ticks.
	ring.AddSpan(TickSpan{TickHeight: 101, Phase: "run", SystemName: "sysA"})

	snap := ring.Snapshot()
	require.Len(t, snap, 2)

	assert.Equal(t, uint64(100), snap[0].TickHeight)
	assert.Equal(t, uint64(101), snap[1].TickHeight)
	assert.Empty(t, snap[0].Spans)
	assert.Len(t, snap[1].Spans, 1)
	assert.Equal(t, "sysA", snap[1].Spans[0].SystemName)
}

// -------------------------------------------------------------------------------------------------
// Concurrent reader/writer fuzz
//
// synctest provides deterministic goroutine scheduling, which is ideal for verifying logical
// concurrency correctness (e.g. monotonic tick ordering, no stale reads). However, it does not
// exercise the memory-access interleavings that Go's race detector checks. Always run these
// tests with `go test -race` in CI to catch data races that deterministic scheduling may miss.
// -------------------------------------------------------------------------------------------------

func TestSpanRing_ConcurrentFuzz(t *testing.T) {
	t.Parallel()

	const (
		maxSpans    = 8
		writerTicks = 200
		readerLoops = 200
		numReaders  = 4
	)

	synctest.Test(t, func(t *testing.T) {
		ring, err := NewSpanRing(16, maxSpans)
		require.NoError(t, err)

		now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

		// Writer goroutine.
		go func() {
			prng := testutils.NewRand(t)
			for tick := range uint64(writerTicks) {
				err := ring.StartTick(tick, now.Add(time.Duration(tick)*time.Second))
				if err != nil {
					panic(fmt.Sprintf("StartTick(%d) failed: %v", tick, err))
				}
				numSpans := 1 + prng.IntN(5)
				for j := range numSpans {
					ring.AddSpan(TickSpan{
						TickHeight: tick,
						Phase:      "run",
						SystemName: fmt.Sprintf("sys%d", j),
					})
				}
				time.Sleep(time.Millisecond)
			}
		}()

		// Reader goroutines.
		var wg sync.WaitGroup
		for range numReaders {
			wg.Go(func() {
				for range readerLoops {
					snap := ring.Snapshot()

					// Verify monotonically increasing tick heights.
					for i := 1; i < len(snap); i++ {
						assert.Greater(t, snap[i].TickHeight, snap[i-1].TickHeight,
							"tick heights must be monotonically increasing")
					}

					// Verify span count per tick does not exceed maxSpansPerTick.
					for _, entry := range snap {
						assert.LessOrEqual(t, len(entry.Spans), maxSpans,
							"spans per tick must not exceed maxSpansPerTick")
					}

					time.Sleep(time.Millisecond)
				}
			})
		}
		wg.Wait()
	})
}
