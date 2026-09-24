// Package e2e_test drives the physics2d end-to-end suite under go test: headless
// Cardinal worlds whose only job is to exercise the plugin against Box2D's documented
// behaviour. The scenarios, harness and crash-restore driver live under internal/;
// cmd/physics2d-e2e is the same suite as a CLI, for -digest and -serve.
//
// Every scenario runs in a world of its own, in parallel, and every check reports
// through the subtest's testing.T with the scenario's line as the failure site.
package e2e_test

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	physcomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/restore"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/scenario"
	"github.com/stretchr/testify/require"
)

// hostileEnv names the crash-prone case a child test process should run alone.
const hostileEnv = "PHYSICS2D_E2E_HOSTILE_CASE"

// e2eConfig is the suite's standard configuration, bound to t so every check lands
// in the test log at the scenario's own line.
func e2eConfig(t *testing.T, workers int) harness.Config {
	t.Helper()
	return harness.Config{
		Gravity:      physics.Vec2{X: 0, Y: -10},
		SubStepCount: 4,
		Workers:      workers,
		Verbose:      testing.Verbose(),
		TB:           t,
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

// TestScenarios runs each scenario in its own world as a parallel subtest, so
// `-run TestScenarios/flags` works and a failure names the scenario and its line.
func TestScenarios(t *testing.T) {
	t.Parallel()
	for _, sc := range scenario.All() {
		t.Run(sc.Name, func(t *testing.T) {
			t.Parallel()
			runner, _, code := runSuite(t, []harness.Scenario{sc}, e2eConfig(t, 0))
			pass, fail, skip := runner.Report().Totals()
			t.Logf("%d pass, %d fail, %d skip", pass, fail, skip)
			require.Zero(t, code, "%d check(s) failed", fail)
		})
	}
}

// TestRestore snapshots a world, rebuilds it in a fresh one and simulates both on,
// with and without the documented Plugin.Reset after FromProto.
func TestRestore(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		reset bool
	}{
		{name: "with-plugin-reset", reset: true},
		{name: "without-plugin-reset", reset: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			code := restore.Run(e2eConfig(t, 0), tc.reset)
			require.Zero(t, code, "crash-restore check failed")
		})
	}
}

// TestWorldWatchdogFlagsUnannouncedReset pins the watchdog's hook order: an
// unannounced Plugin.Reset must fail the run on the next tick, while the
// PreUpdate check still sees the nil world the pipeline is about to rebuild.
func TestWorldWatchdogFlagsUnannouncedReset(t *testing.T) {
	t.Parallel()
	sc := harness.Scenario{
		Name: "reset-no-expect",
		Setup: func(c *harness.Ctx) {
			c.Spawn("pad", 0, -1,
				physcomp.NewPhysicsBody2D(physics.BodyTypeStatic, scenario.SampleShape(scenario.KindBox).Spawn(c)))
			c.Spawn("rester", 0, 3,
				physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, scenario.SampleShape(scenario.KindBox).Spawn(c)))
		},
		Steps: []harness.Step{
			{Tick: 10, Do: func(c *harness.Ctx) { c.Plugin().Reset() }}, // unannounced
		},
	}
	// Unbound from t: the expected failure would otherwise call t.Errorf before
	// the assertions below can read it. The verdict comes from the exit code and
	// the report, as it does for the CLI.
	cfg := e2eConfig(t, 0)
	cfg.TB = nil
	cfg.Verbose = false
	runner := harness.New([]harness.Scenario{sc}, cfg)
	world, err := runner.BuildWorld(cfg)
	require.NoError(t, err, "build world")
	code := runner.Run(world)
	require.NotZero(t, code, "the watchdog must fail an unannounced Plugin.Reset")

	var watchdog harness.Result
	found := false
	for _, f := range runner.Report().Failures() {
		if f.Check == "the Box2D world stays alive" {
			watchdog = f
			found = true
			break
		}
	}
	require.True(t, found,
		"expected a 'the Box2D world stays alive' failure; got %+v", runner.Report().Failures())
	// Reset ran on tick 10's Update, so tick 11's PreUpdate is where it shows.
	require.Equal(t, uint64(11), watchdog.Tick,
		"watchdog fired on the wrong tick: %+v", watchdog)
	require.Contains(t, watchdog.Detail, "Plugin.Engine() went nil",
		"watchdog reported the wrong detail: %+v", watchdog)
}

