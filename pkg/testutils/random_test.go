package testutils_test

import (
	"math/rand/v2"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/require"
)

// TestRandWeightedOp_DeterminismContract pins the contract that RandWeightedOp is a pure function
// of (prng, weights): two *rand.Rand instances constructed with identical PCG seeds must yield
// identical op sequences for the same OpWeights map.
//
// DST advertises this contract ("to reproduce: TEST_SEED=0x%x", pkg/testutils/random.go and
// README.md). Before the fix, RandWeightedOp iterated the OpWeights map with `range`, which the
// Go runtime randomizes per iteration (independent of the prng), so two runs with the same seed
// diverged. After the fix, RandWeightedOp iterates sorted keys, so the chosen op for a given prng
// pick is deterministic and the contract holds.
//
// This test isolates the RandWeightedOp map-iteration bug surface (#2 in the report) without
// involving command.Manager.Names() or RandOpWeights.
func TestRandWeightedOp_DeterminismContract(t *testing.T) {
	t.Parallel()

	weights := testutils.OpWeights{
		"op_00": 10, "op_01": 20, "op_02": 30, "op_03": 40, "op_04": 50,
		"op_05": 60, "op_06": 70, "op_07": 80, "op_08": 90, "op_09": 100,
	}

	const draws = 32

	// Two fresh prngs with identical PCG seeds; the only intentional source of variation is the
	// order in which RandWeightedOp iterates `weights`.
	newPrng := func() *rand.Rand { return rand.New(rand.NewPCG(0xdeadbeef, 0xcafebabe)) }

	draw := func(prng *rand.Rand) []string {
		ops := make([]string, 0, draws)
		for range draws {
			ops = append(ops, testutils.RandWeightedOp(prng, weights))
		}
		return ops
	}

	seq1 := draw(newPrng())
	seq2 := draw(newPrng())

	require.Equal(t, seq1, seq2,
		"RandWeightedOp must produce identical sequences for identical prng seeds "+
			"(determinism contract advertised by TEST_SEED). Divergence means map iteration order in "+
			"`for op, weight := range ops` is randomized per call and independent of the prng.")
}

// TestRandWeightedOp_PreservesWeightedDistribution confirms the fix (sorted-key cumulative walk)
// is statistically equivalent to the original behavior — each op k is selected with probability
// w_k / sum(weights) regardless of map iteration order. This guards against a regression that
// accidentally biases selection.
func TestRandWeightedOp_PreservesWeightedDistribution(t *testing.T) {
	t.Parallel()

	weights := testutils.OpWeights{
		"rare": 1, "mid": 19, "freq": 80,
	}
	const total = 100 // sum of weights

	const draws = 200_000
	prng := rand.New(rand.NewPCG(seedFor(t), seedFor(t)<<1))

	counts := map[string]int{}
	for range draws {
		op := testutils.RandWeightedOp(prng, weights)
		counts[op]++
	}

	// Allow a relative tolerance that absorbs normal sampling noise at 200k draws.
	const relTol = 0.05
	for op, w := range weights {
		expected := float64(draws) * float64(w) / float64(total)
		got := float64(counts[op])
		require.InDelta(t, expected, got, expected*relTol,
			"op %q: expected ~%.0f selections (weight %d/%d), got %.0f", op, expected, w, total, got)
	}
}

// TestRandWeightedOp_SingleEntry confirms the degenerate case: with one op, every draw returns it.
func TestRandWeightedOp_SingleEntry(t *testing.T) {
	t.Parallel()

	weights := testutils.OpWeights{"only": 7}

	prng := rand.New(rand.NewPCG(seedFor(t), seedFor(t)<<1))
	for range 16 {
		require.Equal(t, "only", testutils.RandWeightedOp(prng, weights))
	}
}

// seedFor returns a stable seed derived from the test name so the distribution test is itself
// reproducible across runs (independent of TEST_SEED). It mirrors the spirit of testutils.NewRand
// without requiring a *testing.T-derived prng (we want to avoid depending on package-internal state).
func seedFor(t *testing.T) uint64 {
	t.Helper()
	// FNV-1a over the test name — deterministic per test, no global Seed dependency.
	const offset = 1469598103934665603
	const prime = 1099511628211
	h := uint64(offset)
	for _, b := range []byte(t.Name()) {
		h ^= uint64(b)
		h *= prime
	}
	return h
}
