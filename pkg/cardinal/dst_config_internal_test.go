package cardinal

import (
	"math/rand/v2"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/require"
)

// TestNewDSTConfig_TickWeightRealizesFullRange pins that opTick's weight is drawn on the intended
// [1,100] range. The cap-on-tick bug (commit 740f2e8d) collapsed this to [1,5] in ~95% of runs by
// iterating engineOps — which no longer held the disruptive ops — so the cap landed on opTick. With
// the cap re-targeted to the disruptive ops by name, opTick must reach the full range again.
func TestNewDSTConfig_TickWeightRealizesFullRange(t *testing.T) {
	t.Parallel()
	const seeds = 10000
	var minW, maxW uint64 = 101, 0
	over5 := 0
	for i := range seeds {
		rng := rand.New(rand.NewPCG(uint64(i), uint64(i)+1))
		cfg := newDSTConfig(rng)
		w := cfg.OpWeights[opTick]
		require.GreaterOrEqualf(t, w, uint64(1), "seed %d: opTick weight %d < 1 (tick must always be enabled)", i, w)
		require.LessOrEqualf(t, w, uint64(100), "seed %d: opTick weight %d > 100", i, w)
		if w > 5 {
			over5++
		}
		if w < minW {
			minW = w
		}
		if w > maxW {
			maxW = w
		}
	}
	// The cap-on-tick bug confined opTick to [1,5]; the fix restores the intended [1,100] range.
	require.Greaterf(t, maxW, uint64(5), "opTick never exceeded 5 over %d seeds (cap still applied to tick?)", seeds)
	require.GreaterOrEqualf(t, over5, seeds*9/10,
		"opTick weight was >5 in only %d/%d seeds (expected ~95%% for a [1,100] draw)", over5, seeds)
	t.Logf("opTick weight over %d seeds: min=%d max=%d (>5: %d, <=5: %d)",
		seeds, minW, maxW, over5, seeds-over5)
}

// TestCapDisruptiveOpWeights_CapsOnlyDisruptiveOps pins the cap's target: it must bound
// opRestart/opSnapshotRestore to [1,5] and leave every other op — opTick and command: ops —
// untouched. This is the future-proof guarantee: if the disruptive ops are re-enabled in
// engineOps, the cap applies to them by name rather than to whatever engineOps happens to hold.
func TestCapDisruptiveOpWeights_CapsOnlyDisruptiveOps(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(1, 2))
	weights := testutils.OpWeights{
		opTick:            50,
		opRestart:         80,
		opSnapshotRestore: 99,
		"command:foo":     30,
		"command:bar":     7,
	}
	capDisruptiveOpWeights(weights, rng)

	require.Equal(t, uint64(50), weights[opTick], "opTick must never be capped")
	require.Equal(t, uint64(30), weights["command:foo"], "command ops must not be capped")
	require.Equal(t, uint64(7), weights["command:bar"], "command ops must not be capped")

	require.GreaterOrEqual(t, weights[opRestart], uint64(1), "opRestart must remain >= 1")
	require.LessOrEqual(t, weights[opRestart], uint64(5), "opRestart must be capped to [1,5]")
	require.GreaterOrEqual(t, weights[opSnapshotRestore], uint64(1), "opSnapshotRestore must remain >= 1")
	require.LessOrEqual(t, weights[opSnapshotRestore], uint64(5), "opSnapshotRestore must be capped to [1,5]")
}
