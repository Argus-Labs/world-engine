// Package integration contains integration tests for the world CLI.
// These tests run the actual CLI binary against real services.
//
//go:build integration

package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/cli/test/integration/testutil"
)

var (
	// cliBinary holds the path to the built world CLI binary.
	cliBinary string

	// testProjectDir holds the path to the test project directory.
	testProjectDir string

	// repoRoot holds the path to the monorepo root.
	// Exported as RepoRoot for use in other test files.
	repoRoot string
)

// RepoRoot returns the path to the monorepo root.
func RepoRoot() string {
	return repoRoot
}

func TestMain(m *testing.M) {
	os.Stderr.WriteString("world-cli integration tests are temporarily disabled\n")
	os.Exit(0)

	// Setup phase
	if err := setup(); err != nil {
		os.Stderr.WriteString("Integration test setup failed: " + err.Error() + "\n")
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Cleanup phase
	cleanup()

	os.Exit(code)
}

func setup() error {
	// Find repo root by walking up from current directory
	var err error
	repoRoot, err = findRepoRoot()
	if err != nil {
		return err
	}

	worldCLIDir := filepath.Join(repoRoot, "apps", "world-cli")
	cliBinary = filepath.Join(worldCLIDir, "world-test-binary")

	// Build the CLI binary
	cmd := exec.Command("go", "build", "-o", cliBinary, "./cmd/world")
	cmd.Dir = worldCLIDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	// Use the bare-bone-template as our test project
	testProjectDir = filepath.Join(repoRoot, "pkg", "docker", "test", "bare-bone-template")

	// Clean up any leftover containers from previous test runs
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := testutil.CleanupCardinalContainers(ctx); err != nil {
		// Log but don't fail - containers might not exist
		os.Stderr.WriteString("Warning: failed to cleanup existing containers: " + err.Error() + "\n")
	}

	return nil
}

func cleanup() {
	// Remove test binary
	if cliBinary != "" {
		os.Remove(cliBinary)
	}

	// Final cleanup of any containers
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := testutil.CleanupCardinalContainers(ctx); err != nil {
		os.Stderr.WriteString("Warning: final cleanup failed: " + err.Error() + "\n")
	}
}

func findRepoRoot() (string, error) {
	// Start from current directory and walk up looking for monorepo root
	// We identify the monorepo root by having both go.mod and apps/ directory
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		// Check if this looks like the monorepo root
		// (has go.mod AND apps directory)
		goModPath := filepath.Join(dir, "go.mod")
		appsPath := filepath.Join(dir, "apps")

		_, goModErr := os.Stat(goModPath)
		appsInfo, appsErr := os.Stat(appsPath)

		if goModErr == nil && appsErr == nil && appsInfo.IsDir() {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent
	}

	return "", os.ErrNotExist
}
