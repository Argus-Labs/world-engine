//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/test/integration/testutil"
)

// TestLocalDevelopment groups tests for the local development workflow.
// This simulates what a developer does when running their game locally:
// start → stop → restart → purge
//
// These tests require Docker.
func TestLocalDevelopment(t *testing.T) {
	// Skip if Docker is not available
	dockerCli, err := testutil.NewDockerClient()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer dockerCli.Close()

	// Use a longer timeout for container operations
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Clean up any existing containers before starting
	t.Log("Cleaning up any existing containers...")
	_ = testutil.CleanupCardinalContainers(ctx)

	// Step 1: Start containers using `world start`
	t.Run("Start", func(t *testing.T) {
		t.Log("Running 'world start' via CLI in background...")

		// Create a cancellable context for the start command
		startCtx, startCancel := context.WithTimeout(ctx, 7*time.Minute)
		defer startCancel()

		// Run world start in goroutine (it's long-running)
		// Use a channel to capture the result so we can log it on failure
		resultChan := make(chan testutil.CLIResult, 1)
		go func() {
			// Provide newline for the "Press ENTER" prompt
			result := testutil.RunCLIWithStdin(startCtx, cliBinary, testProjectDir, "\n", "start")
			resultChan <- result
		}()

		// Helper to capture and log world start output on failure
		captureStartOutput := func() {
			startCancel() // Cancel the context to stop world start
			select {
			case res := <-resultChan:
				t.Logf("world start exited with code %d", res.ExitCode)
				t.Logf("world start stdout:\n%s", res.Stdout)
				t.Logf("world start stderr:\n%s", res.Stderr)
			case <-time.After(30 * time.Second):
				t.Log("Timeout waiting for world start to exit")
			}
		}

		// Wait for services to be reachable (proves containers are up)
		t.Log("Waiting for NATS to be reachable...")
		if !testutil.WaitForNATS(5 * time.Minute) {
			captureStartOutput()
			t.Fatal("NATS did not become reachable")
		}

		// Verify containers are running
		containers, err := dockerCli.ListCardinalContainers(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(containers), 2, "expected at least 2 containers running (NATS + Cardinal)")

		// Kill the world start process (containers keep running)
		t.Log("Cancelling world start process...")
		startCancel()
		select {
		case <-resultChan:
		case <-time.After(30 * time.Second):
			t.Log("Timeout waiting for world start to exit")
		}
	})

	// Step 2: Stop containers using `world stop`
	t.Run("Stop", func(t *testing.T) {
		t.Log("Running 'world stop' via CLI...")
		result := testutil.RunCLIInDir(ctx, cliBinary, testProjectDir, "stop")
		t.Logf("stop stdout: %s", result.Stdout)
		t.Logf("stop stderr: %s", result.Stderr)

		require.True(t, result.Success(), "stop should succeed, got exit code %d: %s", result.ExitCode, result.Stderr)

		// Wait for services to be down
		t.Log("Waiting for NATS to be stopped...")
		assert.True(t, testutil.WaitForNATSDown(60*time.Second), "NATS should be stopped after CLI stop")
	})

	// Step 3: Restart containers using `world start`
	t.Run("Restart", func(t *testing.T) {
		t.Log("Running 'world start' again via CLI in background...")

		// Create a cancellable context for the start command
		startCtx, startCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer startCancel()

		// Run world start in goroutine
		// Use a channel to capture the result so we can log it on failure
		resultChan := make(chan testutil.CLIResult, 1)
		go func() {
			result := testutil.RunCLIWithStdin(startCtx, cliBinary, testProjectDir, "\n", "start")
			resultChan <- result
		}()

		// Helper to capture and log world start output on failure
		captureStartOutput := func() {
			startCancel() // Cancel the context to stop world start
			select {
			case res := <-resultChan:
				t.Logf("world start exited with code %d", res.ExitCode)
				t.Logf("world start stdout:\n%s", res.Stdout)
				t.Logf("world start stderr:\n%s", res.Stderr)
			case <-time.After(30 * time.Second):
				t.Log("Timeout waiting for world start to exit")
			}
		}

		// Wait for services to be reachable again
		t.Log("Waiting for NATS to be reachable...")
		if !testutil.WaitForNATS(3 * time.Minute) {
			captureStartOutput()
			t.Fatal("NATS did not become reachable")
		}

		// Kill the world start process
		t.Log("Cancelling world start process...")
		startCancel()
		select {
		case <-resultChan:
		case <-time.After(30 * time.Second):
			t.Log("Timeout waiting for world start to exit")
		}
	})

	// Step 4: Purge containers using `world purge`
	t.Run("Purge", func(t *testing.T) {
		t.Log("Running 'world purge' via CLI...")
		result := testutil.RunCLIInDir(ctx, cliBinary, testProjectDir, "purge")
		t.Logf("purge stdout: %s", result.Stdout)
		t.Logf("purge stderr: %s", result.Stderr)

		require.True(t, result.Success(), "purge should succeed, got exit code %d: %s", result.ExitCode, result.Stderr)

		// Give Docker a moment to remove containers
		time.Sleep(2 * time.Second)

		// Verify all containers are removed
		containers, err := dockerCli.ListCardinalContainers(ctx)
		require.NoError(t, err)
		assert.Empty(t, containers, "all containers should be removed after purge")
	})

	// Final cleanup
	t.Log("Final cleanup...")
	_ = testutil.CleanupCardinalContainers(ctx)
}

// TestLocalDevelopmentErrorHandling tests error scenarios during local development.
func TestLocalDevelopmentErrorHandling(t *testing.T) {
	t.Run("StopWithoutWorldToml", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.ShortTimeout())
		defer cancel()

		// Create a temporary directory without world.toml
		tmpDir := t.TempDir()

		// Run stop from a directory without world.toml
		result := testutil.RunCLIInDir(ctx, cliBinary, tmpDir, "stop")

		t.Logf("stop stdout: %s", result.Stdout)
		t.Logf("stop stderr: %s", result.Stderr)

		// The command should fail and mention world.toml
		if !result.Success() {
			assert.True(t,
				result.OutputContains("world.toml") ||
					result.OutputContains("config") ||
					result.OutputContains("not found"),
				"error should mention missing config, got: %s %s", result.Stdout, result.Stderr,
			)
		}
		// If it succeeds, that's also acceptable behavior (no containers to stop)
	})

	t.Run("PurgeCleanState", func(t *testing.T) {
		// Skip if Docker is not available
		dockerCli, err := testutil.NewDockerClient()
		if err != nil {
			t.Skipf("Docker not available: %v", err)
		}
		defer dockerCli.Close()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTimeout())
		defer cancel()

		// Ensure no containers exist first
		err = testutil.CleanupCardinalContainers(ctx)
		require.NoError(t, err, "failed to cleanup existing containers")

		// Run purge from the test project directory
		result := testutil.RunCLIInDir(ctx, cliBinary, testProjectDir, "purge")

		t.Logf("purge stdout: %s", result.Stdout)
		t.Logf("purge stderr: %s", result.Stderr)

		// Should succeed even with no containers
		assert.True(t, result.Success() || result.OutputContains("purged"),
			"purge should handle empty state gracefully")
	})
}
