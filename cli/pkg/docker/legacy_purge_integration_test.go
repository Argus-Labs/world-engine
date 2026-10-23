//go:build integration

package docker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// execDockerBuild shells out to `docker build` for the given context dir and
// tag. Shelling out keeps this end-to-end test simple and faithful to how a
// legacy Cardinal image is actually created on a real daemon.
func execDockerBuild(ctx context.Context, dir, tag string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", "build", "-t", tag, dir)
	return cmd.CombinedOutput()
}

// TestPruneCardinalImages_RealDaemon_RemovesLegacyStaleIDImage is an end-to-end
// integration test against a real Docker daemon. It builds legacy-labeled
// images (world-cli=cardinal + world-cli-image-name, NO project label, modeling
// what released CLIs stamp) whose shard/service IDs are absent from the current
// owned set, runs pruneCardinalImages against the real daemon, and asserts the
// same-project legacy stale-ID images are reclaimed while another project's
// legacy image survives.
func TestPruneCardinalImages_RealDaemon_RemovesLegacyStaleIDImage(t *testing.T) {
	cli, err := client.New(client.FromEnv)
	require.NoError(t, err, "Docker daemon must be available for this integration test")
	defer cli.Close()

	// Legacy images model what a released CLI built: only world-cli=cardinal and
	// world-cli-image-name are stamped, no world.argus.gg/project label.
	legacyImages := []struct {
		tag  string
		name string // world-cli-image-name label value
	}{
		{"rampage-removed-shard:latest", "rampage-removed-shard"},     // stale shard ID, this project
		{"rampage-removed-service:latest", "rampage-removed-service"}, // stale service ID, this project
		{"other-game-shard:latest", "other-game-shard"},               // another project, must survive
	}

	ctx := context.Background()

	// Build each legacy image with the labels an older CLI stamped (no project label).
	for _, li := range legacyImages {
		dir := t.TempDir()
		dockerfile := "FROM scratch\n" +
			"LABEL world-cli=cardinal\n" +
			"LABEL world-cli-image-name=" + li.name + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o644))
		out, berr := execDockerBuild(ctx, dir, li.tag)
		require.NoErrorf(t, berr, "docker build %q failed: %s", li.tag, out)
	}

	// Ensure cleanup of any images that survive the purge.
	t.Cleanup(func() {
		for _, li := range legacyImages {
			_, _ = cli.ImageRemove(context.Background(), li.tag, client.ImageRemoveOptions{Force: true, PruneChildren: true})
		}
	})

	// owned is the current world.toml build set: it contains "gameplay" but NOT
	// "removed" — the exact stale-ID scenario. The legacy images' names are absent.
	owned := map[string]struct{}{
		"rampage-gameplay-shard": {},
	}

	logger := discardLogger()
	purgeCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	require.NoError(t, pruneCardinalImages(purgeCtx, cli, logger, "rampage", owned),
		"pruneCardinalImages must succeed against the real daemon")

	// Assert the same-project stale-ID shard and service images are reclaimed.
	_, err = cli.ImageInspect(ctx, "rampage-removed-shard:latest")
	assert.Error(t, err, "legacy stale-ID shard image must be reclaimed by purge (end-to-end)")
	_, err = cli.ImageInspect(ctx, "rampage-removed-service:latest")
	assert.Error(t, err, "legacy stale-ID service image must be reclaimed by purge (end-to-end)")

	// Assert the other-project legacy image survives.
	_, err = cli.ImageInspect(ctx, "other-game-shard:latest")
	assert.NoError(t, err, "legacy image of another project must survive this project's purge (end-to-end)")
}