// TestWorldWatchdogAcceptsAnnouncedReset is the companion to the regression
// above: ExpectWorldReset must keep a deliberate Reset from tripping the
// watchdog, so the permission path fails here rather than as a knock-on
// elsewhere.
func TestWorldWatchdogAcceptsAnnouncedReset(t *testing.T) {
	t.Parallel()
	sc := harness.Scenario{
		Name: "reset-with-expect",
		Setup: func(c *harness.Ctx) {
			c.Spawn("pad", 0, -1,
				physcomp.NewPhysicsBody2D(physics.BodyTypeStatic, scenario.SampleShape(scenario.KindBox).Spawn(c)))
			c.Spawn("rester", 0, 3,
				physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, scenario.SampleShape(scenario.KindBox).Spawn(c)))
		},
		Steps: []harness.Step{
			{Tick: 10, Do: func(c *harness.Ctx) {
				c.ExpectWorldReset()
				c.Plugin().Reset()
			}},
		},
	}
	runner, _, code := runSuite(t, []harness.Scenario{sc}, e2eConfig(t, 0))
	require.Zero(t, code, "the watchdog must not flag an announced Plugin.Reset")
	for _, f := range runner.Report().Failures() {
		require.NotEqual(t, "the Box2D world stays alive", f.Check,
			"watchdog wrongly flagged an announced reset: %+v", f)
	}
}

// TestWorldWatchdogFlagsChainedUnannouncedReset pins a one-tick blind spot: an
// unannounced Plugin.Reset on the tick after an announced one must still fail
// the run. The watchdog used to clear worldSeen on the allowed nil, so the next
// unannounced Reset passed as a cold start.
func TestWorldWatchdogFlagsChainedUnannouncedReset(t *testing.T) {
	t.Parallel()
	sc := harness.Scenario{
		Name: "chained-reset",
		Setup: func(c *harness.Ctx) {
			c.Spawn("pad", 0, -1,
				physcomp.NewPhysicsBody2D(physics.BodyTypeStatic, scenario.SampleShape(scenario.KindBox).Spawn(c)))
			c.Spawn("rester", 0, 3,
				physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, scenario.SampleShape(scenario.KindBox).Spawn(c)))
		},
		Steps: []harness.Step{
			{Tick: 10, Do: func(c *harness.Ctx) {
				c.ExpectWorldReset()
				c.Plugin().Reset()
			}},
			{Tick: 11, Do: func(c *harness.Ctx) { c.Plugin().Reset() }}, // unannounced
		},
	}
	// Unbound from t: the expected failure would otherwise call t.Errorf before
	// the assertions below read it. The verdict comes from the exit code.
	cfg := e2eConfig(t, 0)
	cfg.TB = nil
	cfg.Verbose = false
	runner := harness.New([]harness.Scenario{sc}, cfg)
	world, err := runner.BuildWorld(cfg)
	require.NoError(t, err, "build world")
	code := runner.Run(world)
	require.NotZero(t, code,
		"the watchdog must fail the chained unannounced Plugin.Reset on tick 11")

	var watchdog harness.Result
	found := false
	for _, f := range runner.Report().Failures() {
		if f.Check == "the Box2D world stays alive" {
			watchdog = f
			found = true
			break
		}
	}
	require.True(t, found,
		"expected a 'the Box2D world stays alive' failure; got %+v", runner.Report().Failures())
	// The unannounced Reset ran on tick 11's Update, so tick 12's PreUpdate is
	// where the watchdog first observes the resulting nil world.
	require.Equal(t, uint64(12), watchdog.Tick,
		"watchdog fired on the wrong tick: %+v", watchdog)
	require.Contains(t, watchdog.Detail, "Plugin.Engine() went nil",
		"watchdog reported the wrong detail: %+v", watchdog)
}

// TestDigestIsWorkerInvariant runs every scenario together in one world three times
// and compares a hash of every body's final state: the same configuration twice,
// which is the only thing that catches a Go map iterated for its side effects, then
// a different worker count, because the engine's worker pool must be a pure
// throughput knob.
func TestDigestIsWorkerInvariant(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	// Cases that fail today for a documented reason. A case that starts passing must
	// be removed from here, which is the point: the test then reports that the engine
	// changed instead of silently absorbing it.
	knownFailures := map[string]string{}

	for _, name := range scenario.HostileNames() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestHostileChild$", "-test.v")
			// LOG_LEVEL is pinned for the child rather than inherited: a rejected shape
			// logs its reconcile failure every tick, and those lines are the diagnosis
			// when a case fails, so a developer running the parent at "disabled" must
			// not lose them.
			cmd.Env = append(os.Environ(), hostileEnv+"="+name, "LOG_LEVEL=error")
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
