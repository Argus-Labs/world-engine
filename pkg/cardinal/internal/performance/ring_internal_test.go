package performance

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/testutils"
)

// -------------------------------------------------------------------------------------------------
// roundUpPowerOfTwo
// -------------------------------------------------------------------------------------------------

func TestRoundUpPowerOfTwo_Examples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in, want int
	}{
		{1, 1}, {2, 2}, {3, 4}, {4, 4}, {5, 8}, {7, 8}, {8, 8},
		{9, 16}, {16, 16}, {17, 32}, {1023, 1024}, {1024, 1024}, {1025, 2048},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, roundUpPowerOfTwo(tt.in), "roundUpPowerOfTwo(%d)", tt.in)
	}
}

// -------------------------------------------------------------------------------------------------
// NewTickRing errors
// -------------------------------------------------------------------------------------------------

func TestNewTickRing_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cap     int
		wantErr bool
	}{
		{"zero", 0, true},
		{"negative one", -1, true},
		{"negative hundred", -100, true},
		{"one", 1, false},
		{"three", 3, false},
		{"hundred", 100, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ring, err := NewTickRing(tt.cap)
			if tt.wantErr {
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
// Model-based fuzz
// -------------------------------------------------------------------------------------------------

func TestTickRing_ModelFuzz(t *testing.T) {
	t.Parallel()

	r := testutils.NewRand(t)

	for range 4096 {
		capacity := 1 + r.IntN(64)
		actualCap := roundUpPowerOfTwo(capacity)

		ring, err := NewTickRing(capacity)
		require.NoError(t, err)

		var model []TickSample
		var allAdvanced int
		var dst []TickSample

		numOps := r.IntN(257) // 0-256
		weights := testutils.RandOpWeights(r, []string{"advance", "snapshot"})

		for range numOps {
			switch testutils.RandWeightedOp(r, weights) {
			case "advance":
				s := TickSample{TickHeight: uint64(allAdvanced), DurationMs: float64(allAdvanced)}
				ring.Advance(s)
				allAdvanced++
				model = append(model, s)
				if len(model) > actualCap {
					model = model[len(model)-actualCap:]
				}
			case "snapshot":
				dst = ring.SnapshotInto(dst)
				assert.Equal(t, sliceOrNil(model), dst)
				assert.LessOrEqual(t, len(dst), actualCap)
			}
		}
	}
}

// -------------------------------------------------------------------------------------------------
// Exhaustive for small sizes
// -------------------------------------------------------------------------------------------------

func TestTickRing_Exhaustive(t *testing.T) {
	t.Parallel()

	for capIdx := range 4 {
		capacity := capIdx + 1 // 1..4
		actualCap := roundUpPowerOfTwo(capacity)

		g := testutils.NewGen()
		for !g.Done() {
			numAdvances := g.Intn(8) // 0..8

			ring, err := NewTickRing(capacity)
			require.NoError(t, err)

			var model []TickSample
			for i := range numAdvances {
				s := TickSample{TickHeight: uint64(i), DurationMs: float64(i)}
				ring.Advance(s)
				model = append(model, s)
				if len(model) > actualCap {
					model = model[len(model)-actualCap:]
				}
			}

			got := ring.SnapshotInto(nil)
			assert.Equal(t, sliceOrNil(model), got,
				"capacity=%d numAdvances=%d", capacity, numAdvances)
		}
	}
}

// -------------------------------------------------------------------------------------------------
// Reset
// -------------------------------------------------------------------------------------------------

func TestTickRing_Reset(t *testing.T) {
	t.Parallel()

	ring, err := NewTickRing(4)
	require.NoError(t, err)

	// Advance 6 samples (exceeds capacity of 4).
	for i := range 6 {
		ring.Advance(TickSample{TickHeight: uint64(i), DurationMs: float64(i)})
	}
	snap := ring.SnapshotInto(nil)
	require.Len(t, snap, 4)

	// Reset clears everything.
	ring.Reset()
	snap = ring.SnapshotInto(nil)
	assert.Nil(t, snap)

	// Advance 2 more samples after reset.
	ring.Advance(TickSample{TickHeight: 100, DurationMs: 100})
	ring.Advance(TickSample{TickHeight: 101, DurationMs: 101})

	snap = ring.SnapshotInto(nil)
	require.Len(t, snap, 2)
	assert.Equal(t, uint64(100), snap[0].TickHeight)
	assert.Equal(t, uint64(101), snap[1].TickHeight)
}

// -------------------------------------------------------------------------------------------------
// Concurrent fuzz
// -------------------------------------------------------------------------------------------------

func TestTickRing_ConcurrentFuzz(t *testing.T) {
	t.Parallel()

	const (
		numWrites  = 500
		numReaders = 4
	)

	synctest.Test(t, func(t *testing.T) {
		ring, err := NewTickRing(64)
		require.NoError(t, err)
		actualCap := roundUpPowerOfTwo(64)

		// Writer goroutine.
		go func() {
			for i := range numWrites {
				ring.Advance(TickSample{
					At:         time.Now(),
					TickHeight: uint64(i),
					DurationMs: float64(i),
				})
				time.Sleep(time.Millisecond)
			}
		}()

		// Reader goroutines.
		var wg sync.WaitGroup
		for range numReaders {
			wg.Go(func() {
				var dst []TickSample
				for range numWrites {
					dst = ring.SnapshotInto(dst)
					if len(dst) > 0 {
						assert.LessOrEqual(t, len(dst), actualCap)
						// Tick heights must be strictly monotonically increasing.
						for j := 1; j < len(dst); j++ {
							assert.Greater(t, dst[j].TickHeight, dst[j-1].TickHeight,
								"tick heights not strictly increasing at index %d", j)
						}
					}
					time.Sleep(time.Millisecond)
				}
			})
		}
		wg.Wait()
	})
}

// -------------------------------------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------------------------------------

func sliceOrNil[T any](s []T) []T {
	if len(s) == 0 {
		return nil
	}
	return s
}
