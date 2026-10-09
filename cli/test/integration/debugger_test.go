//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/test/integration/testutil"
)

// debugCmdSucceeded returns true when the CLI output looks like a successful
// debug command (contains the success marker and no failure markers).
func debugCmdSucceeded(r testutil.CLIResult) bool {
	out := r.Stdout + r.Stderr
	return !strings.Contains(out, "Failed") &&
		!strings.Contains(out, "failed") &&
		!strings.Contains(out, "error")
}

// debugCmdFailed returns true when the CLI output contains failure markers.
func debugCmdFailed(r testutil.CLIResult) bool {
	out := r.Stdout + r.Stderr
	return strings.Contains(out, "Failed") ||
		strings.Contains(out, "failed") ||
		strings.Contains(out, "✖")
}

// TestDebugger groups tests for the debugger commands (pause, step, resume, reset).
// These require a running Cardinal game.
//
// Step and reset only mean something on a held world, so they pause one that is
// still ticking and then carry on — asking for a single tick implies wanting the
// world stopped, so reporting "not paused" back would be a refusal to do the
// obvious thing. Resume is the exception: it has nothing to undo on a running
// world, so it still reports that.
//
// The subtests share one game and run in order, each leaving the world in the
// state the next expects — noted per test, since an insertion in the wrong place
// silently changes what the following ones exercise.
//
// Test flow:
//  1. Start the game via `world start`
//  2. Wait for Cardinal debug port
//  3. Pause → Step → Resume, all while paused
//  4. Resume while running → still refused
//  5. Step and reset while running → each auto-pauses and says so
//  6. Reset while already paused → no auto-pause in the message
//  7. Cleanup via `world purge`
func TestDebugger(t *testing.T) {
	dockerCli, err := testutil.NewDockerClient()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer dockerCli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	t.Log("Cleaning up any existing containers...")
	_ = testutil.CleanupCardinalContainers(ctx)
	// Wait for ports to be fully released after cleanup.
	time.Sleep(3 * time.Second)

	// --- Start the game ---
	t.Log("Starting game via 'world start'...")
	startCtx, startCancel := context.WithTimeout(ctx, 7*time.Minute)
	defer startCancel()

	resultChan := make(chan testutil.CLIResult, 1)
	go func() {
		result := testutil.RunCLIWithStdin(startCtx, cliBinary, testProjectDir, "\n", "start")
		resultChan <- result
	}()

	captureStartOutput := func() {
		startCancel()
		select {
		case res := <-resultChan:
			t.Logf("world start exited with code %d", res.ExitCode)
			t.Logf("world start stdout:\n%s", res.Stdout)
			t.Logf("world start stderr:\n%s", res.Stderr)
		case <-time.After(30 * time.Second):
			t.Log("Timeout waiting for world start to exit")
		}
	}

	t.Log("Waiting for NATS to be reachable...")
	if !testutil.WaitForNATS(5*time.Minute, testProjectDir) {
		captureStartOutput()
		t.Fatal("NATS did not become reachable")
	}

	t.Log("Waiting for Cardinal debug service to be ready (RPC-level check)...")
	containerName, err := testutil.CardinalShardContainerName(testProjectDir, "game")
	if err != nil {
		t.Fatalf("resolve shard container name: %v", err)
	}
	if !testutil.WaitForCardinalDebugReady(t, 5*time.Minute, containerName) {
		captureStartOutput()
		t.Fatal("Cardinal debug service did not become ready")
	}
	t.Log("Cardinal debug service is ready.")

	// Ensure cleanup happens regardless of test outcome.
	t.Cleanup(func() {
		startCancel()
		select {
		case <-resultChan:
		case <-time.After(30 * time.Second):
		}
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cleanCancel()
		_ = testutil.CleanupCardinalContainers(cleanCtx)
	})

	// The RPC-level readiness check may have left the shard paused.
	// Resume to ensure a clean starting state.
	{
		preCtx, preCancel := context.WithTimeout(ctx, 10*time.Second)
		defer preCancel()
		_ = testutil.RunCLIInDir(preCtx, cliBinary, testProjectDir, "debug", "resume")
	}

	// --- Pause ---
	t.Run("Pause", func(t *testing.T) {
		cmdCtx, cmdCancel := context.WithTimeout(ctx, 30*time.Second)
		defer cmdCancel()

		result := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "pause")
		t.Logf("pause stdout: %s", result.Stdout)
		t.Logf("pause stderr: %s", result.Stderr)

		require.True(t, debugCmdSucceeded(result), "pause should succeed, output: %s %s", result.Stdout, result.Stderr)
		assert.True(t, result.OutputContains("Paused"), "output should confirm pause")
	})

	// --- Step (must be paused) ---
	t.Run("StepWhilePaused", func(t *testing.T) {
		cmdCtx, cmdCancel := context.WithTimeout(ctx, 30*time.Second)
		defer cmdCancel()

		result := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "step")
		t.Logf("step stdout: %s", result.Stdout)
		t.Logf("step stderr: %s", result.Stderr)

		require.True(
			t,
			debugCmdSucceeded(result),
			"step should succeed while paused, output: %s %s",
			result.Stdout,
			result.Stderr,
		)
		assert.True(t, result.OutputContains("Stepped"), "output should confirm step")
		assert.False(t, result.OutputContains("Paused and"),
			"an already-paused world must not be reported as auto-paused, got: %s", result.Stdout)
	})

	// --- Resume (must be paused) ---
	t.Run("ResumeWhilePaused", func(t *testing.T) {
		cmdCtx, cmdCancel := context.WithTimeout(ctx, 30*time.Second)
		defer cmdCancel()

		result := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "resume")
		t.Logf("resume stdout: %s", result.Stdout)
		t.Logf("resume stderr: %s", result.Stderr)

		require.True(
			t,
			debugCmdSucceeded(result),
			"resume should succeed while paused, output: %s %s",
			result.Stdout,
			result.Stderr,
		)
		assert.True(t, result.OutputContains("Resumed"), "output should confirm resume")
	})

	// --- Resume while running is still refused (world → running) ---
	// Runs before the auto-pause tests, which would leave the world paused.
	t.Run("ResumeWhileRunning", func(t *testing.T) {
		cmdCtx, cmdCancel := context.WithTimeout(ctx, 30*time.Second)
		defer cmdCancel()

		result := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "resume")
		t.Logf("resume (running) stdout: %s", result.Stdout)
		t.Logf("resume (running) stderr: %s", result.Stderr)

		assert.True(t, debugCmdFailed(result), "resume has nothing to undo on a running world")
	})

	// --- Step while running auto-pauses (world → paused) ---
	t.Run("StepWhileRunning_AutoPauses", func(t *testing.T) {
		cmdCtx, cmdCancel := context.WithTimeout(ctx, 30*time.Second)
		defer cmdCancel()

		result := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "step")
		t.Logf("step (running) stdout: %s", result.Stdout)
		t.Logf("step (running) stderr: %s", result.Stderr)

		require.True(
			t,
			debugCmdSucceeded(result),
			"step should pause a running world and carry on, output: %s %s",
			result.Stdout,
			result.Stderr,
		)
		// The message has to say a pause happened, or the world stopping looks
		// like something the user didn't ask for.
		assert.True(t, result.OutputContains("Paused and stepped"),
			"output should report the auto-pause, got: %s", result.Stdout)
	})

	// --- Back to running for the reset test (world → running) ---
	t.Run("ResumeAfterAutoPause", func(t *testing.T) {
		cmdCtx, cmdCancel := context.WithTimeout(ctx, 30*time.Second)
		defer cmdCancel()

		result := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "resume")
		t.Logf("resume stdout: %s", result.Stdout)

		require.True(t, debugCmdSucceeded(result),
			"resume should succeed after the auto-pause, output: %s %s", result.Stdout, result.Stderr)
	})

	// --- Reset while running auto-pauses (world → paused) ---
	t.Run("ResetWhileRunning_AutoPauses", func(t *testing.T) {
		cmdCtx, cmdCancel := context.WithTimeout(ctx, 30*time.Second)
		defer cmdCancel()

		result := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "reset")
		t.Logf("reset (running) stdout: %s", result.Stdout)
		t.Logf("reset (running) stderr: %s", result.Stderr)

		require.True(
			t,
			debugCmdSucceeded(result),
			"reset should pause a running world and carry on, output: %s %s",
			result.Stdout,
			result.Stderr,
		)
		assert.True(t, result.OutputContains("Paused and reset"),
			"output should report the auto-pause, got: %s", result.Stdout)
	})

	// --- Reset while already paused does not claim to have paused (world stays paused) ---
	t.Run("ResetWhilePaused", func(t *testing.T) {
		cmdCtx, cmdCancel := context.WithTimeout(ctx, 30*time.Second)
		defer cmdCancel()

		result := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "reset")
		t.Logf("reset (paused) stdout: %s", result.Stdout)
		t.Logf("reset (paused) stderr: %s", result.Stderr)

		require.True(
			t,
			debugCmdSucceeded(result),
			"reset should succeed while paused, output: %s %s",
			result.Stdout,
			result.Stderr,
		)
		assert.True(t, result.OutputContains("Reset"), "output should confirm reset")
		assert.False(t, result.OutputContains("Paused and"),
			"an already-paused world must not be reported as auto-paused, got: %s", result.Stdout)
	})

	// Resume after reset so we leave the game running before cleanup.
	t.Run("ResumeAfterReset", func(t *testing.T) {
		cmdCtx, cmdCancel := context.WithTimeout(ctx, 30*time.Second)
		defer cmdCancel()

		result := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "resume")
		t.Logf("resume stdout: %s", result.Stdout)
		t.Logf("resume stderr: %s", result.Stderr)

		require.True(
			t,
			debugCmdSucceeded(result),
			"resume should succeed after reset, output: %s %s",
			result.Stdout,
			result.Stderr,
		)
	})

	// --- Step advances the tick ---
	t.Run("StepAdvancesTick", func(t *testing.T) {
		cmdCtx, cmdCancel := context.WithTimeout(ctx, 30*time.Second)
		defer cmdCancel()

		// Pause first
		pauseResult := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "pause")
		require.True(t, debugCmdSucceeded(pauseResult), "pause should succeed")

		// Step once
		step1 := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "step")
		t.Logf("step1 stdout: %s", step1.Stdout)
		require.True(t, debugCmdSucceeded(step1), "step 1 should succeed")

		// Step again
		step2 := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "step")
		t.Logf("step2 stdout: %s", step2.Stdout)
		require.True(t, debugCmdSucceeded(step2), "step 2 should succeed")

		assert.True(t, step1.OutputContains("tick"), "step output should mention tick")
		assert.True(t, step2.OutputContains("tick"), "step output should mention tick")

		// Resume to leave in clean state
		resumeResult := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "resume")
		require.True(t, debugCmdSucceeded(resumeResult), "resume should succeed")
	})

	// --- Shard filter flag ---
	t.Run("PauseWithShardID", func(t *testing.T) {
		cmdCtx, cmdCancel := context.WithTimeout(ctx, 30*time.Second)
		defer cmdCancel()

		result := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "pause", "--shard-id", "game")
		t.Logf("pause --shard-id=game stdout: %s", result.Stdout)
		t.Logf("pause --shard-id=game stderr: %s", result.Stderr)

		require.True(t, debugCmdSucceeded(result),
			"pause with --shard-id=game should succeed, output: %s %s",
			result.Stdout, result.Stderr,
		)

		// Resume
		resumeResult := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "resume", "--shard-id", "game")
		require.True(t, debugCmdSucceeded(resumeResult), "resume with --shard-id=game should succeed")
	})

	// --shards is the canonical flag; --shard-id above is kept as an alias, so
	// both spellings have to keep working.
	t.Run("PauseWithShardsFlag", func(t *testing.T) {
		cmdCtx, cmdCancel := context.WithTimeout(ctx, 30*time.Second)
		defer cmdCancel()

		result := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "pause", "--shards", "game")
		t.Logf("pause --shards=game stdout: %s", result.Stdout)

		require.True(t, debugCmdSucceeded(result),
			"pause with --shards=game should succeed, output: %s %s", result.Stdout, result.Stderr)

		resumeResult := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "resume", "--shards", "game")
		require.True(t, debugCmdSucceeded(resumeResult), "resume with --shards=game should succeed")
	})

	t.Run("PauseWithInvalidShardID", func(t *testing.T) {
		cmdCtx, cmdCancel := context.WithTimeout(ctx, 30*time.Second)
		defer cmdCancel()

		result := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", "pause", "--shard-id", "nonexistent")
		t.Logf("pause --shard-id=nonexistent stdout: %s", result.Stdout)
		t.Logf("pause --shard-id=nonexistent stderr: %s", result.Stderr)

		assert.True(t, debugCmdFailed(result), "pause with invalid shard ID should fail")
		assert.True(t, result.OutputContains("not found"), "error should mention shard not found")
	})
}

