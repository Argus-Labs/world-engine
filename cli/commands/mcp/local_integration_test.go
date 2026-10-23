//go:build integration

package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
)

// TestResolveShardWorld_AutoDerivesProjectFromRunningContainer is the end-to-end
// proof of the fix: with project omitted and the MCP process working in a
// directory that holds NO world.toml for the shard's project, resolveShardWorld
// still derives the project from a running shard container's labels, listing
// across every project instead of gating on a world.toml.
//
// The container is a stand-in (busybox) carrying the world shard labels; it is
// removed on cleanup. Requires a running Docker daemon and network to pull
// busybox. Skipped when either is unavailable.
func TestResolveShardWorld_AutoDerivesProjectFromRunningContainer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cli, err := client.New(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer cli.Close()

	const image = "busybox:latest"
	pull, err := cli.ImagePull(ctx, image, client.ImagePullOptions{})
	if err != nil {
		t.Skipf("cannot pull %s (network unavailable?): %v", image, err)
	}
	if err := pull.Wait(ctx); err != nil {
		pull.Close()
		t.Skipf("pull %s failed: %v", image, err)
	}
	pull.Close()

	// A unique world identity so this never collides with a real world's shards.
	const (
		project       = "mcp-autodrv-proj"
		shardID       = "mcp-autodrv-game"
		containerName = project + "-" + shardID + "-shard"
	)
	labels := map[string]string{
		service.RoleLabel:     service.RoleShard,
		service.ShardIDLabel:  shardID,
		service.ProjectLabel:  project,
		service.OrgLabel:      "argus",
		service.InstanceLabel: shardID,
	}

	// Remove any leftover from a prior run, then create + start.
	_, _ = cli.ContainerRemove(ctx, containerName, client.ContainerRemoveOptions{Force: true})
	if _, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{Image: image, Cmd: []string{"sleep", "600"}, Labels: labels},
		Name:   containerName,
	}); err != nil {
		t.Fatalf("create container %s: %v", containerName, err)
	}
	defer func() { _, _ = cli.ContainerRemove(ctx, containerName, client.ContainerRemoveOptions{Force: true}) }()
	if _, err := cli.ContainerStart(ctx, containerName, client.ContainerStartOptions{}); err != nil {
		t.Fatalf("start %s: %v", containerName, err)
	}

	// Run from a working directory with no world.toml for this project, so the old
	// cwd fallback could not "work" by accident — the project must come from the
	// container labels.
	t.Chdir(t.TempDir())

	got, err := resolveShardWorld(ctx, shardID, "")
	require.NoError(t, err)
	assert.Equal(t, project, got, "project auto-derived from the running container's labels")

	// The cross-project listing returns the container too — proving the lookup is
	// not scoped to a single project.
	cs, err := listAllShardContainers(ctx)
	require.NoError(t, err)
	var found bool
	for _, c := range cs {
		if c.Name == containerName {
			found = true
			assert.Equal(t, docker.ContainerRunning, c.State)
			assert.Equal(t, project, c.Labels[service.ProjectLabel])
		}
	}
	assert.True(t, found, "listAllShardContainers must list the labeled shard across projects")

	// A shard that is not running yields a clear "not running" error listing what
	// is, rather than a world.toml error.
	_, err = resolveShardWorld(ctx, "mcp-autodrv-nope", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
	assert.NotContains(t, err.Error(), "world.toml")
}
