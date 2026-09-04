// Package e2e_test drives the physics2d end-to-end suite under go test: a headless
// Cardinal world whose only job is to exercise the plugin against Box2D's documented
// behaviour. The scenarios, harness and crash-restore driver live under internal/;
// cmd/physics2d-e2e is the same suite as a CLI, for -v, -digest and -serve.
package e2e_test

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/restore"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/scenario"
	"github.com/stretchr/testify/require"
)

// hostileEnv names the crash-prone case a child test process should run alone.
const hostileEnv = "PHYSICS2D_E2E_HOSTILE_CASE"

// e2eConfig is the suite's standard configuration. Output is plain text for test
// logs, and plugin-side failures are surfaced: the physics pipeline logs reconcile
// and rebuild errors rather than returning them.
func e2eConfig(t *testing.T, workers int) harness.Config {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	if os.Getenv("LOG_LEVEL") == "" {
		t.Setenv("LOG_LEVEL", "error")
	}
	return harness.Config{
		Gravity:      physics.Vec2{X: 0, Y: -10},
		SubStepCount: 4,
		Workers:      workers,
		Verbose:      testing.Verbose(),
	}
}

// runSuite builds and runs scenarios and returns the runner, its world, and the exit
// code the CLI would have produced (0 means every check passed).
func runSuite(t *testing.T, scenarios []harness.Scenario, cfg harness.Config) (*harness.Runner, *cardinal.World, int) {
	t.Helper()
	runner := harness.New(scenarios, cfg)
	world, err := runner.BuildWorld(cfg)
	require.NoError(t, err, "build world")
	return runner, world, runner.Run(world)
}

// TestScenarios runs every scenario in one world and fails on any failed check. The
// full report prints above the failure.
func TestScenarios(t *testing.T) {
	runner, _, code := runSuite(t, scenario.All(), e2eConfig(t, 0))
	pass, fail, skip := runner.Report().Totals()
	t.Logf("scenario suite: %d pass, %d fail, %d skip", pass, fail, skip)
	require.Zero(t, code, "%d scenario check(s) failed; see the report above", fail)
}

// TestRestore snapshots a world, rebuilds it in a fresh one and simulates both on,
// with and without the documented Plugin.Reset after FromProto.
func TestRestore(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reset bool
	}{
		{name: "with-plugin-reset", reset: true},
		{name: "without-plugin-reset", reset: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code := restore.Run(e2eConfig(t, 0), tc.reset)
			require.Zero(t, code, "crash-restore check failed; see the report above")
		})
	}
}

// TestDigestIsWorkerInvariant runs the whole suite three times and compares a hash
// of every body's final state: the same configuration twice, which is the only
// thing that catches a Go map iterated for its side effects, then a different
// worker count, because the engine's worker pool must be a pure throughput knob.
func TestDigestIsWorkerInvariant(t *testing.T) {
	digest := func(workers int) (int, uint64) {
		runner, world, code := runSuite(t, scenario.All(), e2eConfig(t, workers))
		require.Zero(t, code, "suite failed at workers=%d", workers)
		return runner.Digest(world)
	}
	bodies, hash := digest(0)
	bodiesAgain, hashAgain := digest(0)
	bodies4, hash4 := digest(4)

	require.Equal(t, bodies, bodiesAgain)
	require.Equal(t, hash, hashAgain, "the same configuration run twice produced different worlds")
	require.Equal(t, bodies, bodies4)
	require.Equal(t, hash, hash4, "changing the worker count changed the result")
	t.Logf("digest: bodies=%d fnv1a64=%016x (stable across runs and workers 0/4)", bodies, hash)
}

// TestHostile runs each crash-prone case in its own process, as the shell runner
// this replaces did: a case that panics inside a tick would otherwise take every
// later test down with it. The child is TestHostileChild.
func TestHostile(t *testing.T) {
	// Cases that fail today for a documented reason. A case that starts passing must
	// be removed from here, which is the point: the test then reports that the engine
	// changed instead of silently absorbing it.
	knownFailures := map[string]string{
		"zero-extent-box": "the engine builds a (NaN, NaN) body from zero half-extents " +
			"instead of rejecting them as C's assert does, and ColliderShape.Validate lets " +
			"them through; the reconciler then rejects the body every tick",
	}

	for _, name := range scenario.HostileNames() {
		t.Run(name, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestHostileChild$", "-test.v")
			cmd.Env = append(os.Environ(), hostileEnv+"="+name, "NO_COLOR=1", "LOG_LEVEL=error")
			out, err := cmd.CombinedOutput()
			text := string(out)

			// The test binary exits 1 when a check fails and 2 when the process panics.
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == 2 {
				t.Fatalf("FATAL: the case took the process down:\n%s", text)
			}
			if strings.Contains(text, "was rejected") {
				t.Log("rejected cleanly, error logged every tick")
			} else {
				t.Log("accepted and simulated")
			}

			if reason, known := knownFailures[name]; known {
				if err == nil {
					t.Fatalf("known failure now passes; remove it from knownFailures. "+
						"The documented reason was: %s", reason)
				}
				t.Logf("known failure, still failing as documented: %s", reason)
				return
			}
			require.NoError(t, err, "case failed:\n%s", text)
		})
	}
}

// TestHostileChild is one hostile case, run alone in the child process TestHostile
// launches with the case name in hostileEnv. Run directly, it skips.
func TestHostileChild(t *testing.T) {
	name := os.Getenv(hostileEnv)
	if name == "" {
		t.Skip("driven by TestHostile")
	}
	sc, ok := scenario.Hostile(name)
	require.True(t, ok, "unknown hostile case %q", name)
	_, _, code := runSuite(t, []harness.Scenario{sc}, e2eConfig(t, 0))
	require.Zero(t, code)
}
