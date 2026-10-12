package testutils_test

import (
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A logged TEST_SEED must replay the same run, so a pick may depend only on the seed.
func TestRandWeightedOp_SameSeedSamePicks(t *testing.T) {
	t.Parallel()

	ops := make(testutils.OpWeights, 20)
	for i := range 20 {
		ops[fmt.Sprintf("op%02d", i)] = uint64(i + 1)
	}
	picks := func() []string {
		r := rand.New(rand.NewPCG(1, 2))
		out := make([]string, 1000)
		for i := range out {
			out[i] = testutils.RandWeightedOp(r, ops)
		}
		return out
	}

	first, replay := picks(), picks()
	assert.Equal(t, first, replay)
}

func TestRandMapKey_SameSeedSamePicks(t *testing.T) {
	t.Parallel()

	m := make(map[string]struct{}, 20)
	for i := range 20 {
		m[fmt.Sprintf("key%02d", i)] = struct{}{}
	}
	picks := func() []string {
		r := rand.New(rand.NewPCG(1, 2))
		out := make([]string, 1000)
		for i := range out {
			out[i] = testutils.RandMapKey(r, m)
		}
		return out
	}

	first, replay := picks(), picks()
	assert.Equal(t, first, replay)
}

func TestRandMapKey_PicksNthSmallestKey(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(3, 4))
	for size := 1; size <= 300; size++ {
		m := make(map[int]struct{}, size)
		for len(m) < size {
			m[r.IntN(10_000)-5_000] = struct{}{}
		}
		sorted := slices.Sorted(maps.Keys(m))

		seed := r.Uint64()
		got := testutils.RandMapKey(rand.New(rand.NewPCG(seed, 0)), m)
		want := sorted[rand.New(rand.NewPCG(seed, 0)).IntN(size)]
		require.Equal(t, want, got, "size %d", size)
	}
}

func TestRandMapKey_NaNKeysReplay(t *testing.T) {
	t.Parallel()

	m := map[float64]struct{}{math.NaN(): {}, math.NaN(): {}, 1: {}, 2: {}}
	// NaN != NaN, so a pick is recorded as its value, or -1 for NaN.
	picks := func() []float64 {
		r := rand.New(rand.NewPCG(1, 2))
		out := make([]float64, 50)
		for i := range out {
			out[i] = testutils.RandMapKey(r, m)
			if math.IsNaN(out[i]) {
				out[i] = -1
			}
		}
		return out
	}

	first := picks()
	assert.Subset(t, first, []float64{-1, 1, 2}, "50 picks should reach every kind of key")
	for range 100 {
		require.Equal(t, first, picks())
	}
}
