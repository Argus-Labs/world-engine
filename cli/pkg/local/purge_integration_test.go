//go:build integration

package local_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/cli/pkg/docker"
	"github.com/argus-labs/world-engine/cli/pkg/docker/service"
	"github.com/argus-labs/world-engine/cli/pkg/local"
	worldtoml "github.com/argus-labs/world-engine/cli/pkg/toml"
)

// Project "<p>-game" service "api" and project "<p>" service "game-api" both name
// their volume "<p>-game-api-service". Once the owner's container is gone, purging
// the other project must keep the volume, even when that project's own container
// mounts it; the owner's own purge still removes it.
func TestPurgeKeepsAnotherProjectsOrphanedVolume(t *testing.T) {
	cli, err := client.New(client.FromEnv)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()

	if _, err := cli.Ping(pingCtx, client.PingOptions{}); err != nil {
		t.Skipf("Docker not available: %v", err)
	}

	cases := map[string]struct {
		purgedProject   string // appended to the run prefix
		purgedService   string
		purgedContainer bool // the purged project has a container mounting the volume
		wantKept        bool
		wantErr         string
	}{
		"other project with a distinct volume name keeps it": {
			purgedService: "worker",
			wantKept:      true,
		},
		"other project with the same volume name keeps it": {
			purgedService: "game-api",
			wantKept:      true,
			wantErr:       "belongs to project",
		},
		"other project whose container mounts the volume keeps it": {
			purgedService:   "game-api",
			purgedContainer: true,
			wantKept:        true,
			wantErr:         "belongs to project",
		},
		"owner still removes its orphaned volume": {
			purgedProject: "-game",
			purgedService: "api",
		},
	}
	for desc, tc := range cases {
		t.Run(desc, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			// Concurrent and repeated runs must never share, or touch, real resources.
			prefix := "wetest-" + strconv.FormatInt(time.Now().UnixNano(), 36)
			owner := serviceConfig(prefix+"-game", "api")
			purged := serviceConfig(prefix+tc.purgedProject, tc.purgedService)

			volume := service.GameServiceContainerName(owner.Project, "api")
			_, err := cli.VolumeInspect(ctx, volume, client.VolumeInspectOptions{})
			require.Error(t, err, "volume %s already exists; the test must not touch it", volume)
			t.Cleanup(func() {
				_, _ = cli.VolumeRemove(context.Background(), volume, client.VolumeRemoveOptions{Force: true})
			})

			// Creating the owner's container creates its volume; removing it leaves the volume.
			ownerID := createServiceContainer(ctx, t, cli, owner)
			_, err = cli.ContainerRemove(ctx, ownerID, client.ContainerRemoveOptions{})
			require.NoError(t, err)
			_, err = cli.VolumeInspect(ctx, volume, client.VolumeInspectOptions{})
			require.NoError(t, err, "creating the owner's container did not create volume %s", volume)

			if tc.purgedContainer {
				createServiceContainer(ctx, t, cli, purged)
			}

			dc, err := docker.NewClient(purged, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = dc.Close() })

			purgeErr := local.New(dc, purged).Purge(ctx, false, nil)

			_, err = cli.VolumeInspect(ctx, volume, client.VolumeInspectOptions{})
			if tc.wantKept {
				assert.NoError(t, err, "purging %s deleted %s's volume %s", purged.Project, owner.Project, volume)
			} else {
				assert.Error(t, err, "purge left the owner's orphaned volume %s", volume)
			}

			if tc.wantErr != "" {
				assert.ErrorContains(t, purgeErr, tc.wantErr)
				assert.ErrorContains(t, purgeErr, owner.Project)
			} else {
				assert.NoError(t, purgeErr)
			}
		})
	}
}

func serviceConfig(project, id string) *service.Config {
	return &service.Config{Project: project, WorldToml: worldtoml.Config{
		Organization: "o",
		Project:      project,
		Services:     []worldtoml.GameService{{ID: id, Image: "alpine:3", Volume: "/data"}},
	}}
}

// createServiceContainer creates, without starting, the service container world
// would, so Docker creates its named volume as world asks when it is missing.
func createServiceContainer(ctx context.Context, t *testing.T, cli *client.Client, cfg *service.Config) string {
	t.Helper()

	svc := service.GameServiceFromConfig(cfg, cfg.WorldToml.Services[0])
	svc.NetworkMode = "" // the project network is not needed to create the volume

	dc, err := docker.NewClient(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dc.Close() })
	require.NoError(t, dc.PullImages(ctx, []service.Service{svc}, nil))

	res, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     &svc.Config,
		HostConfig: &svc.HostConfig,
		Name:       svc.Name,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = cli.ContainerRemove(context.Background(), res.ID, client.ContainerRemoveOptions{Force: true})
	})

	return res.ID
}
