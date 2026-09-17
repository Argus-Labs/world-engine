package cardinal

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/require"
)

// TestDST_DeterminismContract_EndToEnd asserts the determinism contract of the DST op-generation
// pipeline (pkg/cardinal/dst.go:58–78). Two runs constructed with byte-identical prng seeds and
// identical command catalogs must produce identical op sequences; the only intentional difference
// between the two runs is Go's per-process / per-range map randomization, which TEST_SEED cannot
// control. Before the fixes, two root causes defeated the contract:
//
//  1. command.Manager.Names() ranged over the catalog map (random order) and fed the
//     order-sensitive testutils.RandOpWeights, so the enabled-command subset and weights varied
//     across runs.
//  2. testutils.RandWeightedOp ranged over the OpWeights map in randomized order, so the same prng
//     pick could resolve to different ops across calls.
//
// After the fixes, Names() returns sorted keys and RandWeightedOp walks sorted cumulative
// boundaries, so the op stream is a pure function of (prng, weights) and is reproducible under a
// pinned TEST_SEED.
//
// The run() helper reproduces the real op-generation portion of RunDST against a real
// command.Manager (the only deliberate deviation is that the test captures op names instead of
// dispatching ticks — tick dispatch is irrelevant to the op-schedule determinism).
func TestDST_DeterminismContract_EndToEnd(t *testing.T) {
	t.Parallel()

	const (
		cmdCount = 12
		draws    = 50
	)

	// run echoes dst.go:58–78 against a fresh command.Manager.
	run := func() []string {
		// Identical PCG seed per run — only Go map randomization can vary the result.
		prng := rand.New(rand.NewPCG(0x12345, 0x6789A))

		// Fresh Manager with a fresh catalog map (fresh runtime hash0).
		m := command.NewManager()
		for i := range cmdCount {
			name := fmt.Sprintf("cmd_%02d", i)
			_, err := m.Register(name, command.NewQueue[testutils.SimpleCommand]())
			require.NoError(t, err, "Register failed for %q", name)
		}

		// dst.go:65–71 — Names() → cmdOps → RandOpWeights.
		cmdNames := m.Names()
		cmdOps := make([]string, 0, len(cmdNames))
		for _, name := range cmdNames {
			cmdOps = append(cmdOps, opCommandPrefix+name)
		}
		opWeights := testutils.RandOpWeights(prng, cmdOps)

		// dst.go:77–78 — draw the op schedule via RandWeightedOp.
		ops := make([]string, 0, draws)
		for range draws {
			ops = append(ops, testutils.RandWeightedOp(prng, opWeights))
		}
		return ops
	}

	seq1 := run()
	seq2 := run()
	require.Equal(t, seq1, seq2,
		"DST determinism contract violated: two op-generation runs with identical prng seed and "+
			"identical command catalog produced different op sequences. Root causes are (1) "+
			"Manager.Names() unsorted map iteration feeding RandOpWeights's order-sensitive "+
			"r.Shuffle, and (2) RandWeightedOp's unsorted map iteration over OpWeights.")
}
