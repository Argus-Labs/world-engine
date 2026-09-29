//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/test/integration/testutil"
)

// TestGettingStarted groups tests for new users getting started with World CLI.
// These tests verify help, version, setup, and doctor commands work correctly.
// They do not require Docker.
func TestGettingStarted(t *testing.T) {
	t.Run("Help", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.ShortTimeout())
		defer cancel()

		result := testutil.RunCLI(ctx, cliBinary, "--help")

		assert.True(t, result.Success(), "world --help should succeed, got: %s", result.Stderr)
		assert.True(t, result.OutputContains("setup"), "help should mention setup command")
		assert.True(t, result.OutputContains("doctor"), "help should mention doctor command")
	})

	t.Run("Version", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.ShortTimeout())
		defer cancel()

		result := testutil.RunCLI(ctx, cliBinary, "--version")

		t.Logf("version stdout: %s", result.Stdout)
		t.Logf("version stderr: %s", result.Stderr)

		assert.True(t, result.Success(), "world --version should succeed")
	})

	t.Run("HelpShowsCardinalCommands", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.ShortTimeout())
		defer cancel()

		result := testutil.RunCLI(ctx, cliBinary, "--help")

		assert.True(t, result.Success(), "--help should succeed, got: %s", result.Stderr)
		assert.True(t, result.OutputContains("start"), "help should mention start command")
		assert.True(t, result.OutputContains("stop"), "help should mention stop command")
		assert.True(t, result.OutputContains("purge"), "help should mention purge command")
		assert.True(t, result.OutputContains("Cardinal Commands"), "help should have Cardinal Commands section")
	})

	t.Run("HelpInProjectDirectory", func(t *testing.T) {
		_, err := os.Stat(filepath.Join(testProjectDir, "world.toml"))
		require.NoError(t, err, "test project should have world.toml")

		ctx, cancel := context.WithTimeout(context.Background(), testutil.ShortTimeout())
		defer cancel()

		result := testutil.RunCLIInDir(ctx, cliBinary, testProjectDir, "--help")
		assert.True(t, result.Success(), "--help should succeed in project directory")
	})

	t.Run("SetupHelp", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.ShortTimeout())
		defer cancel()

		result := testutil.RunCLI(ctx, cliBinary, "setup", "--help")

		assert.True(t, result.Success(), "setup --help should succeed, got: %s", result.Stderr)
		assert.True(t, result.OutputContains("Setup"), "help should mention Setup")
		assert.True(t, result.OutputContains("World Engine"), "help should mention World Engine")
		assert.True(t, result.OutputContains("--template"), "help should mention --template flag")
	})

	t.Run("SetupWithTemplateFlag", func(t *testing.T) {
		// Create a temp directory for the test project
		tempDir := t.TempDir()
		projectName := "test-setup-project"

		ctx, cancel := context.WithTimeout(context.Background(), testutil.LongTimeout())
		defer cancel()

		// Run setup with --template flag (non-interactive mode)
		result := testutil.RunCLIInDir(ctx, cliBinary, tempDir, "setup", projectName, "--template", "basic")

		t.Logf("setup stdout: %s", result.Stdout)
		t.Logf("setup stderr: %s", result.Stderr)

		assert.True(t, result.Success(), "setup with --template should succeed, got: %s", result.Stderr)

		// Verify the project was created
		projectPath := filepath.Join(tempDir, projectName)
		_, err := os.Stat(projectPath)
		assert.NoError(t, err, "project directory should exist")

		// Verify world.toml exists in the created project
		_, err = os.Stat(filepath.Join(projectPath, "world.toml"))
		assert.NoError(t, err, "world.toml should exist in the created project")
	})

	t.Run("Doctor", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.DefaultTimeout())
		defer cancel()

		result := testutil.RunCLI(ctx, cliBinary, "doctor")

		t.Logf("doctor stdout: %s", result.Stdout)
		t.Logf("doctor stderr: %s", result.Stderr)

		// Doctor command checks for dependencies like Go, Git, Docker
		// It should succeed if all dependencies are installed
		assert.True(t, result.Success(), "doctor should succeed when dependencies are installed")
	})
}