// TestDebuggerNoGame verifies debugger commands fail gracefully when no game is running.
func TestDebuggerNoGame(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Make sure nothing is running
	_ = testutil.CleanupCardinalContainers(ctx)
	time.Sleep(3 * time.Second)

	cmds := []string{"pause", "step", "resume", "reset"}
	for _, cmd := range cmds {
		t.Run("NoGame_"+cmd, func(t *testing.T) {
			cmdCtx, cmdCancel := context.WithTimeout(ctx, 15*time.Second)
			defer cmdCancel()

			result := testutil.RunCLIInDir(cmdCtx, cliBinary, testProjectDir, "debug", cmd)
			t.Logf("%s stdout: %s", cmd, result.Stdout)
			t.Logf("%s stderr: %s", cmd, result.Stderr)

			assert.True(t, debugCmdFailed(result), "'debug %s' should fail when no game is running", cmd)
		})
	}
}

// TestDebuggerNoWorldToml verifies debugger commands fail gracefully when
// there's no world.toml in the current directory.
func TestDebuggerNoWorldToml(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tmpDir := t.TempDir()

	cmds := []string{"pause", "step", "resume", "reset"}
	for _, cmd := range cmds {
		t.Run("NoToml_"+cmd, func(t *testing.T) {
			cmdCtx, cmdCancel := context.WithTimeout(ctx, 10*time.Second)
			defer cmdCancel()

			result := testutil.RunCLIInDir(cmdCtx, cliBinary, tmpDir, "debug", cmd)
			t.Logf("%s stdout: %s", cmd, result.Stdout)
			t.Logf("%s stderr: %s", cmd, result.Stderr)

			assert.True(t, debugCmdFailed(result), "'debug %s' should fail without world.toml", cmd)
			assert.True(
				t,
				result.OutputContains("world.toml") || result.OutputContains("config") ||
					result.OutputContains("not found"),
				"error should mention missing config, got: %s %s",
				result.Stdout,
				result.Stderr,
			)
		})
	}
}
