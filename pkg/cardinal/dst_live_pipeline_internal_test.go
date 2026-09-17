package cardinal

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/require"
)

// livePipelineSetup is a DSTSetupFunc that registers cmdCount commands with deterministic
// names ("cmd_00".."cmd_%02d") on the world's command manager. The queue type is irrelevant for
// the determinism contract (the test never enqueues — it only drives the op-generation path that
// turns registered names into weighted ops), so a single payload type (testutils.SimpleCommand) is
// reused; only the registered name varies, which is what Names() returns and what the DST schedule
// is built from.
func livePipelineSetup(t *testing.T, cmdCount int) DSTSetupFunc {
	t.Helper()
	return func(w *World) {
		for i := range cmdCount {
			name := fmt.Sprintf("cmd_%02d", i)
			_, err := w.commands.Register(name, command.NewQueue[testutils.SimpleCommand]())
			require.NoError(t, err, "Register failed for %q", name)
		}
	}
}

// runLivePipeline reproduces the op-generation portion of RunDST (pkg/cardinal/dst.go:58–78)
// against the REAL dst pipeline (newDSTConfig + newDSTFixture + Names() + addCommandOps +
// RandWeightedOp), capturing the per-tick op names instead of dispatching ticks.
//
// prng must be the freshly-derived prng the run should consume; both callers pass byte-identical
// prngs so the only intentional source of variation between runs is Go's per-process / per-range
// map randomization.
func runLivePipeline(t *testing.T, prng *rand.Rand, cmdCount, draws int) []string {
	t.Helper()
	cfg := newDSTConfig(prng)
	fix := newDSTFixture(t, cfg, livePipelineSetup(t, cmdCount))

	// dst.go:65–71 — Names() → cmdOps → addCommandOps.
	cmdNames := fix.world.commands.Names()
	cmdOps := make([]string, 0, len(cmdNames))
	for _, name := range cmdNames {
		cmdOps = append(cmdOps, opCommandPrefix+name)
	}
	cfg.addCommandOps(prng, cmdOps)

	// dst.go:77–78 — draw the op schedule via RandWeightedOp.
	ops := make([]string, 0, draws)
	for range draws {
		ops = append(ops, testutils.RandWeightedOp(prng, cfg.OpWeights))
	}
	return ops
}

// TestDST_LivePipeline_DeterminismContract exercises the live DST op-generation pipeline with a
// hardcoded PCG seed. It pins the contract that two runs through newDSTConfig + newDSTFixture +
// (real) Names() + addCommandOps + RandWeightedOp with identical prng seeds produce identical op
// sequences. Before the fixes, the two runs diverged because:
//
//   - newDSTFixture allocates a fresh command.Manager with a fresh catalog map (fresh runtime
//     hash0), so Names() returned a different order across runs;
//   - addCommandOps calls testutils.RandOpWeights, which is order-sensitive (it shuffles the input
//     and enables the first numEnabled items), so the enabled-command subset and weights varied;
//   - each RandWeightedOp call iterated cfg.OpWeights (a fresh map per run) via randomized range.
//
// After both recommended fixes are applied:
//   - Names() returns a sorted slice, defeating the per-map randomization;
//   - RandWeightedOp walks sorted cumulative boundaries, defeating the per-range randomization.
//
// On master: FAILS 100/100 over -count=100. After both fixes: PASSES 100/100.
func TestDST_LivePipeline_DeterminismContract(t *testing.T) {
	const (
		cmdCount = 12
		draws    = 50
	)

	// Two runs with byte-identical PCG seeds.
	newPrng := func() *rand.Rand { return rand.New(rand.NewPCG(0x12345, 0x6789A)) }

	seq1 := runLivePipeline(t, newPrng(), cmdCount, draws)
	seq2 := runLivePipeline(t, newPrng(), cmdCount, draws)

	require.Equal(t, seq1, seq2,
		"DST live-pipeline determinism contract violated: two runs of the real newDSTConfig + "+
			"newDSTFixture + Names() + addCommandOps + RandWeightedOp path with identical prng seed "+
			"produced different op sequences. Root causes: (1) Manager.Names() unsorted map iteration "+
			"feeding RandOpWeights's order-sensitive r.Shuffle, and (2) RandWeightedOp's unsorted map "+
			"iteration over OpWeights.")
}

// TestDST_LivePipeline_TestSeedDeterminismContract closes the gap that
// TestDST_LivePipeline_DeterminismContract leaves open: it drives the op-generation pipeline
// through the actual TEST_SEED reproduction workflow (testutils.NewRand, which derives the run
// prng by folding the global testutils.Seed with fnv(t.Name())). The test pins the global
// testutils.Seed = 0x12345 (exactly the value pkg/testutils/random.go advertises via
// `to reproduce: TEST_SEED=0x%x`), then calls testutils.NewRand(t) once per run. Because both
// calls share the same t.Name() and the same pinned Seed, both runs receive byte-identical prngs;
// if the pipeline were deterministic the two op sequences would be identical.
//
// On master: FAILS 100/100 over -count=100. After both fixes: PASSES 100/100.
//
// This is the in-process mechanistic equivalent of running two `TEST_SEED=0x12345 go test`
// subprocesses: the cross-process vector (fresh map hash0 per makemap) is the same mechanism
// exercised here per-fixture-allocation.
func TestDST_LivePipeline_TestSeedDeterminismContract(t *testing.T) {
	// NOTE: this test is deliberately non-parallel — newDSTFixture calls t.Setenv, and pinning the
	// global testutils.Seed must not race with parallel tests that read it via testutils.NewRand.

	origSeed := testutils.Seed
	testutils.Seed = 0x12345 // the advertised TEST_SEED value
	t.Cleanup(func() { testutils.Seed = origSeed })

	const (
		cmdCount = 12
		draws    = 50
	)

	// Two runs through the real TEST_SEED derivation; both call NewRand(t) with the same t.Name()
	// and the same pinned global Seed, so the prngs are byte-identical.
	seq1 := runLivePipeline(t, testutils.NewRand(t), cmdCount, draws)
	seq2 := runLivePipeline(t, testutils.NewRand(t), cmdCount, draws)

	require.Equal(t, seq1, seq2,
		"DST TEST_SEED determinism contract violated: two runs driven through testutils.NewRand "+
			"(the seed-derivation path advertised by the `to reproduce: TEST_SEED=0x%x` print) with "+
			"the same pinned global Seed and the same test name produced different op sequences. "+
			"Root causes: fresh Go map hash0 per fresh Manager.catalog / cfg.OpWeights allocation, "+
			"plus per-range bucket re-randomization in RandWeightedOp.")
}
